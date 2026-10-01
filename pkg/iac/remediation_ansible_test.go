// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package iac

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/compliance/remediation"
)

// Modules a rendered task may use: the closed action set's Ansible forms.
// No shell, no raw, no script.
var allowedModules = map[string]bool{
	"ansible.builtin.copy": true, "ansible.builtin.file": true, "ansible.builtin.stat": true, "ansible.builtin.find": true,
	"ansible.builtin.replace": true, "ansible.builtin.systemd_service": true, "ansible.builtin.apt": true,
	"ansible.builtin.command": true, "ansible.builtin.slurp": true, "ansible.builtin.assert": true,
	"ansible.builtin.set_fact": true, "ansible.builtin.getent": true, "ansible.builtin.fail": true,
}

var taskKeywords = map[string]bool{"name": true, "register": true, "when": true, "changed_when": true,
	"failed_when": true, "loop": true, "loop_control": true, "block": true, "rescue": true}

func checkTasks(t *testing.T, file string, tasks []map[string]any) {
	t.Helper()
	for i, tk := range tasks {
		if _, ok := tk["name"].(string); !ok {
			t.Errorf("%s task %d has no name", file, i)
		}
		modules := 0
		for k := range tk {
			switch {
			case allowedModules[k]:
				modules++
			case taskKeywords[k]:
			default:
				t.Errorf("%s task %q: %q is not an allowed module or keyword", file, tk["name"], k)
			}
		}
		if b, ok := tk["block"]; ok {
			if modules != 0 {
				t.Errorf("%s: block with a module", file)
			}
			for _, part := range []string{"block", "rescue"} {
				var sub []map[string]any
				raw, _ := yaml.Marshal(tk[part])
				if err := yaml.Unmarshal(raw, &sub); err != nil {
					t.Fatal(err)
				}
				checkTasks(t, file+"/"+part, sub)
			}
			_ = b
			continue
		}
		if modules != 1 {
			t.Errorf("%s task %q uses %d modules, want 1", file, tk["name"], modules)
		}
		if cmd, ok := tk["ansible.builtin.command"].(map[string]any); ok {
			if _, argv := cmd["argv"]; !argv {
				t.Errorf("%s task %q: command without argv (would be split by a shell-like parser)", file, tk["name"])
			}
		}
	}
}

func TestRemediationAnsibleStructure(t *testing.T) {
	repo := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(repo, "compliance/remediation/fixes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := remediation.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	praw, err := os.ReadFile(filepath.Join(repo, "compliance/profiles/profiles.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	ps, err := remediation.ParseProfiles(praw, m)
	if err != nil {
		t.Fatal(err)
	}
	out, err := RenderRemediationAnsible(m, ps, []remediation.Binding{{Host: "west", Profile: "lab", Mode: "audit"}, {Host: "east", Profile: "none", Mode: "audit"}})
	if err != nil {
		t.Fatal(err)
	}
	roles := 0
	for name, body := range out.Files {
		if !strings.HasPrefix(body, "# Generated from compliance/remediation/fixes.yaml") {
			t.Errorf("%s: no generated header", name)
		}
		if strings.Contains(body, "ansible.builtin.shell") || strings.Contains(body, "ansible.builtin.raw") {
			t.Errorf("%s uses a shell module", name)
		}
		if strings.HasPrefix(name, "roles/") {
			roles++
			var tasks []map[string]any
			if err := yaml.Unmarshal([]byte(body), &tasks); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			checkTasks(t, name, tasks)
		} else {
			var v any
			if err := yaml.Unmarshal([]byte(body), &v); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	if roles != len(m.Fixes) {
		t.Errorf("%d roles for %d fixes", roles, len(m.Fixes))
	}
	if !strings.Contains(out.Files["inventory.yml"], "profile_none") || !strings.Contains(out.Files["profile-lab.yml"], "- kmod-cis") {
		t.Errorf("inventory or lab playbook wrong:\n%s\n%s", out.Files["inventory.yml"], out.Files["profile-lab.yml"])
	}
	// Host-specific content is conditioned on the host; sudo-rs content on
	// the fact, as a bool.
	if s := out.Files["roles/sysctl-cis/tasks/main.yml"]; !strings.Contains(s, "inventory_hostname == 'east'") {
		t.Error("sysctl-cis east-only lines not conditioned on the host")
	}
	if s := out.Files["roles/sudo-cis/tasks/main.yml"]; !strings.Contains(s, "uh_sudo_rs | bool") || strings.Contains(s, "inventory_hostname") {
		t.Error("sudo-cis should vary by sudo flavour only")
	}
	// Two variants must never share a register: a skipped task still
	// overwrites it.
	for name, body := range out.Files {
		seen := map[string]int{}
		for _, l := range strings.Split(body, "\n") {
			if r, ok := strings.CutPrefix(strings.TrimSpace(l), "register: "); ok {
				seen[r]++
			}
		}
		for r, n := range seen {
			if n > 1 {
				t.Errorf("%s: register %s used %d times", name, r, n)
			}
		}
	}
}

func TestRemediationAnsibleNeedsModel(t *testing.T) {
	if _, err := RenderRemediationAnsible(nil, nil, nil); err == nil {
		t.Error("rendered without a model")
	}
}
