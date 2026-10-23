// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/zhentools"
)

var _ zhentools.Provider = (*ToolProvider)(nil)

func TestToolProvider_ListsFileToolsWithoutKanban(t *testing.T) {
	c, _ := newTestChampion(t)
	p, err := NewToolProvider(c, "test")
	if err != nil {
		t.Fatal(err)
	}
	tools, err := p.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range tools {
		names = append(names, tl.Name)
		if !json.Valid(tl.InputSchema) {
			t.Errorf("%s: schema is not valid JSON", tl.Name)
		}
	}
	if got := strings.Join(names, ","); got != "read_file,write_file,patch_file" {
		t.Errorf("tools = %s", got)
	}
}

func TestToolProvider_ReadAllowed(t *testing.T) {
	c, _ := newTestChampion(t)
	target := filepath.Join(c.config.ProjectRoot, "a.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, _ := NewToolProvider(c, "test")
	in, _ := json.Marshal(map[string]string{"path": target})
	res, err := p.Call(context.Background(), "read_file", in)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || res.Text != "hello" {
		t.Errorf("res = %+v", res)
	}
}

// A model-originated write carries no justification chain, so Rule 2
// refuses it: the file must not change.
func TestToolProvider_WriteRefusedByGate(t *testing.T) {
	c, _ := newTestChampion(t)
	target := filepath.Join(c.config.ProjectRoot, "b.txt")
	p, _ := NewToolProvider(c, "test")
	in, _ := json.Marshal(map[string]string{"path": target, "content": "x"})

	res, err := p.Call(context.Background(), "write_file", in)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Text, "refused by Champion gate") {
		t.Errorf("res = %+v", res)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("file was written despite refusal")
	}

	_, err = p.Dispatch(context.Background(), "write_file", in)
	var ge *GateError
	if !errors.As(err, &ge) || !ge.PendingConfirmation() {
		t.Errorf("Dispatch err = %v; want pending-confirmation GateError", err)
	}
}

func TestToolProvider_BadInput(t *testing.T) {
	c, _ := newTestChampion(t)
	p, _ := NewToolProvider(c, "test")
	res, err := p.Call(context.Background(), "read_file", json.RawMessage(`[1,2]`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Errorf("non-object input should be a tool error: %+v", res)
	}
}

func TestNewToolProvider_NilChampion(t *testing.T) {
	if _, err := NewToolProvider(nil, ""); err == nil {
		t.Error("want error for nil Champion")
	}
}
