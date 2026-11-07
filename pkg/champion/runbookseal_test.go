// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"unheaded/pkg/gungnir"
)

func liveRun(c *Champion) error {
	_, err := c.RunbookExecute(context.Background(), "infra/safe", false)
	return err
}

func manifestPath(c *Champion) string {
	return filepath.Join(c.GetProjectRoot(), "runbooks", RunbookManifestFile)
}

// H3: the manifest sits in the tree it vouches for, so its own bytes prove
// nothing. A live run needs them sealed by a key the project root cannot hold.
func TestRunbookSeal_ASealedManifestRuns(t *testing.T) {
	needPython(t)
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	if err := liveRun(c); err != nil {
		t.Fatal(err)
	}
}

func TestRunbookSeal_NoSealNoLiveRun(t *testing.T) {
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	if err := os.Remove(manifestPath(c) + RunbookSealSuffix); err != nil {
		t.Fatal(err)
	}
	if err := liveRun(c); !errors.Is(err, ErrRunbookManifestUnsealed) {
		t.Fatalf("got %v", err)
	}
}

func TestRunbookSeal_AManifestChangedAfterSealingDoesNotRun(t *testing.T) {
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	f, err := os.OpenFile(manifestPath(c), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("# a comment changes the bytes\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if err := liveRun(c); !errors.Is(err, ErrRunbookManifestUnsealed) {
		t.Fatalf("got %v", err)
	}
}

func TestRunbookSeal_AnotherKeysSealDoesNotRun(t *testing.T) {
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	raw, err := os.ReadFile(manifestPath(c))
	if err != nil {
		t.Fatal(err)
	}
	seal, err := SealRunbookManifest(raw, gungnir.GenerateDevKey())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath(c)+RunbookSealSuffix, seal, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := liveRun(c); !errors.Is(err, ErrRunbookManifestUnsealed) {
		t.Fatalf("got %v", err)
	}
}

func TestRunbookSeal_NoKeyConfiguredMeansNoLiveRun(t *testing.T) {
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	t.Setenv(RunbookManifestKeyEnv, filepath.Join(t.TempDir(), "absent.pub"))
	if err := liveRun(c); !errors.Is(err, ErrRunbookManifestUnsealed) {
		t.Fatalf("got %v", err)
	}
}

// A key the tree can reach is no anchor: whoever writes the manifest would
// write it too.
func TestRunbookSeal_AKeyInsideTheRootIsRefused(t *testing.T) {
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	inside := filepath.Join(c.GetProjectRoot(), "manifest.pub")
	data, err := os.ReadFile(os.Getenv(RunbookManifestKeyEnv))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, data, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Setenv(RunbookManifestKeyEnv, inside)
	if err := liveRun(c); !errors.Is(err, ErrRunbookManifestUnsealed) {
		t.Fatalf("got %v", err)
	}
	link := filepath.Join(t.TempDir(), "link.pub")
	if err := os.Symlink(inside, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv(RunbookManifestKeyEnv, link)
	if err := liveRun(c); !errors.Is(err, ErrRunbookManifestUnsealed) {
		t.Fatalf("through a link: %v", err)
	}
}

func TestRunbookSeal_AKeyOthersCanWriteIsRefused(t *testing.T) {
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	if err := os.Chmod(os.Getenv(RunbookManifestKeyEnv), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := liveRun(c); !errors.Is(err, ErrRunbookManifestUnsealed) {
		t.Fatalf("got %v", err)
	}
}

// A key directory may let a service open a file by name and list nothing:
// unheaded-keygen makes its directories that way, and a good seal has to verify
// from one.
func TestRunbookSeal_AKeyInADirectoryNobodyCanListStillRuns(t *testing.T) {
	needPython(t)
	if os.Geteuid() == 0 {
		t.Skip("root reads a directory whatever its mode")
	}
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	dir := filepath.Dir(os.Getenv(RunbookManifestKeyEnv))
	if err := os.Chmod(dir, 0o311); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := liveRun(c); err != nil {
		t.Fatal(err)
	}
}

// Whoever writes the key's directory can replace the key, whatever the key
// file's own mode says.
func TestRunbookSeal_AKeyInADirectoryOthersCanWriteIsRefused(t *testing.T) {
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	if err := os.Chmod(filepath.Dir(os.Getenv(RunbookManifestKeyEnv)), 0o775); err != nil {
		t.Fatal(err)
	}
	if err := liveRun(c); !errors.Is(err, ErrRunbookManifestUnsealed) {
		t.Fatalf("got %v", err)
	}
}

// A relative path means whatever the working directory says, so it cannot be
// judged against the sandbox.
func TestRunbookSeal_ARelativeKeyPathIsRefused(t *testing.T) {
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	abs := os.Getenv(RunbookManifestKeyEnv)
	t.Chdir(filepath.Dir(abs))
	t.Setenv(RunbookManifestKeyEnv, filepath.Base(abs))
	if err := liveRun(c); !errors.Is(err, ErrRunbookManifestUnsealed) {
		t.Fatalf("got %v", err)
	}
}

// The default key has to sit where the shipped unit lets the daemon read.
func TestRunbookSeal_TheDefaultKeyIsWhereTheUnitReads(t *testing.T) {
	unit, err := os.ReadFile("../../deploy/systemd/zhen-agentd.service")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(DefaultRunbookManifestKey)
	for _, line := range strings.Split(string(unit), "\n") {
		if rest, ok := strings.CutPrefix(line, "ReadOnlyPaths="); ok && slices.Contains(strings.Fields(rest), dir) {
			return
		}
	}
	t.Fatalf("no ReadOnlyPaths= line lists %s", dir)
}

// A dry run runs nothing, so it needs no seal.
func TestRunbookSeal_ADryRunNeedsNone(t *testing.T) {
	needPython(t)
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	if err := os.Remove(manifestPath(c) + RunbookSealSuffix); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RunbookExecute(context.Background(), "infra/safe", true); err != nil {
		t.Fatal(err)
	}
}

// No file tool reaches the seal: it lives under runbooks/.
func TestRunbookSeal_NoFileToolWritesIt(t *testing.T) {
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	if err := c.WriteFile(context.Background(), manifestPath(c)+RunbookSealSuffix, "x"); err == nil {
		t.Fatal("the seal was written")
	}
}
