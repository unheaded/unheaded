// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/zhentools"
)

// harnessOverheadSeconds is the compile+report budget added to a fuzz duration
// to form the sandbox wall timeout.
const harnessOverheadSeconds = 120

// ErrFuzzBudget is returned when a run would exceed the daily compute budget.
var ErrFuzzBudget = errors.New("lichtools: daily fuzz budget exhausted")

// maxReadCrashBytes bounds how much of a crash artifact is hex-dumped.
const maxReadCrashBytes = 4 << 10

var harnessFileRe = regexp.MustCompile(`^lich_(\d{3})_(.+)_test\.go$`)

// clampDuration maps a model-requested duration into [min,max] server-side.
func (p *Provider) clampDuration(req int) int {
	min, max := p.cfg.MinFuzzSeconds, p.cfg.MaxFuzzSeconds
	if req <= 0 {
		return min
	}
	if req < min {
		return min
	}
	if req > max {
		return max
	}
	return req
}

// reserveFuzzBudget charges secs against today's compute budget (UTC day).
func (p *Provider) reserveFuzzBudget(secs int) error {
	p.budgetMu.Lock()
	defer p.budgetMu.Unlock()
	day := time.Now().UTC().Format("2006-01-02")
	if p.budgetDay != day {
		p.budgetDay = day
		p.budgetUsed = 0
	}
	if p.budgetUsed+secs > p.cfg.MaxDailyFuzzSeconds {
		return fmt.Errorf("%w: used %ds of %ds today, requested %ds", ErrFuzzBudget, p.budgetUsed, p.cfg.MaxDailyFuzzSeconds, secs)
	}
	p.budgetUsed += secs
	return nil
}

// resolveGoEnv resolves GOMODCACHE/GOPATH once for offline in-sandbox builds.
func (p *Provider) resolveGoEnv() (goMod, goPath string) {
	p.goEnvMu.Lock()
	defer p.goEnvMu.Unlock()
	if p.goEnvDone {
		return p.goModDir, p.goPathDir
	}
	p.goEnvDone = true
	home, _ := os.UserHomeDir()
	p.goModDir = filepath.Join(home, "go", "pkg", "mod")
	p.goPathDir = filepath.Join(home, "go")
	if out, err := exec.Command("go", "env", "GOMODCACHE", "GOPATH").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) >= 2 {
			if lines[0] != "" {
				p.goModDir = strings.TrimSpace(lines[0])
			}
			if lines[1] != "" {
				p.goPathDir = strings.TrimSpace(lines[1])
			}
		}
	}
	return p.goModDir, p.goPathDir
}

// harnessArgv builds the confined lich-runner invocation. GOCACHE/GOTMPDIR/HOME
// point at the sandbox's writable tmpfs; GOMODCACHE/GOPATH are the host's
// (read-only but readable) module cache; GOPROXY=off keeps the build offline
// (the sandbox has no egress anyway).
func (p *Provider) harnessArgv(id string, durSecs int) []string {
	goMod, goPath := p.resolveGoEnv()
	runner := filepath.Join(p.lichRoot(), "lich-runner.sh")
	// HOME is left inherited (read-only in-sandbox is fine; go uses the explicit
	// GOCACHE/GOMODCACHE below). GOCACHE points at the writable tmpfs; the
	// module cache is the host's (read-only but readable); GOPROXY=off keeps the
	// build offline (the sandbox has no egress regardless).
	return []string{
		"env",
		"GOCACHE=/tmp/lich-gocache",
		"GOTMPDIR=/tmp",
		"GOPROXY=off",
		"GOFLAGS=-mod=mod",
		"GOMODCACHE=" + goMod,
		"GOPATH=" + goPath,
		"bash", runner, "--harness", id, "--time", strconv.Itoa(durSecs) + "s",
	}
}

// lichRoot is the parent of the harness dir (tomb/lich).
func (p *Provider) lichRoot() string { return filepath.Dir(p.scope.HarnessDir()) }

// --- lich_list_campaigns -------------------------------------------------

type campaignEntry struct {
	ID        string `json:"id"`
	Target    string `json:"target"`
	Status    string `json:"status"`
	Verdict   string `json:"verdict,omitempty"`
	Reference string `json:"reference"`
}

