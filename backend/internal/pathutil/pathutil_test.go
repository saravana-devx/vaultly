package pathutil

import (
	"strings"
	"testing"
)

func TestNorm(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", "/", true},
		{"/", "/", true},
		{"stripe", "/stripe", true},
		{"/a//b/", "/a/b", true},
		{"  /a/b  ", "/a/b", true},
		{"/a/./b", "", false},
		{"/a/../b", "", false},
		{"..", "", false},
		{"/" + strings.Repeat("x", 64), "/" + strings.Repeat("x", 64), true},
		{"/" + strings.Repeat("x", 65), "", false},
	}
	for _, c := range cases {
		got, ok := Norm(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("Norm(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestJoin(t *testing.T) {
	if got := Join("/", "KEY"); got != "/KEY" {
		t.Errorf("got %q", got)
	}
	if got := Join("/a/b", "KEY"); got != "/a/b/KEY" {
		t.Errorf("got %q", got)
	}
}

func TestWithin(t *testing.T) {
	cases := []struct {
		prefix, path string
		want         bool
	}{
		{"/", "/", true},
		{"/", "/anything/deep", true},
		{"/stripe", "/stripe", true},
		{"/stripe", "/stripe/live", true},
		{"/stripe", "/stripe2", false}, // sibling with a shared prefix must not match
		{"/stripe", "/", false},
		{"/stripe/live", "/stripe", false},
	}
	for _, c := range cases {
		if got := Within(c.prefix, c.path); got != c.want {
			t.Errorf("Within(%q, %q) = %v, want %v", c.prefix, c.path, got, c.want)
		}
	}
}
