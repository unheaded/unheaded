// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// wellTestDB opens WELL_TEST_DSN: a throwaway unheaded_ops (migration 039)
// as ops_writer, from scripts/well/test-deploy-journal.sh. Never The Well.
// The tables are append-only, so every test uses ids of its own.
func wellTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("WELL_TEST_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_DSN not set (scripts/well/test-deploy-journal.sh sets it)")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// uniq is a per-run suffix: the tables keep every row ever written.
func uniq() string { return fmt.Sprintf("%d", time.Now().UnixNano()) }

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatalf("unmarshal %s: %v", a, err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	return reflect.DeepEqual(x, y)
}

func TestDeployStoreCheckSchema(t *testing.T) {
	s := NewDeployStore(wellTestDB(t))
	if err := s.CheckSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDeployStore(t *testing.T) {
	db := wellTestDB(t)
	s := NewDeployStore(db)
	ctx := context.Background()
	u := uniq()
	svc := "svc-" + u[len(u)-8:]
	t0 := time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)
	spec := json.RawMessage(`{"strategy":{"type":"rolling","max_surge":1},"health":{"port":8080,"path":"/health"},"labels":{"tier":"web"}}`)
	rec := func(id, version string, at time.Time) DeployRecord {
		return DeployRecord{
			ID: id, Service: svc, Version: version, Strategy: "rolling", Replicas: 2,
			Spec: spec, Host: "west", CreatedAt: at, Status: "pending",
			State: json.RawMessage(fmt.Sprintf(`{"id":%q,"status":"pending"}`, id)),
		}
	}
	idA, idB := "da"+u, "db"+u

	t.Run("record and get round-trip the spec", func(t *testing.T) {
		if err := s.Record(ctx, rec(idA, "v1", t0)); err != nil {
			t.Fatal(err)
		}
		got, err := s.Get(ctx, idA)
		if err != nil {
			t.Fatal(err)
		}
		if !jsonEqual(t, got.Spec, spec) {
			t.Errorf("spec: got %s want %s", got.Spec, spec)
		}
		if got.Status != "pending" || got.Service != svc || got.Version != "v1" || got.Replicas != 2 || got.Host != "west" {
			t.Errorf("record: %+v", got)
		}
		if !got.CreatedAt.Equal(t0) {
			t.Errorf("created_at %v want %v", got.CreatedAt, t0)
		}
	})

	t.Run("a second record with the same id is an error", func(t *testing.T) {
		err := s.Record(ctx, rec(idA, "v1", t0))
		if !errors.Is(err, ErrDuplicate) {
			t.Fatalf("want ErrDuplicate, got %v", err)
		}
	})

	t.Run("transition appends; get reads the newest", func(t *testing.T) {
		for _, step := range []struct{ from, to string }{{"pending", "in_progress"}, {"in_progress", "succeeded"}} {
			state := json.RawMessage(fmt.Sprintf(`{"id":%q,"status":%q}`, idA, step.to))
			if err := s.Transition(ctx, idA, step.from, step.to, "", state); err != nil {
				t.Fatal(err)
			}
		}
		got, err := s.Get(ctx, idA)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "succeeded" || !jsonEqual(t, got.State, []byte(fmt.Sprintf(`{"id":%q,"status":"succeeded"}`, idA))) {
			t.Errorf("after transitions: status %s state %s", got.Status, got.State)
		}
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM deploy_history_transitions WHERE deployment_id = $1`, idA).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 3 {
			t.Errorf("transitions: %d rows, want 3 (record + 2)", n)
		}
	})

	t.Run("list orders newest first and filters by service", func(t *testing.T) {
		if err := s.Record(ctx, rec(idB, "v2", t0.Add(time.Minute))); err != nil {
			t.Fatal(err)
		}
		got, err := s.List(ctx, "", svc, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].ID != idB || got[1].ID != idA {
			t.Fatalf("list: %+v", got)
		}
		if got[1].Status != "succeeded" {
			t.Errorf("list reads the newest status: %s", got[1].Status)
		}
		one, err := s.List(ctx, "west", svc, 1)
		if err != nil || len(one) != 1 || one[0].ID != idB {
			t.Fatalf("limit 1: %+v %v", one, err)
		}
		all, err := s.List(ctx, "", "", 1000)
		if err != nil || len(all) < 2 {
			t.Fatalf("all services: %d %v", len(all), err)
		}
		east, err := s.List(ctx, "east", svc, 10)
		if err != nil || len(east) != 0 {
			t.Fatalf("another host's records: %d %v", len(east), err)
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		if _, err := s.Get(ctx, "dnope"+u); !errors.Is(err, ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
		err := s.Transition(ctx, "dnope"+u, "pending", "failed", "", json.RawMessage(`{}`))
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("transition of an unknown id: want ErrNotFound, got %v", err)
		}
	})

	t.Run("the database refuses what Sword never writes", func(t *testing.T) {
		bad := rec("dbad"+u, "v1", t0)
		bad.Strategy = "yolo"
		if err := s.Record(ctx, bad); err == nil {
			t.Fatal("unknown strategy accepted")
		}
		if err := s.Transition(ctx, idA, "succeeded", "exploded", "", json.RawMessage(`{}`)); err == nil {
			t.Fatal("unknown status accepted")
		}
		if _, err := db.ExecContext(ctx, `UPDATE deploy_history SET version = 'v9' WHERE deployment_id = $1`, idA); err == nil {
			t.Fatal("UPDATE on deploy_history allowed")
		}
	})
}

// The validation that runs before any SQL needs no database.
func TestDeployRecordValidate(t *testing.T) {
	ok := DeployRecord{ID: "d1", Service: "s", Version: "v1", Strategy: "rolling", Replicas: 1,
		Spec: json.RawMessage(`{}`), State: json.RawMessage(`{}`), Host: "west", Status: "pending", CreatedAt: time.Now()}
	if err := ok.validate(); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*DeployRecord){
		"no id":        func(r *DeployRecord) { r.ID = "" },
		"no host":      func(r *DeployRecord) { r.Host = "" },
		"spec not obj": func(r *DeployRecord) { r.Spec = json.RawMessage(`[1]`) },
		"state empty":  func(r *DeployRecord) { r.State = nil },
		"no time":      func(r *DeployRecord) { r.CreatedAt = time.Time{} },
	} {
		r := ok
		mut(&r)
		if err := r.validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
