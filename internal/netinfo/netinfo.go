// Package netinfo enumerates the LAN addresses this show can be reached on
// (D-83/CONN-02).
//
// The package is deliberately tiny and pure: it reads a machine's interfaces
// once, at boot, and answers with join-URL candidates. It never probes routing,
// never touches the network, and never decides which candidate is selected —
// the presenter does that, and the selection is persisted server state (D-84).
package netinfo

import (
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
)

// Interface is ONE system interface plus the addresses configured on it.
//
// It is a projection of net.Interface rather than net.Interface itself because
// the injectable seam has to carry the addresses. net.Interface has no address
// field at all: its Addrs() method queries the LIVE kernel table by interface
// index (net/interface_linux.go: interfaceAddrTable → addrTable). A test that
// built a fake []net.Interface would therefore read the test machine's real
// addresses — verified on Go 1.26.4, where net.Interface{Index: 1}.Addrs()
// returns this machine's 127.0.0.1/8 whatever the test intended. D-83 requires
// a fake IP table, so the seam carries the table.
type Interface struct {
	// Name is the interface's kernel name, and it decides candidacy along with
	// the flags: a container runtime's bridge or veth pair is skipped by name
	// (D-83's dated addendum, 2026-09-16 — see isContainerInterface). Any other
	// name is treated as an ordinary interface.
	Name string
	// Flags decide candidacy: net.FlagUp and net.FlagLoopback only (D-83).
	Flags net.Flags
	// Addrs are the addresses this interface holds, as net.Interface.Addrs()
	// returns them (*net.IPNet).
	Addrs []net.Addr
}

// InterfaceSource is the injectable seam. Production passes System; a test
// passes a table of fake interfaces (D-83: "injectable so tests use a fake IP
// table").
type InterfaceSource func() ([]Interface, error)

// System is the production source: the machine's own interfaces, each with the
// addresses the kernel reports. An interface whose address list cannot be read
// is skipped rather than fatal — enumeration is best-effort and must never stop
// the boot (T-04-11; a machine with no private address simply offers no
// candidate, and the admin's "this page's URL" fallback still works, D-86).
func System() ([]Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("netinfo: listing the system interfaces: %w", err)
	}

	out := make([]Interface, 0, len(ifaces))
	for _, ifi := range ifaces {
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		out = append(out, Interface{Name: ifi.Name, Flags: ifi.Flags, Addrs: addrs})
	}
	return out, nil
}

// Enumerate answers D-83's candidate list: one `http://<ip>:<port>/` URL per
// private IPv4 address on an UP, non-loopback, non-container interface, ranked
// 192.168/16, then 10/8, then 172.16/12, and ordered by address inside one
// class.
//
// The secondary sort key is not decoration: the OS decides the order
// net.Interfaces() reports, D-84's default selection is "the first candidate",
// and without a total order that default would flicker between boots.
//
// Container interfaces are excluded by NAME (see isContainerInterface), which is
// the one addition to D-83 since it was written: a bridge address is a real
// address the machine answers on, but it is never an address a phone in the room
// can reach, and on a Docker host it filled 6 of the presenter's 7 candidate
// rows (UI-review finding 6). The exclusion is name-based, never IP-range-based:
// an ordinary interface holding a private 172.17.x address stays eligible.
func Enumerate(src InterfaceSource, port int) ([]string, error) {
	ifaces, err := src()
	if err != nil {
		return nil, fmt.Errorf("netinfo: enumerating interfaces: %w", err)
	}

	candidates := make([]netip.Addr, 0, len(ifaces))
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue // D-83: down and loopback interfaces are never offered
		}
		if isContainerInterface(ifi.Name) {
			continue // D-83 addendum: container bridges and veth pairs are never offered
		}
		for _, a := range ifi.Addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			addr, ok := netip.AddrFromSlice(ipnet.IP)
			if !ok {
				continue
			}
			// Unmap before classifying: an IPv4-mapped IPv6 address
			// (::ffff:10.0.0.7) is not Is4, and would otherwise slip past the
			// private-IPv4 rule entirely.
			addr = addr.Unmap()
			if !addr.Is4() || !addr.IsPrivate() || addr.IsLoopback() {
				continue
			}
			candidates = append(candidates, addr)
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		ri, rj := rank(candidates[i]), rank(candidates[j])
		if ri != rj {
			return ri < rj
		}
		return candidates[i].Less(candidates[j])
	})

	urls := make([]string, 0, len(candidates))
	for _, addr := range candidates {
		urls = append(urls, fmt.Sprintf("http://%s:%d/", addr, port))
	}
	return urls, nil
}

// isContainerInterface reports whether an interface name belongs to a container
// runtime's private namespace. Six families, and only these six:
//
//   - docker0        Docker's default bridge
//   - br-*           Docker's user-defined bridges
//   - veth*          the host end of a container's veth pair
//   - virbr*         the libvirt bridge
//   - podman*        the Podman bridge
//   - cni*           the Kubernetes/CNI bridge
//
// The predicate is deliberately narrow: it is a name test, not an address test,
// so an ordinary interface that happens to hold 172.17.0.42 (a network that
// really uses that block) is still a candidate. Anything unrecognized keeps the
// D-83 behaviour of being offered to the presenter.
func isContainerInterface(name string) bool {
	if name == "docker0" {
		return true
	}
	for _, prefix := range []string{"br-", "veth", "virbr", "podman", "cni"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// rank is D-83's preference: a 192.168/16 address is offered first, then 10/8,
// then the rest of 172.16/12. Enumerate only ever passes an IPv4 address, so
// the byte access below is safe by construction — the guard keeps a future
// caller from panicking instead.
func rank(addr netip.Addr) int {
	if !addr.Is4() {
		return 3
	}
	b := addr.As4()
	switch {
	case b[0] == 192 && b[1] == 168:
		return 0
	case b[0] == 10:
		return 1
	default:
		return 2
	}
}
