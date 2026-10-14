package sshendpoint

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Endpoint is an OpenSSH destination with an optional explicit port.
// Host aliases and unbracketed IPv6 literals are left intact; an IPv6 address
// with a port must use the conventional [address]:port form.
type Endpoint struct {
	Host string
	Port int
}

// Parse accepts the values users naturally enter in an SSH host field:
// hostname, IPv4 address, SSH config alias, host:port, raw IPv6, or
// [IPv6]:port.
func Parse(value string) (Endpoint, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Endpoint{}, fmt.Errorf("SSH host is required")
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return Endpoint{}, fmt.Errorf("SSH host contains invalid characters")
	}

	if strings.HasPrefix(value, "[") {
		closing := strings.IndexByte(value, ']')
		if closing < 0 {
			return Endpoint{}, fmt.Errorf("invalid SSH host %q: missing closing bracket", value)
		}
		host := value[1:closing]
		if host == "" {
			return Endpoint{}, fmt.Errorf("SSH host is required")
		}
		if closing == len(value)-1 {
			return Endpoint{Host: host}, nil
		}
		if value[closing+1] != ':' {
			return Endpoint{}, fmt.Errorf("invalid SSH host %q", value)
		}
		port, err := parsePort(value[closing+2:])
		if err != nil {
			return Endpoint{}, err
		}
		return Endpoint{Host: host, Port: port}, nil
	}

	// Exactly one colon is unambiguously host:port. Multiple colons are an
	// unbracketed IPv6 literal and deliberately remain a host-only value.
	if strings.Count(value, ":") == 1 {
		host, portText, err := net.SplitHostPort(value)
		if err != nil || strings.TrimSpace(host) == "" {
			return Endpoint{}, fmt.Errorf("invalid SSH host %q; use host:port", value)
		}
		port, err := parsePort(portText)
		if err != nil {
			return Endpoint{}, err
		}
		return Endpoint{Host: host, Port: port}, nil
	}

	return Endpoint{Host: value}, nil
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("SSH port %q must be a number from 1 to 65535", value)
	}
	return port, nil
}

func (e Endpoint) String() string {
	if e.Port == 0 {
		return e.Host
	}
	return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
}
