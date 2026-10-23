// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package privacy keeps Kingdom data from leaving west raw on its way to
// the Claude API (S-TWO-BRAINS Phase 3, ADR-104 §8). Everything bound for
// Claude is classified into a tier and passed through a deterministic
// redactor; large ops payloads are summarised locally first. Failure is
// closed: when the redactor cannot run, the text does not leave.
//
//	T0 repo-public  text of a file tracked by git at HEAD (not on the
//	                deny list): secrets removed, the rest kept.
//	T1 kingdom-ops  everything else the Kingdom produces (Well rows,
//	                service health, logs, runbook output, untracked
//	                files): secrets, addresses, hosts, emails, home paths
//	                removed; summarised locally when large.
//	T2 user data    anything under a configured user-data root: never
//	                leaves (zero user data access).
package privacy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Tier is how far a piece of text may travel.
type Tier int

// Tiers, from most to least shareable.
const (
	TierRepo     Tier = iota // T0
	TierOps                  // T1
	TierUserData             // T2
)

func (t Tier) String() string {
	switch t {
	case TierRepo:
		return "T0"
	case TierOps:
		return "T1"
	case TierUserData:
		return "T2"
	}
	return fmt.Sprintf("T?%d", int(t))
}

// ErrWithheld: T2 text never leaves.
var ErrWithheld = errors.New("withheld: user data")

// Counts is the number of redactions per kind.
type Counts map[string]int

// Total is the number of redactions.
func (c Counts) Total() int {
	n := 0
	for _, v := range c {
		n += v
	}
	return n
}

// Add merges o into c.
func (c Counts) Add(o Counts) {
	for k, v := range o {
		c[k] += v
	}
}

// String is "kind=n,..." in a stable order (no values, safe to log).
func (c Counts) String() string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%s=%d", k, c[k])
	}
	return sb.String()
}

// RedactorConfig configures a Redactor.
type RedactorConfig struct {
	// Scanner is the secret scanner binary (gitleaks 8.19+, `stdin`
	// mode). Default: gitleaks on PATH, then ~/.local/bin/gitleaks.
	Scanner string
	// Timeout bounds one scan. Default 2 s.
	Timeout time.Duration
	// Hosts are host names redacted as whole words (case-insensitive).
	// Default: west, east.
	Hosts []string
}

// Redactor removes secrets and identifiers from text.
type Redactor struct {
	scanner string
	timeout time.Duration
	hostRE  *regexp.Regexp
}

// DefaultHosts are the Kingdom's host names.
var DefaultHosts = []string{"west", "east"}

// NewRedactor finds the scanner and compiles the host list. A missing
// scanner is an error: there is no mode that skips the secret scan.
func NewRedactor(cfg RedactorConfig) (*Redactor, error) {
	scanner := cfg.Scanner
	if scanner == "" {
		if p, err := exec.LookPath("gitleaks"); err == nil {
			scanner = p
		} else if home, err := os.UserHomeDir(); err == nil {
			scanner = filepath.Join(home, ".local", "bin", "gitleaks")
		}
	}
	if scanner == "" {
		return nil, errors.New("privacy: no secret scanner (gitleaks) found")
	}
	if st, err := os.Stat(scanner); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("privacy: secret scanner %s is not an executable file", scanner)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	hosts := cfg.Hosts
	if hosts == nil {
		hosts = DefaultHosts
	}
	r := &Redactor{scanner: scanner, timeout: timeout}
	var alts []string
	for _, h := range hosts {
		if h = strings.TrimSpace(h); h != "" {
			alts = append(alts, regexp.QuoteMeta(h))
		}
	}
	if len(alts) > 0 {
		r.hostRE = regexp.MustCompile(`(?i)\b(?:` + strings.Join(alts, "|") + `)\b`)
	}
	return r, nil
}

// tagger hands out stable placeholders within one payload: the same value
// always gets the same tag, so "the same host" stays recognisable.
type tagger struct {
	seen   map[string]map[string]string
	counts Counts
}

func newTagger() *tagger { return &tagger{seen: map[string]map[string]string{}, counts: Counts{}} }

func (t *tagger) tag(kind, value string) string {
	m := t.seen[kind]
	if m == nil {
		m = map[string]string{}
		t.seen[kind] = m
	}
	t.counts[kind]++
	if tag, ok := m[value]; ok {
		return tag
	}
	tag := fmt.Sprintf("[REDACTED:%s#%d]", kind, len(m)+1)
	m[value] = tag
	return tag
}

