// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// /api/v1/agent/ask runs a model loop and answers whoever called it over
// HTTP, which may itself be a model. A pending-confirmation token in its
// trace would let that caller approve its own write through /confirm
// (S-TWO-BRAINS Phase 5 open item h, closed in Step 222). By default no
// token is issued: the call is queued in the Well (-confirm-queue) and
// only the queue id is returned, or it is denied.

func askServer(t *testing.T, q *memQueue, askTokens bool) (*server, string) {
	t.Helper()
	root := t.TempDir()
	llm := &scriptedLLM{out: []string{
		fmt.Sprintf(`{"thought":"write","tool_call":{"name":"write_file","args":{"path":%q,"content":"x"}}}`, filepath.Join(root, "a.txt")),
		`{"thought":"done","answer":"waiting"}`,
	}}
	srv := &server{
		pool:        newChampionPool(newMetricsActionStore(nopActionStore{}), nil),
		defaultRoot: root,
		allowed:     map[string]struct{}{root: {}},
		retriever:   externalStub(),
		llm:         llm,
		askTokens:   askTokens,
	}
	if q != nil {
		srv.confirmQueue = q
	}
	return srv, root
}

func ask(t *testing.T, srv *server, stream bool) string {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"goal": "write a.txt"})
	rec := httptest.NewRecorder()
	if stream {
		srv.handleAskStream(rec, httptest.NewRequest(http.MethodPost, "/api/v1/agent/ask/stream", bytes.NewReader(b)))
	} else {
		srv.handleAsk(rec, httptest.NewRequest(http.MethodPost, "/api/v1/agent/ask", bytes.NewReader(b)))
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestAsk_NoTokenByDefault(t *testing.T) {
	for _, stream := range []bool{false, true} {
		srv, _ := askServer(t, nil, false)
		body := ask(t, srv, stream)
		if strings.Contains(body, "pending_token") {
			t.Errorf("stream=%v: a token reached the /ask caller: %s", stream, body)
		}
		if !strings.Contains(body, `"pending_confirmation":true`) {
			t.Errorf("stream=%v: the refusal is not reported: %s", stream, body)
		}
		if strings.Contains(body, "queue_id") {
			t.Errorf("stream=%v: queued without a queue: %s", stream, body)
		}
	}
}

func TestAsk_QueuedWithConfirmQueue(t *testing.T) {
	for _, stream := range []bool{false, true} {
		q := &memQueue{}
		srv, _ := askServer(t, q, false)
		body := ask(t, srv, stream)
		if strings.Contains(body, "pending_token") {
			t.Errorf("stream=%v: token in %s", stream, body)
		}
		if len(q.rows) != 1 || q.rows[0].Tool != "write_file" || q.rows[0].SourceService != "zhen-agentd" {
			t.Fatalf("stream=%v: queue = %+v", stream, q.rows)
		}
		if !strings.Contains(body, `"queue_id":"`+q.rows[0].ID+`"`) {
			t.Errorf("stream=%v: queue id missing from %s", stream, body)
		}
	}
}

func TestAsk_LegacyTokensOnlyWhenAsked(t *testing.T) {
	srv, _ := askServer(t, nil, true)
	if body := ask(t, srv, false); !strings.Contains(body, "pending_token") {
		t.Errorf("-ask-tokens did not issue a token: %s", body)
	}
}
