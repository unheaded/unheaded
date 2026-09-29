// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"testing"

	ebpfPkg "unheaded/cmd/dashboard-backend/internal/ebpf"
	wotanClient "unheaded/pkg/wotan-client"
)

// The event streamer and the eBPF ingestor must not share a topic: each
// event would reach the page twice.
func TestEventTopicsDoNotOverlapIngestor(t *testing.T) {
	for _, et := range getEventTopics() {
		for _, it := range ebpfPkg.Topics {
			if wotanClient.MatchTopic(et, it) {
				t.Errorf("event topic %q also covers ingestor topic %q", et, it)
			}
		}
	}
}
