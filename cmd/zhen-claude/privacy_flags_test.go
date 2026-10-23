// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestPrivacyFlag_DefaultsAndValidation(t *testing.T) {
	o, err := parseFlags(baseArgs(t), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if o.privacyMode != "standard" {
		t.Errorf("default -privacy = %q", o.privacyMode)
	}
	noCredentials(t)
	var out, errb bytes.Buffer
	if code := run(append(baseArgs(t), "-privacy", "lax", "-p", "x"), strings.NewReader(""), &out, &errb); code != 2 {
		t.Errorf("-privacy lax: exit %d", code)
	}
}

func TestPrivacy_NoScannerSendsNothing(t *testing.T) {
	f := newFakeClaude(t, "should never be asked")
	withClaude(t, f)
	args := append(baseArgs(t), "-tools", "none", "-p", "hello")
	t.Setenv("ZHEN_SECRET_SCANNER", "/nonexistent/gitleaks")
	var out, errb bytes.Buffer
	code := run(append(args, "-secret-scanner", "/nonexistent/gitleaks"), strings.NewReader(""), &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "secret scan") {
		t.Errorf("exit %d: %s", code, errb.String())
	}
	if f.count() != 0 {
		t.Errorf("%d request(s) reached Claude without a secret scanner", f.count())
	}
}

func TestPrivacy_PromptRedactedAndLogged(t *testing.T) {
	f := newFakeClaude(t, "ok")
	withClaude(t, f)
	var out, errb bytes.Buffer
	code := run(append(baseArgs(t), "-tools", "none", "-privacy", "strict", "-p", "is 10.10.10.10 up?"), strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if f.count() != 1 {
		t.Fatalf("requests = %d", f.count())
	}
	ready := logLine(t, errb.String(), "ready")
	if ready["privacy"] != "strict" {
		t.Errorf("ready line = %v", ready)
	}
	if !strings.Contains(errb.String(), "redacted 1 value") {
		t.Errorf("no redaction warning: %s", errb.String())
	}
	if strings.Contains(errb.String(), "10.10.10.10") {
		t.Error("the address reached the log")
	}
}
