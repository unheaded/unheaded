// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
)

// fakeRunner answers fixed commands; anything else fails. No SSH in tests.
type fakeRunner struct {
	out   map[string]string
	fail  map[string]bool
	calls []string
}

func (f *fakeRunner) Run(_ context.Context, cmd string) (string, error) {
	f.calls = append(f.calls, cmd)
	if f.fail[cmd] {
		return "", errors.New("exit status 1")
	}
	if o, ok := f.out[cmd]; ok {
		return o, nil
	}
	return "", errors.New("unexpected command")
}

// A real-looking WireGuard key (random base64, 32 bytes): the shape the
// guard must catch. Not a key anyone uses.
const fakePSK = "q3mJ0n4kYpVxB8sT2wLz7cR1uE5hG9aD6fK0iN3oP+Q="
const fakePeer = "Zx9vW2bN7mQ4tR8yU1pL6kJ3hG5fD0sA9qE2wC7xV+M="

func eastRunner() *fakeRunner {
	return &fakeRunner{out: map[string]string{
		hostCommands.sshV:     "OpenSSH_10.0p2 Ubuntu-5ubuntu5.4, OpenSSL 3.5.3 16 Sep 2025\n",
		hostCommands.sshdT:    "kexalgorithms mlkem768x25519-sha256,sntrup761x25519-sha512,curve25519-sha256\nciphers chacha20-poly1305@openssh.com,aes256-gcm@openssh.com\n",
		hostCommands.openssl:  "OpenSSL 3.5.3 16 Sep 2025 (Library: OpenSSL 3.5.3 16 Sep 2025)\n",
		hostCommands.wgIfaces: "wg0\n",
		hostCommands.wgPSK:    "wg0 set\n",
		hostCommands.pskDates: "2026-03-25\n",
		hostCommands.postgres: "",
	}}
}

func TestCollectHost_East(t *testing.T) {
	f, err := collectHost(context.Background(), "east", eastRunner())
	if err != nil {
		t.Fatal(err)
	}
	if f.Unreachable || f.SSHD == nil || f.SSHD.Kex[0] != "mlkem768x25519-sha256" {
		t.Fatalf("facts = %+v", f)
	}
	if f.WireGuard == nil || len(f.WireGuard.Peers) != 1 || f.WireGuard.Peers[0] != (WGPeer{"wg0", "set"}) {
		t.Fatalf("wireguard = %+v", f.WireGuard)
	}
	if got := f.WireGuard.PSKFileDates; len(got) != 1 || got[0] != "2026-03-25" {
		t.Errorf("psk dates = %v", got)
	}
}

func TestCollectHost_RunsOnlyReadOnlyCommands(t *testing.T) {
	r := eastRunner()
	if _, err := collectHost(context.Background(), "east", r); err != nil {
		t.Fatal(err)
	}
	mutating := regexp.MustCompile(`\b(set|rm|mv|tee|chmod|chown|systemctl|reload|restart|>[^&/]|genkey|genpsk)\b`)
	for _, c := range r.calls {
		// "wg show" and awk's "set" literal are fine; anything writing is not.
		probe := strings.ReplaceAll(c, `"set"`, "")
		probe = strings.ReplaceAll(probe, "2>/dev/null", "")
		probe = strings.ReplaceAll(probe, "2>&1", "")
		if mutating.MatchString(probe) {
			t.Errorf("collect-host ran a command that is not read-only: %q", c)
		}
	}
}

func TestCollectHost_Unreachable(t *testing.T) {
	r := &fakeRunner{fail: map[string]bool{hostCommands.sshV: true, hostCommands.sshdT: true}}
	f, err := collectHost(context.Background(), "east", r)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Unreachable || f.SSHD != nil {
		t.Errorf("want unreachable with no facts, got %+v", f)
	}
}

func TestCollectHost_RejectsBadHostName(t *testing.T) {
	for _, h := range []string{"", "East", "east;rm -rf /", "192.168.13.2", "a/b", strings.Repeat("a", 64)} {
		if _, err := collectHost(context.Background(), h, eastRunner()); err == nil {
			t.Errorf("host %q accepted", h)
		}
	}
}

