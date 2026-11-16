// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The hold needs a Wotan client, which zhen-agentd builds only with
// -audit-wotan: the shipped overnight unit must pass it, or every night
// runs unheld (review 2026-11-16).
func TestOvernightUnitEnablesTheHold(t *testing.T) {
	b, err := os.ReadFile("../../deploy/systemd/user/zhen-agentd-overnight.service")
	if err != nil {
		t.Fatal(err)
	}
	exec := regexp.MustCompile(`(?m)^ExecStart=(?:.*\\\n)*.*$`).FindString(string(b))
	if exec == "" {
		t.Fatal("no ExecStart= in the overnight unit")
	}
	for _, f := range []string{"-overnight", "-audit-wotan"} {
		if !strings.Contains(exec, f) {
			t.Errorf("overnight ExecStart lacks %s:\n%s", f, exec)
		}
	}
}

// A night held by a critical alert is reported as such, not as GPU lease
// deferrals (review 2026-11-16).
func TestNight_CriticalAlertDefersCountedApart(t *testing.T) {
	rig := newNightRig(t, 1)
	calls := 0
	rig.w.cfg.Hold = func() (bool, string) {
		calls++
		return calls <= 2, "critical alert firing: WellDown"
	}
	rep := runNight(context.Background(), rig.w, rig.oc)
	if rep.Deferrals[deferCritAlert] != 2 || rep.Deferrals[deferGPU] != 0 || rep.Succeeded != 1 {
		t.Errorf("report = %+v", rep)
	}
}
