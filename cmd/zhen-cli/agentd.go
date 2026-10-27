// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// ADR-059 Phases 2b and 3: the slash commands that reach zhen-agentd
// beyond /api/v1/tool/exec. Each one is a request to the daemon; the CLI
// opens no database and runs no script.

// modelSwitchTimeout covers the longest swap agentd allows before it rolls
// back (4x the slowest documented load time plus the GPU lease wait).
const modelSwitchTimeout = 15 * time.Minute

// modelKeyRE is switch-model.sh's key shape (pkg/champion allowlistKey); a
// key outside it is refused here, before any request.
var modelKeyRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func (s *session) w() io.Writer {
	if s.out != nil {
		return s.out
	}
	return os.Stdout
}

func (s *session) ew() io.Writer {
	if s.errOut != nil {
		return s.errOut
	}
	return os.Stderr
}

// setAuth sends the API key both ways zhen-agentd may read it: pkg/auth's
// API-key authenticator reads X-API-Key today; Authorization: Bearer is the
// form S-IRON-GATE's uhk_ keys use.
func (s *session) setAuth(req *http.Request) {
	if s.agentdKey == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+s.agentdKey)
	req.Header.Set("X-API-Key", s.agentdKey)
}

// agentdJSON sends one request to zhen-agentd and decodes its JSON reply
// into out (any status code: refusals carry a body too). It returns the
// status code.
func (s *session) agentdJSON(ctx context.Context, client *http.Client, method, path string, body, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.agentdURL, "/")+path, rd)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	s.setAuth(req)
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("zhen-agentd unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return resp.StatusCode, fmt.Errorf("zhen-agentd refused the credentials (401): set ZHEN_API_KEY or api_key in %s", defaultConfPath())
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return resp.StatusCode, fmt.Errorf("status %d, decode: %w (body: %.200s)", resp.StatusCode, err, raw)
	}
	return resp.StatusCode, nil
}

type memoryRow struct {
	ID        int64     `json:"id"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Source    string    `json:"source"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Score     float64   `json:"score"`
}

type errorBody struct {
	Error string `json:"error"`
}

