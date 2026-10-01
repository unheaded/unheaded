// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package remediation

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// The apply journal (ADR-099): every plan, apply and undo of a fix on a
// host, one JSON object per line in /var/lib/unheaded/remediate/
// journal.jsonl, chained per host: each entry's hash covers the previous
// entry's hash and its own content, so an edited, dropped, reordered or
// inserted entry breaks every hash after it. The collector pulls the file
// and The Well re-verifies the chain on ingest. A broken chain is evidence
// of tampering or corruption, never something to repair.
//
// Limit: whoever can write the file can rewrite its whole tail into a new,
// self-consistent chain. That is caught only against what The Well already
// ingested: ingest requires every entry it already holds to match hash for
// hash, so a rewrite of anything pulled before breaks there.

// GenesisHash is the previous hash of each host's first entry.
var GenesisHash = strings.Repeat("0", 64)

// ErrChainBroken reports a journal whose hashes do not chain.
var ErrChainBroken = errors.New("journal hash chain broken")

const maxJournalLine = 8 << 10

var (
	hexHashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)
	actions   = map[string]bool{"plan": true, "apply": true, "undo": true}
	outcomes  = map[string]bool{"success": true, "failure": true, "refused": true, "restored": true}
)

// JournalEntry is one line of the journal.
type JournalEntry struct {
	Host         string    `json:"host"`
	Seq          int64     `json:"seq"`
	FixID        string    `json:"fix_id"`
	FixSHA256    string    `json:"fix_sha256"`
	ModelCommit  string    `json:"model_commit"`
	Action       string    `json:"action"`
	Outcome      string    `json:"outcome"`
	BackupDir    string    `json:"backup_dir"`
	Operator     string    `json:"operator"`
	Backfilled   bool      `json:"backfilled"`
	At           time.Time `json:"at"`
	PreviousHash string    `json:"previous_hash"`
	Hash         string    `json:"hash"`
}

func (e *JournalEntry) validate() error {
	switch {
	case !isKnownHost(e.Host):
		return invalid("journal: host %q", e.Host)
	case e.Seq < 1:
		return invalid("journal: seq %d", e.Seq)
	case !idRe.MatchString(e.FixID):
		return invalid("journal: fix %q", e.FixID)
	case !hexHashRe.MatchString(e.FixSHA256):
		return invalid("journal: fix sha256 %q", e.FixSHA256)
	case len(e.ModelCommit) > 64 || !printable(e.ModelCommit):
		return invalid("journal: model commit")
	case !actions[e.Action]:
		return invalid("journal: action %q", e.Action)
	case !outcomes[e.Outcome]:
		return invalid("journal: outcome %q", e.Outcome)
	case len(e.BackupDir) > 512 || !printable(e.BackupDir):
		return invalid("journal: backup dir")
	case strings.TrimSpace(e.Operator) == "" || len(e.Operator) > 100 || !printable(e.Operator):
		return invalid("journal: operator")
	case e.At.IsZero():
		return invalid("journal: no time")
	case !hexHashRe.MatchString(e.PreviousHash):
		return invalid("journal: previous hash %q", e.PreviousHash)
	}
	return nil
}

func printable(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// digest is sha256(previous_hash "\n" canonical JSON of every field but
// Hash). Struct field order is fixed and At is normalised to UTC, so the
// encoding is deterministic.
func (e JournalEntry) digest() (string, error) {
	c := e
	c.Hash = ""
	c.At = c.At.UTC()
	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(c.PreviousHash + "\n"))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Seal validates the entry, chains it to prev and sets its Hash.
func (e *JournalEntry) Seal(prev string) error {
	e.PreviousHash = prev
	e.At = e.At.UTC()
	if err := e.validate(); err != nil {
		return err
	}
	d, err := e.digest()
	if err != nil {
		return err
	}
	e.Hash = d
	return nil
}

// VerifyChain checks entries of one host, in order, starting after prev
// (GenesisHash for a whole journal): consecutive sequence numbers, each
// previous hash equal to the last hash, each hash recomputing.
func VerifyChain(entries []JournalEntry, prev string) error {
	if len(entries) == 0 {
		return nil
	}
	host, seq := entries[0].Host, entries[0].Seq
	for i, e := range entries {
		if err := e.validate(); err != nil {
			return fmt.Errorf("%w: entry %d: %v", ErrChainBroken, i+1, err)
		}
		if e.Host != host {
			return fmt.Errorf("%w: entry %d is host %s, chain is %s", ErrChainBroken, i+1, e.Host, host)
		}
		if e.Seq != seq+int64(i) {
			return fmt.Errorf("%w: entry %d has seq %d, want %d", ErrChainBroken, i+1, e.Seq, seq+int64(i))
		}
		if e.PreviousHash != prev {
			return fmt.Errorf("%w: seq %d does not follow the previous entry", ErrChainBroken, e.Seq)
		}
		d, err := e.digest()
		if err != nil {
			return err
		}
		if d != e.Hash {
			return fmt.Errorf("%w: seq %d content does not match its hash", ErrChainBroken, e.Seq)
		}
		prev = e.Hash
	}
	return nil
}

// WriteJSONL appends the entry as one line.
func (e JournalEntry) WriteJSONL(w io.Writer) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// ReadJournal parses a journal file strictly: known fields only, bounded
// lines. It does not verify the chain; VerifyChain does.
func ReadJournal(r io.Reader) ([]JournalEntry, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxJournalLine)
	var out []JournalEntry
	for n := 1; sc.Scan(); n++ {
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		dec.DisallowUnknownFields()
		var e JournalEntry
		if err := dec.Decode(&e); err != nil {
			return nil, invalid("journal line %d: %v", n, err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, invalid("journal: %v", err)
	}
	return out, nil
}

// SHA256 is the fix's content hash: what a journal entry records as applied,
// and what The Well's mirror stores. A changed fix needs re-applying.
func (f Fix) SHA256() (string, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}
