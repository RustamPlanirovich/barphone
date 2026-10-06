//go:build manual

package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

// Lists barphone agents visible via mDNS (run while an agent is up):
//
//	go test -tags manual -run Browse -v ./internal/discovery
func TestBrowse(t *testing.T) {
	r, err := zeroconf.NewResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := make(chan *zeroconf.ServiceEntry)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := r.Browse(ctx, Service, "local.", entries); err != nil {
		t.Fatal(err)
	}
	found := 0
	for {
		select {
		case e := <-entries:
			found++
			t.Logf("%s host=%s port=%d ipv4=%v txt=%v", e.Instance, e.HostName, e.Port, e.AddrIPv4, e.Text)
		case <-ctx.Done():
			if found == 0 {
				t.Fatal("no agents found")
			}
			return
		}
	}
}
