// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package iac

import (
	"bytes"
	"fmt"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/compliance/remediation"
)

// RenderRemediationAnsible renders the ADR-099 fix model to Ansible: one
// role per fix, one playbook per profile, and an inventory grouping hosts
// by their bound profile. The native executor (unheaded-remediate) is the
// reference implementation; these roles follow it: whole-file copies with
// backups, attribute changes only on paths that exist, allowlisted
// commands without a shell, and a block/rescue that restores the files a
// role wrote when a check fails. Unlike the executor, the rescue cannot put
// back a changed mode or owner (Ansible keeps no record of the old ones);
// that difference is why the executor stays the reference.
//
// Output is deterministic, so the rendered tree in the repository is
// checked against a fresh render (the drift test), as baseline.yaml is.
func RenderRemediationAnsible(m *remediation.Model, ps *remediation.ProfileSet, bindings []remediation.Binding) (*RenderOutput, error) {
	if m == nil || ps == nil {
		return nil, fmt.Errorf("remediation ansible: model and profiles are required")
	}
	out := &RenderOutput{Backend: BackendAnsible, Files: map[string]string{}}
	for _, f := range m.Fixes {
		tasks, err := fixTasks(f)
		if err != nil {
			return nil, fmt.Errorf("fix %s: %w", f.ID, err)
		}
		body, err := yamlDoc(header(f.ID+": "+f.Title), tasks)
		if err != nil {
			return nil, err
		}
		out.Files["roles/"+f.ID+"/tasks/main.yml"] = body
	}
	for _, p := range ps.Profiles {
		fixes := ps.Resolve(p.Name)
		play := map[string]any{
			"name":         "Profile " + p.Name + " (ADR-099)",
			"hosts":        "profile_" + strings.ReplaceAll(p.Name, "-", "_"),
			"become":       true,
			"gather_facts": false,
			"roles":        fixes,
		}
		if len(fixes) == 0 {
			play["roles"] = []string{}
		}
		body, err := yamlDoc(header("profile "+p.Name+": "+p.Description), []any{play})
		if err != nil {
			return nil, err
		}
		out.Files["profile-"+p.Name+".yml"] = body
	}
	hostsOf := map[string]map[string]any{}
	for _, b := range bindings {
		g := "profile_" + strings.ReplaceAll(b.Profile, "-", "_")
		if hostsOf[g] == nil {
			hostsOf[g] = map[string]any{}
		}
		hostsOf[g][b.Host] = map[string]any{}
	}
	groups := map[string]any{}
	for g, hosts := range hostsOf {
		groups[g] = map[string]any{"hosts": hosts}
	}
	inv, err := yamlDoc(header("inventory: hosts grouped by their bound profile (The Well's host bindings)"),
		map[string]any{"all": map[string]any{"children": groups}})
	if err != nil {
		return nil, err
	}
	out.Files["inventory.yml"] = inv
	return out, nil
}

func header(what string) string {
	return "# Generated from compliance/remediation/fixes.yaml (ADR-099) by\n" +
		"# `go run ./cmd/compliance-evidence ansible`. Do not edit; edit the model.\n" +
		"# " + strings.ReplaceAll(strings.TrimSpace(what), "\n", " ") + "\n"
}

