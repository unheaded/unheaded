// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package secrets_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"unheaded/pkg/metrics"
	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/store"
)

// The Manager put four unheaded_secrets_* families into the default
// registry, but no binary builds a Manager, so no scrape ever carried them
// (S-WELL-SPRING Step 191, M-086). A served-nowhere metric reads as
// "healthy, zero" on a dashboard; re-add one the day a served binary uses
// the Manager. Until then, using it registers nothing.
func TestManager_RegistersNoMetrics(t *testing.T) {
	mgr, err := secrets.NewManager(secrets.ManagerConfig{Store: store.NewMemoryStore(store.MemoryStoreConfig{})})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	ctx := context.Background()
	if err := mgr.Set(ctx, "app/x", &secrets.Secret{Type: secrets.SecretTypeStatic, Data: map[string][]byte{"v": []byte("1")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Get(ctx, "app/x"); err != nil {
		t.Fatal(err)
	}
	_, _ = mgr.Get(ctx, "app/missing")

	var buf bytes.Buffer
	if err := metrics.DefaultRegistry.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "unheaded_secrets_") {
		t.Fatalf("default registry carries unheaded_secrets_* series:\n%s", buf.String())
	}
}
