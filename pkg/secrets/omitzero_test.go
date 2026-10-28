// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package secrets

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestSecretZeroTimesAreOmitted pins S-TRUE-WIRE Step 371: `omitempty` never
// omits a struct such as time.Time, so a secret that never expires was
// serialised with "expires_at":"0001-01-01T00:00:00Z", which a reader cannot
// tell from a real date. `omitzero` (Go 1.24+) omits the zero time.
func TestSecretZeroTimesAreOmitted(t *testing.T) {
	b, err := json.Marshal(Secret{CreatedAt: time.Unix(1700000000, 0).UTC(), UpdatedAt: time.Unix(1700000000, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, key := range []string{`"expires_at"`, `"rotated_at"`} {
		if strings.Contains(got, key) {
			t.Errorf("zero %s serialised: %s", key, got)
		}
	}
	if strings.Contains(got, "0001-01-01") {
		t.Errorf("a zero time reached the JSON: %s", got)
	}

	exp := time.Unix(1800000000, 0).UTC()
	b, err = json.Marshal(Secret{CreatedAt: exp, UpdatedAt: exp, ExpiresAt: exp})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"expires_at":"`+exp.Format(time.RFC3339)+`"`) {
		t.Errorf("a set expiry was dropped: %s", b)
	}
}
