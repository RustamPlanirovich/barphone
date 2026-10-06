// Package discovery announces the agent on the LAN via mDNS / DNS-SD (_barphone._tcp).
package discovery

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/grandcat/zeroconf"

	"barphone/agent/internal/store"
)

const Service = "_barphone._tcp"

// Announce keeps the service registered until ctx is done, re-registering when the
// machine name changes.
func Announce(ctx context.Context, st *store.Store, port int, osName string, version int, logger *log.Logger) {
	changes, unsubscribe := st.Subscribe()
	defer unsubscribe()

	var server *zeroconf.Server
	var name string
	register := func() {
		cfg := st.Snapshot()
		if server != nil && cfg.Name == name {
			return
		}
		if server != nil {
			server.Shutdown()
			server = nil
		}
		name = cfg.Name
		txt := []string{
			"v=" + strconv.Itoa(version),
			"id=" + cfg.MachineID,
			"name=" + cfg.Name,
			"os=" + osName,
		}
		// Instance names must be unique on the network; the display name travels in TXT.
		instance := fmt.Sprintf("barphone-%s", cfg.MachineID[:12])
		s, err := zeroconf.Register(instance, Service, "local.", port, txt, nil)
		if err != nil {
			logger.Printf("mdns: register: %v (discovery off, QR pairing still works)", err)
			return
		}
		server = s
		logger.Printf("mdns: announced %s.%s.local on port %d", instance, Service, port)
	}

	register()
	retry := time.NewTicker(time.Minute)
	defer retry.Stop()
	for {
		select {
		case <-ctx.Done():
			if server != nil {
				server.Shutdown()
			}
			return
		case <-changes:
			register()
		case <-retry.C:
			if server == nil {
				register()
			}
		}
	}
}