func (p *Provider) listCampaigns() (zhentools.Result, error) {
	byID := map[string]*campaignEntry{}

	// Harnesses under the allowlisted dir.
	if entries, err := os.ReadDir(p.scope.HarnessDir()); err == nil {
		for _, e := range entries {
			m := harnessFileRe.FindStringSubmatch(e.Name())
			if m == nil {
				continue
			}
			id := "LICH-" + m[1]
			target := strings.ReplaceAll(m[2], "_", " ")
			if cur, ok := byID[id]; ok {
				cur.Target += ", " + target
				continue
			}
			byID[id] = &campaignEntry{ID: id, Target: target, Status: "harness-present", Reference: "docs/adr/ADR-062-fuzz-redteam-pentest-framework.md"}
		}
	}

	// Campaign directories (LICH-NNN-slug/README.md) carry richer status.
	if dirs, err := filepath.Glob(filepath.Join(p.lichRoot(), "LICH-*")); err == nil {
		for _, d := range dirs {
			info, err := os.Stat(d)
			if err != nil || !info.IsDir() {
				continue
			}
			id := campaignIDFromName(filepath.Base(d))
			fm := readFrontMatter(filepath.Join(d, "README.md"))
			e, ok := byID[id]
			if !ok {
				e = &campaignEntry{ID: id, Reference: "docs/adr/ADR-062-fuzz-redteam-pentest-framework.md"}
				byID[id] = e
			}
			if fm["Target"] != "" {
				e.Target = fm["Target"]
			}
			if fm["Status"] != "" {
				e.Status = fm["Status"]
			}
			e.Reference = filepath.Join("tomb/lich", filepath.Base(d), "README.md")
		}
	}

	out := make([]*campaignEntry, 0, len(byID))
	for _, e := range byID {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return zhentools.Result{IsError: true, Text: err.Error()}, nil
	}
	return zhentools.Result{Text: fmt.Sprintf("%d Lich campaigns (authoritative inventory: ADR-062):\n%s", len(out), string(b))}, nil
}

// --- lich_show_campaign --------------------------------------------------

func (p *Provider) showCampaign(id string) (zhentools.Result, error) {
	num := digitsOnly(id)
	if num == "" {
		return zhentools.Result{IsError: true, Text: fmt.Sprintf("unknown campaign %q (expected LICH-NNN)", id)}, nil
	}
	var b strings.Builder
	found := false

	// Campaign directory, if any.
	if dirs, _ := filepath.Glob(filepath.Join(p.lichRoot(), "LICH-"+num+"*")); len(dirs) > 0 {
		for _, d := range dirs {
			for _, f := range []string{"README.md", "exit-criteria.md", "status.md"} {
				if txt, err := readBounded(filepath.Join(d, f), 8<<10); err == nil {
					found = true
					fmt.Fprintf(&b, "=== %s ===\n%s\n\n", filepath.Join(filepath.Base(d), f), txt)
				}
			}
		}
	}

	// Harness file header, if a harness exists for this id.
	if matches, _ := filepath.Glob(filepath.Join(p.scope.HarnessDir(), "lich_"+num+"_*")); len(matches) > 0 {
		for _, m := range matches {
			if txt, err := readBounded(m, 2<<10); err == nil {
				found = true
				fmt.Fprintf(&b, "=== harness %s (header) ===\n%s\n\n", filepath.Base(m), firstLines(txt, 25))
			}
		}
	}

	// Results summary (how many runs recorded).
	if runs, _ := filepath.Glob(filepath.Join(p.cfg.ResultsDirAbs(), "*")); len(runs) > 0 {
		fmt.Fprintf(&b, "=== results ===\n%d run directories under %s (use lich_read_crash / lich_triage to inspect)\n", len(runs), p.cfg.Scope.ResultsDir)
	}

	if !found {
		return zhentools.Result{IsError: true, Text: fmt.Sprintf("no campaign or harness found for %q", id)}, nil
	}
	return zhentools.Result{Text: b.String()}, nil
}

// --- lich_read_crash (read-only, never executes) -------------------------