var (
	pemRE      = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]+-----.*?(?:-----END [A-Z0-9 ]+-----|\z)`)
	urlCredRE  = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)([^:/@\s]+):([^@\s]+)@`)
	kvSecretRE = regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|secret[_-]?key|private[_-]?key)(\s*[=:]\s*"?)([^\s"',;]+)`)
	awsKeyRE   = regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)
	emailRE    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	macRE      = regexp.MustCompile(`\b[0-9A-Fa-f]{2}(?::[0-9A-Fa-f]{2}){5}\b`)
	ipv6RE     = regexp.MustCompile(`(?i)(?:[0-9a-f]{0,4}:){2,7}[0-9a-f]{0,4}`)
	ipv4RE     = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	homeRE     = regexp.MustCompile(`/(home|Users)/([^/\s:"']+)`)
)

// Redact removes what tier t must not carry. TierRepo: secrets only.
// TierOps: secrets, then addresses, MACs, emails, home paths and host
// names. TierUserData: ErrWithheld. Any scanner failure is an error and
// the caller must not send the text.
func (r *Redactor) Redact(ctx context.Context, s string, t Tier) (string, Counts, error) {
	if r == nil {
		return "", nil, errors.New("privacy: no redactor")
	}
	if t == TierUserData {
		return "", nil, ErrWithheld
	}
	if biteNoop { // privacy_bite build tag: proves the tests can fail
		return s, Counts{}, nil
	}
	tg := newTagger()
	secrets, err := r.scan(ctx, s)
	if err != nil {
		return "", nil, err
	}
	for _, sec := range secrets {
		if len(sec) >= 4 {
			s = strings.ReplaceAll(s, sec, tg.tag("secret", sec))
		}
	}
	s = pemRE.ReplaceAllStringFunc(s, func(m string) string { return tg.tag("pem", m) })
	s = urlCredRE.ReplaceAllStringFunc(s, func(m string) string {
		sm := urlCredRE.FindStringSubmatch(m)
		return sm[1] + tg.tag("cred", sm[2]+":"+sm[3]) + "@"
	})
	s = kvSecretRE.ReplaceAllStringFunc(s, func(m string) string {
		sm := kvSecretRE.FindStringSubmatch(m)
		if strings.HasPrefix(sm[3], "[REDACTED:") {
			return m
		}
		return sm[1] + sm[2] + tg.tag("secret", sm[3])
	})
	s = awsKeyRE.ReplaceAllStringFunc(s, func(m string) string { return tg.tag("aws", m) })
	if t == TierRepo {
		return s, tg.counts, nil
	}
	s = emailRE.ReplaceAllStringFunc(s, func(m string) string { return tg.tag("email", m) })
	s = macRE.ReplaceAllStringFunc(s, func(m string) string { return tg.tag("mac", strings.ToLower(m)) })
	s = ipv6RE.ReplaceAllStringFunc(s, func(m string) string {
		if strings.Count(m, ":") < 2 {
			return m
		}
		ip := net.ParseIP(m)
		if ip == nil || ip.To4() != nil {
			return m
		}
		return tg.tag("ipv6", ip.String())
	})
	s = ipv4RE.ReplaceAllStringFunc(s, func(m string) string {
		if ip := net.ParseIP(m); ip == nil || ip.To4() == nil {
			return m
		}
		return tg.tag("ipv4", m)
	})
	s = homeRE.ReplaceAllStringFunc(s, func(m string) string {
		sm := homeRE.FindStringSubmatch(m)
		return "/" + sm[1] + "/" + tg.tag("user", sm[2])
	})
	if r.hostRE != nil {
		s = r.hostRE.ReplaceAllStringFunc(s, func(m string) string { return tg.tag("host", strings.ToLower(m)) })
	}
	return s, tg.counts, nil
}

// scan runs the secret scanner on s and returns the secrets it found.
// Any failure (missing binary, timeout, non-JSON output) is an error.
func (r *Redactor) scan(ctx context.Context, s string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	// #nosec G204 -- fixed binary chosen at construction, fixed arguments, no shell
	cmd := exec.CommandContext(ctx, r.scanner, "stdin", "--no-banner", "--log-level", "error",
		"--report-format", "json", "--report-path", "/dev/stdout", "--exit-code", "0")
	cmd.Dir = os.TempDir() // never pick up a repository's scanner config or allowlist
	cmd.Env = scannerEnv()
	cmd.Stdin = strings.NewReader(s)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("privacy: secret scan timed out after %s", r.timeout)
		}
		return nil, fmt.Errorf("privacy: secret scan failed: %w", err)
	}
	var findings []struct {
		Secret string `json:"Secret"`
		Match  string `json:"Match"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &findings); err != nil {
		return nil, errors.New("privacy: secret scan output is not a JSON report")
	}
	var secrets []string
	for _, f := range findings {
		if f.Secret != "" {
			secrets = append(secrets, f.Secret)
		} else if f.Match != "" {
			secrets = append(secrets, f.Match)
		}
	}
	// Longest first, so a secret that contains another is replaced whole.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return secrets, nil
}

// scannerEnv is a minimal environment: no config override from the caller.
func scannerEnv() []string {
	var env []string
	for _, k := range []string{"PATH", "HOME", "TMPDIR", "LANG"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}
