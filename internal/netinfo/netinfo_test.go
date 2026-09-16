package netinfo

import (
	"errors"
	"net"
	"slices"
	"strings"
	"testing"
)

// ipnet builds one *net.IPNet the way net.Interface.Addrs() reports one: the
// interface's own address plus its mask. ParseCIDR's second return value is the
// MASKED network, and using it would hand the ranker 192.168.0.0 where the
// machine really reported 192.168.0.103.
func ipnet(t *testing.T, cidr string) net.IPNet {
	t.Helper()

	ip, block, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("net.ParseCIDR(%q): %v", cidr, err)
	}
	return net.IPNet{IP: ip, Mask: block.Mask}
}

// source is the injectable seam over a literal table: no machine, no kernel.
func source(ifaces ...Interface) InterfaceSource {
	return func() ([]Interface, error) { return ifaces, nil }
}

// failingSource is the error arm.
func failingSource(err error) InterfaceSource {
	return func() ([]Interface, error) { return nil, err }
}

// TestEnumerate is D-83's ranking contract: keep private IPv4 from UP,
// non-loopback, non-container interfaces only, rank 192.168/16 before 10/8
// before 172.16/12, and order equal classes by address so the candidate list is
// total (the secondary sort is not optional — D-84's default selection is "the
// first candidate", and an OS-determined order would make that default flicker
// between boots).
func TestEnumerate(t *testing.T) {
	cases := []struct {
		name    string
		ifaces  []Interface
		want    []string
		wantErr bool
	}{
		{
			name: "ranks 192.168 before 10 before 172.16/12",
			ifaces: []Interface{
				{Name: "eth1", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "172.17.0.1/16"))}},
				{Name: "eth0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "10.0.0.7/8"))}},
				{Name: "wlan0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "192.168.0.103/24"))}},
			},
			want: []string{
				"http://192.168.0.103:8080/",
				"http://10.0.0.7:8080/",
				"http://172.17.0.1:8080/",
			},
		},
		{
			name: "orders one class by address",
			ifaces: []Interface{
				{Name: "enp8s0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "172.20.0.1/16"))}},
				{Name: "enp9s0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "172.18.0.1/16"))}},
				{Name: "wlan0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "192.168.1.5/24"))}},
				{Name: "eth0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "192.168.0.103/24"))}},
			},
			want: []string{
				"http://192.168.0.103:8080/",
				"http://192.168.1.5:8080/",
				"http://172.18.0.1:8080/",
				"http://172.20.0.1:8080/",
			},
		},
		{
			name: "the 172 block is bounded to 172.16-31",
			ifaces: []Interface{
				{Name: "a", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "172.32.0.1/16"))}},
				{Name: "b", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "172.15.0.1/16"))}},
				{Name: "c", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "172.31.255.254/12"))}},
				{Name: "d", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "172.16.0.1/12"))}},
			},
			want: []string{
				"http://172.16.0.1:8080/",
				"http://172.31.255.254:8080/",
			},
		},
		{
			name: "drops loopback, down and non-private addresses",
			ifaces: []Interface{
				{Name: "lo", Flags: net.FlagUp | net.FlagLoopback, Addrs: []net.Addr{ptrAddr(ipnet(t, "127.0.0.1/8"))}},
				{Name: "wlan0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "192.168.0.103/24"))}},
				{Name: "enp8s0", Flags: 0, Addrs: []net.Addr{ptrAddr(ipnet(t, "10.0.0.7/8"))}},
				{Name: "vpn0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "100.64.0.1/10"))}},
				{Name: "gw", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "8.8.8.8/32"))}},
				{Name: "ll", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "169.254.10.10/16"))}},
			},
			want: []string{"http://192.168.0.103:8080/"},
		},
		{
			name: "drops IPv6 and unwraps the IPv4-mapped form",
			ifaces: []Interface{
				{Name: "eth0", Flags: net.FlagUp, Addrs: []net.Addr{
					ptrAddr(ipnet(t, "fe80::1/64")),
					ptrAddr(ipnet(t, "fd00::1/64")),
					ptrAddr(ipnet(t, "::ffff:10.1.2.3/120")),
				}},
				{Name: "eth1", Flags: net.FlagUp, Addrs: []net.Addr{
					ptrAddr(ipnet(t, "::ffff:192.168.0.103/120")),
				}},
			},
			want: []string{
				"http://192.168.0.103:8080/",
				"http://10.1.2.3:8080/",
			},
		},
		{
			name: "tolerates an address-less interface and a non-IPNet address",
			ifaces: []Interface{
				{Name: "enp8s0", Flags: net.FlagUp},
				{Name: "tun0", Flags: net.FlagUp, Addrs: []net.Addr{&net.TCPAddr{IP: net.ParseIP("10.9.9.9"), Port: 5555}}},
				{Name: "wlan0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "10.0.0.7/8"))}},
			},
			want: []string{"http://10.0.0.7:8080/"},
		},
		{
			name:   "no candidates is an empty list, not an error",
			ifaces: []Interface{{Name: "lo", Flags: net.FlagUp | net.FlagLoopback}},
			want:   nil,
		},
		{
			name:    "the source error is reported",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := source(tc.ifaces...)
			if tc.wantErr {
				src = failingSource(errors.New("no interfaces"))
			}
			got, err := Enumerate(src, 8080)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Enumerate = %v, want the source error to be reported", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Enumerate: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Enumerate = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestEnumerateExcludesContainerInterfaces is UI-review finding 6 and D-83's
// dated addendum: a container bridge IS a private IPv4 address the machine
// answers on, but it is not an address a phone in the room can reach, and on a
// Docker host it floods the presenter's panel (6 of 7 rows measured live). The
// exclusion is by interface NAME — the address rule and its ranking are
// untouched, and each family is a separate sub-test so a failure names it.
func TestEnumerateExcludesContainerInterfaces(t *testing.T) {
	families := []struct {
		family string
		iface  string
		addr   string
	}{
		{"the Docker default bridge", "docker0", "172.17.0.1/16"},
		{"a Docker user-defined bridge", "br-1a2b3c4d5e6f", "172.18.0.1/16"},
		{"a container veth pair", "veth9f2a1b7c", "172.19.0.1/16"},
		{"the libvirt bridge", "virbr0", "192.168.122.1/24"},
		{"the Podman bridge", "podman0", "10.88.0.1/16"},
		{"the CNI bridge", "cni0", "10.89.0.1/16"},
	}

	for _, f := range families {
		t.Run(f.family, func(t *testing.T) {
			got, err := Enumerate(source(
				Interface{Name: f.iface, Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, f.addr))}},
				Interface{Name: "wlan0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "192.168.0.103/24"))}},
			), 8080)
			if err != nil {
				t.Fatalf("Enumerate: %v", err)
			}
			want := []string{"http://192.168.0.103:8080/"}
			if !slices.Equal(got, want) {
				t.Errorf("Enumerate = %v, want %v — %s must contribute no candidate", got, want, f.iface)
			}
		})
	}
}

