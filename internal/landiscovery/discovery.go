// Package landiscovery resolves paired computers without treating LAN advertisements
// as trusted identities. Callers must verify the saved certificate before use.
package landiscovery

import (
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const Service = "_onecatch._tcp"

// Instance is derived from the existing certificate, so previously paired phones
// need no new pairing data. The complete certificate is still verified over TLS.
func Instance(fingerprint string) (string, error) {
	value := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(fingerprint), ":", ""))
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != 32 {
		return "", errors.New("LAN discovery requires a paired certificate")
	}
	return "onecatch-" + value[:32], nil
}

// Only query usable LAN interfaces, not loopback or peer-to-peer interfaces
// that expose multicast flags but have no routable address.
func lanInterfaces() ([]string, []net.Interface) {
	var ips []string
	var active []net.Interface
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		found := false
		for _, a := range addresses {
			ip, _, err := net.ParseCIDR(a.String())
			if err == nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified() {
				ips = append(ips, ip.String())
				found = true
			}
		}
		if found {
			active = append(active, iface)
		}
	}
	sort.Strings(ips)
	return ips, active
}

func endpoint(ip string, port int) string {
	if port < 1 || port > 65535 {
		return ""
	}
	address := strings.Split(ip, "%")[0]
	parsed := net.ParseIP(address)
	if parsed == nil || parsed.IsLoopback() || parsed.IsUnspecified() || parsed.IsMulticast() {
		return ""
	}
	return (&url.URL{Scheme: "https", Host: net.JoinHostPort(ip, strconv.Itoa(port))}).String()
}
