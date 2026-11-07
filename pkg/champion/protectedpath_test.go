// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// H3: the trees that decide what runs on the host are out of every file
// tool's reach, confirmed or not.
func TestValidatePath_ProtectedTrees(t *testing.T) {
	c, _ := newTestChampion(t)
	root := c.GetProjectRoot()
	for _, rel := range []string{
		"runbooks", "runbooks/infra/x.yaml", "runbooks/" + RunbookManifestFile,
		"scripts", "scripts/run-runbook.py", "configs/zhen-schedules.yaml",
		".git", ".git/config", ".git/hooks/pre-commit", "pkg/vendored/.git/config", "pkg/.GIT/config",
	} {
		if err := c.validatePath(filepath.Join(root, rel)); err == nil {
			t.Errorf("validatePath allows %s", rel)
		}
	}
	for _, rel := range []string{
		"pkg/ex/ex.go", "README.md", ".gitignore", ".github/workflows/ci.yml", "docs/runbooks/index.md",
		"pkg/scripts/gen.go", "runbooks.md", "scripts.txt", "configs/wotan.yaml", "pkg/configs/zhen-schedules.yaml",
	} {
		if err := c.validatePath(filepath.Join(root, rel)); err != nil {
			t.Errorf("validatePath refuses %s: %v", rel, err)
		}
	}
}

// The check is by path component under the root, so a checkout that merely
// sits under a directory with such a name keeps working.
func TestValidatePath_ProtectedTreesAreRelativeToTheRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scripts", "runbooks", "repo")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{ProjectRoot: root}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.validatePath(filepath.Join(root, "main.go")); err != nil {
		t.Errorf("a file in a root under scripts/: %v", err)
	}
	if err := c.validatePath(filepath.Join(root, "scripts", "x.sh")); err == nil {
		t.Error("the root's own scripts/ is allowed")
	}
}

// The list does not depend on Config.DeniedPaths, which a caller may replace.
func TestValidatePath_ProtectedTreesSurviveACustomDenyList(t *testing.T) {
	root := t.TempDir()
	c, err := New(Config{ProjectRoot: root, DeniedPaths: []string{"nothing-here"}}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.validatePath(filepath.Join(root, "runbooks", "x.yaml")); err == nil {
		t.Error("a custom deny list dropped the protected trees")
	}
}

// A link inside the sandbox is judged by where it leads.
func TestValidatePath_ProtectedTreesThroughASymlink(t *testing.T) {
	c, _ := newTestChampion(t)
	root := c.GetProjectRoot()
	if err := os.MkdirAll(filepath.Join(root, "runbooks"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "runbooks"), filepath.Join(root, "notes")); err != nil {
		t.Fatal(err)
	}
	if err := c.validatePath(filepath.Join(root, "notes", "x.yaml")); err == nil {
		t.Error("a link into runbooks/ is allowed")
	}
}

// A link inside a protected tree stays out of reach wherever it leads: the
// path as given counts too.
func TestValidatePath_ProtectedTreesAsGiven(t *testing.T) {
	c, _ := newTestChampion(t)
	root := c.GetProjectRoot()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(target, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "scripts", "notes")); err != nil {
		t.Fatal(err)
	}
	if err := c.validatePath(filepath.Join(root, "scripts", "notes")); err == nil {
		t.Error("a path under scripts/ is allowed because a link leads out of it")
	}
}

// Rule 1 holds after a person confirms, and in branch-sandbox mode.
func TestProtectedTrees_RefusedThroughTheGateAndAfterConfirmation(t *testing.T) {
	c, _ := newTestChampion(t)
	ctx := context.Background()
	p := filepath.Join(c.GetProjectRoot(), "runbooks", RunbookManifestFile)
	tc := ToolCall{Name: "write_file", Args: map[string]any{"path": p, "content": "x"}, Justification: []Reference{canonicalRef("t")}}
	d, err := c.AcceptToolCall(ctx, tc)
	if err == nil || d.Allow || d.PendingConfirmation {
		t.Fatalf("gate: %+v %v", d, err)
	}
	tc.Principal = "tty:stevie"
	tok, err := c.IssuePendingConfirmation(tc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ConfirmPendingToolCall(ctx, tok, "tty:stevie"); err == nil || !strings.Contains(err.Error(), "path-allowlist") {
		t.Fatalf("confirmed write: %v", err)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("the manifest was written")
	}
}

// Another allowed root may hold further checkouts, so a protected name counts
// at any depth there; the project's own nested directories stay usable.
func TestValidatePath_ProtectedTreesUnderAnotherAllowedRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "projects", "default")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{ProjectRoot: root, AllowedPaths: []string{root, base}}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.validatePath(filepath.Join(base, "projects", "other", "runbooks", "MANIFEST.sha256")); err == nil {
		t.Error("another checkout's runbooks/ is allowed")
	}
	if err := c.validatePath(filepath.Join(root, "raft", "scripts", "x.py")); err != nil {
		t.Errorf("the project's nested scripts dir: %v", err)
	}
}

