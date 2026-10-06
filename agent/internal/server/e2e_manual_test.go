//go:build windows && manual

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
	"golang.org/x/sys/windows"
)

// End-to-end check against a *running* agent, acting as the phone over its LAN address.
// Opens (and closes) a small probe window on the desktop.
//
//	go test -tags manual -run E2E -v ./internal/server
const uiBase = "http://127.0.0.1:47801"

func uiCall(t *testing.T, method, path string, body any) []byte {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, uiBase+path, r)
	req.Header.Set("X-Barphone-UI", "1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, data)
	}
	return data
}

var (
	user32e         = windows.NewLazySystemDLL("user32.dll")
	procFindWindow  = user32e.NewProc("FindWindowW")
	procGetFg       = user32e.NewProc("GetForegroundWindow")
	procPostMessage = user32e.NewProc("PostMessageW")
)

func TestE2E(t *testing.T) {
	var st struct {
		LAN struct {
			Port  int                   `json:"port"`
			Addrs []struct{ IP string } `json:"addrs"`
		} `json:"lan"`
	}
	json.Unmarshal(uiCall(t, "GET", "/api/state", nil), &st)
	if len(st.LAN.Addrs) == 0 {
		t.Fatal("agent reports no LAN address")
	}
	lan := fmt.Sprintf("%s:%d", st.LAN.Addrs[0].IP, st.LAN.Port)
	t.Logf("phone side talks to %s", lan)

	title := fmt.Sprintf("barphone-e2e-%d", time.Now().Unix())
	uiCall(t, "PUT", "/api/deck", map[string]any{"columns": 3, "buttons": []map[string]string{
		{"title": "Калькулятор", "kind": "app", "target": "Microsoft.WindowsCalculator_8wekyb3d8bbwe!App"},
		{"title": "Проба", "kind": "path", "target": "powershell.exe",
			"args": fmt.Sprintf(`-NoProfile -WindowStyle Hidden -Command "Add-Type -AssemblyName System.Windows.Forms; $f=New-Object Windows.Forms.Form; $f.Text='%s'; [void]$f.ShowDialog()"`, title)},
	}})

	var p uiPairing
	json.Unmarshal(uiCall(t, "POST", "/api/pairing", nil), &p)
	body, _ := json.Marshal(map[string]string{"code": p.Code, "deviceId": "e2e-phone", "deviceName": "E2E телефон"})
	resp, err := http.Post("http://"+lan+"/api/v1/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var paired struct{ Token string }
	json.NewDecoder(resp.Body).Decode(&paired)
	resp.Body.Close()
	if paired.Token == "" {
		t.Fatalf("pairing failed: %d", resp.StatusCode)
	}
	defer uiCall(t, "DELETE", "/api/devices/e2e-phone", nil)

	ws, _, err := websocket.DefaultDialer.Dial("ws://"+lan+"/api/v1/ws", http.Header{"Authorization": {"Bearer " + paired.Token}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	var probeID string
	deadline := time.Now().Add(5 * time.Second)
	for probeID == "" || time.Now().Before(deadline) {
		ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		var m stateMsg
		if err := ws.ReadJSON(&m); err != nil {
			t.Fatal(err)
		}
		if m.Type != "state" {
			continue
		}
		withIcons := 0
		for _, b := range m.Deck.Buttons {
			if b.Title == "Проба" {
				probeID = b.ID
			}
			if b.Icon != nil {
				withIcons++
			}
		}
		t.Logf("state: %d buttons, %d with icons", len(m.Deck.Buttons), withIcons)
		if withIcons == len(m.Deck.Buttons) && probeID != "" {
			break
		}
	}

	start := time.Now()
	ws.WriteJSON(map[string]string{"type": "launch", "req": "e2e", "id": probeID})
	for {
		var m map[string]any
		ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		if err := ws.ReadJSON(&m); err != nil {
			t.Fatal(err)
		}
		if m["type"] == "result" {
			if m["ok"] != true {
				t.Fatalf("launch failed: %v", m)
			}
			t.Logf("launch acknowledged in %v", time.Since(start))
			break
		}
	}

	tp, _ := syscall.UTF16PtrFromString(title)
	var hwnd uintptr
	for i := 0; i < 80 && hwnd == 0; i++ {
		time.Sleep(100 * time.Millisecond)
		hwnd, _, _ = procFindWindow.Call(0, uintptr(unsafe.Pointer(tp)))
	}
	if hwnd == 0 {
		t.Fatal("probe window never appeared")
	}
	t.Logf("window visible %v after press", time.Since(start))
	time.Sleep(700 * time.Millisecond)
	fg, _, _ := procGetFg.Call()
	if fg != hwnd {
		t.Error("probe window is not in the foreground")
	} else {
		t.Log("probe window is in the foreground")
	}
	procPostMessage.Call(hwnd, 0x0010, 0, 0)
}
