// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A boot of west (decision pack 2026-11-18 row 22): 01:44 UTC, with the
// digest at 09:00 America/Chicago, 15:00 UTC, the same day.
var (
	bootAt   = time.Date(2026, 11, 21, 1, 44, 0, 0, time.UTC)
	digestAt = time.Date(2026, 11, 21, 9, 0, 0, 0, chicago)
)

const (
	bootA = "11111111-1111-4111-8111-111111111111"
	bootB = "22222222-2222-4222-8222-222222222222"
)

// bootRecord is one line of boots.jsonl as scripts/monitoring/boot-event.sh
// writes it.
func bootRecord(id string, at time.Time, previous, stack string) string {
	return fmt.Sprintf(`{"boot_id":%q,"booted_at":%d,"recorded_at":%d,"host":"west","previous":%q,"shutdown_at":0,`+
		`"stack":%q,"stack_running":0,"stack_total":2,"stack_projects":["unheaded-dev"]}`,
		id, at.Unix(), at.Unix()+40, previous, stack)
}

// hostState is host_state.prom as host-state-textfile.sh writes it: two
// containers of the stack's project and the mailer of the monitoring one.
// running is how many of the stack's two run; started is when they did.
func hostState(lastRun time.Time, running int, started time.Time) string {
	var b strings.Builder
	for i, n := range []string{"unheaded-wotan", "unheaded-postgres"} {
		v := 0
		if i < running {
			v = 1
		}
		fmt.Fprintf(&b, "unheaded_container_running{name=%q,project=\"unheaded-dev\",service=%q} %d\n", n, strings.TrimPrefix(n, "unheaded-"), v)
		fmt.Fprintf(&b, "unheaded_container_started_timestamp_seconds{name=%q} %d\n", n, started.Unix()-int64(60*i))
	}
	b.WriteString("unheaded_container_running{name=\"unheaded-alert-mailer\",project=\"monitoring\",service=\"alert-mailer\"} 1\n")
	fmt.Fprintf(&b, "unheaded_container_started_timestamp_seconds{name=\"unheaded-alert-mailer\"} %d\n", lastRun.Unix()-300)
	fmt.Fprintf(&b, "unheaded_host_state_last_run_timestamp_seconds %d\nunheaded_host_state_docker_ok 1\n", lastRun.Unix())
	return b.String()
}

type bootTest struct {
	m      *Mailer
	f      *fakeSender
	c      *clock
	cfg    Settings
	dir    string
	events string
	state  string
}

// newBootTest is a mailer under delivery: digest that reads boots.jsonl and
// host_state.prom from a temp directory; the clock starts an hour before
// the digest, with a pipeline receipt just in, so nothing else is news.
func newBootTest(t *testing.T, mode string) *bootTest {
	t.Helper()
	dir := t.TempDir()
	bt := &bootTest{f: &fakeSender{}, c: &clock{digestAt.Add(-time.Hour)}, dir: dir,
		events: filepath.Join(dir, "boots.jsonl"), state: filepath.Join(dir, "host_state.prom")}
	bt.cfg = Settings{Mode: mode, MaxPerDay: 6, Reminder: 12 * time.Hour, DigestH: 9, Loc: chicago,
		Delivery: "digest", PipelineSilentAfter: 20 * time.Minute,
		BootEventsFile: bt.events, HostStateFile: bt.state, BootStateMaxAge: 5 * time.Minute}
	bt.restart(t)
	return bt
}

// restart is a new mailer process on the same state directory.
func (bt *bootTest) restart(t *testing.T) {
	t.Helper()
	m, err := NewMailer(bt.cfg, bt.f, bt.dir, bt.c.now)
	if err != nil {
		t.Fatal(err)
	}
	bt.m = m
}

