// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package modules

import (
	"strings"
	"testing"
)

func TestCheckBind(t *testing.T) {
	cases := []struct {
		addr, allow, want string // want: "" ok, else a substring of the error
	}{
		{"127.0.0.1:20111", "", ""},
		{"[::1]:1", "", ""},
		{"0.0.0.0:20111", "", "wildcard"},
		{"[::]:1", "", "wildcard"},
		{":20111", "", "wildcard"},
		{"8.8.8.8:1", "", "globally routable"},
		{"[2606:4700::1]:1", "", "globally routable"},
		{"192.168.69.130:20103", "", "not on this host's allowlist"},
		{"192.168.69.130:20103", "192.168.69.128/25", ""},
		{"[fd00:dead:beef::1]:20111", "", "not on this host's allowlist"},
		{"[fd00:dead:beef::1]:20111", "fd00:dead:beef::1/128", ""},
		{"169.254.1.1:1", "", "link-local"},
		{"[fe80::1%eth0]:1", "", "link-local"},
		{"localhost:1", "", "IP literal"},
		{"8.8.8.8:1", "10.0.0.0/8", "globally routable"},
		{"no-port", "", "host:port"},
		{"[::ffff:8.8.8.8]:1", "", "globally routable"},
		{"[::ffff:127.0.0.1]:1", "", ""},
	}
	for _, c := range cases {
		allow, err := ParseBindAllow(c.allow)
		if err != nil {
			t.Fatalf("ParseBindAllow(%q): %v", c.allow, err)
		}
		err = CheckBind(c.addr, allow)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("CheckBind(%q, %q) = %v, want ok", c.addr, c.allow, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("CheckBind(%q, %q) = %v, want an error containing %q", c.addr, c.allow, err, c.want)
		}
	}
}

func TestParseBindAllow(t *testing.T) {
	got, err := ParseBindAllow("fd00:dead:beef::1/128, 192.168.69.128/25")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v; want two prefixes", got, err)
	}
	if got, err := ParseBindAllow(""); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v, %v", got, err)
	}
	if _, err := ParseBindAllow("10.0.0.0/8,garbage"); err == nil {
		t.Fatal("garbage accepted")
	}
	for _, bad := range []string{"0.0.0.0/0", "8.8.8.0/24", "169.254.0.0/16", "fe80::/64", "::/0", "10.0.0.1"} {
		_, err := ParseBindAllow(bad)
		if err == nil {
			t.Errorf("%q accepted", bad)
			continue
		}
		if bad != "10.0.0.1" && !strings.Contains(err.Error(), "not private") {
			t.Errorf("%q: %v, want \"not private\"", bad, err)
		}
	}
	// A private prefix wider than its private range is refused too.
	if _, err := ParseBindAllow("10.0.0.0/7"); err == nil {
		t.Error("10.0.0.0/7 accepted: it covers 11.0.0.0/8")
	}
}
