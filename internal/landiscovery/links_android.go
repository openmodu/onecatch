//go:build android

package landiscovery

import (
	"errors"
	"net"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Android 11+ denies bind(2) on NETLINK_ROUTE sockets for apps, so Go's
// net.Interfaces fails with "netlinkrib: permission denied". RTM_GETADDR still
// works on an unbound socket; names and flags then come from ioctl.
func systemLinks() ([]link, error) {
	if links, err := netLinks(); err == nil {
		return links, nil
	}
	return netlinkLinks()
}

func netlinkLinks() ([]link, error) {
	msgs, err := routeDump(syscall.RTM_GETADDR)
	if err != nil {
		return nil, err
	}
	byIndex := map[int]*link{}
	var order []int
	for _, m := range msgs {
		if m.Header.Type != syscall.RTM_NEWADDR || len(m.Data) < syscall.SizeofIfAddrmsg {
			continue
		}
		ifam := (*syscall.IfAddrmsg)(unsafe.Pointer(&m.Data[0]))
		attrs, err := syscall.ParseNetlinkRouteAttr(&m)
		if err != nil {
			continue
		}
		ip := addrAttr(ifam.Family, attrs)
		if ip == nil {
			continue
		}
		index := int(ifam.Index)
		l := byIndex[index]
		if l == nil {
			iface, err := interfaceByIndex(index)
			if err != nil {
				continue
			}
			l = &link{iface: iface}
			byIndex[index] = l
			order = append(order, index)
		}
		l.ips = append(l.ips, ip)
	}
	links := make([]link, 0, len(order))
	for _, index := range order {
		links = append(links, *byIndex[index])
	}
	return links, nil
}

// IFA_LOCAL is the interface's own address on point-to-point links, where
// IFA_ADDRESS is the peer; otherwise both carry the same address.
func addrAttr(family uint8, attrs []syscall.NetlinkRouteAttr) net.IP {
	var address, local []byte
	for _, a := range attrs {
		switch a.Attr.Type {
		case syscall.IFA_ADDRESS:
			address = a.Value
		case syscall.IFA_LOCAL:
			local = a.Value
		}
	}
	if family == syscall.AF_INET && local != nil {
		address = local
	}
	switch {
	case family == syscall.AF_INET && len(address) == net.IPv4len,
		family == syscall.AF_INET6 && len(address) == net.IPv6len:
		return net.IP(append([]byte(nil), address...))
	}
	return nil
}

// routeDump mirrors syscall.NetlinkRIB without the bind that Android rejects;
// the kernel assigns the port id on the first send.
func routeDump(proto int) ([]syscall.NetlinkMessage, error) {
	s, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, syscall.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(s)
	sa := &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}
	req := make([]byte, syscall.NLMSG_HDRLEN+syscall.SizeofRtGenmsg)
	hdr := (*syscall.NlMsghdr)(unsafe.Pointer(&req[0]))
	hdr.Len = uint32(len(req))
	hdr.Type = uint16(proto)
	hdr.Flags = syscall.NLM_F_DUMP | syscall.NLM_F_REQUEST
	hdr.Seq = 1
	req[syscall.NLMSG_HDRLEN] = syscall.AF_UNSPEC
	if err := syscall.Sendto(s, req, 0, sa); err != nil {
		return nil, err
	}
	lsa, err := syscall.Getsockname(s)
	if err != nil {
		return nil, err
	}
	local, ok := lsa.(*syscall.SockaddrNetlink)
	if !ok {
		return nil, syscall.EINVAL
	}
	var all []syscall.NetlinkMessage
	buf := make([]byte, 32*1024)
	for {
		n, _, err := syscall.Recvfrom(s, buf, 0)
		if err != nil {
			return nil, err
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return nil, err
		}
		for _, m := range msgs {
			if m.Header.Seq != 1 || m.Header.Pid != local.Pid {
				return nil, syscall.EINVAL
			}
			switch m.Header.Type {
			case syscall.NLMSG_DONE:
				return all, nil
			case syscall.NLMSG_ERROR:
				return nil, errors.New("netlink dump failed")
			}
			m.Data = append([]byte(nil), m.Data...)
			all = append(all, m)
		}
	}
}

func interfaceByIndex(index int) (net.Interface, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return net.Interface{}, err
	}
	defer unix.Close(fd)
	ifr, err := unix.NewIfreq("")
	if err != nil {
		return net.Interface{}, err
	}
	ifr.SetUint32(uint32(index))
	if err := unix.IoctlIfreq(fd, unix.SIOCGIFNAME, ifr); err != nil {
		return net.Interface{}, err
	}
	name := ifr.Name()
	if ifr, err = unix.NewIfreq(name); err != nil {
		return net.Interface{}, err
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCGIFFLAGS, ifr); err != nil {
		return net.Interface{}, err
	}
	raw := ifr.Uint16()
	iface := net.Interface{Index: index, Name: name}
	for _, f := range []struct {
		raw  uint16
		flag net.Flags
	}{
		{unix.IFF_UP, net.FlagUp},
		{unix.IFF_BROADCAST, net.FlagBroadcast},
		{unix.IFF_LOOPBACK, net.FlagLoopback},
		{unix.IFF_POINTOPOINT, net.FlagPointToPoint},
		{unix.IFF_MULTICAST, net.FlagMulticast},
		{unix.IFF_RUNNING, net.FlagRunning},
	} {
		if raw&f.raw != 0 {
			iface.Flags |= f.flag
		}
	}
	if ifr, err = unix.NewIfreq(name); err == nil && unix.IoctlIfreq(fd, unix.SIOCGIFMTU, ifr) == nil {
		iface.MTU = int(ifr.Uint32())
	}
	return iface, nil
}
