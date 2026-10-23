// SPDX-License-Identifier: GPL-3.0-or-later
package signing

import (
	"encoding/hex"
	"testing"
)

// goldenCanonical pins the bytes a topic signature covers
// ("wotan/topic-sig/v2"). It was captured from this package before the
// canonical form moved to pkg/wotansig (ADR-105), and computed
// independently as SHA-256 over the documented layout. If it changes,
// every signature made before the change stops verifying.
const goldenCanonical = "cba2c4b17a7c4b2bb29c36b2bd58d6d460070835c3c4dc8b1360ef42d6182b76"

func TestGoldenCanonical(t *testing.T) {
	got := hex.EncodeToString(canonicalize("config.x", "sender-1", 1700000000000, []byte("payload")))
	if got != goldenCanonical {
		t.Fatalf("canonical form changed:\n got  %s\n want %s", got, goldenCanonical)
	}
}
