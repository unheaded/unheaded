// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package champion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// rbRoot builds a project root with one runbook (runbooks/<name>.yaml), a
// manifest, and a fake scripts/run-runbook.py that prints the file it was
// handed and that file's content, so a test can see what actually ran.
func rbRoot(t *testing.T, name, body string, manifest func(sum string) string) (*Champion, *mockStore, string) {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, "runbooks", name+".yaml")
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	if manifest != nil {
		m := manifest(hex.EncodeToString(sum[:]))
		if err := os.WriteFile(filepath.Join(root, "runbooks", RunbookManifestFile), []byte(m), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o750); err != nil {
		t.Fatal(err)
	}
	fake := "import sys\np = sys.argv[-1]\nprint('RAN', p)\nprint(open(p).read(), end='')\n"
	if err := os.WriteFile(filepath.Join(root, "scripts", "run-runbook.py"), []byte(fake), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &mockStore{}
	c, err := New(Config{ProjectRoot: root}, store)
	if err != nil {
		t.Fatal(err)
	}
	return c, store, p
}

func entry(name string) func(string) string {
	return func(sum string) string { return sum + "  " + name + ".yaml\n" }
}

const safeRunbook = `apiVersion: runbook/v1
kind: Runbook
metadata:
  name: safe
steps:
  - name: look
    action: shell
    command: echo hello
`

func needPython(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
	}
}

func TestRunbookExecute_HashMismatchRefused(t *testing.T) {
	c, store, _ := rbRoot(t, "infra/safe", safeRunbook, func(string) string {
		other := sha256.Sum256([]byte("a different body"))
		return hex.EncodeToString(other[:]) + "  infra/safe.yaml\n"
	})
	_, err := c.RunbookExecute(context.Background(), "infra/safe", false)
	if !errors.Is(err, ErrRunbookUnapproved) {
		t.Fatalf("err = %v, want ErrRunbookUnapproved", err)
	}
	if got := store.actions[len(store.actions)-1].Status; got != "denied" {
		t.Errorf("action status = %q, want denied", got)
	}
}

func TestRunbookExecute_NotInManifestRefused(t *testing.T) {
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, func(sum string) string {
		return sum + "  infra/other.yaml\n"
	})
	if _, err := c.RunbookExecute(context.Background(), "infra/safe", false); !errors.Is(err, ErrRunbookUnapproved) {
		t.Fatalf("err = %v, want ErrRunbookUnapproved", err)
	}
}

func TestRunbookExecute_MissingManifestRefused(t *testing.T) {
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, nil)
	if _, err := c.RunbookExecute(context.Background(), "infra/safe", false); !errors.Is(err, ErrRunbookUnapproved) {
		t.Fatalf("err = %v, want ErrRunbookUnapproved", err)
	}
}

func TestRunbookExecute_DryRunIgnoresManifest(t *testing.T) {
	needPython(t)
	c, _, p := rbRoot(t, "infra/safe", safeRunbook, nil)
	res, err := c.RunbookExecute(context.Background(), "infra/safe", true)
	if err != nil {
		t.Fatalf("dry run refused: %v", err)
	}
	if !strings.Contains(res.Output, "RAN "+p) {
		t.Errorf("dry run should read the runbook in place; output %q", res.Output)
	}
}

func TestRunbookExecute_ExecutesTheVerifiedCopy(t *testing.T) {
	needPython(t)
	c, _, p := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	res, err := c.RunbookExecute(context.Background(), "infra/safe", false)
	if err != nil {
		t.Fatalf("verified runbook refused: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit %d: %s", res.ExitCode, res.Output)
	}
	// The subprocess reads a private copy of the bytes that were hashed,
	// not the path a writer could swap after the check.
	if strings.Contains(res.Output, "RAN "+p) || !strings.Contains(res.Output, "RAN ") {
		t.Errorf("expected a private verified copy, output %q", res.Output)
	}
	if !strings.HasSuffix(res.Output, safeRunbook) {
		t.Errorf("executed content differs from the verified body: %q", res.Output)
	}
}

const destructiveRunbook = `apiVersion: runbook/v1
kind: Runbook
metadata:
  name: wipe-scratch
%s
steps:
  - name: clear
    command: rm -rf /tmp/scratch
`

