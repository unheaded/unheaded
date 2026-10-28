// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The `runbook` judge (runbook-draft family, S-CHAMPION-HARDENING Phase 7):
// the answer's runbook YAML is written to <work>/rb-*/<name>.yaml and must
// pass three mechanical checks, all of them, each reported:
//
//	dry_run       scripts/run-runbook.py --dry-run exits 0
//	check         scripts/check-runbooks.py --one exits 0 (strict draft rules,
//	              status: draft, no baseline)
//	requirements  every `requires` regex of the task's requirements.json
//	              matches the YAML text and no `forbids` regex does
//
// Nothing in the draft is executed: the dry run prints commands, the
// checker runs `bash -n` (parse only). So no sandbox is needed, only a
// timeout. python3 is ZHEN_EVAL_PYTHON or the one on PATH (PyYAML).

// systemRunbook is the system prompt for runbook-draft tasks: the same for
// both brains, the runbook/v1 contract the judge enforces.
const systemRunbook = "You write operational runbooks for the Unheaded repository in its runbook/v1 YAML format.\n" +
	"Reply with the complete runbook in one fenced code block whose opening line is ```file:<name>.yaml (the task names the runbook).\n" +
	"Format: apiVersion: runbook/v1 and kind: Runbook; metadata with name, description, owner, estimated_duration, " +
	"risk (low, medium, high or critical), requires_sudo, status: draft, authored_by (zhen-local, claude or human), " +
	"reviewed_by: \"\" and approved_by: \"\"; steps: a list of {name, action: shell or manual, command, and optionally " +
	"verify, description, host: west or east}. Optional sections: parameters (name, description, default), " +
	"preconditions (description, check), and verification (name, description, command), which runs after the steps.\n" +
	"Rules: step names are unique; every shell command, verify, precondition check and verification command passes `bash -n`; " +
	"a multi-line shell command or verify starts with `set -euo pipefail`; a line `! cmd` may only be the last line of a command; " +
	"anything irreversible (git push, DROP, pg_restore, rm -rf, shred, prune) is an action: manual step; " +
	"no credentials in the file (read them from the environment as ${VAR:?message})."

// maxRunbookBytes caps the YAML the judge writes and parses.
const maxRunbookBytes = 256 << 10

var runbookNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// rbRequirements is a runbook-draft task's requirements.json.
type rbRequirements struct {
	Name     string   `json:"name"`
	Requires []string `json:"requires"`
	Forbids  []string `json:"forbids"`

	req, forb []*regexp.Regexp
}

// runbookTask is one runbook-draft task: the prompt (TASK.md without its
// SPDX comment) and its requirements. reference.yaml, a draft that passes,
// sits beside them for the tests and is never shown to a model.
type runbookTask struct {
	prompt string
	req    *rbRequirements
}

func loadRunbookTask(dir string) (*runbookTask, error) {
	task, err := os.ReadFile(filepath.Join(dir, "TASK.md")) // #nosec G304 -- the repo's own fixture
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "requirements.json")) // #nosec G304 -- the repo's own fixture
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var r rbRequirements
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("%s/requirements.json: %w", dir, err)
	}
	if !runbookNameRE.MatchString(r.Name) {
		return nil, fmt.Errorf("%s/requirements.json: name %q is not a runbook name", dir, r.Name)
	}
	if len(r.Requires) == 0 {
		return nil, fmt.Errorf("%s/requirements.json: no requires", dir)
	}
	for _, p := range r.Requires {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("%s/requirements.json: %w", dir, err)
		}
		r.req = append(r.req, re)
	}
	for _, p := range r.Forbids {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("%s/requirements.json: %w", dir, err)
		}
		r.forb = append(r.forb, re)
	}
	return &runbookTask{prompt: strings.TrimSpace(stripSPDXComment(string(task))), req: &r}, nil
}

// RunbookTools runs the two repository scripts on a draft file and reports
// the exit code and (truncated) output. err is for a script that could not
// run or timed out, not for a non-zero exit.
type RunbookTools interface {
	DryRun(ctx context.Context, path string) (exit int, output string, err error)
	Check(ctx context.Context, path string) (exit int, output string, err error)
}

// scriptTools runs scripts/run-runbook.py and scripts/check-runbooks.py
// from root with python.
type scriptTools struct {
	python, root string
	timeout      time.Duration
}

func (s scriptTools) DryRun(ctx context.Context, path string) (int, string, error) {
	return s.run(ctx, "run-runbook.py", "--dry-run", path)
}

