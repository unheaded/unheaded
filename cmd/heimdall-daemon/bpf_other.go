// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux

package main

import "errors"

type bpfHooks struct{ Verifier map[string]int }

func loadHooks(HookConfig) (*bpfHooks, []error) {
	return nil, []error{errors.New("BPF hooks need Linux")}
}

func (h *bpfHooks) Events() <-chan hookEvent        { return nil }
func (h *bpfHooks) HornCounters() map[string]uint64 { return map[string]uint64{} }
func (h *bpfHooks) Close()                          {}
