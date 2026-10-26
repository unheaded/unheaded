// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

// FuzzContainerSpec: the desired spec arrives from the network (PUT
// /api/v1/containers/{id}, 1 MiB cap). Decoding plus validate must never
// panic, whatever the bytes.
func FuzzContainerSpec(f *testing.F) {
	f.Add([]byte(`{"id":"web","name":"web","image":"steel-echo-v1","health":{"port":8080,"path":"/health"},"proxy":{"listen":"tcp:127.0.0.1:26601","connect":"tcp:127.0.0.1:8080"},"labels":{"chaos":"allowed"}}`))
	f.Add([]byte(`{"name":"../x","health":{"port":70000,"path":"/../"},"proxy":{"listen":"tcp:0.0.0.0:22","connect":"udp:1.2.3.4:1"}}`))
	f.Add([]byte(`{"cpu":-1,"memory":1e400}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(``))
	f.Fuzz(func(t *testing.T, b []byte) {
		var spec ContainerSpec
		if err := json.NewDecoder(io.LimitReader(bytes.NewReader(b), 1*1024*1024)).Decode(&spec); err != nil {
			return
		}
		_ = spec.validate(nil)
		_ = spec.validate([]string{"10.10.20.1"})
	})
}
