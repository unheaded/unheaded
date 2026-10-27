// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// The runbook_url gate (ADR-100 section 4, S-WELL-SPRING Step 173): every
// alert, generated or hand-written, carries annotations.runbook_url, and it
// names a file in this repository. Strict from the start, no baseline
// (Appendix A6: point the alert at a runbook, or write one).
//
//	runbooks/observe/x.yaml                                   repo path: must exist
//	https://github.com/unheaded/unheaded/blob/<ref>/<path>    <path> must exist
//	any other URL                                              its last segment
//	                                                           must name a file
//	                                                           under runbooks/ or
//	                                                           wiki/ (".md" optional)

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var blobRE = regexp.MustCompile(`^https://github\.com/[^/]+/[^/]+/blob/[^/]+/(.+)$`)

func checkRunbookURLs(root string) []error {
	files, err := filepath.Glob(filepath.Join(root, rulesDir, "*.yml"))
	if err != nil || len(files) == 0 {
		return []error{fmt.Errorf("no rule files under %s", rulesDir)}
	}
	sort.Strings(files)
	pages := map[string]bool{}
	for _, d := range []string{"runbooks", "wiki"} {
		_ = filepath.WalkDir(filepath.Join(root, d), func(_ string, e fs.DirEntry, err error) error {
			if err == nil && !e.IsDir() {
				pages[e.Name()] = true
			}
			return nil
		})
	}
	exists := func(rel string) bool {
		if rel == "" || path.IsAbs(rel) || strings.Contains("/"+rel+"/", "/../") {
			return false
		}
		st, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil && !st.IsDir()
	}
	var errs []error
	for _, f := range files {
		data, err := os.ReadFile(f) // #nosec G304 -- repo file
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var rf ruleFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", rel(root, f), err))
			continue
		}
		for _, g := range rf.Groups {
			for _, r := range g.Rules {
				if r.Alert == "" {
					continue
				}
				u := strings.TrimSpace(r.Annotations["runbook_url"])
				var why string
				switch m := blobRE.FindStringSubmatch(u); {
				case u == "":
					why = "no annotations.runbook_url"
				case m != nil:
					if !exists(m[1]) {
						why = fmt.Sprintf("%s: %s is not in the repository", u, m[1])
					}
				case strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://"):
					seg := path.Base(strings.TrimRight(u, "/"))
					if !pages[seg] && !pages[seg+".md"] {
						why = fmt.Sprintf("%s: no file %q under runbooks/ or wiki/", u, seg)
					}
				default:
					if !exists(u) {
						why = fmt.Sprintf("%s does not exist", u)
					}
				}
				if why != "" {
					errs = append(errs, fmt.Errorf("%s: alert %s: %s", rel(root, f), r.Alert, why))
				}
			}
		}
	}
	return errs
}
