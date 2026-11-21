// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

// Boots in the digest (decision pack 2026-11-18 row 22, ADR-150 choice 15;
// ADR-147's finding "an unplanned boot is its own event"). West's stack does
// not start at boot, so when the host comes up Prometheus, Alertmanager and
// this mailer are down and no rule can say so. The host writes each boot
// down itself (unheaded-boot-event.service, scripts/monitoring/boot-event.sh:
// one JSON line per boot in boots.jsonl); the mailer reads that file when
// the digest is due and gives each boot it has not reported one plain line:
//
//	west booted 2026-11-21 01:44Z; stack down; previous shutdown: not clean
//
// For the newest boot the middle part is the stack's state now, from
// host_state.prom (unheaded-host-state.timer writes it every 30 s whether
// the stack runs or not): "stack down", "stack up since <UTC>" or "stack
// partly up (R of N containers running)". When that file is missing, older
// than boot.state_max_age or from before the boot, and for every earlier
// boot, it is what the boot recorded: "stack down at boot".
//
// Reported means: the digest that first carries a boot's line, sent or (in
// dry-run) recorded, puts its boot id in the mailer's state. A digest a cap
// held back or the relay refused reports nothing, and the next one carries
// the line. Nothing is mailed for a boot outside the digest, under either
// delivery. The line does not depend on the stack being down: when the
// stack is started at boot (the intended later change) the same boot reads
// "stack up since ...".
//
// Both files are another process's output, read through read-only mounts
// (BOOT_EVENTS_FILE, HOST_STATE_FILE): sizes are bounded, every field is
// checked, and a line that is not a record is skipped.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// bootFileMax bounds boots.jsonl (a record is about 300 bytes).
	bootFileMax = 256 << 10
	// hostStateFileMax bounds host_state.prom (huginn's own limit is 64 KiB).
	hostStateFileMax = 1 << 20
	// bootsConsidered is how many of the newest records are read; older
	// ones are never news.
	bootsConsidered = 64
	// bootsReportedKeep is how many reported boot ids the state keeps: more
	// than bootsConsidered, so a reported boot cannot leave the list while
	// it is still read.
	bootsReportedKeep = 256
	// defaultBootStateMaxAge is boot.state_max_age when the policy has none:
	// HostStateStale's own bar.
	defaultBootStateMaxAge = 5 * time.Minute
	bootTimeLayout         = "2006-01-02 15:04Z"
)

var (
	bootIDRE   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	bootHostRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,62}$`)
	bootNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

	// host_state.prom, as scripts/monitoring/host-state-textfile.sh writes it.
	hsRunningRE = regexp.MustCompile(`^unheaded_container_running\{name="((?:[^"\\]|\\.)*)",project="((?:[^"\\]|\\.)*)",service="((?:[^"\\]|\\.)*)"\} ([01])$`)
	hsStartedRE = regexp.MustCompile(`^unheaded_container_started_timestamp_seconds\{name="((?:[^"\\]|\\.)*)"\} ([0-9]{1,12})$`)
	hsLastRunRE = regexp.MustCompile(`^unheaded_host_state_last_run_timestamp_seconds ([0-9]{1,12})$`)
	hsDockerRE  = regexp.MustCompile(`^unheaded_host_state_docker_ok ([01])$`)
)

// BootEvent is one line of boots.jsonl.
type BootEvent struct {
	BootID        string   `json:"boot_id"`
	BootedAt      int64    `json:"booted_at"`
	Host          string   `json:"host"`
	Previous      string   `json:"previous"` // planned, clean, unclean, unknown
	Stack         string   `json:"stack"`    // up, down, partial, unknown: when the boot was recorded
	StackRunning  int      `json:"stack_running"`
	StackTotal    int      `json:"stack_total"`
	StackProjects []string `json:"stack_projects"`
	// StackOneshot (BOOT_EVENT_ONESHOT) is the compose services that never
	// count: one-shots, and services that run only on demand.
	StackOneshot []string `json:"stack_oneshot"`
}