// handleRecall: /recall <text> → GET /api/v1/memory/recall.
func (s *session) handleRecall(q string) {
	if q == "" {
		s.warn("/recall <text>")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var resp struct {
		Memories []memoryRow `json:"memories"`
		errorBody
	}
	code, err := s.agentdJSON(ctx, s.client, http.MethodGet, "/api/v1/memory/recall?"+url.Values{"q": {q}}.Encode(), nil, &resp)
	if err != nil {
		s.errf("/recall: %v", err)
		return
	}
	if code != http.StatusOK {
		s.errf("/recall: %d %s", code, resp.Error)
		return
	}
	if len(resp.Memories) == 0 {
		s.dim("zhen-cli: nothing remembered matches.")
		return
	}
	for _, m := range resp.Memories {
		meta := []string{m.Source}
		if m.Model != "" {
			meta = append(meta, m.Model)
		}
		if !m.CreatedAt.IsZero() {
			meta = append(meta, m.CreatedAt.UTC().Format("2006-01-02 15:04"))
		}
		meta = append(meta, fmt.Sprintf("score %.2f", m.Score))
		s.dim(fmt.Sprintf("#%d [%s] %s", m.ID, strings.Join(meta, " · "), m.Question))
		fmt.Fprintln(s.w(), clipContent(m.Answer, s.maxChars))
	}
}

// handleRemember: /remember <text> stores text as a note; /remember alone
// stores the last question and answer of this session.
func (s *session) handleRemember(text string) {
	body := map[string]any{"emitted_by": "zhen-cli"}
	switch {
	case text != "":
		body["question"] = "note"
		body["answer"] = text
	case len(s.history) >= 2:
		body["question"] = s.history[len(s.history)-2].Content
		body["answer"] = s.history[len(s.history)-1].Content
		body["model"] = s.model
	default:
		s.warn("/remember <text>, or ask something first to remember the answer")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var resp struct {
		toolExecResponse
		errorBody
	}
	code, err := s.agentdJSON(ctx, s.client, http.MethodPost, "/api/v1/memory/remember", body, &resp)
	if err != nil {
		s.errf("/remember: %v", err)
		return
	}
	if resp.Status == "" {
		s.errf("/remember: %d %s", code, resp.Error)
		return
	}
	if resp.Status == "ok" {
		if m, ok := resp.Result.(map[string]any); ok {
			if id, ok := m["id"].(float64); ok {
				s.ok(fmt.Sprintf("remembered #%d", int64(id)))
				return
			}
		}
	}
	s.printToolExecResponse("memory_write", &resp.toolExecResponse)
}

// handleConfirm: /confirm <token> → POST /api/v1/agent/confirm. The daemon
// binds the token to the principal whose request caused it; the CLI's key
// decides who that is.
func (s *session) handleConfirm(args []string) {
	if len(args) != 1 {
		s.warn("/confirm <token>")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()
	var resp struct {
		toolExecResponse
		errorBody
	}
	code, err := s.agentdJSON(ctx, s.client, http.MethodPost, "/api/v1/agent/confirm", map[string]string{"token": args[0]}, &resp)
	if err != nil {
		s.errf("/confirm: %v", err)
		return
	}
	if resp.Status == "" {
		s.errf("/confirm: %d %s", code, resp.Error)
		return
	}
	s.printToolExecResponse("confirm", &resp.toolExecResponse)
}

type modelInfo struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Present     bool    `json:"present"`
	Pinned      bool    `json:"pinned"`
	LoadSeconds float64 `json:"load_seconds"`
}

// handleModel: /model lists from GET /api/v1/models; /model <key> asks
// agentd's model_switch (Champion-gated, audited, rolled back on timeout).
func (s *session) handleModel(args []string) {
	if len(args) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var st struct {
			Active string      `json:"active"`
			Port   int         `json:"port"`
			Models []modelInfo `json:"models"`
			errorBody
		}
		code, err := s.agentdJSON(ctx, s.client, http.MethodGet, "/api/v1/models", nil, &st)
		if err != nil {
			s.errf("/model: %v", err)
			return
		}
		if code != http.StatusOK {
			s.errf("/model: %d %s", code, st.Error)
			return
		}
		if st.Active == "" {
			s.dim(fmt.Sprintf("llama-server on :%d serves no listed model", st.Port))
		}
		for _, m := range st.Models {
			mark := " "
			if m.Key == st.Active {
				mark = "*"
			}
			note := ""
			if !m.Present {
				note = "  (file missing)"
			} else if !m.Pinned {
				note = "  (no sha256 pin: refused)"
			}
			fmt.Fprintf(s.w(), "%s %-16s %-36s ~%.0fs%s\n", mark, m.Key, m.Name, m.LoadSeconds, note)
		}
		return
	}
	key := args[0]
	if len(args) != 1 || !modelKeyRE.MatchString(key) {
		s.warn("/model <key>  (a key from /model)")
		return
	}
	s.dim(fmt.Sprintf("zhen-cli: swapping llama-server to %s; chat is down until it is healthy.", key))
	ctx, cancel := context.WithTimeout(context.Background(), modelSwitchTimeout)
	defer cancel()
	// The swap outlives the session client's timeout; this request gets
	// its own, bounded by ctx.
	long := &http.Client{Transport: s.client.Transport}
	var resp toolExecResponse
	if _, err := s.agentdJSON(ctx, long, http.MethodPost, "/api/v1/tool/exec", s.toolExecBody("model_switch", map[string]any{"key": key}), &resp); err != nil {
		s.errf("/model: %v", err)
		return
	}
	s.printToolExecResponse("model_switch", &resp)
}
