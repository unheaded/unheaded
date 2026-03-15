// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc_policy

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PolicyFile is the top-level YAML structure for policy files.
type PolicyFile struct {
	Version  string      `yaml:"version"`
	Policies []AppPolicy `yaml:"policies"`
}

// ParsePolicyFile reads and parses a YAML policy file from disk.
func ParsePolicyFile(path string) (*PolicyFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("pqc_policy: read file: %w", err)
	}
	return ParsePolicyBytes(data)
}

// ParsePolicyBytes parses a YAML policy from a byte slice.
func ParsePolicyBytes(data []byte) (*PolicyFile, error) {
	var pf PolicyFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("pqc_policy: parse yaml: %w", err)
	}
	if pf.Version == "" {
		pf.Version = "1.0"
	}
	return &pf, nil
}
