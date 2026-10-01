// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package well

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/compliance/remediation"
)

// Export writes The Well's profiles and host bindings back to the YAML
// fallback (COMPLIANCE-WELL step 98), so the fallback shows the state The
// Well last had rather than the original seed. profiles.yaml is
// regenerated whole; in each host file only the profile and mode lines
// change, so its comments and the applied history stay. Every result is
// parsed and validated before it is written. Returns the files changed.
func Export(ctx context.Context, s Store, repoDir string) ([]string, error) {
	if s.Source() != SourceWell {
		return nil, errors.New("export reads The Well; this store is the YAML fallback itself")
	}
	ps, err := s.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	bs, err := s.Bindings(ctx)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(repoDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	var changed []string
	write := func(p string, body []byte) error {
		old, err := root.ReadFile(p)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if bytes.Equal(old, body) {
			return nil
		}
		changed = append(changed, p)
		return root.WriteFile(p, body, 0o644) // #nosec G306 -- a tracked repo file, world-readable like the rest
	}

	var doc bytes.Buffer
	doc.WriteString(profilesHeader)
	body, err := yaml.Marshal(struct {
		Profiles []remediation.Profile `yaml:"profiles"`
	}{ps.Profiles})
	if err != nil {
		return nil, err
	}
	doc.Write(body)
	if _, err := remediation.ParseProfiles(doc.Bytes(), s.Model()); err != nil {
		return nil, fmt.Errorf("exported profiles do not parse: %w", err)
	}
	if err := write("compliance/profiles/profiles.yaml", doc.Bytes()); err != nil {
		return nil, err
	}

	for _, b := range bs {
		p := path.Join("compliance/hosts", b.Host+".yaml")
		old, err := root.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			old = []byte(fmt.Sprintf(hostHeader+"host: %s\nprofile: %s\nmode: %s\n", b.Host, b.Profile, b.Mode))
		} else if err != nil {
			return nil, err
		}
		out := setLine(setLine(old, "profile", b.Profile), "mode", b.Mode)
		if _, err := remediation.ParseBinding(out, ps); err != nil {
			return nil, fmt.Errorf("exported %s does not parse: %w", p, err)
		}
		if err := write(p, out); err != nil {
			return nil, err
		}
	}
	return changed, nil
}

const profilesHeader = `# Control profiles (ADR-099): which fixes a host gets. Profiles switch
# remediation only; every check still runs and every failure is reported.
# Source of truth is The Well (unheaded_config.compliance_profiles); this
# file is the read-only fallback, written by
# ` + "`compliance-evidence profile export`" + ` from The Well.
`

const hostHeader = `# Host binding (ADR-098 mode, ADR-099 profile). Source of truth is The Well
# (unheaded_config.compliance_host_bindings, with history); this file is the
# read-only fallback when The Well is down.
`

// setLine replaces a top-level "key: value" line (values here are
// validated names, so no quoting is needed).
func setLine(doc []byte, key, value string) []byte {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `: .*$`)
	return re.ReplaceAllLiteral(doc, []byte(key+": "+value))
}
