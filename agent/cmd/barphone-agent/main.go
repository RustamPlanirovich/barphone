// Command barphone-agent runs on each computer: it serves the deck to paired phones over
// the LAN and hosts the configuration UI on http://127.0.0.1:47801.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"barphone/agent/internal/autostart"
	"barphone/agent/internal/discovery"
	"barphone/agent/internal/firewall"
	"barphone/agent/internal/icons"
	"barphone/agent/internal/launch"
	"barphone/agent/internal/netinfo"
	"barphone/agent/internal/pairing"
	"barphone/agent/internal/server"
	"barphone/agent/internal/store"
	"barphone/agent/internal/tray"
)

func main() {
	// Elevated helper mode, started by the agent itself via UAC for the firewall button:
	// add the inbound rule for this exe and exit. Nothing else runs as administrator.
	if len(os.Args) == 2 && os.Args[1] == firewall.ElevatedFlag {
		if err := firewall.ApplyAllowRule(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	lanAddr := flag.String("lan", ":47800", "address for the phone API (all interfaces)")
	uiAddr := flag.String("ui", "127.0.0.1:47801", "address for the configuration UI (loopback only)")
	dir := flag.String("config-dir", "", "config directory (default: <user config dir>/barphone)")
	noBrowser := flag.Bool("no-browser", false, "do not open the configuration UI on start")
	noTray := flag.Bool("no-tray", false, "run without a tray icon")
	notifyText := flag.String("notify", "", "send this text to the phones through the running agent, then exit")
	notifyTitle := flag.String("title", "", "with -notify: a title")
	notifyLevel := flag.String("level", "info", "with -notify: info, ok or error")
	flag.Parse()

	if *notifyText != "" || *notifyTitle != "" {
		os.Exit(sendNotice(*uiAddr, *notifyTitle, *notifyText, *notifyLevel))
	}

	if *dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			fatal(nil, "config dir: %v", err)
		}
		*dir = filepath.Join(base, "barphone")
	}
	if err := os.MkdirAll(*dir, 0o700); err != nil {
		fatal(nil, "config dir: %v", err)
	}
	logger := openLog(filepath.Join(*dir, "agent.log"))

	if !server.LoopbackOnly(*uiAddr) {
		fatal(logger, "-ui must be a loopback address, got %q", *uiAddr)
	}

	l := launch.New()
	uiURL := "http://" + *uiAddr + "/"

	// The UI port doubles as a single-instance lock: if it is taken, another agent is
	// running, so just bring its UI up.
	uiLn, err := net.Listen("tcp", *uiAddr)
	if err != nil {
		logger.Printf("ui listen %s: %v — agent already running? opening its UI", *uiAddr, err)
		l.OpenURL(uiURL)
		return
	}
	lanLn, err := net.Listen("tcp", *lanAddr)
	if err != nil {
		fatal(logger, "lan listen %s: %v", *lanAddr, err)
	}

	host, _ := os.Hostname()
	st, err := store.Open(*dir, host)
	if err != nil {
		fatal(logger, "config: %v", err)
	}
	ic, err := icons.Open(filepath.Join(*dir, "icons"))
	if err != nil {
		fatal(logger, "icons: %v", err)
	}

	srv := &server.Server{
		Store:     st,
		Icons:     ic,
		Launcher:  l,
		Apps:      &launch.AppCache{L: l, TTL: 10 * time.Minute},
		Pairing:   &pairing.Sessions{},
		Log:       logger,
		LANPort:   lanLn.Addr().(*net.TCPAddr).Port,
		UIPort:    uiLn.Addr().(*net.TCPAddr).Port,
		Autostart: autostart.Login{},
	}

	if runtime.GOOS == "windows" {
		srv.Firewall = &firewall.Checker{Iface: func() string {
			for _, a := range netinfo.LANAddrs() {
				if !a.Virtual {
					return a.Iface
				}
			}
			return ""
		}}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go srv.Run(ctx)
	go serve(ctx, logger, "lan", lanLn, srv.LANHandler())
	go serve(ctx, logger, "ui", uiLn, srv.UIHandler())
	go discovery.Announce(ctx, st, srv.LANPort, server.OSName(), server.ProtocolVersion, logger)
	go srv.Apps.Get(ctx, false) // warm the picker cache (also names Store apps for profiles)
	go launch.WatchForeground(ctx, srv.SetForeground)

	logger.Printf("barphone agent: phones -> %s, UI -> %s, config -> %s", lanLn.Addr(), uiURL, *dir)

	if cfg := st.Snapshot(); !*noBrowser && (len(cfg.Devices) == 0 || len(cfg.AllButtons()) == 0) {
		l.OpenURL(uiURL)
	}

	go func() {
		<-ctx.Done()
		tray.Quit()
	}()
	if *noTray {
		<-ctx.Done()
		return
	}
	tray.Run("barphone — дека для телефона", tray.Menu{
		OpenUI: func() { l.OpenURL(uiURL) },
		Pair:   func() { l.OpenURL(uiURL + "#pair") },
		Quit:   cancel,
	})
	cancel()
	time.Sleep(200 * time.Millisecond) // let phones receive the close frame
}

func serve(ctx context.Context, logger *log.Logger, name string, ln net.Listener, h http.Handler) {
	hs := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, ErrorLog: logger}
	go func() {
		<-ctx.Done()
		shutdownCtx, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		hs.Shutdown(shutdownCtx)
	}()
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Printf("%s server: %v", name, err)
	}
}

// openLog writes to <dir>/agent.log (truncated past 5 MB) and stderr when there is one.
func openLog(path string) *log.Logger {
	if st, err := os.Stat(path); err == nil && st.Size() > 5<<20 {
		os.Remove(path)
	}
	var w io.Writer = os.Stderr
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		// File first: in a -H=windowsgui build stderr is invalid and MultiWriter stops at the first error.
		w = io.MultiWriter(f, os.Stderr)
	}
	return log.New(w, "", log.LstdFlags)
}

func fatal(logger *log.Logger, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if logger != nil {
		logger.Print(msg)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	os.Exit(1)
}

// sendNotice is "barphone-agent -notify": scripts on this PC tell the phones something
// through the agent that is already running (its loopback UI port).
func sendNotice(uiAddr, title, text, level string) int {
	body, _ := json.Marshal(map[string]string{"title": title, "text": text, "level": level})
	req, err := http.NewRequest(http.MethodPost, "http://"+uiAddr+"/api/notify", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Barphone-UI", "1")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "barphone agent is not running:", err)
		return 1
	}
	defer resp.Body.Close()
	answer, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "barphone: %s %s\n", resp.Status, answer)
		return 1
	}
	fmt.Println(string(answer))
	return 0
}
