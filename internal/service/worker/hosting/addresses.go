package hosting

import (
	"fmt"
	"net"
	"sort"
)

// LANAddresses lists the base URLs a phone on the same network can reach this
// machine at. The desktop shows them during pairing, so the user does not have
// to go looking for their own IP address.
func LANAddresses(port int) []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var addresses []net.Addr
	for _, item := range interfaces {
		// A down or loopback interface can only ever offer an address the phone
		// cannot dial.
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 {
			continue
		}
		found, err := item.Addrs()
		if err != nil {
			continue
		}
		addresses = append(addresses, found...)
	}
	return baseURLsFrom(addresses, port)
}

func baseURLsFrom(addresses []net.Addr, port int) []string {
	var hosts []string
	for _, address := range addresses {
		ip := addressIP(address)
		// IPv6 is skipped deliberately: link-local addresses carry a zone that
		// does not survive being typed into a phone, and a home network that
		// only has IPv6 is rare enough to not be worth the confusion.
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.To4() == nil {
			continue
		}
		hosts = append(hosts, ip.String())
	}
	sort.Strings(hosts)
	urls := make([]string, 0, len(hosts))
	for index, host := range hosts {
		if index > 0 && host == hosts[index-1] {
			continue
		}
		urls = append(urls, fmt.Sprintf("https://%s:%d", host, port))
	}
	return urls
}

func addressIP(address net.Addr) net.IP {
	switch value := address.(type) {
	case *net.IPNet:
		return value.IP
	case *net.IPAddr:
		return value.IP
	default:
		return nil
	}
}
