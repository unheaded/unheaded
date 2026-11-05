// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package scriptlib

import (
	"regexp"
	"strings"
	"testing"
)

// The 2026-11-04 scan's open-webui row describes a digest the tree has since
// left, so the record must name the commit that moved it.
func TestImageScanRecordsTheOpenWebUIPinMove(t *testing.T) {
	const scanned = "sha256:4860ba00"
	compose := moveoutRepoFile(t, "deploy/sophia-eye/docker-compose.yml")
	if strings.Contains(compose, scanned) {
		t.Fatalf("the tree still pins %s; the record below would be false", scanned)
	}
	scan := moveoutRepoFile(t, "docs/security/image-scan-2026-11-04.md")
	if !regexp.MustCompile("(?m)^.*open-webui.*moved.*`f11da98d`").MatchString(scan) {
		t.Error("image-scan-2026-11-04.md does not say f11da98d moved the open-webui pin")
	}
}
