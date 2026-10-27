// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"unheaded/pkg/champion"
)

// The scheduler's HTTP surface (S-CHAMPION-HARDENING Step 165). The web
// UI's `schedule`, `unschedule`, `list schedules` and `emergency stop`
// commands call these on loopback with the daemon's auth.
//
//	POST /api/v1/scheduler/add    {name, runbook?, every, enabled?, justification, emitted_by?}
//	POST /api/v1/scheduler/remove {name, justification, emitted_by?}
//	POST /api/v1/scheduler/pause  {paused, justification, emitted_by?}
//	GET|POST /api/v1/scheduler/list
//
// add, remove and pause are the mutating tool schedule_update and take
// exactly /api/v1/tool/exec's path (justification required, an agent
// loop's direct-user stripped, its call queued or denied), so a model
// cannot schedule anything. list is read-only. Without -scheduler they
// answer 503.

// schedulerOps are the args each change op takes from the body.
var schedulerOps = map[string][]string{
	"add":    {"name", "runbook", "every", "enabled"},
	"remove": {"name"},
	"pause":  {"paused"},
}

// attachScheduler serves sch and lets every Champion of the pool apply
// schedule_update to it.
func (s *server) attachScheduler(sch *scheduler) {
	s.sched = sch
	s.pool.setScheduler(sch)
}

// registerScheduler adds the scheduler routes to mux.
func (s *server) registerScheduler(mux *http.ServeMux) {
	mux.Handle("/api/v1/scheduler/list", instrument("/api/v1/scheduler/list", http.HandlerFunc(s.handleSchedulerList)))
	for op := range schedulerOps {
		path := "/api/v1/scheduler/" + op
		mux.Handle(path, instrument(path, s.handleSchedulerChange(op)))
	}
}

// schedulerMux is the scheduler routes alone (tests).
func (s *server) schedulerMux() *http.ServeMux {
	mux := http.NewServeMux()
	s.registerScheduler(mux)
	return mux
}

func schedulerOff(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, toolExecResponse{Status: "error",
		Reason: "scheduler not running: start zhen-agentd with -scheduler -schedules <file>"})
}

func (s *server) handleSchedulerList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.sched == nil {
		schedulerOff(w)
		return
	}
	writeJSON(w, http.StatusOK, s.sched.view())
}

func (s *server) handleSchedulerChange(op string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if s.sched == nil {
			schedulerOff(w)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "read body: %v", err)
			return
		}
		defer r.Body.Close()
		var in map[string]json.RawMessage
		if err := json.Unmarshal(body, &in); err != nil {
			writeJSONError(w, http.StatusBadRequest, "decode body: %v", err)
			return
		}
		req := toolExecRequest{Tool: "schedule_update", Args: map[string]any{"op": op}}
		for k, raw := range in {
			switch k {
			case "justification":
				var refs []champion.Reference
				if err := json.Unmarshal(raw, &refs); err != nil {
					writeJSONError(w, http.StatusBadRequest, "justification: %v", err)
					return
				}
				req.Justification = refs
			case "emitted_by", "session_id":
				var v string
				if err := json.Unmarshal(raw, &v); err != nil {
					writeJSONError(w, http.StatusBadRequest, "%s: %v", k, err)
					return
				}
				if k == "emitted_by" {
					req.EmittedBy = strings.TrimSpace(v)
				} else {
					req.SessionID = v
				}
			default:
				allowed := false
				for _, a := range schedulerOps[op] {
					allowed = allowed || a == k
				}
				if !allowed {
					writeJSONError(w, http.StatusBadRequest, "%s: unknown field %q", op, k)
					return
				}
				var v any
				if err := json.Unmarshal(raw, &v); err != nil {
					writeJSONError(w, http.StatusBadRequest, "%s: %v", k, err)
					return
				}
				req.Args[k] = v
			}
		}
		s.execToolRequest(w, r, req)
	}
}
