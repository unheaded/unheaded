// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package uids

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeToRegistry maps a compose service name to its registry name where
// the two differ.
var composeToRegistry = map[string]string{
	"victoria":      "victoriametrics",
	"vllm-deepseek": "vllm",
}

var composeUserRe = regexp.MustCompile(`^(\d+):(\d+)$`)

// rootByDesign are compose services that must start as root, with why. Each
// is also held to cap_drop ALL by scripts/check-compose-hardening.sh.
var rootByDesign = map[string]string{
	"volume-init": "one-shot chown of named volumes to their service uids; " +
		"CAP_CHOWN only, no network, read-only, exits",
}

// TestComposeMatchesRegistry is TestManifestsMatchRegistry for the compose
// stack that actually runs on west: every service names its own numeric
// user:group, equal to the registry, never root, never 1000 (govan on the
// hosts, so every bind-mounted file would be his), and no two services share
// one. An image default does not count: it is how 11 services ended up as
// uid 1000 with a shell.
func TestComposeMatchesRegistry(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			User string `yaml:"user"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Services) == 0 {
		t.Fatal("no compose services found; refusing to pass vacuously")
	}
	for name := range rootByDesign {
		if _, ok := doc.Services[name]; !ok {
			t.Errorf("rootByDesign lists %q, which is not a compose service: remove it", name)
		}
	}
	seen := map[int]string{}
	for name, svc := range doc.Services {
		if why, ok := rootByDesign[name]; ok {
			if svc.User != "0:0" {
				t.Errorf("compose service %q is listed as root by design (%s) but runs as %q: take it off rootByDesign", name, why, svc.User)
			}
			continue
		}
		reg := name
		if r, ok := composeToRegistry[name]; ok {
			reg = r
		}
		want, ok := Registry[reg]
		if !ok {
			t.Errorf("compose service %q has no UID in pkg/uids: add one", name)
			continue
		}
		m := composeUserRe.FindStringSubmatch(svc.User)
		if m == nil {
			t.Errorf("compose service %q: user %q, want \"%d:%d\" (numeric user:group)", name, svc.User, want, want)
			continue
		}
		uid, _ := strconv.Atoi(m[1])
		gid, _ := strconv.Atoi(m[2])
		if uid != want || gid != want {
			t.Errorf("compose service %q: user %s, registry says %d:%d", name, svc.User, want, want)
		}
		if uid == 0 || uid == 1000 || gid == 0 || gid == 1000 {
			t.Errorf("compose service %q: user %s is root or a human account", name, svc.User)
		}
		if other, dup := seen[uid]; dup {
			t.Errorf("compose services %q and %q share uid %d", other, name, uid)
		}
		seen[uid] = name
	}
}

// TestDockerfileStagesMatchRegistry: each runtime stage of the root
// Dockerfile ends on a numeric USER equal to the registry (the unnamed
// default stage is all-in-one-dev); only the builder may run as root.
func TestDockerfileStagesMatchRegistry(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	type stage struct{ name, user string }
	var stages []stage
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		switch {
		case len(fields) >= 2 && strings.EqualFold(fields[0], "FROM"):
			name := "all-in-one-dev"
			if len(fields) >= 4 && strings.EqualFold(fields[2], "AS") {
				name = fields[3]
			}
			stages = append(stages, stage{name: name})
		case len(fields) == 2 && strings.EqualFold(fields[0], "USER") && len(stages) > 0:
			stages[len(stages)-1].user = fields[1]
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(stages) < 2 {
		t.Fatal("fewer than 2 Dockerfile stages; refusing to pass vacuously")
	}
	for _, s := range stages {
		if s.name == "builder" {
			continue
		}
		want, ok := Registry[s.name]
		if !ok {
			t.Errorf("Dockerfile stage %q has no UID in pkg/uids: add one", s.name)
			continue
		}
		if s.user != strconv.Itoa(want)+":"+strconv.Itoa(want) {
			t.Errorf("Dockerfile stage %q: USER %q, want %d:%d", s.name, s.user, want, want)
		}
	}
}

// TestComposeOnlyListed keeps composeOnly honest: each entry must still be a
// compose service (in docker-compose.yml or one of the alternate files) or a
// Dockerfile stage, or it is exempting nothing.
func TestComposeOnlyListed(t *testing.T) {
	compose, err := os.ReadFile(filepath.Join("..", "..", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	dockerfile, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	alt := altComposeUsers(t)
	for name := range composeOnly {
		_, inAlt := alt[name]
		inCompose := inAlt || regexp.MustCompile(`(?m)^  `+regexp.QuoteMeta(name)+`:\s*$`).Match(compose)
		inDockerfile := name == "all-in-one-dev" || regexp.MustCompile(`(?mi)^FROM\s+\S+\s+AS\s+`+regexp.QuoteMeta(name)+`\s*$`).Match(dockerfile)
		if !inCompose && !inDockerfile {
			t.Errorf("composeOnly lists %q, which is neither a compose service nor a Dockerfile stage", name)
		}
	}
}

// altComposeUsers reads every compose file other than docker-compose.yml
// (the per-host stacks, monitoring, telemetry, ...; tomb/ excepted, as in
// scripts/check-compose-hardening.sh) and returns, per registry name, the
// file:service of each service that names a user. Files at the repo root are
// skipped: docker-compose.override.yml there is host-local and untracked.
func altComposeUsers(t *testing.T) map[string][]altService {
	t.Helper()
	root := filepath.Join("..", "..")
	skip := map[string]bool{".git": true, "tomb": true, "target": true, "node_modules": true, "vendor": true, "dist": true}
	name := regexp.MustCompile(`^docker-compose[^/]*\.ya?ml$`)
	out := map[string][]altService{}
	files := 0
	err := filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if skip[e.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Dir(p) == root || !name.MatchString(e.Name()) {
			return nil
		}
		data, err := os.ReadFile(p) //nolint:gosec // path built by walking the repo
		if err != nil {
			return err
		}
		var doc struct {
			Services map[string]struct {
				User string `yaml:"user"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return err
		}
		files++
		rel, _ := filepath.Rel(root, p)
		for svc, spec := range doc.Services {
			if spec.User == "" {
				continue
			}
			reg := svc
			if r, ok := composeToRegistry[svc]; ok {
				reg = r
			}
			out[reg] = append(out[reg], altService{where: rel + ":" + svc, user: spec.User})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("no alternate compose files found; refusing to pass vacuously")
	}
	return out
}

