package netutil

import (
	"net/http"
	"net/netip"
	"testing"
)

func TestEndpointsAndProxyTrust(t *testing.T) {
	for _, s := range []string{"127.0.0.1:65536", "127.0.0.1:-1", "[::1]:80", "127.0.0.999:80", "host:80"} {
		if _, err := ParseV4(s, 80); err == nil {
			t.Fatal("accepted", s)
		}
	}
	a, err := ParseV4("192.168.1.1", 80)
	if err != nil || a.String() != "192.168.1.1:80" {
		t.Fatal(a, err)
	}
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	r := &http.Request{RemoteAddr: "10.0.0.1:4000", Header: make(http.Header)}
	r.Header.Set("X-Forwarded-For", "192.0.2.99, 198.51.100.7, 10.0.0.2")
	ip, err := ClientIP(r, trusted)
	if err != nil || ip.String() != "198.51.100.7" {
		t.Fatal(ip, err)
	}
	if r.RemoteAddr != "10.0.0.1:4000" {
		t.Fatal("request mutated")
	}
	ip, err = ClientIP(r, nil)
	if err != nil || ip.String() != "10.0.0.1" {
		t.Fatal(ip, err)
	}
	r.Header.Set("X-Forwarded-For", "bad")
	if _, err := ClientIP(r, trusted); err == nil {
		t.Fatal("bad proxy header accepted")
	}
	r.RemoteAddr = "[::ffff:192.0.2.5]:90"
	ip, err = PeerIP(r)
	if err != nil || ip.String() != "192.0.2.5" {
		t.Fatal(ip, err)
	}
}