// valid reports whether e can be put in a mail: an id, a time, a host name,
// and project and service names of the expected shape (boot-event.sh
// refuses the same settings).
func (e BootEvent) valid() bool {
	if !bootIDRE.MatchString(e.BootID) || !bootHostRE.MatchString(e.Host) || e.BootedAt <= 0 ||
		len(e.StackProjects) > 16 || len(e.StackOneshot) > 64 {
		return false
	}
	for _, l := range [][]string{e.StackProjects, e.StackOneshot} {
		for _, n := range l {
			if !bootNameRE.MatchString(n) {
				return false
			}
		}
	}
	return true
}

// bootFilesFromEnv takes the two paths from the environment; unset is off.
func bootFilesFromEnv(s *Settings, getenv func(string) string) {
	s.BootEventsFile = getenv("BOOT_EVENTS_FILE")
	s.HostStateFile = getenv("HOST_STATE_FILE")
}

// readBounded reads path, refusing a file over limit bytes. The file is
// opened inside its directory (os.Root): a symlink planted in the mounted
// directory cannot lead the read anywhere else.
func readBounded(path string, limit int64) ([]byte, error) {
	dir, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	f, err := dir.Open(filepath.Base(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: over %d bytes", path, limit)
	}
	return data, nil
}

// readBootEvents returns the newest bootsConsidered valid records of path,
// oldest first, and how many lines it skipped. No file is no boots.
func readBootEvents(path string) (events []BootEvent, skipped int, err error) {
	data, err := readBounded(path, bootFileMax)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 4096), bootFileMax+1)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e BootEvent
		if json.Unmarshal(line, &e) != nil || !e.valid() || seen[e.BootID] {
			skipped++
			continue
		}
		seen[e.BootID] = true
		events = append(events, e)
	}
	if n := len(events); n > bootsConsidered {
		events = events[n-bootsConsidered:]
	}
	return events, skipped, nil
}

// stackNow is the stack's state from host_state.prom.
type stackNow struct {
	known          bool // the file is current and from this boot
	running, total int
	since          int64 // newest start among the running containers
}

// readStackNow reads host_state.prom for the containers of the given
// compose projects, leaving out the services in skip. known is false when the file is missing, was last
// written before bootedAt or more than maxAge ago, or Docker did not answer.
func readStackNow(path string, projects, skip []string, bootedAt int64, now time.Time, maxAge time.Duration) stackNow {
	var s stackNow
	if path == "" {
		return s
	}
	data, err := readBounded(path, hostStateFileMax)
	if err != nil {
		return s
	}
	stack := map[string]bool{}
	for _, p := range projects {
		stack[p] = true
	}
	never := map[string]bool{}
	for _, svc := range skip {
		never[svc] = true
	}
	running := map[string]bool{} // the stack's containers by name
	started := map[string]int64{}
	var lastRun int64
	dockerOK := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 4096), hostStateFileMax+1)
	for sc.Scan() {
		line := sc.Bytes()
		if m := hsRunningRE.FindSubmatch(line); m != nil {
			if stack[string(m[2])] && !never[string(m[3])] {
				running[string(m[1])] = string(m[4]) == "1"
			}
		} else if m := hsStartedRE.FindSubmatch(line); m != nil {
			started[string(m[1])], _ = strconv.ParseInt(string(m[2]), 10, 64)
		} else if m := hsLastRunRE.FindSubmatch(line); m != nil {
			lastRun, _ = strconv.ParseInt(string(m[1]), 10, 64)
		} else if m := hsDockerRE.FindSubmatch(line); m != nil {
			dockerOK = string(m[1]) == "1"
		}
	}
	age := now.Sub(time.Unix(lastRun, 0))
	if !dockerOK || lastRun < bootedAt || age > maxAge || age < -time.Minute {
		return s
	}
	s.known = true
	for name, up := range running {
		s.total++
		if up {
			s.running++
			if t := started[name]; t > s.since {
				s.since = t
			}
		}
	}
	return s
}

