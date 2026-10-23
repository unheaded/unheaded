// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package privacy

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The planted values are built at run time, never written in source or
// testdata: a literal key or PEM header here would trip the repository's
// own secret scanning gate.
func syntheticAWSKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "AKIA" + base32.StdEncoding.EncodeToString(b)[:16]
}

func pemHeader() string { return "-----BEGIN " + "RSA PRIVATE KEY-----" }

type planted struct {
	name, value string
}

func plantedFixture(t *testing.T) (string, []planted) {
	t.Helper()
	aws := syntheticAWSKey(t)
	vals := []planted{
		{"aws", aws},
		{"pem", pemHeader()},
		{"password", "hunter2"},
		{"dsn-password", "s3cr3tpw"},
		{"ipv4-a", "192.168.13.2"},
		{"ipv4-b", "10.10.10.10"},
		{"ipv6", "fd00:dead:beef::1"},
		{"mac", "3c:52:82:aa:bb:cc"},
		{"email", "stevie@bellis.tech"},
		{"home", "/home/govan"},
		{"host-west", "west"},
		{"host-east", "east"},
	}
	text := strings.Join([]string{
		"aws_access_key_id = " + aws,
		pemHeader(),
		"MIIEowIBAAKCAQEAsynthetic",
		"-----END " + "RSA PRIVATE KEY-----",
		"password=hunter2",
		"dsn: postgres://ops_writer:s3cr3tpw@10.10.10.10:5432/unheaded_ops",
		"wotan on 10.10.10.10, peer 192.168.13.2, again 10.10.10.10",
		"overlay fd00:dead:beef::1 via 3c:52:82:aa:bb:cc",
		"mail stevie@bellis.tech; logs in /home/govan/tmp/unheaded/logs",
		"west is up, east is degraded; WEST again",
		"timestamps 10:20:30 and version 1.2.3 stay",
	}, "\n")
	return text, vals
}

func newTestRedactor(t *testing.T) *Redactor {
	t.Helper()
	r, err := NewRedactor(RedactorConfig{})
	if err != nil {
		t.Skip("secret scanner unavailable:", err)
	}
	return r
}

func TestRedact_NoPlantedValueSurvives(t *testing.T) {
	r := newTestRedactor(t)
	text, vals := plantedFixture(t)
	out, counts, err := r.Redact(context.Background(), text, TierOps)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range vals {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(p.value) + `\b`)
		if p.name == "pem" || p.name == "ipv6" || p.name == "home" {
			re = regexp.MustCompile(regexp.QuoteMeta(p.value))
		}
		if re.MatchString(out) {
			t.Errorf("%s survived: %q in\n%s", p.name, p.value, out)
		}
	}
	if counts.Total() < 12 {
		t.Errorf("counts = %v (total %d)", counts, counts.Total())
	}
	if !strings.Contains(out, "10:20:30") || !strings.Contains(out, "1.2.3") {
		t.Errorf("harmless text was mangled:\n%s", out)
	}
}

func TestRedact_PlaceholdersStableWithinPayload(t *testing.T) {
	r := newTestRedactor(t)
	out, _, err := r.Redact(context.Background(), "a 10.10.10.10 b 192.168.13.2 c 10.10.10.10", TierOps)
	if err != nil {
		t.Fatal(err)
	}
	tags := regexp.MustCompile(`\[REDACTED:ipv4#\d+\]`).FindAllString(out, -1)
	if len(tags) != 3 || tags[0] != tags[2] || tags[0] == tags[1] {
		t.Errorf("tags = %v (same IP must keep its tag, different IPs differ): %s", tags, out)
	}
}

func TestRedact_RepoTierRemovesSecretsOnly(t *testing.T) {
	r := newTestRedactor(t)
	aws := syntheticAWSKey(t)
	out, counts, err := r.Redact(context.Background(), "host west at 10.10.10.10 key "+aws, TierRepo)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, aws) {
		t.Error("a secret survived the T0 pass")
	}
	if !strings.Contains(out, "10.10.10.10") || !strings.Contains(out, "west") {
		t.Errorf("T0 should keep repo text (ADRs name hosts): %s", out)
	}
	if counts.Total() != 1 {
		t.Errorf("counts = %v", counts)
	}
}