func yamlDoc(head string, v any) (string, error) {
	var b bytes.Buffer
	b.WriteString(head)
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// task is one Ansible task; yaml.v3 sorts map keys, so output is stable.
type task = map[string]any

var argvFor = map[string][]string{
	"sysctl --system":              {"sysctl", "--system"},
	"systemctl reload ssh.service": {"systemctl", "reload", "ssh.service"},
	"sshd -t":                      {"sshd", "-t"},
	"visudo -c":                    {"visudo", "-c"},
	"augenrules --load":            {"augenrules", "--load"},
	"systemctl restart systemd-journald.service": {"systemctl", "restart", "systemd-journald.service"},
	"systemctl restart chrony.service":           {"systemctl", "restart", "chrony.service"},
}

const auditdHalt = `(?im)^[ \t]*(disk_full_action|disk_error_action|admin_space_left_action)[ \t]*=[ \t]*halt`

// fixTasks renders one fix: facts it needs, then its actions in a block
// whose rescue restores the files it changed.
func fixTasks(f remediation.Fix) ([]any, error) {
	var pre, body, rescue []any
	needSudoFact, needGroup := false, map[string]bool{}
	for i, a := range f.Actions {
		reg := fmt.Sprintf("uh_%s_%d", strings.ReplaceAll(f.ID, "-", "_"), i+1)
		switch {
		case a.File != nil:
			ts, regs, needsSudo, group, err := fileTasks(a.File, reg)
			if err != nil {
				return nil, err
			}
			// copy does not create a missing parent. Created 0755 only when
			// absent, so an existing directory (sudoers.d is 0750) keeps its
			// mode; removed on rescue only if this role created it.
			dir := path.Dir(a.File.Path)
			body = append(body,
				task{"name": "Is " + dir + " there", "ansible.builtin.stat": task{"path": dir}, "register": reg + "_dir"},
				task{"name": "Create " + dir, "ansible.builtin.file": task{"path": dir, "state": "directory", "mode": "0755", "owner": "root", "group": "root"},
					"when": "not " + reg + "_dir.stat.exists"})
			body = append(body, ts...)
			needSudoFact = needSudoFact || needsSudo
			if group != "" {
				needGroup[group] = true
			}
			// One register per variant: a skipped task still overwrites its
			// register, so variants must not share one.
			for _, r := range regs {
				rescue = append(rescue,
					task{"name": "Restore " + a.File.Path, "ansible.builtin.copy": task{"src": "{{ " + r + ".backup_file }}", "dest": a.File.Path, "remote_src": true},
						"when": r + ".backup_file is defined"},
					task{"name": "Remove " + a.File.Path + " if this role created it", "ansible.builtin.file": task{"path": a.File.Path, "state": "absent"},
						"when": r + " is changed and " + r + ".backup_file is not defined"})
			}
			// After the file: a directory this role created is empty again.
			rescue = append(rescue, task{"name": "Remove " + dir + " if this role created it", "ansible.builtin.file": task{"path": dir, "state": "absent"},
				"when": "not " + reg + "_dir.stat.exists"})
		case a.Setting != nil:
			body = append(body,
				task{"name": "Is " + a.Setting.Path + " there", "ansible.builtin.stat": task{"path": a.Setting.Path}, "register": reg + "_stat"},
				task{"name": "Set " + a.Setting.Key + "=" + a.Setting.Value + " in " + a.Setting.Path,
					"ansible.builtin.replace": task{"path": a.Setting.Path, "regexp": "^" + regexpQuote(a.Setting.Key) + "=.*$",
						"replace": a.Setting.Key + "=" + a.Setting.Value, "backup": true},
					"when": reg + "_stat.stat.exists", "register": reg})
		case a.Mode != nil:
			if a.Mode.Glob != "" {
				dir := strings.TrimSuffix(a.Mode.Glob, "/*")
				body = append(body,
					task{"name": "Files in " + dir, "ansible.builtin.find": task{"paths": dir, "file_type": "any"}, "register": reg + "_find"},
					task{"name": "Mode " + a.Mode.Mode + " " + a.Mode.Owner + ":" + a.Mode.Group + " on " + a.Mode.Glob,
						"ansible.builtin.file": task{"path": "{{ item.path }}", "mode": a.Mode.Mode, "owner": a.Mode.Owner, "group": a.Mode.Group},
						"loop":                 "{{ " + reg + "_find.files }}", "loop_control": task{"label": "{{ item.path }}"}})
				continue
			}
			for j, p := range a.Mode.Paths {
				r := fmt.Sprintf("%s_%d", reg, j+1)
				body = append(body,
					task{"name": "Is " + p + " there", "ansible.builtin.stat": task{"path": p}, "register": r + "_stat"},
					task{"name": "Mode " + a.Mode.Mode + " " + a.Mode.Owner + ":" + a.Mode.Group + " on " + p,
						"ansible.builtin.file": task{"path": p, "mode": a.Mode.Mode, "owner": a.Mode.Owner, "group": a.Mode.Group},
						"when":                 r + "_stat.stat.exists"})
			}
		case a.Absent != "":
			body = append(body, task{"name": "Remove " + a.Absent, "ansible.builtin.file": task{"path": a.Absent, "state": "absent"}})
		case a.Service != nil:
			svc := task{"name": a.Service.Name}
			if a.Service.Enabled != nil {
				svc["enabled"] = *a.Service.Enabled
			}
			if a.Service.Active != nil && !*a.Service.Active || a.Service.Enabled != nil && !*a.Service.Enabled {
				svc["state"] = "stopped"
			}
			if a.Service.Masked != nil {
				svc["masked"] = *a.Service.Masked
			}
			body = append(body, task{"name": "Service " + a.Service.Name, "ansible.builtin.systemd_service": svc})
		case a.Package != nil:
			body = append(body, task{"name": "Install " + a.Package.Name, "ansible.builtin.apt": task{"name": a.Package.Name, "state": "present"}})
		case a.Run != "":
			argv, ok := argvFor[a.Run]
			if !ok {
				return nil, fmt.Errorf("run %q has no Ansible form", a.Run)
			}
			body = append(body, task{"name": "Run " + a.Run, "ansible.builtin.command": task{"argv": argv}, "changed_when": true})
		case a.Validate == "auditd-never-halts":
			body = append(body,
				task{"name": "Read auditd.conf", "ansible.builtin.slurp": task{"src": "/etc/audit/auditd.conf"}, "register": reg + "_conf", "failed_when": false},
				task{"name": "auditd never halts the host (ADR-098)", "ansible.builtin.assert": task{
					"that":     []string{reg + "_conf.content is not defined or not (" + reg + "_conf.content | b64decode | regex_search('" + auditdHalt + "'))"},
					"fail_msg": "auditd.conf halts the host on a full or failing disk"}})
		case a.Validate != "":
			argv, ok := argvFor[a.Validate]
			if !ok {
				return nil, fmt.Errorf("validate %q has no Ansible form", a.Validate)
			}
			body = append(body, task{"name": "Check " + a.Validate, "ansible.builtin.command": task{"argv": argv}, "changed_when": false})
		}
	}
	if needSudoFact {
		pre = append(pre,
			task{"name": "Which sudo (sudo-rs prints its version on stderr)", "ansible.builtin.command": task{"argv": []string{"sudo", "--version"}},
				"register": "uh_sudo_version", "changed_when": false, "failed_when": false},
			task{"name": "Note sudo-rs", "ansible.builtin.set_fact": task{"uh_sudo_rs": "{{ (uh_sudo_version.stdout + uh_sudo_version.stderr) is search('(?m)^sudo-rs') }}"}})
	}
	groups := make([]string, 0, len(needGroup))
	for g := range needGroup {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	for _, g := range groups {
		pre = append(pre, task{"name": "Members of group " + g, "ansible.builtin.getent": task{"database": "group", "key": g}})
	}
	rescue = append(rescue, task{"name": "Fail after restoring", "ansible.builtin.fail": task{"msg": f.ID + " failed a check; every file it changed was restored"}})
	return append(pre, task{"name": f.ID, "block": body, "rescue": rescue}), nil
}

// fileTasks renders a file action: one copy when the content is the same
// everywhere, per sudo flavour when only that matters, else per host (and
// flavour); or a generated cron.allow from the group's members. It returns
// the register of each copy task.
func fileTasks(fa *remediation.FileAction, reg string) ([]any, []string, bool, string, error) {
	owner, group := fa.Owner, fa.Group
	if owner == "" {
		owner = "root"
	}
	if group == "" {
		group = "root"
	}
	var regs []string
	copyTask := func(name, content, when string) task {
		r := fmt.Sprintf("%s_v%d", reg, len(regs)+1)
		regs = append(regs, r)
		cp := task{"dest": fa.Path, "content": content, "mode": fa.Mode, "owner": owner, "group": group, "backup": true}
		if fa.Precheck == "visudo -cf" {
			cp["validate"] = "visudo -cf %s"
		}
		t := task{"name": name, "ansible.builtin.copy": cp, "register": r}
		if when != "" {
			t["when"] = when
		}
		return t
	}
	if fa.Generator == remediation.GenRootAndGroupMembers {
		content := "{{ (['root'] + (ansible_facts.getent_group['" + fa.GeneratorGroup + "'][2].split(',') | select | list)) | unique | join('\\n') }}\n"
		return []any{copyTask("Write "+fa.Path+" (root and group "+fa.GeneratorGroup+")", content, "")}, regs, false, fa.GeneratorGroup, nil
	}
	usesFact := false
	for _, p := range fa.Parts {
		usesFact = usesFact || p.UnlessFact != ""
	}
	flavours := []remediation.Facts{{}}
	if usesFact {
		flavours = []remediation.Facts{{SudoRS: false}, {SudoRS: true}}
	}
	factWhen := func(f remediation.Facts) string {
		if !usesFact {
			return ""
		}
		// | bool: depending on the Ansible version set_fact may store the
		// string "False", which a bare when: would read as true.
		if f.SudoRS {
			return "uh_sudo_rs | bool"
		}
		return "not (uh_sudo_rs | bool)"
	}
	content := map[string]map[bool]string{}
	for _, h := range remediation.KnownHosts {
		content[h] = map[bool]string{}
		for _, f := range flavours {
			c, err := fa.Render(h, f, nil)
			if err != nil {
				return nil, nil, false, "", err
			}
			content[h][f.SudoRS] = c
		}
	}
	hostsAgree := true
	for _, h := range remediation.KnownHosts {
		for _, f := range flavours {
			hostsAgree = hostsAgree && content[h][f.SudoRS] == content[remediation.KnownHosts[0]][f.SudoRS]
		}
	}
	var out []any
	for _, h := range remediation.KnownHosts {
		for _, f := range flavours {
			when := factWhen(f)
			if !hostsAgree {
				hw := "inventory_hostname == '" + h + "'"
				if when != "" {
					hw += " and " + when
				}
				when = hw
			}
			name := "Write " + fa.Path
			if when != "" {
				name += " (" + when + ")"
			}
			out = append(out, copyTask(name, content[h][f.SudoRS], when))
		}
		if hostsAgree {
			break
		}
	}
	return out, regs, usesFact, "", nil
}

func regexpQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `.`, `\.`, `*`, `\*`, `+`, `\+`, `?`, `\?`, `(`, `\(`, `)`, `\)`, `[`, `\[`, `]`, `\]`, `{`, `\{`, `}`, `\}`, `^`, `\^`, `$`, `\$`, `|`, `\|`)
	return r.Replace(s)
}