// bootLine is the digest's line for one boot. latest marks the newest boot
// on record, the only one now can describe.
func bootLine(e BootEvent, latest bool, now stackNow) string {
	var stack string
	switch {
	case latest && now.known && now.running == 0:
		stack = "stack down"
	case latest && now.known && now.running == now.total && now.since > 0:
		stack = "stack up since " + time.Unix(now.since, 0).UTC().Format(bootTimeLayout)
	case latest && now.known && now.running == now.total:
		stack = "stack up"
	case latest && now.known:
		stack = fmt.Sprintf("stack partly up (%d of %d containers running)", now.running, now.total)
	default:
		switch e.Stack {
		case "up":
			stack = "stack up at boot"
		case "down":
			stack = "stack down at boot"
		case "partial":
			stack = "stack partly up at boot"
		default:
			stack = "stack state unknown at boot"
		}
		if latest {
			stack += " (no current host state)"
		}
	}
	previous := "unknown"
	switch e.Previous {
	case "planned", "clean":
		previous = e.Previous
	case "unclean":
		previous = "not clean"
	}
	return fmt.Sprintf("%s booted %s; %s; previous shutdown: %s",
		e.Host, time.Unix(e.BootedAt, 0).UTC().Format(bootTimeLayout), stack, previous)
}

// pendingBoots is the boots the digest has not reported yet.
type pendingBoots struct {
	ids   []string
	lines []string
	host  string // of the newest one
}

// subject is the digest subject's part: ", west booted" or ", west booted 3 times".
func (p pendingBoots) subject() string {
	switch len(p.ids) {
	case 0:
		return ""
	case 1:
		return ", " + p.host + " booted"
	}
	return fmt.Sprintf(", %s booted %d times", p.host, len(p.ids))
}

// pendingBoots reads the boot record and returns a line for every boot not
// yet reported, oldest first. A record that cannot be read is logged and is
// no boots: the digest must still go out. Called with m.mu held.
func (m *Mailer) pendingBoots() pendingBoots {
	var p pendingBoots
	if m.cfg.BootEventsFile == "" {
		return p
	}
	events, skipped, err := readBootEvents(m.cfg.BootEventsFile)
	if err != nil {
		log.Warn().Err(err).Str("file", m.cfg.BootEventsFile).Msg("boot record not read; the digest carries no boot line")
		return p
	}
	if skipped > 0 {
		log.Warn().Int("lines", skipped).Str("file", m.cfg.BootEventsFile).Msg("boot record: lines that are not records skipped")
	}
	reported := map[string]bool{}
	for _, id := range m.st.BootsReported {
		reported[id] = true
	}
	maxAge := m.cfg.BootStateMaxAge
	if maxAge <= 0 {
		maxAge = defaultBootStateMaxAge
	}
	for i, e := range events {
		if reported[e.BootID] {
			continue
		}
		latest := i == len(events)-1
		var now stackNow
		if latest {
			now = readStackNow(m.cfg.HostStateFile, e.StackProjects, e.StackOneshot, e.BootedAt, m.now(), maxAge)
		}
		p.ids = append(p.ids, e.BootID)
		p.lines = append(p.lines, bootLine(e, latest, now))
		p.host = e.Host
	}
	return p
}

// bootsReported marks the boots a delivered digest carried. Called with
// m.mu held; the caller saves.
func (m *Mailer) bootsReported(p pendingBoots) {
	m.st.BootsReported = append(m.st.BootsReported, p.ids...)
	if n := len(m.st.BootsReported); n > bootsReportedKeep {
		m.st.BootsReported = m.st.BootsReported[n-bootsReportedKeep:]
	}
}
