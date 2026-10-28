// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package doom

import (
	"strings"
	"testing"
)

// S-ASCEND-2 Step 169: the ROM HMAC key has no shipped default. It comes
// from the flag, else UPC_HMAC_KEY, else the command fails loudly.
func TestResolveHMACKey(t *testing.T) {
	cases := []struct {
		name, flag, env, want string
		wantErr               bool
	}{
		{"flag wins", "from-flag", "from-env", "from-flag", false},
		{"env when no flag", "", "from-env", "from-env", false},
		{"neither", "", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("UPC_HMAC_KEY", c.env)
			got, err := ResolveHMACKey(c.flag)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want an error, got key %q", got)
				}
				if !strings.Contains(err.Error(), "UPC_HMAC_KEY is required; credentials are not stored in this repo") {
					t.Fatalf("error does not say how to supply the key: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Fatalf("key = %q, want %q", got, c.want)
			}
		})
	}
}
