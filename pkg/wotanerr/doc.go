// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package wotanerr is the structured error code of
// draft-bellis-unheaded-wotan-memory-03 §Error Code Taxonomy: a 32-bit
// value of severity (bits 31-29), origin (28-24), category (23-16) and
// detail (7-0). The tables here are checked against the draft and its IETF
// twin by wotanerr_test.go.
//
// The code is wire-visible only at Wotan's API boundary: an HTTP error
// response carries it in the X-Wotan-Error header and a gRPC error in the
// x-wotan-error trailer, both as "0x" and eight upper-case hex digits.
// pkg/wotan-client decodes either into a *Code reachable with errors.As.
// Wotan's internal code keeps plain Go errors (S-TRUE-WIRE Steps 143-145).
//
// A code says which subsystem refused and why, never with what: no key,
// address, topic name or payload is ever encoded in it (draft §Error Code
// Information Leakage).
package wotanerr
