// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	pqtls "unheaded/pkg/tls"
	"unheaded/pkg/uids"
)

const serviceUnit = "../../deploy/systemd/unheaded-zhen-ui-front.service"

// directives reads KEY=value lines, every value per key, in order.
func directives(t *testing.T, path string) map[string][]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || line[0] == '#' || line[0] == '[' {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			out[k] = append(out[k], v)
		}
	}
	return out
}

func one(t *testing.T, d map[string][]string, key string) string {
	t.Helper()
	if len(d[key]) != 1 {
		t.Fatalf("%s=%q, want exactly one", key, d[key])
	}
	return d[key][0]
}

// The unit's command line parses, and the kernel's bind list names exactly
// the ports it asks for.
func TestUnitExecStartParsesAndMatchesBindList(t *testing.T) {
	svc := directives(t, serviceUnit)
	argv := strings.Fields(one(t, svc, "ExecStart"))
	if argv[0] != "/usr/local/bin/zhen-ui-front" {
		t.Errorf("binary %s", argv[0])
	}
	cfg, err := parseFlags(argv[1:], io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("the unit's ExecStart does not parse: %v", err)
	}
	var want []string
	for _, l := range cfg.listens {
		want = append(want, fmt.Sprintf("tcp:%d", l.Port()))
	}
	want = append(want, fmt.Sprintf("tcp:%d", cfg.metrics.Port()))
	got := slices.Clone(svc["SocketBindAllow"])
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, slices.Compact(want)) {
		t.Errorf("SocketBindAllow %v, but ExecStart binds %v", got, want)
	}
	if v := one(t, svc, "SocketBindDeny"); v != "any" {
		t.Errorf("SocketBindDeny=%s", v)
	}
	for _, l := range cfg.listens {
		if !l.Addr().IsLoopback() {
			t.Errorf("the shipped unit listens on %s; a LAN address is a host's drop-in, with its IPAddressAllow", l)
		}
	}
	if !filepath.IsAbs(cfg.socket) || !strings.HasPrefix(cfg.socket, "/run/") {
		t.Errorf("socket %q is not under /run", cfg.socket)
	}
}

func TestUnitRunsAsItsOwnAccountWithNoCapability(t *testing.T) {
	svc := directives(t, serviceUnit)
	if uid, err := uids.Lookup(serviceName); err != nil || uid != uids.ZhenUIFront {
		t.Fatalf("pkg/uids: %s is %d, %v", serviceName, uid, err)
	}
	account := "unheaded-" + serviceName
	if got := one(t, svc, "User"); got != account {
		t.Errorf("User=%s, want %s", got, account)
	}
	if got := one(t, svc, "Group"); got != account {
		t.Errorf("Group=%s, want %s", got, account)
	}
	if v, ok := svc["AmbientCapabilities"]; ok {
		t.Errorf("AmbientCapabilities=%v", v)
	}
	if got := one(t, svc, "CapabilityBoundingSet"); got != "" {
		t.Errorf("CapabilityBoundingSet=%q, want it empty", got)
	}
	for key, want := range map[string]string{
		"PrivateUsers":            "yes",
		"NoNewPrivileges":         "yes",
		"ProtectSystem":           "strict",
		"ProtectHome":             "yes",
		"MemoryDenyWriteExecute":  "yes",
		"IPAddressDeny":           "any",
		"RestrictAddressFamilies": "AF_UNIX AF_INET AF_INET6",
	} {
		if got := one(t, svc, key); got != want {
			t.Errorf("%s=%s, want %s", key, got, want)
		}
	}
	// It writes nothing: no state, no socket of its own.
	for _, key := range []string{"ReadWritePaths", "StateDirectory", "RuntimeDirectory", "SupplementaryGroups"} {
		if v, ok := svc[key]; ok {
			t.Errorf("%s=%v: the front owns no directory and joins no group", key, v)
		}
	}
}

// The certificate and key come from the environment the unit sets, in the
// identity's own directory, and nothing in the unit can turn Strict off.
func TestUnitTLSEnvironment(t *testing.T) {
	svc := directives(t, serviceUnit)
	env := map[string]string{}
	for _, e := range svc["Environment"] {
		k, v, _ := strings.Cut(e, "=")
		env[k] = v
	}
	if env[pqtls.EnvEnabled] != "true" {
		t.Errorf("%s=%q: the .deb's postinst reads it to find the files the unit needs", pqtls.EnvEnabled, env[pqtls.EnvEnabled])
	}
	if got, want := env[pqtls.EnvCert], "/etc/unheaded/tls/zhen-ui-front/zhen-ui-front.pem"; got != want {
		t.Errorf("%s=%q, want %q", pqtls.EnvCert, got, want)
	}
	if got, want := env[pqtls.EnvKeyFile], "/etc/unheaded/tls/zhen-ui-front/zhen-ui-front-key.pem"; got != want {
		t.Errorf("%s=%q, want %q", pqtls.EnvKeyFile, got, want)
	}
	for _, k := range []string{pqtls.EnvProfile, pqtls.EnvMTLS, pqtls.EnvCACert} {
		if v, ok := env[k]; ok {
			t.Errorf("%s=%q is set; the front reads neither a profile nor a CA", k, v)
		}
	}
	if _, ok := env["GOMEMLIMIT"]; !ok {
		t.Error("GOMEMLIMIT is not set")
	}
}
