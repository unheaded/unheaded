// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package champion

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/zhenrag"
)

// ------------------------------------------------------------------
// Runbook body checks before execution (RT-08, S-CHAMPION-HARDENING
// Phase 4).
//
// RunbookExecute used to check only that the name stayed under runbooks/.
// A runbook's body could change on disk after review, and the commands in
// it never met Rule 3: a tool call naming a runbook carries no destructive
// verb even when the runbook runs `rm -rf`. Before a live (non-dry-run)
// execution the Champion now
//
//  1. reads the runbook once and checks its sha256 against
//     runbooks/MANIFEST.sha256, the committed set
//     (scripts/runbook-manifest.sh writes it; CI fails when it is stale);
//  2. scans the commands run-runbook.py would execute (steps, rollback and
//     verification entries that are not `action: manual`: their `command`
//     and `verify`) with the same DestructivePattern as Rule 3, and refuses
//     a match unless the runbook is approved (metadata.status: approved
//     with a non-empty approved_by, which only Stevie sets: ADR-024 §6);
//  3. hands run-runbook.py a private copy of the bytes it verified, so a
//     file swapped after the check is not what runs.
//
// A dry run reads the runbook in place and skips all three: it executes
// nothing (run-runbook.py --dry-run), and tests/runbooks drives it.
// ------------------------------------------------------------------

// RunbookManifestFile is the manifest's name inside runbooks/.
const RunbookManifestFile = "MANIFEST.sha256"

// ErrRunbookUnapproved: the runbook's bytes are not the committed ones (no
// manifest, not listed, or a different sha256).
var ErrRunbookUnapproved = errors.New("runbook does not match runbooks/" + RunbookManifestFile)

// ErrRunbookDestructive: the runbook would execute a destructive command
// and is not approved.
var ErrRunbookDestructive = errors.New("runbook executes a destructive command and is not approved")

// parseRunbookManifest reads sha256sum output ("<hex>  <path>", or
// "<hex> *<path>" in binary mode). Paths are relative to runbooks/ and must
// stay inside it; blank lines and # comments are skipped.
func parseRunbookManifest(raw []byte) (map[string]string, error) {
	m := map[string]string{}
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sum, rest, ok := strings.Cut(line, " ")
		if !ok || len(sum) != sha256.Size*2 {
			return nil, fmt.Errorf("manifest line %d: want \"<sha256>  <path>\"", i+1)
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return nil, fmt.Errorf("manifest line %d: sha256 is not hex", i+1)
		}
		name := strings.TrimPrefix(strings.TrimPrefix(rest, " "), "*")
		if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.HasPrefix(name, "../") || name == ".." {
			return nil, fmt.Errorf("manifest line %d: path %q must be clean and relative to runbooks/", i+1, name)
		}
		if _, dup := m[name]; dup {
			return nil, fmt.Errorf("manifest line %d: %s listed twice", i+1, name)
		}
		m[name] = strings.ToLower(sum)
	}
	return m, nil
}

// runbookDoc is the part of runbook/v1 the scan reads.
type runbookDoc struct {
	Metadata struct {
		Status     string `yaml:"status"`
		ApprovedBy string `yaml:"approved_by"`
	} `yaml:"metadata"`
	Steps        []runbookStep `yaml:"steps"`
	Rollback     yaml.Node     `yaml:"rollback"` // a list, or {description, steps: [...]}
	Verification []runbookStep `yaml:"verification"`
}

type runbookStep struct {
	Name    string `yaml:"name"`
	Action  string `yaml:"action"` // "" means shell (run-runbook.py's default)
	Command string `yaml:"command"`
	Verify  string `yaml:"verify"`
}

