// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/pkg/auth"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/zhenagent"
)

// optInStore is a board store with kanban_zhen_optin behind it.
type optInStore struct {
	stubStore
	rows map[string]ZhenOptIn
}

func (o *optInStore) GetTask(id string) (*Task, error) {
	for i := range o.tasks {
		if o.tasks[i].ID == id {
			return &o.tasks[i], nil
		}
	}
	return nil, ErrStoreNotFound
}

func (o *optInStore) ZhenOptInFor(_ context.Context, id string) (*ZhenOptIn, error) {
	if r, ok := o.rows[id]; ok {
		return &r, nil
	}
	return nil, nil
}

func (o *optInStore) SetZhenOptIn(_ context.Context, id, sum, by string) error {
	o.rows[id] = ZhenOptIn{DescriptionSHA256: sum, By: by, At: time.Now()}
	return nil
}

func (o *optInStore) ClearZhenOptIn(_ context.Context, id string) error {
	delete(o.rows, id)
	return nil
}

type fixedID struct{ id *auth.Identity }

func (f fixedID) Authenticate(context.Context, *http.Request) (*auth.Identity, error) {
	return f.id, nil
}

func optInServer() (*Server, *optInStore) {
	st := &optInStore{rows: map[string]ZhenOptIn{}}
	st.tasks = []Task{
		{ID: "card-1", Title: "t", Description: "do the thing", Status: "todo", Type: "task", Owner: zhenagent.Owner},
		{ID: "card-2", Title: "t", Status: "todo", Type: "task", Owner: "stevie"},
	}
	return &Server{store: st, ctx: context.Background()}, st
}

func optInCall(s *Server, id *auth.Identity, method, card string) *httptest.ResponseRecorder {
	var h http.Handler = http.HandlerFunc(s.handleTaskByID)
	if id != nil {
		h = auth.Middleware(fixedID{id})(h)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/tasks/"+card+"/zhen-opt-in", nil))
	return rec
}

// H4: an operator's login hands a card over, and the record says who and what.
func TestZhenOptIn_AnOperatorHandsACardOver(t *testing.T) {
	s, st := optInServer()
	for _, role := range []string{auth.RoleOperator, auth.RoleAdmin} {
		rec := optInCall(s, &auth.Identity{Subject: "stevie", Roles: []string{role}}, http.MethodPost, "card-1")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", role, rec.Code, rec.Body)
		}
	}
	row := st.rows["card-1"]
	if row.By != "stevie" || row.DescriptionSHA256 != zhenagent.DescriptionSHA256("do the thing") {
		t.Fatalf("row = %+v", row)
	}
	if rec := optInCall(s, &auth.Identity{Subject: "stevie", Roles: []string{auth.RoleOperator}}, http.MethodDelete, "card-1"); rec.Code != http.StatusNoContent {
		t.Fatalf("withdraw: %d", rec.Code)
	}
	if _, ok := st.rows["card-1"]; ok {
		t.Fatal("the opt-in survived its withdrawal")
	}
}

// Only a person with an operator's role: no anonymous caller, no read-only
// login, and no service, whoever that service says it acts for.
func TestZhenOptIn_NobodyElseCan(t *testing.T) {
	for name, id := range map[string]*auth.Identity{
		"no identity": nil,
		"viewer":      {Subject: "watcher", Roles: []string{auth.RoleViewer}},
		"observer":    {Subject: "watcher", Roles: []string{auth.RoleObserver}},
		"service":     {Subject: "zhen-agentd", Roles: []string{auth.RoleService}},
		"service and operator": {Subject: "dashboard-backend", Roles: []string{auth.RoleService, auth.RoleOperator},
			Extra: map[string]string{"act": "stevie"}},
	} {
		s, st := optInServer()
		for _, method := range []string{http.MethodPost, http.MethodDelete} {
			if rec := optInCall(s, id, method, "card-1"); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s: %d, want 403", name, method, rec.Code)
			}
		}
		if len(st.rows) != 0 {
			t.Errorf("%s: wrote %v", name, st.rows)
		}
	}
}

func TestZhenOptIn_OnlyZhensCardsAndOnlyRealOnes(t *testing.T) {
	s, st := optInServer()
	op := &auth.Identity{Subject: "stevie", Roles: []string{auth.RoleOperator}}
	if rec := optInCall(s, op, http.MethodPost, "card-2"); rec.Code != http.StatusConflict {
		t.Errorf("a card owned by someone else: %d, want 409", rec.Code)
	}
	if rec := optInCall(s, op, http.MethodPost, "no-such"); rec.Code != http.StatusNotFound {
		t.Errorf("no such card: %d, want 404", rec.Code)
	}
	if len(st.rows) != 0 {
		t.Fatalf("wrote %v", st.rows)
	}
}

// A store without the table (SQLite, a dev board) has no opt-in to give.
func TestZhenOptIn_NeedsTheWell(t *testing.T) {
	s := &Server{store: &stubStore{tasks: []Task{{ID: "card-1", Status: "todo", Owner: zhenagent.Owner}}}, ctx: context.Background()}
	rec := optInCall(s, &auth.Identity{Subject: "stevie", Roles: []string{auth.RoleOperator}}, http.MethodPost, "card-1")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("%d, want 503", rec.Code)
	}
}

// The card modal shows the opt-in, and whether it still covers the card.
func TestZhenOptIn_TheModalSeesIt(t *testing.T) {
	s, st := optInServer()
	st.rows["card-1"] = ZhenOptIn{DescriptionSHA256: zhenagent.DescriptionSHA256("something older"), By: "stevie", At: time.Now()}
	rec := httptest.NewRecorder()
	s.handleTaskByID(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/card-1", nil))
	var d struct {
		ZhenOptIn *ZhenOptIn `json:"zhen_opt_in"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.ZhenOptIn == nil || d.ZhenOptIn.By != "stevie" || d.ZhenOptIn.Current {
		t.Fatalf("detail = %+v", d.ZhenOptIn)
	}
}

// Wotan reaches a card, never the opt-in: nothing a tasks.* message says about
// it lands anywhere, and a forged edit leaves a record that no longer matches.
func TestZhenOptIn_NoWotanMessageSetsIt(t *testing.T) {
	st := &optInStore{rows: map[string]ZhenOptIn{}}
	st.tasks = []Task{{ID: "card-1", Title: "t", Status: "todo", Owner: zhenagent.Owner}}
	tm := &TaskManager{store: st, tasks: map[string]*Task{}, broadcast: func(string, interface{}) {}}
	payload := `{"id":"card-1","title":"t","status":"todo","owner":"zhen","zhen_opt_in":{"by":"stevie","current":true},"opt_in_sha256":"x"}`
	if err := tm.handleMessage(&wotanClient.Message{Topic: TopicTasksUpdated, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if len(st.rows) != 0 {
		t.Fatalf("a Wotan message wrote the opt-in: %v", st.rows)
	}
}
