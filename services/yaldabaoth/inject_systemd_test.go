// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func east() (string, error) { return "east", nil }
func west() (string, error) { return "west", nil }

func sysdStop(run CommandRunner, host func() (string, error)) *SystemdInjector {
	s := NewSystemdStop(run, host, nil)
	s.inactiveGrace = 10 * time.Millisecond
	s.poll = 2 * time.Millisecond
	return s
}

func sysdKill(run CommandRunner, host func() (string, error)) *SystemdInjector {
	s := NewSystemdKill(run, host, nil)
	s.inactiveGrace = 10 * time.Millisecond
	s.poll = 2 * time.Millisecond
	return s
}

const timeguru = "unheaded-timeguru.service"

func TestSystemd_RefusesOffEast(t *testing.T) {
	inj := sysdStop(&fakeRunner{}, west)
	if _, err := inj.Plan(t.Context(), ResolvedTarget{Kind: KindSystemd, ID: timeguru}, nil); err == nil ||
		!strings.Contains(err.Error(), "east") {
		t.Fatalf("err = %v, want an east-only refusal", err)
	}
}

func TestSystemd_HardDenyEvenIfAsked(t *testing.T) {
	inj := sysdStop(&fakeRunner{}, east)
	for _, unit := range []string{"basic.target", "sshd.service", "systemd-journald.service", "dbus.service"} {
		if _, err := inj.Plan(t.Context(), ResolvedTarget{Kind: KindSystemd, ID: unit}, nil); err == nil {
			t.Errorf("%s was not hard-denied", unit)
		}
	}
}

func TestSystemd_StopArgvExact(t *testing.T) {
	run := &fakeRunner{}
	inj := sysdStop(run, east)
	r, err := inj.Plan(t.Context(), ResolvedTarget{Kind: KindSystemd, ID: timeguru}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := inj.Inject(t.Context(), ResolvedTarget{Kind: KindSystemd, ID: timeguru}, nil, r); err != nil {
		t.Fatal(err)
	}
	want := "systemctl --no-ask-password stop " + timeguru
	if got := run.argv("stop"); got != want {
		t.Errorf("stop argv:\n got %q\nwant %q", got, want)
	}
	if bad := run.nonSystemctl(); bad != nil {
		t.Errorf("ran non-systemctl binaries: %v", bad)
	}
}

func TestSystemd_KillArgvExact(t *testing.T) {
	run := &fakeRunner{}
	inj := sysdKill(run, east)
	r, _ := inj.Plan(t.Context(), ResolvedTarget{Kind: KindSystemd, ID: timeguru}, nil)
	if err := inj.Inject(t.Context(), ResolvedTarget{Kind: KindSystemd, ID: timeguru}, nil, r); err != nil {
		t.Fatal(err)
	}
	want := "systemctl --no-ask-password kill --signal=SIGKILL " + timeguru
	if got := run.argv("kill"); got != want {
		t.Errorf("kill argv:\n got %q\nwant %q", got, want)
	}
}

func TestSystemd_RevertStartsAndVerifies(t *testing.T) {
	run := &fakeRunner{} // starts inactive; the explicit start revives it
	inj := sysdStop(run, east)
	recipe := Recipe{"unit": timeguru, "verb": "stop"}
	if err := inj.Revert(t.Context(), recipe); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	want := "systemctl --no-ask-password start " + timeguru
	if got := run.argv("start"); got != want {
		t.Errorf("start argv:\n got %q\nwant %q", got, want)
	}
	if gone, err := inj.Reverted(t.Context(), recipe); err != nil || !gone {
		t.Errorf("Reverted = %v,%v; want true,nil", gone, err)
	}
}

func TestSystemd_KillRevertRelivesOnRestartFirst(t *testing.T) {
	// systemd's Restart= already brought it back: Revert must not start it.
	run := &fakeRunner{active: true}
	inj := sysdKill(run, east)
	if err := inj.Revert(t.Context(), Recipe{"unit": timeguru, "verb": "kill"}); err != nil {
		t.Fatal(err)
	}
	if run.argv("start") != "" {
		t.Error("kill-revert started the unit though Restart= had already revived it")
	}
}

func TestSystemd_KillRevertStartsIfStillDown(t *testing.T) {
	// Inactive through the grace window; only the explicit start revives it.
	run := &fakeRunner{active: false}
	inj := sysdKill(run, east)
	if err := inj.Revert(t.Context(), Recipe{"unit": timeguru, "verb": "kill"}); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if run.argv("start") == "" {
		t.Error("kill-revert never started the unit though it stayed inactive past the grace")
	}
}

func TestSystemd_RevertedReflectsIsActive(t *testing.T) {
	run := &fakeRunner{active: false}
	inj := sysdStop(run, east)
	if gone, _ := inj.Reverted(t.Context(), Recipe{"unit": timeguru, "verb": "stop"}); gone {
		t.Error("Reverted true while is-active reports inactive")
	}
}

// The polkit rule rendered from the allowlist carries exactly the allowlist's
// units and the fixed subject/verb scope: change the allowlist, change the
// rule (D-23 one source).
func TestPolkitRuleMatchesAllowlist(t *testing.T) {
	a := testAllowlist(t)
	rule, err := RenderPolkitRule(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range a.Systemd.Units {
		if !strings.Contains(rule, `"`+u+`": true`) {
			t.Errorf("rendered rule is missing allowlisted unit %q", u)
		}
	}
	if strings.Contains(rule, `"sshd.service"`) || strings.Contains(rule, `"basic.target"`) {
		t.Error("rendered rule names a unit not in the allowlist")
	}
	for _, must := range []string{
		`action.id != "org.freedesktop.systemd1.manage-units"`,
		`subject.user != "unheaded-chaos"`,
		`verb != "start" && verb != "stop" && verb != "kill"`,
	} {
		if !strings.Contains(rule, must) {
			t.Errorf("rendered rule lacks %q", must)
		}
	}
}

// The repo template file and the embedded template render identically: one
// source of truth for the polkit rule's shape.
func TestPolkitTemplateFileMatchesEmbedded(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "deploy", "polkit", "60-unheaded-chaos.rules.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != strings.TrimSpace(polkitTemplate) {
		t.Error("deploy/polkit/60-unheaded-chaos.rules.tmpl has drifted from the embedded template")
	}
}
