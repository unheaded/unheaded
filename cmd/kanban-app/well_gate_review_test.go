// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotan-client/mock"
)

// stubWellDown makes connectTaskStore fail until release is closed, then
// hand out store; it counts the calls.
func stubWellDown(t *testing.T, store TaskStore) (release func(), calls func() int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	up := make(chan struct{})
	prev, prevWait := connectTaskStore, wellWait
	connectTaskStore = func(context.Context) (TaskStore, error) {
		mu.Lock()
		n++
		mu.Unlock()
		select {
		case <-up:
			return store, nil
		default:
			return nil, errors.New("connection refused")
		}
	}
	wellWait = [2]time.Duration{time.Millisecond, 2 * time.Millisecond}
	t.Cleanup(func() { connectTaskStore, wellWait = prev, prevWait })
	var once sync.Once
	return func() { once.Do(func() { close(up) }) }, func() int { mu.Lock(); defer mu.Unlock(); return n }
}

// REV-5: when Wotan is late the TaskManager does not start and the server
// falls back to standalone; it keeps the one Well gate main opened instead
// of opening a second (whose pool and retry loop leaked).
func TestWotanLateKeepsOneWellGate(t *testing.T) {
	release, _ := stubWellDown(t, &stubStore{})
	defer release()
	gate := newWellGate()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	openWell(ctx, gate)

	late := mock.NewMockClient(mock.WithSubscribeError(errors.New("wotan: connection refused")))
	s, err := serverWithWotan(Config{Port: "0"}, late, gate, 50*time.Millisecond)
	if err == nil {
		t.Fatal("a Wotan that refuses every subscribe: no error, want the fallback's")
	}
	defer s.cancel()
	if s.well != gate {
		t.Fatal("the fallback server opened a second Well gate")
	}
	if g, ok := s.store.(*wellGate); !ok || g != gate {
		t.Fatalf("the fallback server's store is %T, want main's gate", s.store)
	}
}

// failNextStore refuses writes with errWellUnavailable-like errors while
// down is set, and records the saved tasks.
type recordingStore struct {
	stubStore
	mu    sync.Mutex
	saved map[string]Task
}

func (r *recordingStore) SaveTask(t *Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saved == nil {
		r.saved = map[string]Task{}
	}
	r.saved[t.ID] = *t
	return nil
}

func (r *recordingStore) DeleteTask(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.saved, id)
	return nil
}

func (r *recordingStore) has(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.saved[id]
	return ok
}

func taskMsg(t *testing.T, topic, id string) *wotanClient.Message {
	t.Helper()
	b, err := json.Marshal(Task{ID: id, Title: "inbound " + id, Status: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	return &wotanClient.Message{Topic: topic, Payload: string(b)}
}

// REV-5: a card that arrives on the bus while the Well is down is not put
// on the board (it was shown and never saved); it waits in a bounded queue
// that /health shows, and is saved and shown when the Well answers.
func TestInboundCardWhileWellDownIsQueued(t *testing.T) {
	store := &recordingStore{}
	release, _ := stubWellDown(t, store)
	gate := newWellGate()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	openWell(ctx, gate)

	var bmu sync.Mutex
	var broadcasts []string
	tm, err := NewTaskManager(mock.NewMockClient(mock.WithAutoApprove()), func(kind string, _ interface{}) {
		bmu.Lock()
		broadcasts = append(broadcasts, kind)
		bmu.Unlock()
	}, gate)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tm.Close() }()
	if err := tm.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := tm.handleMessage(taskMsg(t, TopicTasksCreated, "card-1")); err != nil {
		t.Fatalf("handleMessage while the Well is down: %v", err)
	}
	if _, err := tm.GetTask("card-1"); err == nil {
		t.Fatal("a card the Well never saved is on the board")
	}
	bmu.Lock()
	if len(broadcasts) != 0 {
		t.Errorf("broadcast %v for an unsaved card", broadcasts)
	}
	bmu.Unlock()
	if n := tm.WellQueued(); n != 1 {
		t.Fatalf("WellQueued() = %d, want 1", n)
	}
	s := NewServerWithTaskManager(Config{Port: "0"}, tm, gate)
	s.well = gate
	defer s.cancel()
	w := httptest.NewRecorder()
	s.handleHealth(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["well"] != "waiting" || body["well_queued"] != float64(1) {
		t.Errorf("/health well = %v, well_queued = %v; want waiting, 1", body["well"], body["well_queued"])
	}

	release()
	deadline := time.Now().Add(5 * time.Second)
	for !store.has("card-1") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !store.has("card-1") {
		t.Fatal("the queued card was not saved when the Well answered")
	}
	for tm.WellQueued() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if _, err := tm.GetTask("card-1"); err != nil {
		t.Fatalf("the saved card is not on the board: %v", err)
	}
}

// The queue is bounded: past wellQueueMax an inbound card is refused and
// counted, never shown.
func TestInboundQueueBounded(t *testing.T) {
	release, _ := stubWellDown(t, &recordingStore{})
	defer release()
	gate := newWellGate()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	openWell(ctx, gate)
	tm, err := NewTaskManager(mock.NewMockClient(mock.WithAutoApprove()), func(string, interface{}) {}, gate)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tm.Close() }()
	for i := 0; i < wellQueueMax; i++ {
		if err := tm.handleMessage(taskMsg(t, TopicTasksCreated, "q-"+itoa(i))); err != nil {
			t.Fatal(err)
		}
	}
	if err := tm.handleMessage(taskMsg(t, TopicTasksCreated, "one-too-many")); !errors.Is(err, errWellUnavailable) {
		t.Fatalf("past the queue's bound: %v, want errWellUnavailable", err)
	}
	if n := tm.WellQueued(); n != wellQueueMax {
		t.Fatalf("WellQueued() = %d, want %d", n, wellQueueMax)
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}
