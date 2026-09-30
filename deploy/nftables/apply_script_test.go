// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package nftables holds the host firewall rulesets (ADR-098 step 6); this
// test drives scripts/host/nft-apply-confirm.sh against fake nft, systemd-run
// and systemctl binaries.
package nftables

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type harness struct {
	t     *testing.T
	dir   string
	state string
	log   string
	env   []string
}

// fake nft: `list table inet unheaded` prints $DIR/table if it exists;
// `-f F` copies F to $DIR/table unless F contains FAIL_APPLY; `delete table`
// removes it; `-c -f F` fails if F contains FAIL_CHECK.
const fakeNft = `#!/usr/bin/env bash
echo "nft $*" >>"$FAKE_DIR/calls"
case "$*" in
"list table inet unheaded") [[ -f "$FAKE_DIR/table" ]] && cat "$FAKE_DIR/table" && exit 0; exit 1 ;;
"delete table inet unheaded") [[ -f "$FAKE_DIR/table" ]] && rm "$FAKE_DIR/table" && exit 0; exit 1 ;;
"-c -f "*) grep -q FAIL_CHECK "$3" && exit 1; exit 0 ;;
"-f "*) grep -q FAIL_APPLY "$2" && exit 1; grep -v '^table inet unheaded$\|^delete table' "$2" >"$FAKE_DIR/table"; exit 0 ;;
esac
exit 2
`

const fakeRecorder = `#!/usr/bin/env bash
echo "$(basename "$0") $*" >>"$FAKE_DIR/calls"
[[ -e "$FAKE_DIR/fail-$(basename "$0")" ]] && exit 1
exit 0
`

func newHarness(t *testing.T) *harness {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"nft": fakeNft, "systemd-run": fakeRecorder, "systemctl": fakeRecorder, "logger": "#!/bin/sh\nexit 0\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	h := &harness{t: t, dir: dir, state: filepath.Join(dir, "state"), log: filepath.Join(dir, "calls")}
	h.env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "FAKE_DIR="+dir,
		"UNHEADED_NFT_STATE="+h.state, "UNHEADED_NFT_ALLOW_NONROOT=1")
	return h
}

func (h *harness) run(args ...string) (string, error) {
	h.t.Helper()
	cmd := exec.Command("bash", append([]string{"../../scripts/host/nft-apply-confirm.sh"}, args...)...)
	cmd.Env = h.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (h *harness) ruleset(name, body string) string {
	h.t.Helper()
	p := filepath.Join(h.dir, name)
	if err := os.WriteFile(p, []byte("table inet unheaded\ndelete table inet unheaded\n"+body), 0o644); err != nil {
		h.t.Fatal(err)
	}
	return p
}

func (h *harness) calls() string {
	b, _ := os.ReadFile(h.log)
	return string(b)
}

func (h *harness) table() string {
	b, err := os.ReadFile(filepath.Join(h.dir, "table"))
	if err != nil {
		return "<none>"
	}
	return strings.TrimSpace(string(b))
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestApplyArmsBeforeApplyingThenConfirm(t *testing.T) {
	h := newHarness(t)
	f := h.ruleset("east.nft", "table inet unheaded { v1 }\n")
	if out, err := h.run("apply", f, "60"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	calls := h.calls()
	arm, apply := strings.Index(calls, "systemd-run --unit=unheaded-nft-rollback --on-active=60s"), strings.Index(calls, "nft -f "+f)
	if arm < 0 || apply < 0 || arm > apply {
		t.Fatalf("rollback must be armed before the ruleset is applied:\n%s", calls)
	}
	if h.table() != "table inet unheaded { v1 }" {
		t.Errorf("table = %q", h.table())
	}
	if out, err := h.run("apply", f); err == nil || !strings.Contains(out, "already pending") {
		t.Errorf("second apply while pending: %v %s", err, out)
	}
	if out, err := h.run("confirm"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(h.calls(), "systemctl stop unheaded-nft-rollback.timer") || !exists(filepath.Join(h.state, "confirmed.nft")) ||
		exists(filepath.Join(h.state, "pending.nft")) {
		t.Errorf("confirm did not stop the timer and record the ruleset:\n%s", h.calls())
	}
}

func TestRollbackRestoresPreviousTable(t *testing.T) {
	h := newHarness(t)
	v1 := h.ruleset("v1.nft", "table inet unheaded { v1 }\n")
	v2 := h.ruleset("v2.nft", "table inet unheaded { v2 }\n")
	if _, err := h.run("apply", v1); err != nil {
		t.Fatal(err)
	}
	if _, err := h.run("confirm"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.run("apply", v2); err != nil {
		t.Fatal(err)
	}
	if h.table() != "table inet unheaded { v2 }" {
		t.Fatalf("table = %q", h.table())
	}
	if out, err := h.run("rollback"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if h.table() != "table inet unheaded { v1 }" {
		t.Errorf("after rollback table = %q, want the confirmed v1", h.table())
	}
	if out, _ := h.run("rollback"); !strings.Contains(out, "nothing pending") {
		t.Errorf("second rollback: %s", out)
	}
}

func TestRollbackWithNoPreviousTableRemovesIt(t *testing.T) {
	h := newHarness(t)
	f := h.ruleset("east.nft", "table inet unheaded { v1 }\n")
	if _, err := h.run("apply", f); err != nil {
		t.Fatal(err)
	}
	if _, err := h.run("rollback"); err != nil {
		t.Fatal(err)
	}
	if h.table() != "<none>" {
		t.Errorf("table = %q, want removed", h.table())
	}
}

func TestApplyRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		body, secs, failBin, want string
	}{
		"fails nft -c":         {"table inet unheaded { FAIL_CHECK }\n", "120", "", "fails nft -c"},
		"other table":          {"table ip filter { }\n", "120", "", "touches something other"},
		"flush ruleset":        {"flush ruleset\n", "120", "", "touches something other"},
		"timeout too short":    {"table inet unheaded { }\n", "5", "", "30-3600"},
		"cannot arm the timer": {"table inet unheaded { }\n", "120", "systemd-run", "could not arm"},
		"apply fails":          {"table inet unheaded { FAIL_APPLY }\n", "120", "", "applying"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			if tc.failBin != "" {
				if err := os.WriteFile(filepath.Join(h.dir, "fail-"+tc.failBin), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			f := h.ruleset("x.nft", tc.body)
			out, err := h.run("apply", f, tc.secs)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Fatalf("err %v out %s, want %q", err, out, tc.want)
			}
			if exists(filepath.Join(h.state, "pending.nft")) {
				t.Error("a refused apply left a pending ruleset")
			}
			if h.table() != "<none>" && !strings.Contains(tc.body, "FAIL_APPLY") {
				t.Errorf("table changed on refusal: %q", h.table())
			}
		})
	}
}

// The shipped rulesets only ever define the table this script owns.
func TestShippedRulesetsTouchOnlyTheirTable(t *testing.T) {
	for _, f := range []string{"west.nft", "east.nft"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			l := strings.TrimSpace(line)
			if (strings.HasPrefix(l, "table ") || strings.HasPrefix(l, "delete table") || strings.HasPrefix(l, "flush")) &&
				!strings.Contains(l, "inet unheaded") {
				t.Errorf("%s: %q", f, l)
			}
		}
		if !strings.Contains(string(b), "policy drop") || !strings.Contains(string(b), "ct state established,related accept") {
			t.Errorf("%s: not a default-deny input chain with established traffic allowed", f)
		}
	}
}
