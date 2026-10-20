// SPDX-License-Identifier: GPL-3.0-or-later
package enkrateia

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDriftEventEmitsAlert(t *testing.T) {
	a := NewAggregator(10)
	a.HandleDrift(DriftEvent{
		NodeID:     "east",
		Path:       "/etc/sysctl.d/99-hardening.conf",
		Severity:   "alert",
		DetectedAt: time.Now(),
	})

	select {
	case alert := <-a.Alerts():
		if alert.Event.NodeID != "east" {
			t.Fatalf("expected east, got %s", alert.Event.NodeID)
		}
		if alert.Message == "" {
			t.Fatal("expected non-empty alert message")
		}
	case <-time.After(time.Second):
		t.Fatal("expected alert within 1s")
	}
}

func TestNoFileSystemMutationsOnDrift(t *testing.T) {
	// HARD CONDITION #1: enkrateia must NOT mutate the filesystem.
	// This test verifies by inspection that no os.* write functions are called.
	// A future enhancement could use a syscall tracer for stronger validation.
	a := NewAggregator(10)

	// Trigger a drift event with all fields populated
	a.HandleDrift(DriftEvent{
		NodeID:       "test-node",
		Path:         "/test/path",
		HashActual:   []byte("actual"),
		HashExpected: []byte("expected"),
		Severity:     "alert",
		DetectedAt:   time.Now(),
	})

	// Drain alert
	select {
	case <-a.Alerts():
	case <-time.After(time.Second):
		t.Fatal("expected alert")
	}
}

// TestImportsCannotTouchTheFilesystem makes HARD CONDITION #1 a check that
// can fail (LICH-012 L12c): TestNoFileSystemMutationsOnDrift above only
// exercises the path. The package may import nothing that reaches the
// filesystem, a process or the network; a new import has to be argued onto
// this list.
func TestImportsCannotTouchTheFilesystem(t *testing.T) {
	allowed := map[string]bool{"sync": true, "time": true, "errors": true, "fmt": true, "context": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if !allowed[path] {
				t.Errorf("%s imports %q: enkrateia is alerts-only and may not reach the filesystem (ADR-043 #1)", f, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no package sources found; refusing to pass vacuously")
	}
}

func TestBufferOverflowDropsGracefully(t *testing.T) {
	a := NewAggregator(2)
	for i := 0; i < 10; i++ {
		a.HandleDrift(DriftEvent{NodeID: "east", Path: "/x", Severity: "alert"})
	}
	// Should not panic or block
}
