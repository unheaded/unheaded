// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"unheaded/pkg/logger"
)

// The systemd injector (S-REAL-STEEL D-23), east only. It runs a fixed
// systemctl argv as the unheaded-chaos user; authorization is systemd's own,
// granted by a polkit rule rendered from the same allowlist (render-polkit).
// There is no sudo, no root helper and no wildcard. The injector refuses on
// any host but east and hard-denies *.target, ssh*, systemd-* and dbus* even
// if the allowlist somehow listed them. Revert is `start`, verified with
// `systemctl is-active`; for a kill it leans on the unit's own Restart= first
// and only starts the unit if it is still inactive after a grace window.

// SystemdInjector stops or kills (SIGKILL) one allowlisted unit.
type SystemdInjector struct {
	runner        CommandRunner
	hostname      func() (string, error)
	verb          string // "stop" or "kill"
	inactiveGrace time.Duration
	poll          time.Duration
	log           *logger.Logger
}

func newSystemd(verb string, runner CommandRunner, hostname func() (string, error), log *logger.Logger) *SystemdInjector {
	if runner == nil {
		runner = ExecRunner{}
	}
	if log == nil {
		log = logger.New(io.Discard)
	}
	return &SystemdInjector{runner: runner, hostname: hostname, verb: verb, inactiveGrace: 30 * time.Second, poll: 2 * time.Second, log: log}
}

// NewSystemdStop builds the service_stop injector (clean stop).
func NewSystemdStop(runner CommandRunner, hostname func() (string, error), log *logger.Logger) *SystemdInjector {
	return newSystemd("stop", runner, hostname, log)
}

// NewSystemdKill builds the process_kill injector (SIGKILL).
func NewSystemdKill(runner CommandRunner, hostname func() (string, error), log *logger.Logger) *SystemdInjector {
	return newSystemd("kill", runner, hostname, log)
}

// requireEast returns the unit after checking the host is east and the unit
// is not hard-denied. It never widens the allowlist; the allowlist already
// checked the host, the explicit unit list and the deny patterns before the
// experiment started. This is the injector's own, independent check.
func (n *SystemdInjector) requireEast(unit string) error {
	if n.hostname == nil {
		return errors.New("systemd: no hostname function")
	}
	host, err := n.hostname()
	if err != nil {
		return fmt.Errorf("systemd: cannot read hostname: %w", err)
	}
	if host != "east" {
		return fmt.Errorf("systemd faults run only on east, this is %q", host)
	}
	if !unitNameRe.MatchString(unit) || deniedUnitRe.MatchString(unit) {
		return fmt.Errorf("systemd: %q is never a chaos target", unit)
	}
	return nil
}

// Plan checks the host and unit and records the revert recipe.
func (n *SystemdInjector) Plan(_ context.Context, t ResolvedTarget, _ Params) (Recipe, error) {
	if t.Kind != KindSystemd || t.ID == "" {
		return nil, fmt.Errorf("systemd fault needs a systemd target, got %q/%q", t.Kind, t.ID)
	}
	if err := n.requireEast(t.ID); err != nil {
		return nil, err
	}
	return Recipe{"unit": t.ID, "verb": n.verb}, nil
}

// Inject stops or kills the unit.
func (n *SystemdInjector) Inject(ctx context.Context, _ ResolvedTarget, _ Params, r Recipe) error {
	unit := r["unit"]
	if err := n.requireEast(unit); err != nil {
		return err
	}
	var argv []string
	switch n.verb {
	case "stop":
		argv = []string{"--no-ask-password", "stop", unit}
	case "kill":
		argv = []string{"--no-ask-password", "kill", "--signal=SIGKILL", unit}
	default:
		return fmt.Errorf("systemd: unknown verb %q", n.verb)
	}
	if _, err := n.runner.Run(ctx, "systemctl", argv...); err != nil {
		return fmt.Errorf("systemd %s %s: %w", n.verb, unit, err)
	}
	return nil
}