func (p *Provider) readCrash(in readCrashInput) (zhentools.Result, error) {
	resultsDir, err := p.resolveResultsDir(in.ResultsDir)
	if err != nil {
		return refused(err), nil
	}
	if in.CrashID == "" {
		return zhentools.Result{IsError: true, Text: "crash_id is required"}, nil
	}
	// The crash id must name a file directly under <results>/crashes, with no
	// path separators or traversal.
	if strings.ContainsAny(in.CrashID, "/\\") || strings.Contains(in.CrashID, "..") {
		return zhentools.Result{IsError: true, Text: "crash_id must be a bare artifact name"}, nil
	}
	crashPath := filepath.Join(resultsDir, "crashes", in.CrashID)
	if !within(filepath.Join(resultsDir, "crashes"), crashPath) {
		return zhentools.Result{IsError: true, Text: "crash path escapes the results tree"}, nil
	}
	data, err := os.ReadFile(crashPath) // #nosec G304 -- path validated: within <results>/crashes, no separators/traversal
	if err != nil {
		// List what is available to help the caller.
		avail, _ := filepath.Glob(filepath.Join(resultsDir, "crashes", "*"))
		names := make([]string, 0, len(avail))
		for _, a := range avail {
			names = append(names, filepath.Base(a))
		}
		return zhentools.Result{IsError: true, Text: fmt.Sprintf("crash %q not found; available: %v", in.CrashID, names)}, nil
	}
	sum := sha256.Sum256(data)
	dump := data
	if len(dump) > maxReadCrashBytes {
		dump = dump[:maxReadCrashBytes]
	}
	return zhentools.Result{Text: fmt.Sprintf(
		"crash %s\nsize: %d bytes\nsha256: %s\nhex (first %d bytes):\n%s\n(NOTE: the crashing input is NEVER executed by this tool)",
		in.CrashID, len(data), hex.EncodeToString(sum[:]), len(dump), hex.EncodeToString(dump))}, nil
}

// --- lich_triage (crash-triage.sh in the sandbox) ------------------------

func (p *Provider) triage(ctx context.Context, in triageInput) (zhentools.Result, error) {
	resultsDir, err := p.resolveResultsDir(in.ResultsDir)
	if err != nil {
		return refused(err), nil
	}
	if p.sandbox == nil {
		return zhentools.Result{IsError: true, Text: "lich_triage: no confinement substrate (fail-closed)"}, nil
	}
	if p.ks != nil && p.ks.Tripped() {
		return zhentools.Result{IsError: true, Text: ErrHalted.Error()}, nil
	}
	lim := p.cfg.Limits
	lim.WallTimeout = 120 * time.Second
	h, err := p.sandbox.Spawn(ctx, Target{Kind: KindHarness, Name: "triage"}, lim)
	if err != nil {
		return zhentools.Result{IsError: true, Text: "lich_triage: " + err.Error()}, nil
	}
	h.WorkDir = p.cfg.ProjectRoot
	if p.ks != nil {
		p.ks.Register(h)
	}
	defer func() {
		if p.ks != nil {
			p.ks.Unregister(h)
		}
		_ = p.sandbox.Destroy(h)
	}()
	script := filepath.Join(p.lichRoot(), "crash-triage.sh")
	res, err := p.sandbox.Exec(ctx, h, []string{"bash", script, resultsDir})
	if err != nil {
		return zhentools.Result{IsError: true, Text: "lich_triage confined exec: " + err.Error()}, nil
	}
	severity := extractSeverity(res.Stdout)
	norm := normalizeStack(res.Stdout)
	dedup := sha256.Sum256([]byte(norm))
	return zhentools.Result{Text: fmt.Sprintf(
		"triage of %s (confined): severity=%s dedup_key=%s\n--- report ---\n%s",
		in.ResultsDir, severity, hex.EncodeToString(dedup[:]), tailLines(res.Stdout, 40))}, nil
}

// --- lich_file_finding (ADR-098 register; pending-confirmation; no auto-publish) ---

