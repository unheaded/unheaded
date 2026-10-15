// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package database_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"

	"unheaded/pkg/database"
	"unheaded/pkg/health"
)

func jsonTags(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		out = append(out, t.Field(i).Tag.Get("json")+" "+t.Field(i).Type.String())
	}
	sort.Strings(out)
	return out
}

// The bridge reads what Akira publishes. The two structs live in different
// packages; this pins their JSON shape together (the bridge was written for
// a topic and shape Akira never published, ERDA W-12).
func TestHealthReportContract(t *testing.T) {
	if a, b := jsonTags(reflect.TypeOf(health.HealthReport{})), jsonTags(reflect.TypeOf(database.HealthReport{})); !reflect.DeepEqual(a, b) {
		t.Fatalf("shapes differ:\n akira  %v\n bridge %v", a, b)
	}
	in := health.HealthReport{Service: "wotan", Reporter: "west", Healthy: false, Latency: 1500 * time.Millisecond,
		Error: "connection refused", Timestamp: time.Date(2026, 10, 15, 1, 2, 3, 0, time.UTC)}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out database.HealthReport
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Service != in.Service || out.Reporter != in.Reporter || out.Healthy != in.Healthy ||
		out.Latency != in.Latency || out.Error != in.Error || !out.Timestamp.Equal(in.Timestamp) {
		t.Fatalf("round trip: %+v", out)
	}
}
