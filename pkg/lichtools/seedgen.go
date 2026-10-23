// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The local-model driver (S-LICH-AWAKENS Phase 5, Steps 123-139). The free
// local model proposes candidate inputs in a campaign's wire format; the harness
// is the oracle. Seeds are written as INERT DATA — raw bytes in a file named by
// their own content hash — under the campaign's seed directory only, scoped
// server-side. A "seed" is never a command, never executed as code: it is input
// bytes a confined fuzzer reads. Everything stays on the host and costs nothing.

// SeedFormat is the wire format a campaign's seeds must satisfy. The generator
// validates each candidate against it and drops the rest.
type SeedFormat string

const (
	// FormatMonadWire is a 20-byte Monad register (LICH-001/002).
	FormatMonadWire SeedFormat = "monad-wire"
	// FormatMBC is an MBC program: bounded bytes, no exact length.
	FormatMBC SeedFormat = "mbc"
	// FormatBytes is any bounded byte string (the general case).
	FormatBytes SeedFormat = "bytes"
)

// MonadWireLen is the frozen Monad register size (ADR: wire format v0x01).
const MonadWireLen = 20

// DefaultSeedMaxBytes bounds any single seed.
const DefaultSeedMaxBytes = 4096

// SeedSpec asks for a batch of seeds for one campaign.
type SeedSpec struct {
	Campaign string     // seed-dir slug, validated; e.g. lich_001_monad_wire
	Format   SeedFormat // the wire format to validate candidates against
	Count    int        // how many candidates to ask for (bounded)
	MaxBytes int        // per-seed byte cap; 0 uses DefaultSeedMaxBytes
	// Extra is optional operator guidance appended to the prompt scaffold. It is
	// never trusted as code; it only shapes what the model proposes.
	Extra string
}

// SeedGenResult reports what one batch produced.
type SeedGenResult struct {
	Campaign   string   `json:"campaign"`
	Dir        string   `json:"dir"`
	Requested  int      `json:"requested"`
	Parsed     int      `json:"parsed"`
	Written    int      `json:"written"`
	Duplicates int      `json:"duplicates"`
	Dropped    int      `json:"dropped"`
	Files      []string `json:"files"`
}

// MaxSeedBatch bounds how many candidates one call may request.
const MaxSeedBatch = 256

// campaignSlugRe keeps a campaign name to a safe directory component: lowercase
// alnum + underscore, no separators, no dots. This, plus a within() check on
// the resolved path, prevents any traversal out of the seeds tree.
var campaignSlugRe = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// ErrSeedScope is returned when a campaign resolves outside the seeds tree.
var ErrSeedScope = errors.New("lichtools: seed target out of scope")

// GenerateSeeds asks the local model for a batch of candidate inputs, validates
// each against the campaign format, dedups by content, and writes the new ones
// as inert byte files under the campaign's seed directory. It never executes a
// candidate and never writes outside the seeds tree.
func (p *Provider) GenerateSeeds(ctx context.Context, spec SeedSpec) (SeedGenResult, error) {
	if p.local == nil {
		return SeedGenResult{}, errors.New("lichtools: no local model endpoint configured")
	}
	if !campaignSlugRe.MatchString(spec.Campaign) {
		return SeedGenResult{}, fmt.Errorf("%w: campaign %q is not a safe slug", ErrSeedScope, spec.Campaign)
	}
	count := spec.Count
	if count <= 0 {
		count = 16
	}
	if count > MaxSeedBatch {
		count = MaxSeedBatch
	}
	maxBytes := spec.MaxBytes
	if maxBytes <= 0 || maxBytes > DefaultSeedMaxBytes {
		maxBytes = DefaultSeedMaxBytes
	}
	format := spec.Format
	if format == "" {
		format = FormatBytes
	}

	// Resolve and scope the seed directory. It MUST stay inside the configured
	// seeds tree — a crafted slug cannot escape (within() check + no separators).
	seedsRoot := p.cfg.SeedsDirAbs()
	dir := filepath.Join(seedsRoot, spec.Campaign)
	realRoot, err := filepath.EvalSymlinks(seedsRoot)
	if err != nil {
		realRoot = seedsRoot
	}
	if !within(realRoot, dir) {
		return SeedGenResult{}, fmt.Errorf("%w: %s escapes %s", ErrSeedScope, dir, seedsRoot)
	}
	// #nosec G703 -- dir = seedsRoot/<campaign>; campaign is validated to [a-z0-9_] (no separators, no dots) and the resolved path is within()-checked against the seeds tree above.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return SeedGenResult{}, fmt.Errorf("lichtools: seed dir: %w", err)
	}

	// Charge the compute budget, then ask the model once.
	if err := p.reserveLocalGen(); err != nil {
		return SeedGenResult{}, err
	}
	prompt := seedPrompt(format, count, maxBytes, spec.Extra)
	ans, err := p.local.complete(ctx, seedSystemPrompt, prompt, 0, 0.8)
	if err != nil {
		return SeedGenResult{}, fmt.Errorf("lichtools: local model: %w", err)
	}

	res := SeedGenResult{Campaign: spec.Campaign, Dir: dir, Requested: count}
	seen := map[string]bool{}
	for _, line := range strings.Split(ans, "\n") {
		raw, ok := parseSeedLine(line)
		if !ok {
			continue
		}
		res.Parsed++
		if !validSeed(raw, format, maxBytes) {
			res.Dropped++
			continue
		}
		sum := sha256.Sum256(raw)
		key := hex.EncodeToString(sum[:])
		if seen[key] {
			res.Duplicates++
			continue
		}
		seen[key] = true
		name := "seed-" + key[:16] // name is a server-chosen content hash, not model input
		path := filepath.Join(dir, name)
		// #nosec G703 -- dir is scope-checked (see above) and name is a hex content hash, not tainted input; no traversal is possible.
		if _, serr := os.Stat(path); serr == nil {
			res.Duplicates++ // already on disk from a prior batch
			continue
		}
		// #nosec G703 -- same: scope-checked dir + server-chosen hex filename; the seed bytes are data written 0600, never executed.
		if werr := os.WriteFile(path, raw, 0o600); werr != nil {
			return res, fmt.Errorf("lichtools: write seed: %w", werr)
		}
		res.Written++
		res.Files = append(res.Files, name)
	}
	return res, nil
}