// Step 65: raw `wg show all preshared-keys` output (as if the host-side awk
// were bypassed) must reduce to set/none with no key surviving.
func TestCollectHostNoKeyMaterial(t *testing.T) {
	r := eastRunner()
	r.out[hostCommands.wgPSK] = "wg0\t" + fakePeer + "\t" + fakePSK + "\nwg1\t" + fakePeer + "\t(none)\n"
	r.out[hostCommands.wgIfaces] = "wg0 wg1\n"
	f, err := collectHost(context.Background(), "east", r)
	if err != nil {
		t.Fatal(err)
	}
	b, err := encodeHostFacts(f)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if regexp.MustCompile(`[A-Za-z0-9+/]{43}=`).Match(b) {
		t.Fatalf("host facts contain a 44-char base64 string:\n%s", b)
	}
	if strings.Contains(string(b), fakePSK[:10]) || strings.Contains(string(b), fakePeer[:10]) {
		t.Fatalf("key material survived:\n%s", b)
	}
	want := []WGPeer{{"wg0", "set"}, {"wg1", "none"}}
	if len(f.WireGuard.Peers) != 2 || f.WireGuard.Peers[0] != want[0] || f.WireGuard.Peers[1] != want[1] {
		t.Errorf("peers = %+v, want %+v", f.WireGuard.Peers, want)
	}
}

func TestEncodeHostFacts_RefusesKeysAndAddresses(t *testing.T) {
	tests := map[string]*HostFacts{
		"key in version": {Host: "east", OpenSSL: "x " + fakePSK},
		"ipv4":           {Host: "east", Postgres: "listening on 192.168.13.2"},
		"ula ipv6":       {Host: "east", Postgres: "fd00:dead:beef::1"},
	}
	for name, f := range tests {
		if _, err := encodeHostFacts(f); !errors.Is(err, errLeak) {
			t.Errorf("%s: err = %v, want errLeak", name, err)
		}
	}
	if _, err := encodeHostFacts(&HostFacts{Host: "east", OpenSSL: "OpenSSL 3.5.3 16 Sep 2025"}); err != nil {
		t.Errorf("clean facts refused: %v", err)
	}
}

func TestParseWG_HostileLines(t *testing.T) {
	w := parseWG("wg0 ../../etc bad;name\n", "wg0 maybe\nwg0\n\n;;; set\nwg0 set extra junk here\n", "garbage 2026-03-25T10:00 x")
	if w == nil || len(w.Interfaces) != 1 || w.Interfaces[0] != "wg0" {
		t.Fatalf("interfaces = %+v", w)
	}
	if len(w.Peers) != 0 {
		t.Errorf("hostile peer lines accepted: %+v", w.Peers)
	}
	if len(w.PSKFileDates) != 1 || w.PSKFileDates[0] != "2026-03-25" {
		t.Errorf("dates = %v", w.PSKFileDates)
	}
}

func TestSSHTargetValidation(t *testing.T) {
	for _, ok := range []string{"govan@east", "east", "a.b-c@h_1"} {
		if !reSSHTarget.MatchString(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"-oProxyCommand=x", "a b@c", "a@b;c", "", "@east"} {
		if reSSHTarget.MatchString(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestScanHosts(t *testing.T) {
	assets, err := scanHosts(fixtureTree(t, "config"))
	if err != nil {
		t.Fatal(err)
	}
	sshd := one(t, assets, "compliance/crypto/hosts/east.json", 0, kindHostSSHD)
	if sshd.PQStatus != pqHybridOpp {
		t.Errorf("east sshd = %q, want %q", sshd.PQStatus, pqHybridOpp)
	}
	wg := one(t, assets, "compliance/crypto/hosts/east.json", 0, kindHostWG)
	if wg.PQStatus != pqPSKStatic || !strings.Contains(wg.Detail, "2026-03-25") {
		t.Errorf("east wg = %+v", wg)
	}
	if u := one(t, assets, "compliance/crypto/hosts/north.json", 0, kindHostUnreachable); u.PQStatus != pqUnknown {
		t.Errorf("unreachable host = %q, want unknown", u.PQStatus)
	}
}

func TestScanHosts_RejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "compliance/crypto/hosts/x.json", `{"host":"x","psk":"`+fakePSK+`"}`)
	tr, err := dirTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanHosts(tr); err == nil {
		t.Error("an unknown field (here a psk) must fail the scan")
	}
}
