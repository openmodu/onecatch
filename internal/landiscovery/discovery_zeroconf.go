package landiscovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/zeroconf/v2"
)

// Advertise rebinds when interfaces change; the zeroconf server snapshots its
// addresses at registration, so keeping it forever would advertise the old IP.
func advertiseLAN(fingerprint string, port int) (func(), error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid discovery port")
	}
	instance, err := Instance(fingerprint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		var server *zeroconf.Server
		defer func() {
			if server != nil {
				server.Shutdown()
			}
		}()
		previous := ""
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			addresses, ifaces := lanInterfaces()
			current := strings.Join(addresses, ",")
			for _, iface := range ifaces {
				current += fmt.Sprintf("/%d:%s", iface.Index, iface.Name)
			}
			if server == nil || current != previous {
				if server != nil {
					server.Shutdown()
					server = nil
				}
				if len(addresses) > 0 {
					server, _ = zeroconf.RegisterProxy(instance, Service, "local.", port, instance+".local.", addresses, []string{"v=1"}, ifaces)
				}
				previous = current
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
	return func() { cancel(); <-done }, nil
}

func resolveLAN(ctx context.Context, fingerprint string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	instance, err := Instance(fingerprint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	_, ifaces := lanInterfaces()
	if len(ifaces) == 0 {
		return nil, fmt.Errorf("no active LAN interface")
	}
	entries := make(chan *zeroconf.ServiceEntry, 16)
	finished := make(chan error, 1)
	go func() {
		finished <- zeroconf.Lookup(ctx, instance, Service, "local.", entries, zeroconf.SelectIfaces(ifaces))
	}()
	for {
		select {
		case err := <-finished:
			if err == nil {
				err = context.DeadlineExceeded
			}
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		case entry, ok := <-entries:
			if !ok {
				return nil, context.DeadlineExceeded
			}
			var urls []string
			for _, ip := range append(entry.AddrIPv4, entry.AddrIPv6...) {
				if ip.IsLinkLocalUnicast() {
					continue
				}
				if u := endpoint(ip.String(), entry.Port); u != "" {
					urls = append(urls, u)
				}
			}
			if len(urls) > 0 {
				return urls, nil
			}
		}
	}
}

func browseLAN(parent context.Context) ([]Candidate, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	_, ifaces := lanInterfaces()
	if len(ifaces) == 0 {
		return nil, fmt.Errorf("no active LAN interface")
	}
	entries := make(chan *zeroconf.ServiceEntry, 32)
	finished := make(chan error, 1)
	go func() { finished <- zeroconf.Browse(ctx, Service, "local.", entries, zeroconf.SelectIfaces(ifaces)) }()
	items := make(map[string]Candidate)
	for {
		select {
		case <-ctx.Done():
			if err := parent.Err(); err != nil {
				return nil, err
			}
			return sortedCandidates(items), nil
		case err := <-finished:
			if err != nil && ctx.Err() == nil {
				return nil, err
			}
			return sortedCandidates(items), parent.Err()
		case entry, ok := <-entries:
			if !ok {
				return sortedCandidates(items), parent.Err()
			}
			if len(items) >= 32 {
				continue
			}
			addresses := []string{}
			for _, ip := range append(entry.AddrIPv4, entry.AddrIPv6...) {
				if ip.IsLinkLocalUnicast() {
					continue
				}
				if address := endpoint(ip.String(), entry.Port); address != "" {
					addresses = append(addresses, address)
				}
			}
			if len(addresses) > 0 {
				items[entry.Instance] = candidate(entry.Instance, entry.HostName, addresses)
			}
		}
	}
}
