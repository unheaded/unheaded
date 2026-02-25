package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanner_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	scanner := NewScanner(dir)

	entries, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestScanner_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	svcDir := filepath.Join(dir, "timeguru")
	os.MkdirAll(svcDir, 0755)
	os.WriteFile(filepath.Join(svcDir, "config.yaml"), []byte(`
name: timeguru
port: 19000
proto: http
health: /health
`), 0644)

	scanner := NewScanner(dir)
	entries, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Name != "timeguru" {
		t.Errorf("name = %q, want %q", entries[0].Name, "timeguru")
	}
	if entries[0].Port != 19000 {
		t.Errorf("port = %d, want 19000", entries[0].Port)
	}
}

func TestScanner_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	svcDir := filepath.Join(dir, "broken")
	os.MkdirAll(svcDir, 0755)
	os.WriteFile(filepath.Join(svcDir, "config.yaml"), []byte("{{not yaml"), 0644)

	scanner := NewScanner(dir)
	entries, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	// Invalid YAML is silently skipped
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 (invalid YAML should be skipped)", len(entries))
	}
}

func TestScanner_MultipleServices(t *testing.T) {
	dir := t.TempDir()
	for _, svc := range []struct {
		name string
		port int
	}{
		{"timeguru", 19000},
		{"captain", 19002},
		{"architect", 19001},
	} {
		svcDir := filepath.Join(dir, svc.name)
		os.MkdirAll(svcDir, 0755)
		content := "name: " + svc.name + "\nport: " + itoa(svc.port) + "\nproto: http\n"
		os.WriteFile(filepath.Join(svcDir, "config.yaml"), []byte(content), 0644)
	}

	scanner := NewScanner(dir)
	entries, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("got %d entries, want 3", len(entries))
	}
}

func TestScanner_MissingName_UsesDir(t *testing.T) {
	dir := t.TempDir()
	svcDir := filepath.Join(dir, "myservice")
	os.MkdirAll(svcDir, 0755)
	os.WriteFile(filepath.Join(svcDir, "config.yaml"), []byte("port: 19000\nproto: http\n"), 0644)

	scanner := NewScanner(dir)
	entries, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Name != "myservice" {
		t.Errorf("name = %q, want %q (should fall back to dir name)", entries[0].Name, "myservice")
	}
}

func TestScanner_MissingPort_Skipped(t *testing.T) {
	dir := t.TempDir()
	svcDir := filepath.Join(dir, "noport")
	os.MkdirAll(svcDir, 0755)
	os.WriteFile(filepath.Join(svcDir, "config.yaml"), []byte("name: noport\nproto: http\n"), 0644)

	scanner := NewScanner(dir)
	entries, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 (missing port should be skipped)", len(entries))
	}
}

func TestScanner_NonexistentBase(t *testing.T) {
	scanner := NewScanner("/nonexistent/path")
	_, err := scanner.Scan()
	if err == nil {
		t.Error("expected error for nonexistent base path")
	}
}

func TestScanner_ScanService(t *testing.T) {
	dir := t.TempDir()
	svcDir := filepath.Join(dir, "timeguru")
	os.MkdirAll(svcDir, 0755)
	os.WriteFile(filepath.Join(svcDir, "config.yaml"), []byte("name: timeguru\nport: 19000\nproto: http\n"), 0644)

	scanner := NewScanner(dir)
	entry, err := scanner.ScanService("timeguru")
	if err != nil {
		t.Fatalf("ScanService: %v", err)
	}
	if entry.Port != 19000 {
		t.Errorf("port = %d, want 19000", entry.Port)
	}
}

func TestScanner_ScanServiceNotFound(t *testing.T) {
	dir := t.TempDir()
	scanner := NewScanner(dir)
	_, err := scanner.ScanService("nonexistent")
	if err == nil {
		t.Error("expected error for missing service")
	}
}

func TestNewScanner_DefaultPath(t *testing.T) {
	scanner := NewScanner("")
	if scanner.basePath != DefaultBasePath {
		t.Errorf("basePath = %q, want %q", scanner.basePath, DefaultBasePath)
	}
}

// itoa is a simple int-to-string helper to avoid importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
