// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package kev

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Rule matches catalog entries by CISA's own names. An empty Product
// matches every product of Vendor.
type Rule struct {
	Vendor  string `yaml:"vendor"`
	Product string `yaml:"product"`
}

// Category is one line of the threat register's "Active threat categories".
type Category struct {
	Name  string `yaml:"name"`
	Title string `yaml:"title"`
	Match []Rule `yaml:"match"`
}

// Stack is what the Kingdom runs, as KEV names it.
type Stack struct {
	Categories []Category `yaml:"categories"`
}

// LoadStack reads and validates a stack file. Unknown keys are errors, so a
// misspelt "prodcut:" cannot silently widen a rule to every product.
func LoadStack(path string) (*Stack, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- operator-configured path
	if err != nil {
		return nil, err
	}
	return ParseStack(b)
}

// ParseStack decodes and validates stack YAML.
func ParseStack(b []byte) (*Stack, error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var s Stack
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("stack: %w", err)
	}
	if len(s.Categories) == 0 {
		return nil, errors.New("stack: no categories")
	}
	seen := map[string]bool{}
	for i, c := range s.Categories {
		if c.Name == "" || seen[c.Name] {
			return nil, fmt.Errorf("stack: category %d: name empty or repeated (%q)", i, c.Name)
		}
		seen[c.Name] = true
		if len(c.Match) == 0 {
			return nil, fmt.Errorf("stack: category %s matches nothing", c.Name)
		}
		for j, r := range c.Match {
			if strings.TrimSpace(r.Vendor) == "" {
				return nil, fmt.Errorf("stack: category %s rule %d: vendor is empty", c.Name, j)
			}
		}
	}
	return &s, nil
}

// Classify returns the names of the categories v falls in, sorted; nil when
// none (or when s is nil).
func (s *Stack) Classify(v Vulnerability) []string {
	if s == nil {
		return nil
	}
	var out []string
	for _, c := range s.Categories {
		for _, r := range c.Match {
			if strings.EqualFold(r.Vendor, v.VendorProject) &&
				(r.Product == "" || strings.EqualFold(r.Product, v.Product)) {
				out = append(out, c.Name)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// Relevant returns the entries of vs that fall in at least one category,
// with their categories, in the order given.
func (s *Stack) Relevant(vs []Vulnerability) []Tagged {
	var out []Tagged
	for _, v := range vs {
		if cats := s.Classify(v); len(cats) > 0 {
			out = append(out, Tagged{Vulnerability: v, Categories: cats})
		}
	}
	return out
}

// Tagged is a catalog entry with the stack categories it falls in.
type Tagged struct {
	Vulnerability
	Categories []string `json:"categories,omitempty"`
}