func TestRedact_ScannerFailureFailsClosed(t *testing.T) {
	r, err := NewRedactor(RedactorConfig{Scanner: "/bin/false"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Redact(context.Background(), "anything", TierOps); err == nil {
		t.Fatal("a failed scanner let text through")
	}
	if _, err := NewRedactor(RedactorConfig{Scanner: "/nonexistent/gitleaks"}); err == nil {
		t.Error("a missing scanner was accepted")
	}
}

func TestRedact_ScannerTimeoutFailsClosed(t *testing.T) {
	dir := t.TempDir()
	slow := filepath.Join(dir, "slow")
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nsleep 5\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	r, err := NewRedactor(RedactorConfig{Scanner: slow, Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	if _, _, err := r.Redact(context.Background(), "x", TierOps); err == nil {
		t.Fatal("a hung scanner let text through")
	}
	if time.Since(t0) > 3*time.Second {
		t.Error("the timeout did not bound the scan")
	}
}

func TestRedact_WithheldTierIsAnError(t *testing.T) {
	r := newTestRedactor(t)
	if _, _, err := r.Redact(context.Background(), "x", TierUserData); !errors.Is(err, ErrWithheld) {
		t.Errorf("T2: %v", err)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@example.invalid"}, {"config", "user.name", "t"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("git: %v %s", err, out)
		}
	}
	for name, body := range map[string]string{"README.md": "hi", "docs/adr.md": "adr", ".env": "K=V"} {
		p := filepath.Join(dir, name)
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", dir, "add", "-f", "README.md", "docs/adr.md", ".env").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "-q", "--no-gpg-sign", "-m", "x").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.log"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestClassify(t *testing.T) {
	root := gitRepo(t)
	userData := t.TempDir()
	c := Classifier{UserDataRoots: []string{userData}}
	cases := []struct {
		tool string
		args map[string]any
		want Tier
	}{
		{"corpus_search", map[string]any{"q": "x"}, TierRepo},
		{"runbook_list", nil, TierRepo},
		{"runbook_show", map[string]any{"name": "a"}, TierRepo},
		{"read_file", map[string]any{"path": filepath.Join(root, "README.md")}, TierRepo},
		{"file_read", map[string]any{"path": filepath.Join(root, "docs/adr.md")}, TierRepo},
		{"read_file", map[string]any{"path": filepath.Join(root, ".env")}, TierOps}, // tracked, but on the deny list
		{"read_file", map[string]any{"path": filepath.Join(root, "untracked.log")}, TierOps},
		{"read_file", map[string]any{"path": filepath.Join(root, "missing.txt")}, TierOps},
		{"read_file", map[string]any{"path": "/etc/hostname"}, TierOps},
		{"read_file", map[string]any{"path": filepath.Join(userData, "customers.csv")}, TierUserData},
		{"service_health", nil, TierOps},
		{"kanban_list", nil, TierOps},
		{"local_llm", map[string]any{"prompt": "x"}, TierOps},
		{"made_up_tool", nil, TierOps},
	}
	for _, tc := range cases {
		if got := c.Classify(tc.tool, tc.args, root); got != tc.want {
			t.Errorf("Classify(%s %v) = %v, want %v", tc.tool, tc.args, got, tc.want)
		}
	}
}

func TestClassify_SymlinkToUntrackedIsOps(t *testing.T) {
	root := gitRepo(t)
	link := filepath.Join(root, "link.md")
	if err := os.Symlink(filepath.Join(root, "untracked.log"), link); err != nil {
		t.Skip(err)
	}
	if got := (Classifier{}).Classify("read_file", map[string]any{"path": link}, root); got != TierOps {
		t.Errorf("symlink to an untracked file = %v", got)
	}
}
