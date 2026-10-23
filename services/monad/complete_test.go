// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package monad

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// An operation is in s.operations from the moment it starts, and readers
// (Stats, GetOperation, ListOperations and the JSON encode of what they
// return) see it while it runs. Its outcome used to be written without the
// lock: run with -race, this test fails on the old code.
func TestOperationCompletion_NoRaceWithReaders(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil)
	svc.RegisterQuery("slow", func(context.Context, interface{}) (interface{}, error) {
		time.Sleep(time.Millisecond)
		return "done", nil
	})
	svc.RegisterEffect("slow", func(context.Context, interface{}) error {
		time.Sleep(time.Millisecond)
		return nil
	})

	stop := make(chan struct{})
	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = svc.Stats()
			ops := svc.ListOperations(0)
			if _, err := json.Marshal(ops); err != nil {
				t.Error(err)
			}
			if len(ops) > 0 {
				if op, ok := svc.GetOperation(ops[0].ID); ok {
					_, _ = json.Marshal(op)
				}
			}
		}
	}()

	var writers sync.WaitGroup
	for i := 0; i < 8; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for j := 0; j < 10; j++ {
				svc.Query(context.Background(), "slow", nil)
				svc.Effect(context.Background(), "slow", nil)
			}
		}()
	}
	writers.Wait()
	close(stop)
	readers.Wait()

	if got := svc.Stats()["completed_operations"]; got != 160 {
		t.Errorf("completed_operations = %v, want 160", got)
	}
}

// The monad.operations event for a mutation carries how long it took. It
// was published before the deferred completion set the duration, so it said
// 0 for every operation.
func TestMutateEvent_CarriesDuration(t *testing.T) {
	payloads := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/subscribe"):
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"subscriber": map[string]interface{}{
				"subscriber_id": "s", "topic": "monad.operations", "status": "approved",
			}})
		case strings.Contains(r.URL.Path, "/publish"):
			var body struct {
				Payload string `json:"payload"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			payloads <- body.Payload
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := newTestServiceWithWotan(server)
	if _, err := svc.wotan.Subscribe(context.Background(), "monad.operations", "monad-service"); err != nil {
		t.Fatal(err)
	}
	svc.RegisterMutation("slow", func(context.Context, interface{}) (interface{}, []StateChange, error) {
		time.Sleep(5 * time.Millisecond)
		return "ok", nil, nil
	})
	if res := svc.Mutate(context.Background(), "slow", nil); res.IsErr() {
		t.Fatal(res.Error())
	}

	select {
	case p := <-payloads:
		var event struct {
			Status   string `json:"status"`
			Duration int64  `json:"duration"`
		}
		if err := json.Unmarshal([]byte(p), &event); err != nil {
			t.Fatalf("event %q: %v", p, err)
		}
		if event.Status != string(OpStatusCompleted) || event.Duration < 5 {
			t.Errorf("event = %+v, want status completed and duration >= 5 (ms)", event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no operation event published")
	}
}
