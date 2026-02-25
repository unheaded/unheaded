package discovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolver_ViaRegistry(t *testing.T) {
	mock := newMockWotan()
	reg := New(mock)
	reg.Register(ServiceEntry{Name: "timeguru", Address: "localhost:19000", Port: 19000, Health: "healthy"})

	resolver := NewResolver(reg, nil, nil)
	entry, err := resolver.Resolve(context.Background(), "timeguru")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if entry.Port != 19000 {
		t.Errorf("port = %d, want 19000", entry.Port)
	}
}

func TestResolver_FallbackToStatic(t *testing.T) {
	static := &StaticConfig{
		Services: map[string]StaticEntry{
			"captain": {Host: "localhost", Port: 19002, Proto: "http"},
		},
	}

	// Empty registry — nothing registered
	mock := newMockWotan()
	reg := New(mock)

	resolver := NewResolver(reg, nil, static)
	entry, err := resolver.Resolve(context.Background(), "captain")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if entry.Port != 19002 {
		t.Errorf("port = %d, want 19002", entry.Port)
	}
}

func TestResolver_FallbackToConvention(t *testing.T) {
	dir := t.TempDir()
	svcDir := filepath.Join(dir, "architect")
	os.MkdirAll(svcDir, 0755)
	os.WriteFile(filepath.Join(svcDir, "config.yaml"), []byte("name: architect\nport: 19001\nproto: http\n"), 0644)

	scanner := NewScanner(dir)

	// Empty registry, no static
	mock := newMockWotan()
	reg := New(mock)

	resolver := NewResolver(reg, scanner, nil)
	entry, err := resolver.Resolve(context.Background(), "architect")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if entry.Port != 19001 {
		t.Errorf("port = %d, want 19001", entry.Port)
	}
}

func TestResolver_NotFound(t *testing.T) {
	resolver := NewResolver(nil, nil, nil)
	_, err := resolver.Resolve(context.Background(), "nonexistent")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("expected ErrServiceNotFound, got %v", err)
	}
}

func TestResolver_AllLayersDown(t *testing.T) {
	// All layers nil
	resolver := NewResolver(nil, nil, nil)
	_, err := resolver.Resolve(context.Background(), "anything")
	if err == nil {
		t.Error("expected error when all layers are nil")
	}
}

func TestResolver_ResolveAll_Deduplicates(t *testing.T) {
	mock := newMockWotan()
	reg := New(mock)
	reg.Register(ServiceEntry{Name: "timeguru", Address: "localhost:19000", Port: 19000, Health: "healthy"})

	static := &StaticConfig{
		Services: map[string]StaticEntry{
			"timeguru": {Host: "localhost", Port: 19000, Proto: "http"},
			"captain":  {Host: "localhost", Port: 19002, Proto: "http"},
		},
	}

	resolver := NewResolver(reg, nil, static)
	all := resolver.ResolveAll(context.Background())

	// timeguru from registry + captain from static = 2 (deduplicated)
	if len(all) != 2 {
		t.Errorf("got %d entries, want 2 (deduplicated)", len(all))
	}

	names := make(map[string]bool)
	for _, e := range all {
		names[e.Name] = true
	}
	if !names["timeguru"] {
		t.Error("missing timeguru in ResolveAll")
	}
	if !names["captain"] {
		t.Error("missing captain in ResolveAll")
	}
}

func TestResolver_ResolveAll_AllNil(t *testing.T) {
	resolver := NewResolver(nil, nil, nil)
	all := resolver.ResolveAll(context.Background())
	if len(all) != 0 {
		t.Errorf("got %d entries, want 0", len(all))
	}
}
