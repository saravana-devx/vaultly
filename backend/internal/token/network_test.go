package token

import (
	"fmt"
	"reflect"
	"testing"
)

func TestNormCIDRs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
		ok   bool
	}{
		{"empty", nil, []string{}, true},
		{"blank entries skipped", []string{""}, []string{}, true},
		{"single IPv4 becomes /32", []string{"10.0.0.5"}, []string{"10.0.0.5/32"}, true},
		{"single IPv6 becomes /128", []string{"2001:db8::1"}, []string{"2001:db8::1/128"}, true},
		{"block is canonicalised", []string{"10.1.2.3/8"}, []string{"10.0.0.0/8"}, true},
		{"garbage rejected", []string{"not-an-ip"}, nil, false},
		{"bad mask rejected", []string{"10.0.0.0/33"}, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := normCIDRs(c.in)
			if ok != c.ok || (c.ok && !reflect.DeepEqual(got, c.want)) {
				t.Fatalf("got %v, %v; want %v, %v", got, ok, c.want, c.ok)
			}
		})
	}
}

func TestNormCIDRsLimit(t *testing.T) {
	in := make([]string, 21)
	for i := range in {
		in[i] = fmt.Sprintf("10.0.0.%d", i)
	}
	if _, ok := normCIDRs(in[:20]); !ok {
		t.Fatal("20 entries should be accepted")
	}
	if _, ok := normCIDRs(in); ok {
		t.Fatal("21 entries should be rejected")
	}
}

func TestIPAllowed(t *testing.T) {
	cidrs := []string{"10.0.0.0/8", "203.0.113.7/32", "2001:db8::/32"}
	cases := []struct {
		ip   string
		want bool
	}{
		{"10.20.30.40", true},
		{"203.0.113.7", true},
		{"203.0.113.8", false},
		{"2001:db8::abcd", true},
		{"2001:db9::1", false},
		{"192.168.1.1", false},
		{"", false},
		{"garbage", false},
	}
	for _, c := range cases {
		if got := ipAllowed(cidrs, c.ip); got != c.want {
			t.Errorf("ipAllowed(%q) = %v, want %v", c.ip, got, c.want)
		}
	}
	if !ipAllowed(nil, "198.51.100.1") {
		t.Error("no allowlist means any IP is allowed")
	}
}
