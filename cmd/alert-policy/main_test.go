// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// The repo's generated files match its policy, and every hand-written rule
// is urgency none. CI runs the same check (security.yml, prometheus-rules).
func TestRepoIsCurrent(t *testing.T) {
	if errs := run(filepath.Join("..", ".."), true); len(errs) > 0 {
		t.Fatal(errs)
	}
}

// tree copies the policy and rules into a temp repo for the edit cases.
func tree(t *testing.T, policyEdit func(string) string, extraRule string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"observability/alerting", rulesDir, "monitoring/alertmanager"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p, err := os.ReadFile(filepath.Join("..", "..", policyPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, policyPath), []byte(policyEdit(string(p))), 0o644); err != nil {
		t.Fatal(err)
	}
	if extraRule != "" {
		if err := os.WriteFile(filepath.Join(root, rulesDir, "extra.yml"), []byte(extraRule), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func same(s string) string { return s }

// copyHandRules copies the repo's hand-written rules, the platform's and every
// module's (ADR-149), into root at the same relative paths.
func copyHandRules(t *testing.T, root string) {
	t.Helper()
	repo := filepath.Join("..", "..")
	for _, m := range handRuleFiles(repo) {
		rel, err := filepath.Rel(repo, m)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(m) // #nosec G304 -- repo file
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// promoteRE finds the promote: key, empty ("promote: []") or a list.
var promoteRE = regexp.MustCompile(`(?m)^promote:( \[\])?$`)

// addPromotion returns an edit that puts one entry first in promote:,
// whatever the live policy already promotes.
func addPromotion(t *testing.T, entry string) func(string) string {
	return func(s string) string {
		if !promoteRE.MatchString(s) {
			t.Fatalf("no promote: line in policy")
		}
		return promoteRE.ReplaceAllLiteralString(s, "promote:\n"+entry)
	}
}

func TestRefusals(t *testing.T) {
	replace := func(old, new string) func(string) string {
		return func(s string) string {
			if !strings.Contains(s, old) {
				t.Fatalf("anchor %q not in policy", old)
			}
			return strings.Replace(s, old, new, 1)
		}
	}
	cases := []struct {
		name  string
		edit  func(string) string
		rule  string
		wants string
	}{
		{"hand rule that pages", same, "groups:\n  - name: x\n    rules:\n      - alert: Loud\n        expr: vector(1)\n        labels:\n          urgency: page\n", "hand-written rules are urgency \"none\""},
		{"hand rule without urgency", same, "groups:\n  - name: x\n    rules:\n      - alert: Unlabelled\n        expr: vector(1)\n", "Unlabelled has urgency \"\""},
		{"objective the fast tier can never reach", replace("objective: 99.5", "objective: 90"), "", "want 95 <= objective < 100"},
		{"objective finer than the Well stores", replace("objective: 99.5", "objective: 99.555"), "", "at most two decimals"},
		{"cap of zero", replace("max_per_day: 6", "max_per_day: 0"), "", "max_per_day 0"},
		{"cap too big to mean anything", replace("max_per_day: 6", "max_per_day: 500"), "", "max_per_day 500"},
		{"hourly cap of zero", replace("max_per_hour: 3", "max_per_hour: 0"), "", "max_per_hour 0"},
		{"hourly cap above the daily one", replace("max_per_hour: 3", "max_per_hour: 7"), "", "max_per_hour 7"},
		// The live mode changes (dry-run → send on 2026-10-27), so anchor on the
		// mailer's mode line whatever it holds, not on one literal value.
		{"unknown mode", func(s string) string {
			re := regexp.MustCompile(`(?m)^  mode: (dry-run|send)$`)
			if !re.MatchString(s) {
				t.Fatalf("no mailer mode line in policy")
			}
			return re.ReplaceAllString(s, "  mode: yolo")
		}, "", `mailer.mode "yolo"`},
		{"template in text", replace("description: The message bus answers on west", "description: hi {{ $labels }}"), "", "may not contain {{"},
		{"duplicate slo", replace("  - name: dashboard\n", "  - name: wotan\n"), "", "duplicate name"},
		{"promotion of an alert that does not exist", addPromotion(t, "  - alert: Nope\n    urgency: ticket\n    reason: test"), "", "Nope is not an alert"},
		{"promotion without a reason", addPromotion(t, "  - alert: HostDiskFull\n    urgency: ticket\n    reason: \"\""), "", "a reason is required"},
		{"unknown policy key", replace("window: 28d", "window: 28d\nwindw: 7d"), "", "field windw not found"},
		{"slo runbook outside runbooks/", replace("  - name: dashboard\n", "  - name: dashboard\n    runbook: ../etc/passwd\n"), "", "runbook \"../etc/passwd\": want runbooks/<area>/<name>.yaml"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := tree(t, c.edit, c.rule)
			// Hand rules from the repo, so promote: can name real alerts.
			copyHandRules(t, root)
			errs := run(root, false)
			var all []string
			for _, e := range errs {
				all = append(all, e.Error())
			}
			if !strings.Contains(strings.Join(all, "\n"), c.wants) {
				t.Fatalf("want an error containing %q, got %v", c.wants, all)
			}
		})
	}
}

// mailer.delivery and mailer.immediate (Stevie, 2026-11-17): the generator
// knows both keys, and refuses an immediate alert that could never reach the
// mailer's page hook.
func TestValidateDelivery(t *testing.T) {
	replace := func(old, new string) func(string) string {
		return func(s string) string {
			if !strings.Contains(s, old) {
				t.Fatalf("anchor %q not in policy", old)
			}
			return strings.Replace(s, old, new, 1)
		}
	}
	for _, c := range []struct {
		name  string
		edit  func(string) string
		wants string
	}{
		{"the repo's own", same, ""},
		{"immediate", replace("  delivery: digest", "  delivery: immediate"), ""},
		{"unknown delivery", replace("  delivery: digest", "  delivery: weekly"), `mailer.delivery "weekly"`},
		{"a promoted page", replace("  immediate: []", "  immediate: [ContainerStopped]"), ""},
		{"a ticket cannot be immediate", replace("  immediate: []", "  immediate: [HostSwapping]"), "mailer.immediate: HostSwapping is not promoted with urgency page"},
		{"an alert nobody promoted", replace("  immediate: []", "  immediate: [Nope]"), "mailer.immediate: Nope is not promoted with urgency page"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := tree(t, c.edit, "")
			copyHandRules(t, root)
			var all []string
			for _, e := range run(root, false) {
				all = append(all, e.Error())
			}
			got := strings.Join(all, "\n")
			if c.wants == "" && got != "" {
				t.Fatalf("want no error, got %s", got)
			}
			if !strings.Contains(got, c.wants) {
				t.Fatalf("want an error containing %q, got %q", c.wants, got)
			}
		})
	}
}

// mailer.run_days (ADR-147 decision 2): the generator knows the block and
// refuses what the mailer refuses.
func TestValidateRunDays(t *testing.T) {
	block := func(day, hour int, date string) func(string) string {
		return func(s string) string {
			re := regexp.MustCompile(`(?m)^  timezone: .*$`)
			if !re.MatchString(s) {
				t.Fatalf("no mailer timezone line in policy")
			}
			s = regexp.MustCompile(`(?m)^  run_days:\n(    .*\n)+`).ReplaceAllString(s, "")
			return re.ReplaceAllStringFunc(s, func(l string) string {
				return l + "\n  run_days:\n    max_per_day: " + strconv.Itoa(day) + "\n    max_per_hour: " + strconv.Itoa(hour) + "\n    dates: [\"" + date + "\"]"
			})
		}
	}
	for _, c := range []struct {
		name  string
		edit  func(string) string
		wants string
	}{
		{"valid", block(24, 6, "2026-12-01"), ""},
		{"bad date", block(24, 6, "2026-13-01"), "mailer.run_days.dates"},
		{"below normal", block(3, 3, "2026-12-01"), "at least mailer.max_per_day"},
		{"over 50", block(51, 6, "2026-12-01"), "1..50"},
		{"hour over day", block(24, 30, "2026-12-01"), "1..run_days.max_per_day"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := tree(t, c.edit, "")
			copyHandRules(t, root) // platform and module rules (ADR-149)
			var all []string
			for _, e := range run(root, false) {
				all = append(all, e.Error())
			}
			got := strings.Join(all, "\n")
			if c.wants == "" && got != "" {
				t.Fatalf("want no error, got %s", got)
			}
			if !strings.Contains(got, c.wants) {
				t.Fatalf("want an error containing %q, got %q", c.wants, got)
			}
		})
	}
}

// renderedRules renders slo.yml from the repo's policy with edit applied.
func renderedRules(t *testing.T, edit func(*Policy)) []rule {
	t.Helper()
	p, err := loadPolicy(filepath.Join("..", "..", policyPath))
	if err != nil {
		t.Fatal(err)
	}
	edit(p)
	out, err := renderRules(p)
	if err != nil {
		t.Fatal(err)
	}
	var rf ruleFile
	if err := yaml.Unmarshal(out, &rf); err != nil {
		t.Fatal(err)
	}
	return rf.Groups[0].Rules
}

// ADR-147 decision 4: a minute under a planned-stop mark for the SLO, or
// for all, counts as good.
func TestGoodHonoursPlannedStop(t *testing.T) {
	want := `clamp_max(((avg(up{job="wotan"})) or on() vector(0)) + ((max(unheaded_planned_stop_until_timestamp_seconds{scope=~"all|wotan"}) > bool time()) or on() vector(0)), 1)`
	for _, r := range renderedRules(t, func(*Policy) {}) {
		if r.Record == "slo:good" && r.Labels["slo"] == "wotan" {
			if r.Expr != want {
				t.Fatalf("slo:good for wotan:\n got %s\nwant %s", r.Expr, want)
			}
			return
		}
	}
	t.Fatal("no slo:good for wotan")
}

func countFor(rules []rule, slo string) (records map[string]int, burns int) {
	records = map[string]int{}
	for _, r := range rules {
		if r.Labels["slo"] != slo {
			continue
		}
		if r.Record != "" {
			records[r.Record]++
		}
		if r.Alert == "ErrorBudgetBurn" {
			burns++
		}
	}
	return records, burns
}

// notify: false records the SLO and notifies nobody (ADR-147 decision 4:
// ci-develop leaves the alert set).
func TestNotifyFalse(t *testing.T) {
	no := false
	rules := renderedRules(t, func(p *Policy) { p.SLOs[0].Notify = &no })
	p, _ := loadPolicy(filepath.Join("..", "..", policyPath))
	rec, burns := countFor(rules, p.SLOs[0].Name)
	if burns != 0 {
		t.Fatalf("%d ErrorBudgetBurn rules for an SLO with notify: false", burns)
	}
	for _, name := range []string{"slo:good", "slo:objective", "slo:error_budget_remaining"} {
		if rec[name] != 1 {
			t.Fatalf("notify: false: %s rendered %d times, want 1", name, rec[name])
		}
	}
}

func TestNotifyDefaultTrue(t *testing.T) {
	rules := renderedRules(t, func(p *Policy) { p.SLOs[0].Notify = nil })
	p, _ := loadPolicy(filepath.Join("..", "..", policyPath))
	if _, burns := countFor(rules, p.SLOs[0].Name); burns != len(tiers) {
		t.Fatalf("no notify field: %d tiers rendered, want %d", burns, len(tiers))
	}
}

// renderAM renders alertmanager.yml from the repo's policy with edit applied.
func renderAM(t *testing.T, edit func(*Policy)) amFile {
	t.Helper()
	p, err := loadPolicy(filepath.Join("..", "..", policyPath))
	if err != nil {
		t.Fatal(err)
	}
	edit(p)
	out, err := renderAlertmanager(p)
	if err != nil {
		t.Fatal(err)
	}
	var f amFile
	if err := yaml.Unmarshal(out, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func withPipeline(v string) func(*Policy) { return func(p *Policy) { p.Pipeline.SilentAfter = v } }

// The pipeline receipt (ADR-116 R4): AlertPipelineHeartbeat goes to the
// mailer before any other route can catch it.
func TestPipelineRouteFirst(t *testing.T) {
	f := renderAM(t, withPipeline("20m"))
	r := f.Route.Routes[0]
	if r.Receiver != "pipeline" || len(r.Matchers) != 1 || r.Matchers[0] != `alertname="AlertPipelineHeartbeat"` {
		t.Fatalf("route 0: %+v", r)
	}
	if r.GroupWait != "30s" || r.GroupInterval != "1m" || r.RepeatInterval != "5m" {
		t.Fatalf("route 0 timing %s/%s/%s, want 30s/1m/5m", r.GroupWait, r.GroupInterval, r.RepeatInterval)
	}
}

func TestPipelineReceiverURL(t *testing.T) {
	f := renderAM(t, withPipeline("20m"))
	for _, rc := range f.Receivers {
		if rc.Name == "pipeline" {
			if len(rc.WebhookConfigs) != 1 || rc.WebhookConfigs[0].URL != "http://alert-mailer:19130/v1/alerts/pipeline" || rc.WebhookConfigs[0].SendResolved {
				t.Fatalf("pipeline receiver: %+v", rc.WebhookConfigs)
			}
			return
		}
	}
	t.Fatal("no pipeline receiver")
}

func TestPipelineAbsentNoRoute(t *testing.T) {
	f := renderAM(t, withPipeline(""))
	for _, r := range f.Route.Routes {
		if r.Receiver == "pipeline" {
			t.Fatal("pipeline route without pipeline.silent_after")
		}
	}
	for _, rc := range f.Receivers {
		if rc.Name == "pipeline" {
			t.Fatal("pipeline receiver without pipeline.silent_after")
		}
	}
}

func TestValidatePipeline(t *testing.T) {
	for v, ok := range map[string]bool{"": true, "20m": true, "10m": true, "5m": false, "soon": false} {
		p, err := loadPolicy(filepath.Join("..", "..", policyPath))
		if err != nil {
			t.Fatal(err)
		}
		p.Pipeline.SilentAfter = v
		var hit bool
		for _, e := range p.validate() {
			hit = hit || strings.Contains(e.Error(), "pipeline.silent_after")
		}
		if hit == ok {
			t.Errorf("silent_after %q: refused=%v, want %v", v, hit, !ok)
		}
	}
}

// boot.state_max_age (the mailer's boot line, decision pack 2026-11-18 row
// 22): absent is the mailer's default, under 1m is refused. The mailer alone
// reads it; the repository's policy carries it.
func TestValidateBoot(t *testing.T) {
	for v, ok := range map[string]bool{"": true, "5m": true, "1m": true, "30s": false, "soon": false} {
		p, err := loadPolicy(filepath.Join("..", "..", policyPath))
		if err != nil {
			t.Fatal(err)
		}
		if p.Boot.StateMaxAge == "" {
			t.Fatal("the repository's policy has no boot.state_max_age")
		}
		p.Boot.StateMaxAge = v
		var hit bool
		for _, e := range p.validate() {
			hit = hit || strings.Contains(e.Error(), "boot.state_max_age")
		}
		if hit == ok {
			t.Errorf("state_max_age %q: refused=%v, want %v", v, hit, !ok)
		}
	}
}

// A promotion routes exactly that alert, ahead of the urgency routes.
func TestPromotionRoutesOneAlert(t *testing.T) {
	root := tree(t, addPromotion(t, "  - alert: HostDiskFull\n    urgency: ticket\n    reason: test"), "")
	copyHandRules(t, root)
	if errs := run(root, false); len(errs) > 0 {
		t.Fatal(errs)
	}
	am, _ := os.ReadFile(filepath.Join(root, amConfig))
	s := string(am)
	promo := strings.Index(s, `alertname="HostDiskFull"`)
	page := strings.Index(s, `- urgency="page"`)
	if promo < 0 || page < 0 || promo > page {
		t.Fatalf("promotion route missing or after the urgency routes:\n%s", s)
	}
}

// A tier keeps firing until its short window is clean again, so a silence
// that ends when the work ends still mails about the planned outage (the
// medium page fired 15m after a restart, 2026-10-21). maintenance.sh adds
// a tail per urgency; each must cover the longest short window that can
// notify at that urgency. Page tiers notify as tickets for SLOs that do
// not page, so every tier counts for tickets.
func TestMaintenanceTailsCoverShortWindows(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "scripts", "monitoring", "maintenance.sh"))
	if err != nil {
		t.Fatal(err)
	}
	tail := func(name string) time.Duration {
		m := regexp.MustCompile(`(?m)^` + name + `=(\d+)$`).FindSubmatch(src)
		if m == nil {
			t.Fatalf("maintenance.sh: no %s=<minutes> line", name)
		}
		n, _ := strconv.Atoi(string(m[1]))
		return time.Duration(n) * time.Minute
	}
	var page, ticket time.Duration
	for _, tr := range tiers {
		d, err := time.ParseDuration(tr.short)
		if err != nil {
			t.Fatalf("tier %s: short %q: %v", tr.name, tr.short, err)
		}
		if tr.pages && d > page {
			page = d
		}
		if d > ticket {
			ticket = d
		}
	}
	if got := tail("PAGE_TAIL_MIN"); got < page {
		t.Errorf("PAGE_TAIL_MIN = %v, the page tiers' short windows reach %v", got, page)
	}
	if got := tail("TICKET_TAIL_MIN"); got < ticket {
		t.Errorf("TICKET_TAIL_MIN = %v, the short windows reach %v", got, ticket)
	}
}

// An SLO with its own runbook: its burn alerts link to it; the others keep
// the shared slo-error-budget-burn runbook.
func TestSLORunbookOverride(t *testing.T) {
	p, err := loadPolicy(filepath.Join("..", "..", policyPath))
	if err != nil {
		t.Fatal(err)
	}
	p.SLOs[0].Runbook = "runbooks/observe/ci-red.yaml"
	out, err := renderRules(p)
	if err != nil {
		t.Fatal(err)
	}
	var rf ruleFile
	if err := yaml.Unmarshal(out, &rf); err != nil {
		t.Fatal(err)
	}
	own, shared := 0, 0
	for _, r := range rf.Groups[0].Rules {
		if r.Alert == "" {
			continue
		}
		switch u := r.Annotations["runbook_url"]; {
		case r.Labels["slo"] == p.SLOs[0].Name && u == "https://github.com/unheaded/unheaded/blob/develop/runbooks/observe/ci-red.yaml":
			own++
		case r.Labels["slo"] != p.SLOs[0].Name && (u == runbookURL || strings.HasPrefix(u, repoBlob+"runbooks/")):
			shared++
		default:
			t.Fatalf("%s %s: runbook_url %q", r.Alert, r.Labels["slo"], u)
		}
	}
	if own != len(tiers) || shared == 0 {
		t.Fatalf("own %d (want %d), shared %d", own, len(tiers), shared)
	}
}

// Every alert in the repo names a runbook that exists (ADR-100 section 4;
// scripts/check-prometheus-rules.sh runs the same check through -runbooks).
func TestRepoRunbookURLs(t *testing.T) {
	if errs := checkRunbookURLs(filepath.Join("..", "..")); len(errs) > 0 {
		t.Fatal(errs)
	}
}

// The runbook_url gate refuses an alert without one, a repo path that does
// not exist, and a URL whose file is not in the repo; it accepts both forms
// when the file exists. No baseline: it starts strict.
func TestRunbookURLRefusals(t *testing.T) {
	alert := func(name, annotation string) string {
		s := "groups:\n  - name: x\n    rules:\n      - alert: " + name + "\n        expr: vector(1)\n        labels:\n          urgency: none\n"
		if annotation != "" {
			s += "        annotations:\n          runbook_url: \"" + annotation + "\"\n"
		}
		return s
	}
	cases := []struct {
		name, rule, wants string
	}{
		{"no runbook_url", alert("Bare", ""), "Bare: no annotations.runbook_url"},
		{"repo path missing", alert("Lost", "runbooks/observe/nope.yaml"), "Lost: runbooks/observe/nope.yaml does not exist"},
		{"path escapes the repo", alert("Escape", "../outside.yaml"), "Escape: ../outside.yaml"},
		{"blob URL to a missing file", alert("Remote", "https://github.com/unheaded/unheaded/blob/develop/runbooks/observe/nope.yaml"), "Remote: "},
		{"other URL, no such page", alert("Wiki", "https://github.com/unheaded/unheaded/wiki/No-Such-Page"), "Wiki: "},
		{"repo path ok", alert("Fine", "runbooks/observe/ok.yaml"), ""},
		{"blob URL ok", alert("FineURL", "https://github.com/unheaded/unheaded/blob/develop/runbooks/observe/ok.yaml"), ""},
		{"wiki URL ok", alert("FineWiki", "https://github.com/unheaded/unheaded/wiki/Alerting"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			for _, d := range []string{rulesDir, "runbooks/observe", "wiki"} {
				if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for f, body := range map[string]string{"runbooks/observe/ok.yaml": "x", "wiki/Alerting.md": "x", rulesDir + "/x.yml": c.rule} {
				if err := os.WriteFile(filepath.Join(root, f), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var all []string
			for _, e := range checkRunbookURLs(root) {
				all = append(all, e.Error())
			}
			got := strings.Join(all, "\n")
			if c.wants == "" && got != "" {
				t.Fatalf("want no error, got %s", got)
			}
			if c.wants != "" && !strings.Contains(got, c.wants) {
				t.Fatalf("want an error containing %q, got %q", c.wants, got)
			}
		})
	}
}
