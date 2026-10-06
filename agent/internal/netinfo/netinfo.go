// Package netinfo lists the LAN addresses a phone can reach this machine on.
package netinfo

import (
	"net"
	"sort"
	"strings"
)

type Addr struct {
	IP      string `json:"ip"`
	Iface   string `json:"iface"`
	Virtual bool   `json:"virtual"`
	MAC     string `json:"-"`
}

// Adapter names that are almost never the network the phone is on.
var virtualHints = []string{
	"vethernet", "virtual", "vmware", "vbox", "hyper-v", "wsl", "docker", "podman",
	"tailscale", "zerotier", "wireguard", "openvpn", "vpn", "tap", "tun", "utun",
	"bridge", "awdl", "llw", "anpi", "loopback", "bluetooth", "dco",
}

func isVirtual(name string) bool {
	n := strings.ToLower(name)
	for _, h := range virtualHints {
		if strings.Contains(n, h) {
			return true
		}
	}
	return false
}

// LANAddrs returns private IPv4 addresses of interfaces that are up, physical ones first.
func LANAddrs() []Addr {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []Addr
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip == nil || !ip.IsPrivate() {
				continue
			}
			out = append(out, Addr{IP: ip.String(), Iface: ifc.Name, Virtual: isVirtual(ifc.Name), MAC: ifc.HardwareAddr.String()})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return !out[i].Virtual && out[j].Virtual })
	return out
}

// MACs returns hardware addresses of physical adapters that have a LAN address (for Wake-on-LAN).
func MACs(addrs []Addr) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range addrs {
		if a.Virtual || a.MAC == "" || seen[a.MAC] {
			continue
		}
		seen[a.MAC] = true
		out = append(out, a.MAC)
	}
	return out
}

func IPs(addrs []Addr) []string {
	out := make([]string, len(addrs))
	for i, a := range addrs {
		out[i] = a.IP
	}
	return out
}