// TestEnumerateKeepsOrdinaryPrivateAddresses is the other half of the rule: the
// exclusion is a NAME rule, never an IP-range one. A real 172.17.x address on an
// ordinary interface stays eligible (and keeps its 172.16/12 rank), because the
// presenter may well be on a network that really uses that block.
func TestEnumerateKeepsOrdinaryPrivateAddresses(t *testing.T) {
	got, err := Enumerate(source(
		Interface{Name: "enp8s0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "172.17.0.42/16"))}},
		Interface{Name: "wlan0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "192.168.0.103/24"))}},
	), 8080)
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	want := []string{
		"http://192.168.0.103:8080/",
		"http://172.17.0.42:8080/",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Enumerate = %v, want %v", got, want)
	}
}

// TestEnumerateUsesThePort is the other half of the URL contract: the port is
// the boot port, not a constant.
func TestEnumerateUsesThePort(t *testing.T) {
	got, err := Enumerate(source(Interface{
		Name: "wlan0", Flags: net.FlagUp, Addrs: []net.Addr{ptrAddr(ipnet(t, "192.168.0.103/24"))},
	}), 7070)
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	want := "http://192.168.0.103:7070/"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Enumerate = %v, want [%s]", got, want)
	}
	if strings.Count(got[0], ":") != 2 {
		t.Errorf("Enumerate = %q, want exactly one host:port pair", got[0])
	}
}

// ptrAddr is the *net.IPNet that Addrs() reports, as a net.Addr.
func ptrAddr(n net.IPNet) net.Addr {
	return &n
}
