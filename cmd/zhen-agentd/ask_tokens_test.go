// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/agent"
	"unheaded/pkg/champion"
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

// recordingRetriever keeps every query it is asked.
type recordingRetriever struct {
	inner agent.Retriever
	calls []string
}

func (r *recordingRetriever) Retrieve(ctx context.Context, q string, k int) ([]champion.Reference, []agent.TopicContent, error) {
	r.calls = append(r.calls, q)
	return r.inner.Retrieve(ctx, q, k)
}

// The page wraps the question in a preamble and sends the question beside
// it. Both ask routes seed retrieval with the question, and with the goal
// when no question came.
func TestAsk_QuestionSeedsRetrieval(t *testing.T) {
	const goal = "Prior conversation:\nUser: tell me about kerberos\n\nUser question: what is wotan"
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct{ question, want string }{
			{" what is wotan ", "what is wotan"},
			{"", goal},
		} {
			srv, _ := askServer(t, nil, false)
			rec := &recordingRetriever{inner: srv.retriever}
			srv.retriever = rec
			srv.llm = &scriptedLLM{out: []string{`{"thought":"t","answer":"a"}`}}
			b, _ := json.Marshal(map[string]any{"goal": goal, "question": tc.question})
			w := httptest.NewRecorder()
			if stream {
				srv.handleAskStream(w, httptest.NewRequest(http.MethodPost, "/api/v1/agent/ask/stream", bytes.NewReader(b)))
			} else {
				srv.handleAsk(w, httptest.NewRequest(http.MethodPost, "/api/v1/agent/ask", bytes.NewReader(b)))
			}
			if w.Code != http.StatusOK {
				t.Fatalf("stream=%v: status %d: %s", stream, w.Code, w.Body.String())
			}
			if len(rec.calls) != 1 || rec.calls[0] != tc.want {
				t.Errorf("stream=%v question=%q: retrieval seeded with %q; want [%q]", stream, tc.question, rec.calls, tc.want)
			}
		}
	}
}