func (bt *bootTest) write(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// at moves the clock, hands over a pipeline receipt (the alert path is fine)
// and ticks.
func (bt *bootTest) at(t *testing.T, when time.Time) {
	t.Helper()
	bt.c.t = when
	if err := bt.m.PipelineSeen(); err != nil {
		t.Fatal(err)
	}
	if err := bt.m.Tick(); err != nil {
		t.Fatal(err)
	}
}

// A boot on record and not yet reported is one plain line in the day's
// digest, which goes out for it even with nothing else open. Nothing is
// mailed before the digest hour: a boot is not an intrusion.
func TestBootUnreportedIsADigestLine(t *testing.T) {
	bt := newBootTest(t, "send")
	bt.write(t, bt.events, bootRecord(bootA, bootAt, "unclean", "down"))
	bt.write(t, bt.state, hostState(digestAt.Add(-20*time.Second), 0, bootAt))
	bt.at(t, digestAt.Add(-time.Hour))
	if len(bt.f.sent) != 0 {
		t.Fatalf("a boot mailed before the digest hour: %q", bt.f.sent[0].Subject)
	}
	bt.at(t, digestAt)
	if len(bt.f.sent) != 1 {
		t.Fatalf("%d mails at the digest hour, want 1", len(bt.f.sent))
	}
	d := bt.f.sent[0]
	if d.Subject != "[digest] 0 open, west booted" {
		t.Fatalf("subject %q", d.Subject)
	}
	const line = "west booted 2026-11-21 01:44Z; stack down; previous shutdown: not clean\n"
	if !strings.HasPrefix(d.Body, line) {
		t.Fatalf("the digest must open with %q:\n%s", line, d.Body)
	}
}

// One boot is reported once: the digest that carried it marks it, across a
// restart of the mailer too.
func TestBootReportedOnce(t *testing.T) {
	bt := newBootTest(t, "send")
	bt.write(t, bt.events, bootRecord(bootA, bootAt, "planned", "down"))
	bt.at(t, digestAt)
	if len(bt.f.sent) != 1 || !strings.Contains(bt.f.sent[0].Body, "west booted") {
		t.Fatalf("first digest: %+v", bt.f.sent)
	}
	bt.restart(t)
	bt.at(t, digestAt.Add(24*time.Hour))
	bt.at(t, digestAt.Add(48*time.Hour))
	if len(bt.f.sent) != 1 {
		t.Fatalf("the boot was reported again: %q\n%s", bt.f.sent[1].Subject, bt.f.sent[1].Body)
	}
	// A ticket the day after: its digest does not repeat the boot.
	if err := bt.m.Ticket(Webhook{Alerts: []Alert{alert("firing", "kev-feed", "slow")}}); err != nil {
		t.Fatal(err)
	}
	bt.at(t, digestAt.Add(72*time.Hour))
	if len(bt.f.sent) != 2 || strings.Contains(bt.f.sent[1].Body, "booted") || strings.Contains(bt.f.sent[1].Subject, "booted") {
		t.Fatalf("a later digest repeats the boot: %+v", bt.f.sent[len(bt.f.sent)-1])
	}
}

// Two boots since the last digest give two lines, oldest first. Only the
// newest boot can be given the stack's state now; the earlier one says what
// it was when that boot was recorded.
func TestBootTwoSinceLastDigest(t *testing.T) {
	bt := newBootTest(t, "send")
	first := bootAt.Add(-3 * time.Hour) // 22:44Z the evening before
	bt.write(t, bt.events, bootRecord(bootA, first, "unclean", "down"), bootRecord(bootB, bootAt, "clean", "down"))
	up := time.Date(2026, 11, 21, 14, 2, 0, 0, time.UTC) // a person started it at 14:02Z
	bt.write(t, bt.state, hostState(digestAt.Add(-10*time.Second), 2, up))
	bt.at(t, digestAt)
	if len(bt.f.sent) != 1 {
		t.Fatalf("%d mails, want 1", len(bt.f.sent))
	}
	d := bt.f.sent[0]
	if d.Subject != "[digest] 0 open, west booted 2 times" {
		t.Fatalf("subject %q", d.Subject)
	}
	want := "west booted 2026-11-20 22:44Z; stack down at boot; previous shutdown: not clean\n" +
		"west booted 2026-11-21 01:44Z; stack up since 2026-11-21 14:02Z; previous shutdown: clean\n\n"
	if !strings.HasPrefix(d.Body, want) {
		t.Fatalf("want the digest to open with\n%s\ngot\n%s", want, d.Body)
	}
	bt.at(t, digestAt.Add(24*time.Hour))
	if len(bt.f.sent) != 1 {
		t.Fatalf("both boots were reported; a second digest went out: %q", bt.f.sent[1].Subject)
	}
}

// No boot on record, no file, or the feature not configured: no line, and
// no digest on a quiet day.
func TestBootNoneNoLine(t *testing.T) {
	for name, prepare := range map[string]func(*testing.T, *bootTest){
		"no file":        func(*testing.T, *bootTest) {},
		"empty file":     func(t *testing.T, bt *bootTest) { bt.write(t, bt.events) },
		"not configured": func(t *testing.T, bt *bootTest) { bt.cfg.BootEventsFile = ""; bt.restart(t) },
	} {
		bt := newBootTest(t, "send")
		prepare(t, bt)
		bt.at(t, digestAt)
		if len(bt.f.sent) != 0 {
			t.Fatalf("%s: a quiet day sent a digest: %q", name, bt.f.sent[0].Subject)
		}
		if err := bt.m.Ticket(Webhook{Alerts: []Alert{alert("firing", "kev-feed", "slow")}}); err != nil {
			t.Fatal(err)
		}
		bt.at(t, digestAt.Add(24*time.Hour))
		if len(bt.f.sent) != 1 || strings.Contains(bt.f.sent[0].Body, "booted") || bt.f.sent[0].Subject != "[digest] 1 open" {
			t.Fatalf("%s: digest %+v", name, bt.f.sent)
		}
	}
}

// mode: dry-run records the digest, with the boot line, in the outbox and
// sends nothing; the boot is then reported, as a sent one is.
func TestBootDryRunRecordsAndSendsNothing(t *testing.T) {
	bt := newBootTest(t, "dry-run")
	bt.write(t, bt.events, bootRecord(bootA, bootAt, "unclean", "down"))
	bt.at(t, digestAt)
	if len(bt.f.sent) != 0 {
		t.Fatalf("dry-run sent %d mails", len(bt.f.sent))
	}
	var digests []OutboxEntry
	for _, e := range bt.m.Status(true).Outbox {
		if e.Urgency == "ticket" {
			digests = append(digests, e)
		}
	}
	if len(digests) != 1 || digests[0].Outcome != "dry-run" || digests[0].Subject != "[digest] 0 open, west booted" ||
		!strings.Contains(digests[0].Body, "west booted 2026-11-21 01:44Z; ") {
		t.Fatalf("outbox digests: %+v", digests)
	}
	bt.at(t, digestAt.Add(24*time.Hour))
	n := 0
	for _, e := range bt.m.Status(true).Outbox {
		if e.Urgency == "ticket" {
			n++
		}
	}
	if n != 1 || len(bt.f.sent) != 0 {
		t.Fatalf("dry-run: %d digests in the outbox after a second day, want 1; sent %d", n, len(bt.f.sent))
	}
}

// What the line says about the stack, for the newest boot: its state now
// from host_state.prom when that file is current, else what the boot
// recorded.
func TestBootLineStackState(t *testing.T) {
	fresh := digestAt.Add(-15 * time.Second)
	up := time.Date(2026, 11, 21, 14, 2, 0, 0, time.UTC)
	cases := []struct {
		name  string
		atBot string // the record's stack field
		state string // host_state.prom; "" is no file
		want  string
	}{
		{"down now", "down", hostState(fresh, 0, bootAt), "stack down"},
		{"up now, down at boot", "down", hostState(fresh, 2, up), "stack up since 2026-11-21 14:02Z"},
		{"partly up", "down", hostState(fresh, 1, up), "stack partly up (1 of 2 containers running)"},
		{"no host state", "down", "", "stack down at boot (no current host state)"},
		{"host state stale", "up", hostState(digestAt.Add(-6*time.Minute), 0, bootAt), "stack up at boot (no current host state)"},
		{"host state of the boot before", "partial", hostState(bootAt.Add(-time.Minute), 2, bootAt.Add(-time.Hour)), "stack partly up at boot (no current host state)"},
		{"docker did not answer", "unknown", strings.Replace(hostState(fresh, 2, up), "docker_ok 1", "docker_ok 0", 1), "stack state unknown at boot (no current host state)"},
		{"only the mailer runs", "down", hostState(fresh, 0, bootAt), "stack down"},
	}
	for _, tc := range cases {
		bt := newBootTest(t, "send")
		bt.write(t, bt.events, bootRecord(bootA, bootAt, "planned", tc.atBot))
		if tc.state != "" {
			bt.write(t, bt.state, tc.state)
		}
		// A stale host_state.prom on a day the pipeline is fine: the test
		// for "stale" moves the last run, not the clock.
		bt.at(t, digestAt)
		want := "west booted 2026-11-21 01:44Z; " + tc.want + "; previous shutdown: planned\n"
		if len(bt.f.sent) != 1 || !strings.HasPrefix(bt.f.sent[0].Body, want) {
			t.Errorf("%s: want %q, got %+v", tc.name, want, bt.f.sent)
		}
	}
}

// A digest a cap held back did not report the boot: the next one carries it.
func TestBootHeldDigestKeepsTheBoot(t *testing.T) {
	bt := newBootTest(t, "send")
	bt.cfg.MaxPerDay, bt.cfg.Delivery = 1, "immediate"
	bt.restart(t)
	bt.write(t, bt.events, bootRecord(bootA, bootAt, "unclean", "down"))
	bt.c.t = digestAt.Add(-time.Hour)
	_ = bt.m.PipelineSeen()
	if err := page(bt.m, alert("firing", "wotan", "fast")); err != nil { // the day's one mail
		t.Fatal(err)
	}
	bt.at(t, digestAt)
	if len(bt.f.sent) != 1 || strings.Contains(bt.f.sent[0].Body, "booted") {
		t.Fatalf("want the page only: %+v", bt.f.sent)
	}
	bt.at(t, digestAt.Add(24*time.Hour))
	if len(bt.f.sent) != 2 || !strings.Contains(bt.f.sent[1].Body, "west booted 2026-11-21 01:44Z; ") {
		t.Fatalf("the held digest lost the boot: %+v", bt.f.sent[len(bt.f.sent)-1])
	}
	bt.at(t, digestAt.Add(48*time.Hour))
	for _, s := range bt.f.sent[2:] {
		if strings.Contains(s.Body, "booted") {
			t.Fatalf("the boot was reported twice:\n%s", s.Body)
		}
	}
}

// A relay that refuses the digest did not report the boot either.
func TestBootFailedDigestKeepsTheBoot(t *testing.T) {
	bt := newBootTest(t, "send")
	bt.write(t, bt.events, bootRecord(bootA, bootAt, "unclean", "down"))
	bt.f.err = fmt.Errorf("relay down")
	bt.c.t = digestAt
	_ = bt.m.PipelineSeen()
	if err := bt.m.Tick(); err == nil {
		t.Fatal("a refused digest must be an error (the next minute retries)")
	}
	bt.f.err = nil
	bt.at(t, digestAt.Add(time.Minute))
	if len(bt.f.sent) != 1 || !strings.Contains(bt.f.sent[0].Body, "west booted") {
		t.Fatalf("the retry lost the boot: %+v", bt.f.sent)
	}
}

// With the alert path silent too (the stack is down: so are Prometheus and
// Alertmanager) the silent path stays the digest's first line and the boot
// follows it.
func TestBootLineFollowsPipelineSilent(t *testing.T) {
	bt := newBootTest(t, "send")
	bt.write(t, bt.events, bootRecord(bootA, bootAt, "unclean", "down"))
	bt.c.t = digestAt // no receipt since the start, an hour ago
	if err := bt.m.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(bt.f.sent) != 1 || bt.f.sent[0].Subject != "[digest] 0 open, alert pipeline silent, west booted" {
		t.Fatalf("digest: %+v", bt.f.sent)
	}
	b := bt.f.sent[0].Body
	silent, boot := strings.Index(b, "Alert pipeline silent"), strings.Index(b, "west booted")
	if silent != 0 || boot < silent {
		t.Fatalf("the silent alert path must stay first, the boot after it:\n%s", b)
	}
}

// The record is input from another process: a line that is not a record is
// skipped, and nothing from it reaches the subject or the body.
func TestBootRecordsAreChecked(t *testing.T) {
	good := bootRecord(bootB, bootAt, "clean", "down")
	bad := []string{
		`not json`,
		`{"boot_id":"x","booted_at":1,"host":"west"}`,
		strings.Replace(bootRecord(bootA, bootAt, "clean", "down"), `"host":"west"`, `"host":"west\r\nBcc: someone@example.org"`, 1),
		strings.Replace(bootRecord(bootA, bootAt, "clean", "down"), `"host":"west"`, `"host":""`, 1),
		fmt.Sprintf(`{"boot_id":%q,"booted_at":-5,"host":"west","previous":"clean","stack":"down"}`, bootA),
		strings.Replace(bootRecord(bootA, bootAt, "clean", "down"), `["unheaded-dev"]`, `["a\"} 1\nx"]`, 1),
	}
	bt := newBootTest(t, "send")
	bt.write(t, bt.events, append(bad, good)...)
	bt.at(t, digestAt)
	if len(bt.f.sent) != 1 {
		t.Fatalf("%d mails, want 1", len(bt.f.sent))
	}
	d := bt.f.sent[0]
	if d.Subject != "[digest] 0 open, west booted" || strings.Count(d.Body, "booted") != 1 || strings.Contains(d.Body, "Bcc") {
		t.Fatalf("only the one good record may be reported: %q\n%s", d.Subject, d.Body)
	}
	// An unknown value of previous or stack is said as unknown, not copied.
	bt = newBootTest(t, "send")
	bt.write(t, bt.events, strings.Replace(bootRecord(bootA, bootAt, "exploded", "sideways"), `"host":"west"`, `"host":"east"`, 1))
	bt.at(t, digestAt)
	want := "east booted 2026-11-21 01:44Z; stack state unknown at boot (no current host state); previous shutdown: unknown\n"
	if len(bt.f.sent) != 1 || !strings.HasPrefix(bt.f.sent[0].Body, want) {
		t.Fatalf("want %q, got %+v", want, bt.f.sent)
	}
	// A file past the size bound is not read at all.
	bt = newBootTest(t, "send")
	bt.write(t, bt.events, good, strings.Repeat("x", bootFileMax))
	bt.at(t, digestAt)
	if len(bt.f.sent) != 0 {
		t.Fatalf("an oversized record file was read: %q", bt.f.sent[0].Subject)
	}
}

// Only the newest records are considered, and the reported list is longer
// than that, so an old boot cannot come back as news.
func TestBootHistoryBounds(t *testing.T) {
	bt := newBootTest(t, "send")
	var lines []string
	for i := 0; i < bootsConsidered+10; i++ {
		lines = append(lines, bootRecord(fmt.Sprintf("%08d-1111-4111-8111-111111111111", i), bootAt.Add(time.Duration(i)*time.Minute), "clean", "down"))
	}
	bt.write(t, bt.events, lines...)
	bt.at(t, digestAt)
	if len(bt.f.sent) != 1 || bt.f.sent[0].Subject != fmt.Sprintf("[digest] 0 open, west booted %d times", bootsConsidered) {
		t.Fatalf("digest: %+v", bt.f.sent[0].Subject)
	}
	bt.at(t, digestAt.Add(24*time.Hour))
	if len(bt.f.sent) != 1 {
		t.Fatalf("old boots reported again: %q", bt.f.sent[1].Subject)
	}
	if bootsReportedKeep < 2*bootsConsidered {
		t.Fatalf("bootsReportedKeep %d must be at least twice bootsConsidered %d", bootsReportedKeep, bootsConsidered)
	}
}

// boot.state_max_age in the policy: absent is 5m, under 1m is refused.
func TestLoadSettingsBoot(t *testing.T) {
	for v, want := range map[string]string{"": "5m0s", "boot:\n  state_max_age: 2m\n": "2m0s", "boot:\n  state_max_age: 10s\n": "error", "boot:\n  state_max_age: soon\n": "error"} {
		p := writePolicy(t, "")
		b, _ := os.ReadFile(p)
		if err := os.WriteFile(p, append(b, v...), 0o600); err != nil {
			t.Fatal(err)
		}
		s, err := loadSettings(p)
		switch {
		case want == "error" && (err == nil || !strings.Contains(err.Error(), "boot.state_max_age")):
			t.Errorf("%q: err %v, want a boot.state_max_age refusal", v, err)
		case want != "error" && err != nil:
			t.Errorf("%q: %v", v, err)
		case want != "error" && s.BootStateMaxAge.String() != want:
			t.Errorf("%q: %s, want %s", v, s.BootStateMaxAge, want)
		}
	}
}

// The two files come from the environment (the compose file sets them to
// the read-only mounts); unset is off.
func TestBootFilesFromEnv(t *testing.T) {
	env := map[string]string{"BOOT_EVENTS_FILE": "/run/host/boot-event/boots.jsonl", "HOST_STATE_FILE": "/run/host/textfile/host_state.prom"}
	var s Settings
	bootFilesFromEnv(&s, func(k string) string { return env[k] })
	if s.BootEventsFile != env["BOOT_EVENTS_FILE"] || s.HostStateFile != env["HOST_STATE_FILE"] {
		t.Fatalf("settings %+v", s)
	}
	s = Settings{}
	bootFilesFromEnv(&s, func(string) string { return "" })
	if s.BootEventsFile != "" || s.HostStateFile != "" {
		t.Fatalf("unset must be off: %+v", s)
	}
}
