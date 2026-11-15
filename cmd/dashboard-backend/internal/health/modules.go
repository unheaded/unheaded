// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package health

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"unheaded/pkg/logger"
	"unheaded/pkg/modules"
	utls "unheaded/pkg/tls"
)

// ModuleTagPrefix marks a target that belongs to an optional module
// (ADR-149): "module:<name>". Such a target is listed with the platform's but
// never decides the platform's overall state, and it goes when the module's
// switch goes off.
const ModuleTagPrefix = "module:"

// ModuleFragment is the path, relative to a module's directory under
// /run/unheaded/modules, of its health targets (services.txt format).
const ModuleFragment = "dashboard/services.txt"

// maxServicesFile bounds one services file; a real one is a few hundred bytes.
const maxServicesFile = 64 << 10

// ParseServiceEndpoints reads "name=host:port" lines (a scheme prefix such as
// https:// is kept); blank lines and # comments are skipped. Any other line
// is an error: a services file is operator-written and a typo must show.
func ParseServiceEndpoints(r io.Reader) (map[string]string, error) {
	endpoints := make(map[string]string)
	scanner := bufio.NewScanner(io.LimitReader(r, maxServicesFile))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line (expected name=host:port): %q", line)
		}
		name := strings.TrimSpace(parts[0])
		addr := strings.TrimSpace(parts[1])
		if name == "" || addr == "" {
			return nil, fmt.Errorf("empty name or address in: %q", line)
		}
		endpoints[name] = addr
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read services file: %w", err)
	}
	return endpoints, nil
}

// warnedFragments remembers path+content hashes already warned about, so a
// broken fragment is reported once, not every 30 s.
var warnedFragments sync.Map

// ModuleEndpoints reads every module's health targets from the fragments
// published under modules.Dir() (pkg/modules). No module on is an empty map.
// A fragment that does not parse is skipped with one warning per path and
// content.
func ModuleEndpoints(log *logger.Logger) map[string]map[string]string {
	out := make(map[string]map[string]string)
	for _, path := range modules.Fragments(ModuleFragment) {
		mod := modules.ModuleOf(path)
		if mod == "" {
			continue
		}
		// path comes from modules.Fragments: a regular file under the
		// modules directory, with a validated module name.
		data, err := os.ReadFile(path) // #nosec G304 -- path from modules.Fragments
		if err != nil {
			continue
		}
		eps, err := ParseServiceEndpoints(strings.NewReader(string(data)))
		if err != nil {
			sum := sha256.Sum256(data)
			key := path + "\x00" + hex.EncodeToString(sum[:])
			if _, seen := warnedFragments.LoadOrStore(key, true); !seen && log != nil {
				log.Warn().Err(err).Str("module", mod).Str("file", path).Msg("module health targets skipped")
			}
			continue
		}
		out[mod] = eps
	}
	return out
}

// moduleOf returns the module named by a target's tags, or "".
func moduleOf(tags []string) string {
	for _, t := range tags {
		if strings.HasPrefix(t, ModuleTagPrefix) {
			return strings.TrimPrefix(t, ModuleTagPrefix)
		}
	}
	return ""
}

// SyncModuleServices makes the registered module targets exactly byModule:
// it registers what is wanted and missing (built as RegisterKingdomServices
// builds a target, tagged module:<name>) and unregisters module targets no
// longer wanted. A name a platform target already holds is never taken, and
// a platform target is never removed.
func (m *Monitor) SyncModuleServices(byModule map[string]map[string]string) (added, removed int) {
	want := make(map[string]string) // target name -> module
	wantAddr := make(map[string]string)
	for mod, eps := range byModule {
		if !modules.ValidName(mod) {
			continue
		}
		for name, hostPort := range eps {
			if name == "" || hostPort == "" {
				continue
			}
			want[name] = mod
			wantAddr[name] = hostPort
		}
	}

	m.targetsMu.RLock()
	existing := make(map[string]string, len(m.targets)) // name -> module ("" platform)
	for name, t := range m.targets {
		existing[name] = moduleOf(t.Tags)
	}
	m.targetsMu.RUnlock()

	for name, mod := range existing {
		if mod == "" {
			continue
		}
		if wm, ok := want[name]; ok && wm == mod {
			continue
		}
		if err := m.UnregisterTarget(name); err == nil {
			removed++
		}
	}
	for name, mod := range want {
		if cur, ok := existing[name]; ok {
			if cur == "" || cur == mod {
				continue // a platform name, or already registered
			}
		}
		path := "/health"
		if p, ok := serviceHealthPaths[name]; ok {
			path = p
		}
		base := utls.EndpointURL(wantAddr[name], FirstParty[name])
		target := &ServiceTarget{
			Name:      name,
			HealthURL: base + path,
			ReadyURL:  base + path,
			Tags:      []string{ModuleTagPrefix + mod},
		}
		if err := m.RegisterTarget(target); err != nil {
			m.log.Warn().Err(err).Str("target", name).Str("module", mod).Msg("module target not registered")
			continue
		}
		added++
	}
	return added, removed
}

// TargetCounts returns the number of registered targets per module, "" for
// the platform's own (unheaded_dashboard_health_targets{module}).
func (m *Monitor) TargetCounts() map[string]int {
	m.targetsMu.RLock()
	defer m.targetsMu.RUnlock()
	out := map[string]int{"": 0}
	for _, t := range m.targets {
		out[moduleOf(t.Tags)]++
	}
	return out
}