func TestRunbookExecute_DestructiveUnapprovedRefused(t *testing.T) {
	for _, status := range []string{"", "  status: draft\n  approved_by: \"\"", "  status: approved\n  approved_by: \"\""} {
		body := strings.Replace(destructiveRunbook, "%s\n", status+"\n", 1)
		c, store, _ := rbRoot(t, "data/wipe", body, entry("data/wipe"))
		_, err := c.RunbookExecute(context.Background(), "data/wipe", false)
		if !errors.Is(err, ErrRunbookDestructive) {
			t.Fatalf("status %q: err = %v, want ErrRunbookDestructive", status, err)
		}
		if got := store.actions[len(store.actions)-1]; got.Status != "denied" {
			t.Errorf("status %q: action %+v, want denied", status, got)
		}
	}
}

func TestRunbookExecute_DestructiveApprovedRuns(t *testing.T) {
	needPython(t)
	body := strings.Replace(destructiveRunbook, "%s\n", "  status: approved\n  approved_by: stevie\n", 1)
	c, _, _ := rbRoot(t, "data/wipe", body, entry("data/wipe"))
	if _, err := c.RunbookExecute(context.Background(), "data/wipe", false); err != nil {
		t.Fatalf("approved runbook refused: %v", err)
	}
}

func TestScanRunbook_ExecutableFieldsOnly(t *testing.T) {
	t.Parallel()
	body := `metadata: {name: x}
notes: "never reboot west by hand"
steps:
  - name: manual-only
    action: manual
    command: sudo reboot
  - name: default-shell
    command: echo ok
    verify: "test -f /x"
  - name: verify-hit
    action: shell
    command: echo ok
    verify: "dd if=/dev/zero of=/dev/sda"
rollback:
  steps:
    - name: undo
      command: git reset --hard HEAD~1
verification:
  - name: check
    action: shell
    command: shred -u key
`
	hits, err := scanRunbook([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(hits, "|")
	for _, want := range []string{"verify-hit", "rollback/undo", "verification/check"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing hit %q in %q", want, got)
		}
	}
	for _, not := range []string{"manual-only", "default-shell", "notes"} {
		if strings.Contains(got, not) {
			t.Errorf("unexpected hit %q in %q", not, got)
		}
	}
}

func TestScanRunbook_RollbackList(t *testing.T) {
	t.Parallel()
	hits, err := scanRunbook([]byte("rollback:\n  - name: r\n    command: kill -9 1\n"))
	if err != nil || len(hits) != 1 || !strings.Contains(hits[0], "rollback/r") {
		t.Errorf("hits = %q, err = %v", hits, err)
	}
}

func TestScanRunbook_BadYAML(t *testing.T) {
	t.Parallel()
	if _, err := scanRunbook([]byte("steps: [unclosed")); err == nil {
		t.Error("malformed YAML scanned clean")
	}
}

func TestParseRunbookManifest(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte("x"))
	h := hex.EncodeToString(sum[:])
	m, err := parseRunbookManifest([]byte("# comment\n\n" + h + "  infra/a.yaml\n" + h + " *data/b.yaml\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m["infra/a.yaml"] != h || m["data/b.yaml"] != h {
		t.Errorf("parsed %v", m)
	}
	for _, bad := range []string{
		"nothex  a.yaml\n",
		h[:10] + "  a.yaml\n",
		h + "\n",
		h + "  /abs/a.yaml\n",
		h + "  ../a.yaml\n",
		h + "  a.yaml\n" + h + "  a.yaml\n",
	} {
		if _, err := parseRunbookManifest([]byte(bad)); err == nil {
			t.Errorf("accepted bad manifest %q", bad)
		}
	}
}

// The committed manifest covers exactly the committed runbooks with their
// current bytes (the CI freshness check, scripts/runbook-manifest.sh
// --check, runs the same comparison in shell).
func TestCommittedManifest_IsFresh(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "..", "runbooks")
	raw, err := os.ReadFile(filepath.Join(dir, RunbookManifestFile))
	if err != nil {
		t.Fatalf("read manifest: %v (run scripts/runbook-manifest.sh --write)", err)
	}
	m, err := parseRunbookManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".yaml" {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		if m[filepath.ToSlash(rel)] != hex.EncodeToString(sum[:]) {
			t.Errorf("%s: not in the manifest or stale", rel)
		}
		seen++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != len(m) {
		t.Errorf("manifest has %d entries, runbooks/ has %d yaml files", len(m), seen)
	}
}