type altService struct{ where, user string }

// TestAltComposeMatchesRegistry holds the alternate compose files (FND-009)
// to the registry wherever they name a Kingdom uid: a service running as a
// number in the reserved range must be the service the registry gives that
// number to, as numeric user:group. Otherwise two different services could
// share an identity across files, or a typo could hand one service another's
// volumes. Root and the unhardened rest are scripts/check-compose-hardening.sh's
// job.
func TestAltComposeMatchesRegistry(t *testing.T) {
	owner := map[int]string{}
	for svc, uid := range Registry {
		owner[uid] = svc
	}
	for reg, svcs := range altComposeUsers(t) {
		for _, s := range svcs {
			m := composeUserRe.FindStringSubmatch(s.user)
			if m == nil {
				t.Errorf("%s: user %q, want numeric uid:gid", s.where, s.user)
				continue
			}
			uid, _ := strconv.Atoi(m[1])
			gid, _ := strconv.Atoi(m[2])
			if uid < RangeStart || uid > RangeEnd {
				continue
			}
			if uid != gid {
				t.Errorf("%s: user %s, want uid == gid", s.where, s.user)
			}
			if want, ok := Registry[reg]; !ok {
				t.Errorf("%s: runs as %d (registry: %s) but %q has no UID in pkg/uids: add one, or map it in composeToRegistry",
					s.where, uid, owner[uid], reg)
			} else if uid != want {
				t.Errorf("%s: user %s, registry says %d:%d", s.where, s.user, want, want)
			}
		}
	}
}