// Revert brings the unit back to active. For a kill it waits for the unit's
// own Restart= first, and only runs an explicit start if the unit is still
// inactive after the grace window.
func (n *SystemdInjector) Revert(ctx context.Context, r Recipe) error {
	unit := r["unit"]
	if unit == "" {
		return errors.New("systemd: recipe has no unit")
	}
	if n.verb == "kill" {
		if n.waitActive(ctx, unit, n.inactiveGrace) {
			return nil // Restart= revived it; no explicit start needed
		}
	}
	if _, err := n.runner.Run(ctx, "systemctl", "--no-ask-password", "start", unit); err != nil {
		return fmt.Errorf("systemd start %s: %w", unit, err)
	}
	return nil
}

// Reverted is true when the unit is active.
func (n *SystemdInjector) Reverted(ctx context.Context, r Recipe) (bool, error) {
	unit := r["unit"]
	if unit == "" {
		return false, errors.New("systemd: recipe has no unit")
	}
	return n.isActive(ctx, unit), nil
}

// isActive runs `systemctl is-active <unit>` and reports active. is-active
// exits non-zero when inactive but still prints the state, so the stdout is
// authoritative and the error is expected.
func (n *SystemdInjector) isActive(ctx context.Context, unit string) bool {
	out, _ := n.runner.Run(ctx, "systemctl", "is-active", unit)
	return strings.TrimSpace(string(out)) == "active"
}

// waitActive polls is-active until active or the grace window passes.
func (n *SystemdInjector) waitActive(ctx context.Context, unit string, grace time.Duration) bool {
	deadline := time.Now().Add(grace)
	for {
		if n.isActive(ctx, unit) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(n.poll):
		}
	}
}

// --- polkit rule rendering (D-23) ---

// polkitTemplate is the single source for the polkit rule's shape. The repo
// file deploy/polkit/60-unheaded-chaos.rules.tmpl mirrors it (a test fails if
// they drift).
const polkitTemplate = `// SPDX-License-Identifier: GPL-3.0-or-later
// Rendered by ` + "`chaos-controller render-polkit`" + ` from the chaos allowlist
// (S-REAL-STEEL D-23, ADR-107). One source: edit the allowlist, re-render.
// Do not edit by hand.
//
// Lets the unheaded-chaos system user start/stop/kill ONLY the listed units,
// and nothing else. Everything outside this is left to the rest of policy.
polkit.addRule(function(action, subject) {
    if (action.id != "org.freedesktop.systemd1.manage-units") { return polkit.Result.NOT_HANDLED; }
    if (subject.user != "{{.User}}") { return polkit.Result.NOT_HANDLED; }
    var verb = action.lookup("verb");
    if (verb != "start" && verb != "stop" && verb != "kill") { return polkit.Result.NOT_HANDLED; }
    var unit = action.lookup("unit");
    var units = {
{{- range .Units}}
        {{printf "%q" .}}: true,
{{- end}}
    };
    if (units[unit] === true) { return polkit.Result.YES; }
    return polkit.Result.NOT_HANDLED;
});
`

// PolkitUser is the system account the rule authorizes.
const PolkitUser = "unheaded-chaos"

// RenderPolkitRule renders the polkit rule for a's systemd units. It errors
// if the allowlist lists no units (an empty rule would authorize nothing and
// is almost certainly a mistake).
func RenderPolkitRule(a *Allowlist) (string, error) {
	if a == nil {
		return "", errors.New("polkit: no allowlist")
	}
	if len(a.Systemd.Units) == 0 {
		return "", errors.New("polkit: the allowlist lists no systemd units")
	}
	for _, u := range a.Systemd.Units {
		if !unitNameRe.MatchString(u) || deniedUnitRe.MatchString(u) {
			return "", fmt.Errorf("polkit: unit %q cannot be authorized", u)
		}
	}
	tmpl, err := template.New("polkit").Parse(polkitTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct {
		User  string
		Units []string
	}{User: PolkitUser, Units: a.Systemd.Units}); err != nil {
		return "", err
	}
	return buf.String(), nil
}