func (s scriptTools) Check(ctx context.Context, path string) (int, string, error) {
	return s.run(ctx, "check-runbooks.py", "--one", "--root", s.root, path)
}

func (s scriptTools) run(ctx context.Context, script string, args ...string) (int, string, error) {
	cctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	argv := append([]string{"-B", filepath.Join(s.root, "scripts", script)}, args...)
	cmd := exec.CommandContext(cctx, s.python, argv...) // #nosec G204 -- python is the operator's ZHEN_EVAL_PYTHON; the script is the repo's own; path is a file the judge wrote
	cmd.Dir = s.root
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := truncate(out.String(), 8192)
	if cctx.Err() != nil {
		return -1, text, fmt.Errorf("%s: %w", script, cctx.Err())
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), text, nil
	}
	if err != nil {
		return -1, text, fmt.Errorf("%s: %w", script, err)
	}
	return 0, text, nil
}

// RunbookResult is one runbook judgement: Judges holds dry_run, check and
// requirements, each "pass" or why not.
type RunbookResult struct {
	Pass   bool
	Detail string
	Judges map[string]string
}

var yamlFenceRE = regexp.MustCompile("(?s)```ya?ml[^\\n]*\\n(.*?)```")

// extractRunbookYAML: the first ```file:<x>.yaml block, else the first
// ```yaml fence.
func extractRunbookYAML(answer string) (string, bool) {
	if blocks, err := parseFileBlocks(answer); err == nil {
		for _, b := range blocks {
			if ext := strings.ToLower(filepath.Ext(b.Path)); ext == ".yaml" || ext == ".yml" {
				return b.Content, true
			}
		}
	}
	if m := yamlFenceRE.FindStringSubmatch(answer); m != nil {
		return m[1], true
	}
	return "", false
}

func scriptVerdict(exit int, err error) string {
	switch {
	case err != nil:
		return "error: " + truncate(err.Error(), 200)
	case exit != 0:
		return fmt.Sprintf("fail exit=%d", exit)
	}
	return "pass"
}

// judgeRunbookDraft judges one answer to a runbook-draft task.
func judgeRunbookDraft(ctx context.Context, task *runbookTask, work, answer string, tools RunbookTools) RunbookResult {
	text, ok := extractRunbookYAML(answer)
	if !ok {
		return RunbookResult{Detail: "parse_fail"}
	}
	if len(text) > maxRunbookBytes {
		return RunbookResult{Detail: "too_large"}
	}
	dir, err := os.MkdirTemp(work, "rb-")
	if err != nil {
		return RunbookResult{Detail: "check_error: " + err.Error()}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	// The name comes from the eval's task file: one path element, or nothing is written.
	if n := task.req.Name; n == "" || n != filepath.Base(n) || n == "." || n == ".." {
		return RunbookResult{Detail: "check_error: task name " + strconv.Quote(task.req.Name) + " is not a single file name"}
	}
	path := filepath.Join(dir, task.req.Name+".yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil { // #nosec G703 -- a fresh MkdirTemp dir and a name checked to be one path element above
		return RunbookResult{Detail: "check_error: " + err.Error()}
	}

	j := map[string]string{}
	exit, _, err := tools.DryRun(ctx, path)
	j["dry_run"] = scriptVerdict(exit, err)
	exit, _, err = tools.Check(ctx, path)
	j["check"] = scriptVerdict(exit, err)

	var missing, forbidden []string
	for i, re := range task.req.req {
		if !re.MatchString(text) {
			missing = append(missing, fmt.Sprintf("requires[%d]", i))
		}
	}
	for i, re := range task.req.forb {
		if re.MatchString(text) {
			forbidden = append(forbidden, fmt.Sprintf("forbids[%d]", i))
		}
	}
	var why []string
	if len(missing) > 0 {
		why = append(why, "missing "+strings.Join(missing, ","))
	}
	if len(forbidden) > 0 {
		why = append(why, "forbidden "+strings.Join(forbidden, ","))
	}
	j["requirements"] = "pass"
	if len(why) > 0 {
		j["requirements"] = strings.Join(why, "; ")
	}
	pass := j["dry_run"] == "pass" && j["check"] == "pass" && j["requirements"] == "pass"
	return RunbookResult{Pass: pass, Judges: j,
		Detail: fmt.Sprintf("dry_run=%s check=%s requirements=%s", j["dry_run"], j["check"], j["requirements"])}
}