func (p *Provider) fileFinding(in fileFindingInput) (zhentools.Result, error) {
	if in.Severity == "" || in.Summary == "" {
		return zhentools.Result{IsError: true, Text: "severity and summary are required"}, nil
	}
	sev := mapSeverity(in.Severity)
	if sev == "" {
		return zhentools.Result{IsError: true, Text: fmt.Sprintf("invalid severity %q (want critical|high|medium|low|info)", in.Severity)}, nil
	}
	registerPath := p.cfg.RegisterAbs()
	raw, err := os.ReadFile(registerPath) // #nosec G304 -- operator-configured register path
	if err != nil {
		return zhentools.Result{IsError: true, Text: "cannot read register: " + err.Error()}, nil
	}
	body := string(raw)

	dedup := in.DedupKey
	if dedup == "" {
		d := sha256.Sum256([]byte(in.Summary + "|" + in.HarnessID + "|" + in.ReproSHA256))
		dedup = hex.EncodeToString(d[:])[:16]
	}
	marker := "lich-dedup:" + dedup

	// Dedup: an existing entry with this key is updated (its needs-regen note is
	// refreshed) rather than duplicated.
	if strings.Contains(body, marker) {
		p.appendNeedsRegen(registerPath, fmt.Sprintf("duplicate dedup %s re-observed %s", dedup, today()))
		return zhentools.Result{Text: fmt.Sprintf("duplicate finding (dedup %s already in the register); updated, no new entry filed", dedup)}, nil
	}

	fnd := nextFND(body)
	host := in.Host
	if host == "" {
		host = "west"
	}
	entry := composeFinding(fnd, sev, host, in, dedup)
	if err := appendToRegister(registerPath, body, entry); err != nil {
		return zhentools.Result{IsError: true, Text: err.Error()}, nil
	}
	p.appendNeedsRegen(registerPath, fmt.Sprintf("%s filed %s; run `go run ./cmd/compliance-evidence findings` after re-signing", fnd, today()))
	return zhentools.Result{Text: fmt.Sprintf(
		"filed %s (severity=%s) to the ADR-098 register (pending-confirmation path; NOT auto-published). "+
			"A needs-regen marker was written; regenerate evidence after signing. dedup=%s", fnd, sev, dedup)}, nil
}

// composeFinding builds a strict-schema-valid RegisterEntry block (only known
// keys; dedup/repro live in the free-text verify field).
func composeFinding(fnd, sev, host string, in fileFindingInput, dedup string) string {
	remediation := in.Remediation
	if remediation == "" {
		remediation = "triage, write a regression test, fix the root cause, then re-run the harness to confirm"
	}
	class := in.Class
	if class == "" {
		class = "app"
	}
	verify := fmt.Sprintf("crash reproduces via the Lich harness; lich-dedup:%s; lich-repro:harness=%s input_sha256=%s",
		dedup, yamlSafe(in.HarnessID), yamlSafe(in.ReproSHA256))
	var b strings.Builder
	fmt.Fprintf(&b, "  - id: %s\n", fnd)
	fmt.Fprintf(&b, "    title: %s\n", yamlScalar(in.Summary+" (Lich red-team finding)"))
	fmt.Fprintf(&b, "    verify: %s\n", yamlScalar(verify))
	fmt.Fprintf(&b, "    host: %s\n", yamlSafe(host))
	fmt.Fprintf(&b, "    severity: %s\n", sev)
	fmt.Fprintf(&b, "    class: %s\n", yamlSafe(class))
	fmt.Fprintf(&b, "    opened: %s\n", today())
	fmt.Fprintf(&b, "    exposure: found by the Lich red-team fuzzer in a confined, no-egress sandbox\n")
	fmt.Fprintf(&b, "    lockout: none\n")
	fmt.Fprintf(&b, "    step: 99\n")
	fmt.Fprintf(&b, "    plan: %s\n", yamlScalar(remediation))
	return b.String()
}

func appendToRegister(path, body, entry string) error {
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	// #nosec G703 G306 -- path is the operator-configured register (cfg.RegisterAbs), never model-controlled; it is a tracked, non-secret triage file
	return os.WriteFile(path, []byte(body+entry), 0o644)
}

