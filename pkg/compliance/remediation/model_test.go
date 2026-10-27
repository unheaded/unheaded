// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package remediation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/compliance/crosswalk"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..")
}

func repoCatalog(t *testing.T) *crosswalk.Catalog {
	t.Helper()
	cat, err := crosswalk.LoadFS(os.DirFS(repoRoot(t)), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func repoModel(t *testing.T) *Model {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "compliance/remediation/fixes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRepoModelValidAgainstCatalog(t *testing.T) {
	m := repoModel(t)
	if err := m.CheckCatalog(repoCatalog(t)); err != nil {
		t.Fatal(err)
	}
	if len(m.Fixes) != 13 {
		t.Errorf("%d fixes, want the 13 remediate.sh carries", len(m.Fixes))
	}
}

func TestRepoProfilesAndHosts(t *testing.T) {
	m := repoModel(t)
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "compliance/profiles/profiles.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	ps, err := ParseProfiles(raw, m)
	if err != nil {
		t.Fatal(err)
	}
	if got := ps.Resolve("all"); len(got) != len(m.Fixes) {
		t.Errorf("all = %d fixes, want %d", len(got), len(m.Fixes))
	}
	if got := ps.Resolve("none"); len(got) != 0 {
		t.Errorf("none = %v", got)
	}
	for _, h := range KnownHosts {
		raw, err := os.ReadFile(filepath.Join(repoRoot(t), "compliance/hosts", h+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		b, err := ParseBinding(raw, ps)
		if err != nil {
			t.Fatalf("%s: %v", h, err)
		}
		if b.Host != h || b.Mode != "audit" {
			t.Errorf("%s: host %q mode %q", h, b.Host, b.Mode)
		}
		// Everything recorded as applied is in the bound profile: the
		// binding must describe the host as it is.
		on := map[string]bool{}
		for _, id := range ps.Resolve(b.Profile) {
			on[id] = true
		}
		for _, a := range b.Applied {
			if !on[a.Fix] {
				t.Errorf("%s: %s applied but not in profile %s", h, a.Fix, b.Profile)
			}
		}
	}
}

const minimal = `fixes:
  - id: f
    title: t
    controls: [UH-HOST-01]
    closes: []
    risk: none
    class: sysctl
    actions:
`

func TestParseRejects(t *testing.T) {
	cases := []struct{ name, actions string }{
		{"no actions", ""},
		{"two kinds in one action", "      - {run: \"sysctl --system\", validate: \"sshd -t\"}\n"},
		{"empty action", "      - {}\n"},
		{"run not allowlisted", "      - run: \"rm -rf /\"\n"},
		{"validate not allowlisted", "      - validate: \"true\"\n"},
		{"relative path", "      - file: {path: etc/x, mode: \"0644\", parts: [{text: a}]}\n"},
		{"path outside /etc", "      - file: {path: /usr/bin/x, mode: \"0644\", parts: [{text: a}]}\n"},
		{"dotdot", "      - file: {path: /etc/../root/x, mode: \"0644\", parts: [{text: a}]}\n"},
		{"world-writable", "      - file: {path: /etc/x, mode: \"0666\", parts: [{text: a}]}\n"},
		{"setuid", "      - file: {path: /etc/x, mode: \"4644\", parts: [{text: a}]}\n"},
		{"mode not octal", "      - file: {path: /etc/x, mode: \"rw\", parts: [{text: a}]}\n"},
		{"no content", "      - file: {path: /etc/x, mode: \"0644\"}\n"},
		{"parts and generator", "      - file: {path: /etc/x, mode: \"0644\", parts: [{text: a}], generator: root-and-group-members, generator_group: sudo}\n"},
		{"unknown generator", "      - file: {path: /etc/x, mode: \"0644\", generator: shell, generator_group: sudo}\n"},
		{"unknown host", "      - file: {path: /etc/x, mode: \"0644\", parts: [{text: a, hosts: [north]}]}\n"},
		{"unknown fact", "      - file: {path: /etc/x, mode: \"0644\", parts: [{text: a, unless_fact: moon}]}\n"},
		{"precheck not allowlisted", "      - file: {path: /etc/x, mode: \"0644\", precheck: sh, parts: [{text: a}]}\n"},
		{"bad unit", "      - service: {name: \"x; reboot\", enabled: false}\n"},
		{"bad package", "      - package: {name: \"-oDpkg::x\"}\n"},
		{"setting newline", "      - setting: {path: /etc/default/x, key: enabled, value: \"0\\nevil=1\"}\n"},
		{"absent outside /etc", "      - absent: /var/lib/x\n"},
		{"unknown field", "      - file: {path: /etc/x, mode: \"0644\", parts: [{text: a}], shell: yes}\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Parse([]byte(minimal + c.actions)); !errors.Is(err, ErrInvalidModel) {
				t.Fatalf("got %v, want ErrInvalidModel", err)
			}
		})
	}
}

func TestParseRejectsFixLevel(t *testing.T) {
	ok := "      - run: \"sysctl --system\"\n"
	cases := map[string]string{
		"duplicate id": minimal + ok + strings.TrimPrefix(minimal, "fixes:\n") + ok,
		"bad id":       strings.Replace(minimal, "id: f", "id: F!", 1) + ok,
		"bad risk":     strings.Replace(minimal, "risk: none", "risk: spicy", 1) + ok,
		"bad class":    strings.Replace(minimal, "class: sysctl", "class: magic", 1) + ok,
		"too large":    minimal + ok + "# " + strings.Repeat("x", maxModelBytes),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(doc)); !errors.Is(err, ErrInvalidModel) {
				t.Fatalf("got %v, want ErrInvalidModel", err)
			}
		})
	}
}