func (d *runbookDoc) rollbackSteps() ([]runbookStep, error) {
	var steps []runbookStep
	switch d.Rollback.Kind {
	case 0:
		return nil, nil
	case yaml.SequenceNode:
		err := d.Rollback.Decode(&steps)
		return steps, err
	case yaml.MappingNode:
		var nested struct {
			Steps []runbookStep `yaml:"steps"`
		}
		err := d.Rollback.Decode(&nested)
		return nested.Steps, err
	}
	return nil, fmt.Errorf("rollback: want a list or a mapping with steps")
}

// scanRunbook returns "<section>/<step>: <match>" for every destructive
// command run-runbook.py would execute. Manual steps are printed, never
// run, so they are not scanned; neither is prose (notes, descriptions).
func scanRunbook(body []byte) ([]string, error) {
	var d runbookDoc
	if err := yaml.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("parse runbook: %w", err)
	}
	rb, err := d.rollbackSteps()
	if err != nil {
		return nil, err
	}
	var hits []string
	for _, sec := range []struct {
		name  string
		steps []runbookStep
	}{{"steps", d.Steps}, {"rollback", rb}, {"verification", d.Verification}} {
		for _, s := range sec.steps {
			if s.Action == "manual" {
				continue
			}
			for _, text := range []string{s.Command, s.Verify} {
				if m := zhenrag.DestructivePattern.FindString(text); m != "" {
					hits = append(hits, fmt.Sprintf("%s/%s: %q", sec.name, s.Name, m))
				}
			}
		}
	}
	return hits, nil
}

func runbookApproved(body []byte) bool {
	var d runbookDoc
	if yaml.Unmarshal(body, &d) != nil {
		return false
	}
	return d.Metadata.Status == "approved" && strings.TrimSpace(d.Metadata.ApprovedBy) != ""
}

// verifyRunbookForExec applies the three checks above to a live execution
// and returns the path run-runbook.py should read plus a cleanup func. For
// a dry run it returns runbookPath unchanged. relName is the runbook's path
// relative to runbooks/ ("infra/kernel-upgrade.yaml").
func (c *Champion) verifyRunbookForExec(runbookPath, relName string, dryRun bool) (string, func(), error) {
	noop := func() {}
	if dryRun {
		return runbookPath, noop, nil
	}
	runbooksDir := filepath.Join(c.config.ProjectRoot, "runbooks")
	raw, err := os.ReadFile(filepath.Join(runbooksDir, RunbookManifestFile)) // #nosec G304 -- fixed name under the configured project root
	if err != nil {
		return "", noop, fmt.Errorf("%w: %v", ErrRunbookUnapproved, err)
	}
	manifest, err := parseRunbookManifest(raw)
	if err != nil {
		return "", noop, fmt.Errorf("%w: %v", ErrRunbookUnapproved, err)
	}
	body, err := os.ReadFile(runbookPath) // #nosec G304 -- validated under runbooks/ by RunbookExecute
	if err != nil {
		return "", noop, err
	}
	sum := sha256.Sum256(body)
	want, listed := manifest[filepath.ToSlash(relName)]
	if !listed {
		return "", noop, fmt.Errorf("%w: %s is not listed", ErrRunbookUnapproved, relName)
	}
	if want != hex.EncodeToString(sum[:]) {
		return "", noop, fmt.Errorf("%w: %s changed since the manifest was written", ErrRunbookUnapproved, relName)
	}
	hits, err := scanRunbook(body)
	if err != nil {
		return "", noop, err
	}
	if len(hits) > 0 && !runbookApproved(body) {
		return "", noop, fmt.Errorf("%w: %s", ErrRunbookDestructive, strings.Join(hits, "; "))
	}
	dir, err := os.MkdirTemp("", "champion-runbook-")
	if err != nil {
		return "", noop, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	copyPath := filepath.Join(dir, filepath.Base(runbookPath))
	// #nosec G703 -- dir is a fresh MkdirTemp; runbookPath was read as a regular file above, so its base is a single file name
	if err := os.WriteFile(copyPath, body, 0o600); err != nil {
		cleanup()
		return "", noop, err
	}
	return copyPath, cleanup, nil
}
