// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"unheaded/pkg/compliance/crosswalk"
	wotanclient "unheaded/pkg/wotan-client"
)

// wotanDisplayName is the name the agent joins Wotan's topics with; it must
// be on configs/wotan.yaml topics.auto_approve or the publish stays pending.
const wotanDisplayName = "unheaded-baseline"

// deviationTopicPrefix + host is the topic one run publishes on.
const deviationTopicPrefix = "compliance.deviation."

// deviationMessage is the Wotan payload: metadata only. The observed values
// (check details) and what enforcing would do stay in report.json, which
// report_sha256 identifies.
type deviationMessage struct {
	Host         string          `json:"host"`
	Mode         string          `json:"mode"`
	Deviations   []deviationItem `json:"deviations"`
	ReportSHA256 string          `json:"report_sha256"`
	TS           int64           `json:"ts"` // Unix milliseconds of the observation
}

// deviationItem names a failed check. The baseline has no severity field,
// so none is invented here; Kind lets a consumer group by collector.
type deviationItem struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// wotanAddrFrom resolves the Wotan address: the flag wins, then $WOTAN_ADDR,
// else off.
func wotanAddrFrom(flagVal string, getenv func(string) string) string {
	if flagVal != "" {
		return flagVal
	}
	return getenv("WOTAN_ADDR")
}

// publishDeviations sends one message on compliance.deviation.<host> over
// Wotan's HTTP control plane. reportBody is report.json as written.
func publishDeviations(ctx context.Context, addr string, rep *report, reportBody []byte) error {
	sum := sha256.Sum256(reportBody)
	msg := deviationMessage{Host: rep.Host, Mode: rep.ModeEffective,
		ReportSHA256: hex.EncodeToString(sum[:]), TS: rep.ObservedAt.UnixMilli()}
	for _, c := range rep.Checks {
		if c.Verdict != string(crosswalk.VerdictFail) {
			continue
		}
		kind, _, _ := strings.Cut(c.Key, ":")
		msg.Deviations = append(msg.Deviations, deviationItem{ID: c.Key, Kind: kind})
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	client, err := wotanclient.NewClient(addr)
	if err != nil {
		return err
	}
	defer client.Close() // #nosec G104 -- one-shot process; nothing buffered survives exit anyway
	topic := deviationTopicPrefix + rep.Host
	sub, err := client.Subscribe(ctx, topic, wotanDisplayName)
	if err != nil {
		return fmt.Errorf("join %s: %w", topic, err)
	}
	if sub.Status != "approved" {
		return fmt.Errorf("join %s: subscription %s (add %q to topics.auto_approve)", topic, sub.Status, wotanDisplayName)
	}
	if err := client.Publish(ctx, topic, payload); err != nil {
		return fmt.Errorf("publish %s: %w", topic, err)
	}
	return nil
}
