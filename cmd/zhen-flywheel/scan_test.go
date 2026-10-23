// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubGitleaks is a scanner binary that finds nothing (exit 0, "[]") or
// fails (exit 1); the privacy tier's own patterns still run.
func stubGitleaks(t *testing.T, fail bool) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gitleaks")
	body := "#!/bin/sh\ncat >/dev/null\necho '[]'\n"
	if fail {
		body = "#!/bin/sh\ncat >/dev/null\nexit 1\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPrivacyScanner_Kinds(t *testing.T) {
	t.Setenv("ZHEN_SECRET_SCANNER", stubGitleaks(t, false))
	sc, err := newPrivacyScanner()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, tc := range []struct {
		text string
		want string // kind, "" for clean
	}{
		{"set password=hunter2hunter2 in the env", "secret"},
		{"mail alice@corp-internal.net about it", "email"},
		{"see /home/stevie/notes.txt", "user"},
		{"nic 52:54:00:12:34:56 is down", "mac"},
		{"curl user@example.com and bob@mail.example.org", ""},
		{"cd /home/user/project && ls /home/$USER", ""},
		{"listen on 127.0.0.1:8080, west and east are hosts", ""},
		{"-----BEGIN RSA PRIVATE KEY-----\nMIIabc\n-----END RSA PRIVATE KEY-----", "pem"},
		{"a [REDACTED:anthropic_key] placeholder is not a finding", ""},
	} {
		got, err := sc.Scan(ctx, tc.text)
		if err != nil {
			t.Fatal(err)
		}
		if tc.want == "" && len(got) != 0 {
			t.Errorf("%q: clean text flagged %v", tc.text, got)
		}
		if tc.want != "" && got[tc.want] == 0 {
			t.Errorf("%q: want %s, got %v", tc.text, tc.want, got)
		}
	}
}

func TestPrivacyScanner_FailsClosed(t *testing.T) {
	t.Setenv("ZHEN_SECRET_SCANNER", stubGitleaks(t, true))
	sc, err := newPrivacyScanner()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sc.Scan(context.Background(), "anything"); err == nil {
		t.Fatal("a failing secret scanner must be an error")
	}
	dir := captureDir(t)
	writeRecords(t, dir, localRecord("a", "answer a"))
	st := newMemStore()
	rep, err := (&Ingester{Store: st, Scanner: sc}).IngestDir(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.captures) != 0 || len(rep.Errors) != 1 || !strings.Contains(rep.Errors[0], "re-scan failed") {
		t.Errorf("unscanned text reached the Well: store %d, errors %q", len(st.captures), rep.Errors)
	}
}

func TestScanDir(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "train.jsonl"), []byte("{\"answer\":\"fine\"}\n{\"answer\":\"PLANTED-SECRET\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := scanDir(context.Background(), testScanner, d)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 1 || rep.Lines != 2 || len(rep.Findings) != 1 || !strings.HasPrefix(rep.Findings[0], "train.jsonl:2: secret=1") {
		t.Errorf("scan = %+v", rep)
	}
}