// GenRunResult reports one generate→run→collect cycle.
type GenRunResult struct {
	Seeds     SeedGenResult `json:"seeds"`
	HarnessID string        `json:"harness_id"`
	RunText   string        `json:"run_text"`
	RunError  string        `json:"run_error,omitempty"`
}

// GenerateRunCollect runs one campaign cycle: ask the local model for a seed
// batch (free), then run the harness over the corpus inside a confined,
// bounded, no-egress sandbox, and collect its output. There is NO model call on
// the run side — the fuzzer mutates the corpus itself. Everything is
// campaign-scoped: seeds only under the campaign dir, the run only against an
// allowlisted harness. A run-side failure (e.g. no confinement substrate) is
// returned as RunError, not a hard error, so the generated seeds are still
// reported.
func (p *Provider) GenerateRunCollect(ctx context.Context, spec SeedSpec, harnessID string, durationSeconds int) (GenRunResult, error) {
	seeds, err := p.GenerateSeeds(ctx, spec)
	if err != nil {
		return GenRunResult{}, err
	}
	out := GenRunResult{Seeds: seeds, HarnessID: harnessID}

	// Scope is validated inside runHarness (harness id against the allowlist);
	// the run is confined, duration-clamped, and charged to the fuzz budget.
	res, rerr := p.runHarness(ctx, runHarnessInput{
		CampaignID:      spec.Campaign,
		HarnessID:       harnessID,
		DurationSeconds: durationSeconds,
	})
	if rerr != nil {
		out.RunError = rerr.Error()
		return out, nil
	}
	if res.IsError {
		out.RunError = res.Text
		return out, nil
	}
	out.RunText = res.Text
	return out, nil
}

// seedSystemPrompt tells the local model it is proposing fuzzing inputs, not
// running anything. The output is parsed as data; nothing it writes is executed.
const seedSystemPrompt = `You generate fuzzing seed inputs for a security test harness on Stevie Bellis's own Unheaded project.
Output ONLY candidate inputs, one per line, each as space-or-colon-separated hex bytes (for example: "a1 b2 c3" or "a1:b2:c3").
Do not explain. Do not output prose, code, or commands — only hex byte lines. Each line is treated as raw input bytes fed to a parser; it is never executed.`

// mutationStrategies are the BlackMage grimoire scaffolds (Step 124): the model
// proposes; the harness is the oracle.
var mutationStrategies = []string{
	"boundary values (0x00, 0x01, 0x7f, 0x80, 0xff in every byte position)",
	"truncation (inputs shorter than the minimum valid length)",
	"over-length (inputs longer than any valid message)",
	"type confusion (valid header, wrong type/opcode field)",
	"CRC-valid-but-corrupt (a checksum that passes over a corrupted body)",
	"null bytes injected in every position",
	"all-bits-set and all-bits-clear payloads",
}

func seedPrompt(format SeedFormat, count, maxBytes int, extra string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Propose %d distinct candidate seed inputs for the %q wire format.\n", count, format)
	switch format {
	case FormatMonadWire:
		b.WriteString("Each seed is a 20-byte Monad protocol register. Emit exactly 20 hex bytes per line.\n")
	case FormatMBC:
		fmt.Fprintf(&b, "Each seed is an MBC program, up to %d bytes.\n", maxBytes)
	default:
		fmt.Fprintf(&b, "Each seed is up to %d bytes.\n", maxBytes)
	}
	b.WriteString("Vary the candidates across these mutation strategies:\n")
	for _, s := range mutationStrategies {
		b.WriteString("  - " + s + "\n")
	}
	if strings.TrimSpace(extra) != "" {
		b.WriteString("Additional guidance: " + safeSummary(extra) + "\n")
	}
	return b.String()
}

// seedHexRe matches a line of space/colon-separated hex byte pairs. Anything
// else (prose, a shell command, a path) does not match and is dropped — a seed
// line is pure data.
var seedHexRe = regexp.MustCompile(`^[0-9a-fA-F]{2}([ :]+[0-9a-fA-F]{2})*$`)

// listMarkerRe matches a leading list bullet ("- ", "* ", "1. ", "2) ") the
// model might prepend — but NOT a bare hex pair, since it requires a '.'/')'
// or a '-'/'*' bullet followed by whitespace.
var listMarkerRe = regexp.MustCompile(`^([-*]|\d+[.)])\s+`)

// parseSeedLine decodes one hex line to bytes. A non-hex line is not a seed.
func parseSeedLine(line string) ([]byte, bool) {
	s := strings.TrimSpace(line)
	s = listMarkerRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	if s == "" || !seedHexRe.MatchString(s) {
		return nil, false
	}
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ':' })
	out := make([]byte, 0, len(fields))
	for _, f := range fields {
		b, err := hex.DecodeString(f)
		if err != nil || len(b) != 1 {
			return nil, false
		}
		out = append(out, b[0])
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// validSeed checks a decoded candidate against the format and size cap.
func validSeed(raw []byte, format SeedFormat, maxBytes int) bool {
	if len(raw) == 0 || len(raw) > maxBytes {
		return false
	}
	if format == FormatMonadWire && len(raw) != MonadWireLen {
		return false
	}
	return true
}