func TestCheckCatalogRejectsUnknownControlAndSource(t *testing.T) {
	cat := repoCatalog(t)
	for name, doc := range map[string]string{
		"control": strings.Replace(minimal, "UH-HOST-01", "UH-NOPE-99", 1) + "      - run: \"sysctl --system\"\n",
		"source":  strings.Replace(minimal, "closes: []", `closes: ["host-probe:*:no-such-probe"]`, 1) + "      - run: \"sysctl --system\"\n",
		"host":    strings.Replace(minimal, "closes: []", `closes: ["host-probe:north:shell-tmout"]`, 1) + "      - run: \"sysctl --system\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			m, err := Parse([]byte(doc))
			if err != nil {
				t.Fatal(err)
			}
			if err := m.CheckCatalog(cat); !errors.Is(err, ErrInvalidModel) {
				t.Fatalf("got %v, want ErrInvalidModel", err)
			}
		})
	}
}

func TestRenderPartsByHostAndFact(t *testing.T) {
	m, err := Parse([]byte(minimal + `      - file:
          path: /etc/x
          mode: "0644"
          parts:
            - text: "head\n"
            - {hosts: [east], text: "east only\n"}
            - {unless_fact: sudo-rs, text: "not under sudo-rs\n"}
            - text: "tail"
`))
	if err != nil {
		t.Fatal(err)
	}
	fa := m.Fixes[0].Actions[0].File
	for _, c := range []struct {
		host  string
		facts Facts
		want  string
	}{
		{"west", Facts{}, "head\nnot under sudo-rs\ntail\n"},
		{"east", Facts{}, "head\neast only\nnot under sudo-rs\ntail\n"},
		{"east", Facts{SudoRS: true}, "head\neast only\ntail\n"},
	} {
		got, err := fa.Render(c.host, c.facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%s %+v: got %q, want %q", c.host, c.facts, got, c.want)
		}
	}
	if _, err := fa.Render("north", Facts{}, nil); err == nil {
		t.Error("unknown host rendered")
	}
}

func TestRenderGeneratorRootAndGroupMembers(t *testing.T) {
	fa := &FileAction{Path: "/etc/cron.allow", Mode: "0640", Generator: GenRootAndGroupMembers, GeneratorGroup: "sudo"}
	group := "root:x:0:\nsudo:x:27:govan,ops,root,govan\ncrontab:x:107:\n"
	got, err := fa.Render("west", Facts{}, strings.NewReader(group))
	if err != nil {
		t.Fatal(err)
	}
	if want := "root\ngovan\nops\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// A missing group still yields root alone, never an empty allow list
	// (an empty cron.allow would take crontab from everyone).
	got, err = fa.Render("west", Facts{}, strings.NewReader("root:x:0:\n"))
	if err != nil || got != "root\n" {
		t.Errorf("no sudo group: %q %v", got, err)
	}
	if _, err := fa.Render("west", Facts{}, nil); err == nil {
		t.Error("generator rendered without a group file")
	}
}

func TestModeBits(t *testing.T) {
	fa := &FileAction{Mode: "0440"}
	if got, err := fa.Perm(); err != nil || got != 0o440 {
		t.Errorf("Perm = %o %v", got, err)
	}
}

// FND-011 stopgap (CVE-2026-53266): kmod-cis keeps ebtables and the two
// modules the exploit chain loads from being autoloaded, on both hosts,
// until a fixed kernel ships. Nothing on west or east uses ebtables (LXD
// filters with nftables; lsmod shows no ebt* module, 2026-10-27).
// scripts/host/executor_diff_test.go keeps remediate.sh's copy identical.
func TestKmodCISCarriesFND011Stopgap(t *testing.T) {
	m := repoModel(t)
	var fa *FileAction
	for i := range m.Fixes {
		if m.Fixes[i].ID != "kmod-cis" {
			continue
		}
		for _, a := range m.Fixes[i].Actions {
			if a.File != nil && a.File.Path == "/etc/modprobe.d/unheaded-cis.conf" {
				fa = a.File
			}
		}
	}
	if fa == nil {
		t.Fatal("kmod-cis has no /etc/modprobe.d/unheaded-cis.conf action")
	}
	for _, host := range KnownHosts {
		got, err := fa.Render(host, Facts{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, mod := range []string{"ebtables", "ebtable_nat", "ebt_snat"} {
			line := "install " + mod + " /bin/false\n"
			if !strings.Contains(got, line) {
				t.Errorf("%s: kmod-cis lacks %q", host, strings.TrimSpace(line))
			}
		}
		if !strings.Contains(got, "FND-011") {
			t.Errorf("%s: the stopgap lines carry no FND-011 comment", host)
		}
	}
}
