// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package healthreport

import (
	"errors"
	"time"

	"unheaded/pkg/metrics"
)

// refusedQuiet is how long a refusal reason stays quiet after its log line.
const refusedQuiet = time.Hour

// Refusals counts the messages a consumer of Topic refused, by a reason from
// a fixed set, and logs each reason at most once per refusedQuiet: a flood of
// reports costs one line an hour. Used from one goroutine.
type Refusals struct {
	*metrics.CounterVec
	warn   func(reason, sender string)
	now    func() time.Time
	logged map[string]time.Time // reason -> last log line
}

// RefusedMessage is the text of a refusal's log line.
const RefusedMessage = "health report refused; further refusals this hour are only counted"

// NewRefusals makes the counter name, with a "reason" label, and registers it
// on reg. warn writes the log line, in the caller's logger, with
// RefusedMessage. A failed registration comes back with a working Refusals.
func NewRefusals(name, help string, reg *metrics.Registry, warn func(reason, sender string)) (*Refusals, error) {
	cv := metrics.NewCounterVec(name, help, nil, []string{"reason"})
	r := &Refusals{CounterVec: cv, warn: warn, now: time.Now, logged: map[string]time.Time{}}
	return r, reg.Register(cv)
}

// Refuse counts one refusal. The sender is the verified subject, so the log
// line names who did it.
func (r *Refusals) Refuse(reason, sender string) {
	r.WithLabelValues(reason).Inc()
	now := r.now()
	if at, ok := r.logged[reason]; ok && now.Sub(at) < refusedQuiet {
		return
	}
	r.logged[reason] = now
	r.warn(reason, sender)
}

// Reason maps Admit's error to its metric label.
func Reason(err error) string {
	switch {
	case errors.Is(err, ErrUnverified):
		return "unverified"
	case errors.Is(err, ErrNotOwnService):
		return "not_own_service"
	case errors.Is(err, ErrBadNode):
		return "bad_node"
	default:
		return "malformed"
	}
}
