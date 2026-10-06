package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"barphone/agent/internal/tlscert"
)

// pinned is how a phone talks TLS to the agent: no CA, only the fingerprint.
func pinned(fp string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // replaced by the fingerprint check below
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 || tlscert.Fingerprint(raw[0]) != fp {
				return errors.New("fingerprint mismatch")
			}
			return nil
		},
	}
}

func TestPlainAndTLSOnOnePort(t *testing.T) {
	cert, fp, err := tlscert.LoadOrCreate(t.TempDir(), "ПК")
	if err != nil {
		t.Fatal(err)
	}
	e := newEnvWith(t, func(s *Server) { s.TLSFingerprint = fp })
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: e.srv.LANHandler()}
	go srv.Serve(SniffTLS(ln, &tls.Config{Certificates: []tls.Certificate{cert}}))
	t.Cleanup(func() { srv.Close() })
	addr := ln.Addr().String()

	get := func(c *http.Client, url string) string {
		t.Helper()
		resp, err := c.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if body := get(http.DefaultClient, "http://"+addr+"/api/v1/info"); !strings.Contains(body, `"id"`) {
		t.Fatalf("plain: %s", body)
	}
	secure := &http.Client{Transport: &http.Transport{TLSClientConfig: pinned(fp)}}
	if body := get(secure, "https://"+addr+"/api/v1/info"); !strings.Contains(body, `"id"`) {
		t.Fatalf("tls: %s", body)
	}
	wrong := &http.Client{Transport: &http.Transport{TLSClientConfig: pinned(strings.Repeat("A", 43))}}
	if _, err := wrong.Get("https://" + addr + "/api/v1/info"); err == nil {
		t.Fatal("a phone pinned to another certificate must not connect")
	}

	// The QR code carries the fingerprint; pairing works over TLS.
	_, data := e.call("POST", "/api/pairing", nil)
	var p uiPairing
	json.Unmarshal(data, &p)
	if !strings.Contains(p.URI, "fp="+fp) {
		t.Fatalf("pairing URI without the fingerprint: %s", p.URI)
	}
	body, _ := json.Marshal(map[string]string{"code": p.Code, "deviceId": "dev", "deviceName": "Pixel"})
	pr, err := secure.Post("https://"+addr+"/api/v1/pair", "application/json", bytes.NewReader(body))
	if err != nil || pr.StatusCode != 200 {
		t.Fatalf("pairing over TLS: %v %v", pr, err)
	}
	var out map[string]any
	json.NewDecoder(pr.Body).Decode(&out)
	pr.Body.Close()

	d := websocket.Dialer{TLSClientConfig: pinned(fp)}
	ws, _, err := d.Dial("wss://"+addr+"/api/v1/ws?features=windows", http.Header{"Authorization": {"Bearer " + out["token"].(string)}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	state := readMsg(t, ws, "state")
	if tl, _ := state["tls"].(map[string]any); tl["fp"] != fp {
		t.Fatalf("state.tls: %v", state["tls"])
	}
}