// ADR-024: Zhen proposes a runbook as a draft on a zhen/<name> branch. Only
// .git stays out of reach there; a run_check can still name those paths.
func TestBranchSandbox_KeepsTheAuthoringRouteAndNotGit(t *testing.T) {
	c, _, root, agent := sandboxChampion(t, "zhen/draft-runbook")
	if err := c.WithBranchSandbox(agent); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"runbooks/infra/new.yaml", "scripts/new.sh"} {
		if err := c.validatePath(filepath.Join(root, rel)); err != nil {
			t.Errorf("%s in a branch sandbox: %v", rel, err)
		}
	}
	for _, rel := range []string{".git", ".git/config", "pkg/.git/config"} {
		if err := c.validatePath(filepath.Join(root, rel)); err == nil {
			t.Errorf("%s in a branch sandbox is allowed", rel)
		}
	}
}

// M21: key material is out of every file tool's reach, by the name as given
// and as resolved, whatever Config.DeniedPaths says.
func TestValidatePath_KeyMaterial(t *testing.T) {
	root := t.TempDir()
	c, err := New(Config{ProjectRoot: root, DeniedPaths: []string{"nothing-here"}}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"tls/server.key", "tls/ca.pem", "tls/CA.PEM"} {
		if err := c.validatePath(filepath.Join(root, rel)); err == nil {
			t.Errorf("validatePath allows %s", rel)
		}
	}
	for _, rel := range []string{"pkg/keyboard.go", "docs/pem-format.md", "pkg/x.key.md"} {
		if err := c.validatePath(filepath.Join(root, rel)); err != nil {
			t.Errorf("validatePath refuses %s: %v", rel, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "server.key"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "server.key"), filepath.Join(root, "notes")); err != nil {
		t.Fatal(err)
	}
	if err := c.validatePath(filepath.Join(root, "notes")); err == nil {
		t.Error("a link to key material is allowed")
	}
}

// H3: the deny is on writing. Zhen reads runbooks and scripts to author them;
// what it cannot do is change them, and .git stays out of reach both ways: a
// repository's config can hold a credential.
func TestReadFile_ProtectedTreesReadButDoNotWrite(t *testing.T) {
	c, _ := newTestChampion(t)
	ctx := context.Background()
	root := c.GetProjectRoot()
	for _, rel := range []string{"runbooks/infra/x.yaml", "runbooks/" + RunbookManifestFile, "scripts/run-runbook.py", "configs/zhen-schedules.yaml"} {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("body\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, err := c.ReadFile(ctx, p); err != nil || got != "body\n" {
			t.Errorf("read %s: %q %v", rel, got, err)
		}
		read := ToolCall{Name: "read_file", Args: map[string]any{"path": p}}
		if d, err := c.AcceptToolCall(ctx, read); err != nil || !d.Allow {
			t.Errorf("gate on a read of %s: %+v %v", rel, d, err)
		}
		write := ToolCall{Name: "write_file", Args: map[string]any{"path": p, "content": "x"}, Justification: []Reference{canonicalRef("t")}}
		if d, err := c.AcceptToolCall(ctx, write); err == nil || d.Allow {
			t.Errorf("gate on a write of %s: %+v %v", rel, d, err)
		}
		if err := c.WriteFile(ctx, p, "x"); err == nil {
			t.Errorf("WriteFile %s succeeded", rel)
		}
	}
}

func TestReadFile_GitStaysOutOfReach(t *testing.T) {
	c, _ := newTestChampion(t)
	root := c.GetProjectRoot()
	p := filepath.Join(root, ".git", "config")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("[remote]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReadFile(context.Background(), p); err == nil {
		t.Fatal("read .git/config")
	}
	if err := os.Symlink(p, filepath.Join(root, "notes")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReadFile(context.Background(), filepath.Join(root, "notes")); err == nil {
		t.Fatal("read .git/config through a link")
	}
	tc := ToolCall{Name: "read_file", Args: map[string]any{"path": p}}
	if d, err := c.AcceptToolCall(context.Background(), tc); err == nil || d.Allow {
		t.Fatalf("gate on a read of .git/config: %+v %v", d, err)
	}
}

// Reading through a link into a protected tree is reading that tree; the rest
// of what a read refuses is unchanged.
func TestReadFile_TheOtherDeniesStillHold(t *testing.T) {
	c, _ := newTestChampion(t)
	ctx := context.Background()
	root := c.GetProjectRoot()
	for _, rel := range []string{"tls/server.key", "scripts/deploy.pem", "runbooks/.env", "scripts/secrets/x"} {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := c.ReadFile(ctx, p); err == nil {
			t.Errorf("read %s", rel)
		}
	}
	if _, err := c.ReadFile(ctx, filepath.Join(filepath.Dir(root), "elsewhere", "x")); err == nil {
		t.Error("read outside the sandbox")
	}
}

// run_check names a path it will run in, not one it will read.
func TestRunCheck_ProtectedPathsStayRefused(t *testing.T) {
	c, _ := newTestChampion(t)
	p := filepath.Join(c.GetProjectRoot(), "scripts")
	tc := ToolCall{Name: "run_check", Args: map[string]any{"check": "go-test", "path": p}}
	if d, err := c.AcceptToolCall(context.Background(), tc); err == nil || d.Allow {
		t.Fatalf("run_check on scripts/: %+v %v", d, err)
	}
}
