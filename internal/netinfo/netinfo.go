// Package netinfo enumerates the LAN addresses this show can be reached on
// (D-83/CONN-02).
//
// The package is deliberately tiny and pure: it reads a machine's interfaces
// once, at boot, and answers with join-URL candidates. It never probes routing,
// never touches the network, and never decides which candidate is selected —
// the presenter does that, and the selection is persisted server state (D-84).
package netinfo

import (
	"errors"
	"net"
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
	// Name is informational only. D-83 deliberately has NO interface-name
	// deny-list: a Docker bridge is a legitimate candidate if the machine has
	// no other private address.
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

// Enumerate answers D-83's candidate list: one `http://<ip>:<port>/` URL per
// private IPv4 address on an UP, non-loopback interface, ranked 192.168/16,
// then 10/8, then 172.16/12, and ordered by address inside one class.
func Enumerate(src InterfaceSource, port int) ([]string, error) {
	return nil, errors.New("netinfo: Enumerate is not implemented yet")
}