func (p *Provider) appendNeedsRegen(registerPath, note string) {
	marker := filepath.Join(filepath.Dir(registerPath), ".needs-regen")
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644) // #nosec G302,G304 -- non-secret marker beside the register
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(note + "\n")
}

// --- helpers -------------------------------------------------------------

// resolveResultsDir validates a results-dir argument is within the configured
// results tree (symlink-resolved), returning its absolute path.
func (p *Provider) resolveResultsDir(arg string) (string, error) {
	if arg == "" {
		return "", fmt.Errorf("%w: results_dir is required", ErrOutOfScope)
	}
	if strings.Contains(arg, "..") {
		return "", fmt.Errorf("%w: results_dir contains '..'", ErrOutOfScope)
	}
	base := p.cfg.ResultsDirAbs()
	full := arg
	if !filepath.IsAbs(full) {
		full = filepath.Join(base, arg)
	}
	full = filepath.Clean(full)
	if !within(base, full) {
		return "", fmt.Errorf("%w: results_dir %q is outside the results tree %s", ErrOutOfScope, arg, base)
	}
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		realBase = base
	}
	if real, err := filepath.EvalSymlinks(full); err == nil {
		if !within(realBase, real) {
			return "", fmt.Errorf("%w: results_dir %q resolves outside the results tree", ErrOutOfScope, arg)
		}
	}
	return full, nil
}

func campaignIDFromName(dir string) string {
	// "LICH-012-config-convergence" -> "LICH-012"
	parts := strings.SplitN(dir, "-", 3)
	if len(parts) >= 2 {
		return parts[0] + "-" + parts[1]
	}
	return dir
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	for len(d) > 0 && len(d) < 3 {
		d = "0" + d
	}
	if len(d) > 3 {
		d = d[len(d)-3:]
	}
	return d
}

var frontMatterRe = regexp.MustCompile(`(?m)^\*\*([A-Za-z]+)\*\*:\s*(.+)$`)

func readFrontMatter(path string) map[string]string {
	out := map[string]string{}
	txt, err := readBounded(path, 4<<10)
	if err != nil {
		return out
	}
	for _, m := range frontMatterRe.FindAllStringSubmatch(txt, -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

func readBounded(path string, max int) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- server-resolved path under the lich tree / results tree
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, max)
	n, _ := f.Read(buf)
	return string(buf[:n]), nil
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

var sevRe = regexp.MustCompile(`(?i)\b(CRITICAL|HIGH|MEDIUM|LOW|INFO)\b`)

func extractSeverity(out string) string {
	order := map[string]int{"CRITICAL": 5, "HIGH": 4, "MEDIUM": 3, "LOW": 2, "INFO": 1}
	best := ""
	for _, m := range sevRe.FindAllString(out, -1) {
		u := strings.ToUpper(m)
		if order[u] > order[best] {
			best = u
		}
	}
	if best == "" {
		return "none"
	}
	return best
}

// normalizeStack strips run-specific noise (timestamps, paths, hex addresses)
// so two identical crashes produce the same dedup key.
func normalizeStack(out string) string {
	s := regexp.MustCompile(`0x[0-9a-fA-F]+`).ReplaceAllString(out, "0xADDR")
	s = regexp.MustCompile(`\d{8}_\d{6}`).ReplaceAllString(s, "TS")
	s = regexp.MustCompile(`(?m)^\s+`).ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func mapSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	case "low":
		return "low"
	case "info":
		return "low" // the register's lowest band
	default:
		return ""
	}
}

var fndRe = regexp.MustCompile(`FND-(\d+)`)

func nextFND(body string) string {
	max := 0
	for _, m := range fndRe.FindAllStringSubmatch(body, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("FND-%03d", max+1)
}

func today() string { return time.Now().UTC().Format("2006-01-02") }

// yamlSafe reduces a short scalar to a safe single-token-ish string for YAML.
func yamlSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 {
			return ' '
		}
		return r
	}, s)
	if s == "" {
		return `""`
	}
	return yamlScalar(s)
}

// yamlScalar double-quotes a scalar, escaping quotes/backslashes, so arbitrary
// text cannot break the register's YAML structure.
func yamlScalar(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	return "\"" + s + "\""
}
