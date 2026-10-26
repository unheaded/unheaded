// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package sword

import (
	"encoding/json"
	"testing"
)

// FuzzDeployRequest: POST /api/v1/deployments bodies. Decode plus validate
// must never panic; a nil config without an error is a bug.
func FuzzDeployRequest(f *testing.F) {
	f.Add([]byte(`{"service":"steel-echo","version":"v2","replicas":3,"strategy":{"type":"rolling","max_surge":1,"max_unavailable":0},"health":{"port":8080,"path":"/health"},"proxy":{"listen":"tcp:127.0.0.1:26601","connect":"tcp:127.0.0.1:8080"},"labels":{"chaos":"allowed"}}`))
	f.Add([]byte(`{"service":"steel-echo","version":"v4","replicas":3,"strategy":{"type":"canary","steps":[{"weight":33,"pause":"60s","analysis":true},{"weight":100}],"analysis":{"error_rate_threshold":0.01,"min_sample_size":100,"interval":"30s"}},"health":{"port":8080,"path":"/health"}}`))
	f.Add([]byte(`{"service":"a b","version":"../v","replicas":-1,"strategy":{"type":"nope","steps":[{"weight":200}]}}`))
	f.Add([]byte(`{"strategy":{"type":"canary","analysis":{"interval":"-1s","latency_p99_threshold":"1e9h"}}}`))
	f.Add([]byte(``))
	f.Fuzz(func(t *testing.T, b []byte) {
		var req DeployRequest
		if err := json.Unmarshal(b, &req); err != nil {
			return
		}
		for _, have := range []bool{true, false} {
			cfg, err := req.validate(have)
			if err == nil && cfg == nil {
				t.Fatal("validate returned neither a config nor an error")
			}
		}
	})
}
