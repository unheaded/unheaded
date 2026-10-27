// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// alert-policy renders observability/alerting/policy.yaml (ADR-100) into the
// Prometheus SLO rules and the Alertmanager routing, and checks that every
// hand-written alert rule is urgency "none": only an SLO burning its error
// budget may notify anyone.
//
//	go run ./cmd/alert-policy           # write the generated files
//	go run ./cmd/alert-policy --check   # fail if they are stale or the policy is invalid
//	go run ./cmd/alert-policy -runbooks # fail unless every alert's runbook_url names a repo file
//	go run ./cmd/alert-policy -sync-well  # write the SLOs into the Well's slo_definitions (WELL_DB set)
//
// With WELL_DB set, --check also fails when slo_definitions differs from
// the policy (well.go).
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/ports"
)

const (
	policyPath = "observability/alerting/policy.yaml"
	rulesDir   = "monitoring/prometheus/rules"
	sloRules   = "monitoring/prometheus/rules/slo.yml"
	amConfig   = "monitoring/alertmanager/alertmanager.yml"
	repoBlob   = "https://github.com/unheaded/unheaded/blob/develop/"
	runbookURL = repoBlob + "runbooks/observe/slo-error-budget-burn.yaml"
)

// Policy is observability/alerting/policy.yaml.
type Policy struct {
	Mailer struct {
		Mode      string `yaml:"mode"`
		MaxPerDay int    `yaml:"max_per_day"`
		DigestAt  string `yaml:"digest_at"`
		Timezone  string `yaml:"timezone"`
	} `yaml:"mailer"`
	Page struct {
		GroupWait      string `yaml:"group_wait"`
		GroupInterval  string `yaml:"group_interval"`
		RepeatInterval string `yaml:"repeat_interval"`
	} `yaml:"page"`
	Promote []Promotion `yaml:"promote"`
	Window  string      `yaml:"window"`
	SLOs    []SLO       `yaml:"slos"`
}

// Promotion lets one hand-written (urgency none) alert notify anyway.
type Promotion struct {
	Alert   string `yaml:"alert"`
	Urgency string `yaml:"urgency"`
	Reason  string `yaml:"reason"`
}

// SLO is one objective with a time-based SLI.
type SLO struct {
	Name        string  `yaml:"name"`
	Service     string  `yaml:"service"`
	Description string  `yaml:"description"`
	Objective   float64 `yaml:"objective"`
	Page        bool    `yaml:"page"`
	Good        string  `yaml:"good"`
	Checks      string  `yaml:"checks"`
	// Runbook is optional: a runbooks/<area>/<name>.yaml for this SLO's burn
	// alerts. Without it they link the shared slo-error-budget-burn runbook.
	Runbook string `yaml:"runbook,omitempty"`
}

// tier is one multi-window burn-rate alert (SRE Workbook table 5-8).
type tier struct {
	name, long, short string
	burn              float64
	pages             bool // page urgency when the SLO pages; ticket otherwise
}

var tiers = []tier{
	{"fast", "1h", "5m", 14.4, true},
	{"medium", "6h", "30m", 6, true},
	{"slow", "1d", "2h", 3, false},
	{"steady", "3d", "6h", 1, false},
}

// windows are every range the tiers read, recorded once for all SLOs.
var windows = []string{"5m", "30m", "1h", "2h", "6h", "1d", "3d"}

var (
	nameRE     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	durationRE = regexp.MustCompile(`^[0-9]+[smhdw]$`)
	clockRE    = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	runbookRE  = regexp.MustCompile(`^runbooks/[a-z]+/[a-z0-9][a-z0-9-]*\.yaml$`)
)

func loadPolicy(path string) (*Policy, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- repo file
	if err != nil {
		return nil, err
	}
	var p Policy
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &p, nil
}

// validate returns every problem, not just the first.
func (p *Policy) validate() []error {
	var errs []error
	bad := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }

	switch p.Mailer.Mode {
	case "dry-run", "send":
	default:
		bad("mailer.mode %q: want dry-run or send", p.Mailer.Mode)
	}
	if p.Mailer.MaxPerDay < 1 || p.Mailer.MaxPerDay > 50 {
		bad("mailer.max_per_day %d: want 1..50 (the point is a handful)", p.Mailer.MaxPerDay)
	}
	if !clockRE.MatchString(p.Mailer.DigestAt) {
		bad("mailer.digest_at %q: want HH:MM", p.Mailer.DigestAt)
	}
	if _, err := time.LoadLocation(p.Mailer.Timezone); err != nil || p.Mailer.Timezone == "" {
		bad("mailer.timezone %q: not an IANA zone", p.Mailer.Timezone)
	}
	for k, v := range map[string]string{"page.group_wait": p.Page.GroupWait, "page.group_interval": p.Page.GroupInterval, "page.repeat_interval": p.Page.RepeatInterval, "window": p.Window} {
		if !durationRE.MatchString(v) {
			bad("%s %q: want a duration like 5m, 12h, 28d", k, v)
		}
	}
	seen := map[string]bool{}
	for i, s := range p.SLOs {
		where := fmt.Sprintf("slos[%d] %q", i, s.Name)
		if !nameRE.MatchString(s.Name) {
			bad("%s: name must match %s", where, nameRE)
		}
		if seen[s.Name] {
			bad("%s: duplicate name", where)
		}
		seen[s.Name] = true
		if s.Service == "" || s.Description == "" || s.Checks == "" || strings.TrimSpace(s.Good) == "" {
			bad("%s: service, description, good and checks are all required", where)
		}
		// Below 14.4x(1-objective) >= 1 the fast tier could never fire.
		if s.Objective < 95 || s.Objective >= 100 {
			bad("%s: objective %v: want 95 <= objective < 100", where, s.Objective)
		}
		// The Well stores the fraction as numeric(6,4) (db/migrations/040).
		if math.Abs(s.Objective*100-math.Round(s.Objective*100)) > 1e-9 {
			bad("%s: objective %v: at most two decimals", where, s.Objective)
		}
		if s.Runbook != "" && !runbookRE.MatchString(s.Runbook) {
			bad("%s: runbook %q: want runbooks/<area>/<name>.yaml", where, s.Runbook)
		}
		for _, t := range []string{s.Description, s.Checks, s.Service} {
			if strings.Contains(t, "{{") || strings.Contains(t, "}}") {
				bad("%s: text may not contain {{ or }} (it goes into an alert template)", where)
			}
		}
	}
	if len(p.SLOs) == 0 {
		bad("no slos: nothing could ever notify")
	}
	for i, pr := range p.Promote {
		if pr.Urgency != "page" && pr.Urgency != "ticket" {
			bad("promote[%d] %q: urgency %q: want page or ticket", i, pr.Alert, pr.Urgency)
		}
		if strings.TrimSpace(pr.Reason) == "" {
			bad("promote[%d] %q: a reason is required", i, pr.Alert)
		}
	}
	return errs
}

func budget(s SLO) float64 { return (100 - s.Objective) / 100 }

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// --- Prometheus rules --------------------------------------------------------

type ruleFile struct {
	Groups []ruleGroup `yaml:"groups"`
}

type ruleGroup struct {
	Name     string `yaml:"name"`
	Interval string `yaml:"interval,omitempty"`
	Rules    []rule `yaml:"rules"`
}

type rule struct {
	Record      string            `yaml:"record,omitempty"`
	Alert       string            `yaml:"alert,omitempty"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

func renderRules(p *Policy) ([]byte, error) {
	var sli, budgetG, alerts ruleGroup
	for _, s := range p.SLOs {
		ids := map[string]string{"slo": s.Name, "service": s.Service}
		// A missing result counts as bad: a target that disappears must not look healthy.
		sli.Rules = append(sli.Rules,
			rule{Record: "slo:good", Expr: fmt.Sprintf("(%s) or on() vector(0)", oneLine(s.Good)), Labels: ids},
			rule{Record: "slo:objective", Expr: fmt.Sprintf("vector(%s)", num(s.Objective/100)), Labels: ids},
		)
		budgetG.Rules = append(budgetG.Rules, rule{
			Record: "slo:error_budget_remaining",
			Expr:   fmt.Sprintf(`1 - (1 - avg_over_time(slo:good{slo=%q}[%s])) / %s`, s.Name, p.Window, num(budget(s))),
			Labels: ids,
		})
		book := runbookURL
		if s.Runbook != "" {
			book = repoBlob + s.Runbook
		}
		for _, t := range tiers {
			urgency := "ticket"
			if t.pages && s.Page {
				urgency = "page"
			}
			sel := fmt.Sprintf(`{slo=%q}`, s.Name)
			alerts.Rules = append(alerts.Rules, rule{
				Alert: "ErrorBudgetBurn",
				// The value is the burn rate over the long window: 1 spends the
				// budget exactly over the SLO window.
				Expr: fmt.Sprintf("slo:error_ratio:%s%s / %s > %s and slo:error_ratio:%s%s / %s > %s",
					t.long, sel, num(budget(s)), num(t.burn), t.short, sel, num(budget(s)), num(t.burn)),
				For:    "2m",
				Labels: map[string]string{"slo": s.Name, "service": s.Service, "urgency": urgency, "tier": t.name},
				Annotations: map[string]string{
					"summary": fmt.Sprintf("SLO %s: %s", s.Name, s.Description),
					"description": fmt.Sprintf(`Burning the error budget (%s%% over %s) {{ $value | printf "%%.1f" }}x as fast as it allows, over the last %s; `+
						`{{ with query "100 * slo:error_budget_remaining{slo=\"%s\"}" }}{{ . | first | value | printf "%%.0f" }}%%{{ else }}an unknown share{{ end }} of it left. Check: %s`,
						num(s.Objective), p.Window, t.long, s.Name, oneLine(s.Checks)),
					"runbook_url": book,
				},
			})
		}
	}
	// One group: rules in a group run in order on the same tick, so each
	// window and alert reads this minute's SLI. Across groups the order is
	// not fixed and results drift by a sample between runs.
	all := ruleGroup{Name: "slo", Interval: "1m", Rules: sli.Rules}
	for _, w := range windows {
		all.Rules = append(all.Rules, rule{Record: "slo:error_ratio:" + w, Expr: fmt.Sprintf("1 - avg_over_time(slo:good[%s])", w)})
	}
	all.Rules = append(all.Rules, budgetG.Rules...)
	all.Rules = append(all.Rules, alerts.Rules...)
	var buf bytes.Buffer
	buf.WriteString("# GENERATED by go run ./cmd/alert-policy from " + policyPath + ". Do not edit.\n")
	buf.WriteString("# The only rules that notify anyone (ADR-100): error-budget burn per SLO.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(ruleFile{Groups: []ruleGroup{all}}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- Alertmanager -------------------------------------------------------------

type amFile struct {
	Global       map[string]string `yaml:"global"`
	Route        amRoute           `yaml:"route"`
	InhibitRules []amInhibit       `yaml:"inhibit_rules"`
	Receivers    []amReceiver      `yaml:"receivers"`
}

type amRoute struct {
	Receiver       string    `yaml:"receiver"`
	GroupBy        []string  `yaml:"group_by,omitempty"`
	GroupWait      string    `yaml:"group_wait,omitempty"`
	GroupInterval  string    `yaml:"group_interval,omitempty"`
	RepeatInterval string    `yaml:"repeat_interval,omitempty"`
	Matchers       []string  `yaml:"matchers,omitempty"`
	Continue       bool      `yaml:"continue,omitempty"`
	Routes         []amRoute `yaml:"routes,omitempty"`
}

type amInhibit struct {
	SourceMatchers []string `yaml:"source_matchers"`
	TargetMatchers []string `yaml:"target_matchers"`
	Equal          []string `yaml:"equal"`
}

type amReceiver struct {
	Name           string      `yaml:"name"`
	WebhookConfigs []amWebhook `yaml:"webhook_configs,omitempty"`
}

type amWebhook struct {
	URL          string `yaml:"url"`
	SendResolved bool   `yaml:"send_resolved"`
	MaxAlerts    int    `yaml:"max_alerts"`
}

func renderAlertmanager(p *Policy) ([]byte, error) {
	base := fmt.Sprintf("http://alert-mailer:%d/v1/alerts/", ports.AlertMailer)
	page := amRoute{Receiver: "page", Matchers: []string{`urgency="page"`},
		GroupWait: p.Page.GroupWait, GroupInterval: p.Page.GroupInterval, RepeatInterval: p.Page.RepeatInterval}
	// Tickets: Alertmanager just keeps the mailer's list current; the mailer
	// sends them as one digest a day.
	ticket := amRoute{Receiver: "ticket", Matchers: []string{`urgency="ticket"`},
		GroupWait: "1m", GroupInterval: "5m", RepeatInterval: "24h"}
	var routes []amRoute
	promos := append([]Promotion(nil), p.Promote...)
	sort.Slice(promos, func(i, j int) bool { return promos[i].Alert < promos[j].Alert })
	for _, pr := range promos {
		r := ticket
		if pr.Urgency == "page" {
			r = page
		}
		r.Matchers = []string{fmt.Sprintf("alertname=%q", pr.Alert), `urgency="none"`}
		routes = append(routes, r)
	}
	routes = append(routes, page, ticket)
	f := amFile{
		Global: map[string]string{"resolve_timeout": "5m"},
		Route: amRoute{
			// urgency none (and anything unlabelled) goes nowhere.
			Receiver: "none",
			// All pages firing together are one mail; tickets are one digest.
			GroupBy:        []string{"urgency"},
			GroupWait:      "1m",
			GroupInterval:  "5m",
			RepeatInterval: "24h",
			Routes:         routes,
		},
		// A page for an SLO says everything its tickets would.
		InhibitRules: []amInhibit{{SourceMatchers: []string{`urgency="page"`}, TargetMatchers: []string{`urgency="ticket"`}, Equal: []string{"slo"}}},
		Receivers: []amReceiver{
			{Name: "none"},
			{Name: "page", WebhookConfigs: []amWebhook{{URL: base + "page", SendResolved: true, MaxAlerts: 50}}},
			{Name: "ticket", WebhookConfigs: []amWebhook{{URL: base + "ticket", SendResolved: true, MaxAlerts: 50}}},
		},
	}
	var buf bytes.Buffer
	buf.WriteString("# GENERATED by go run ./cmd/alert-policy from " + policyPath + ". Do not edit.\n")
	buf.WriteString("# No secrets here: Alertmanager hands page and ticket alerts to alert-mailer,\n")
	buf.WriteString("# which owns the SMTP login, the daily cap, the digest and dry-run.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- hand-written rules ------------------------------------------------------

// checkHandRules: every alert outside slo.yml carries urgency "none", and
// every promotion names one of them.
func checkHandRules(root string, p *Policy) []error {
	var errs []error
	files, _ := filepath.Glob(filepath.Join(root, rulesDir, "*.yml"))
	alerts := map[string]bool{}
	for _, f := range files {
		if filepath.Base(f) == filepath.Base(sloRules) {
			continue
		}
		data, err := os.ReadFile(f) // #nosec G304 -- repo file
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var rf ruleFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f, err))
			continue
		}
		for _, g := range rf.Groups {
			for _, r := range g.Rules {
				if r.Alert == "" {
					continue
				}
				alerts[r.Alert] = true
				if u := r.Labels["urgency"]; u != "none" {
					errs = append(errs, fmt.Errorf("%s: alert %s has urgency %q: hand-written rules are urgency \"none\"; "+
						"only SLOs notify (to notify anyway, add it to promote: in %s with a reason)", rel(root, f), r.Alert, u, policyPath))
				}
			}
		}
	}
	for _, pr := range p.Promote {
		if !alerts[pr.Alert] {
			errs = append(errs, fmt.Errorf("promote: %s is not an alert in %s", pr.Alert, rulesDir))
		}
	}
	return errs
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}

// run generates (or with check, compares) everything; it returns every problem.
func run(root string, check bool) []error {
	p, err := loadPolicy(filepath.Join(root, policyPath))
	if err != nil {
		return []error{err}
	}
	errs := p.validate()
	errs = append(errs, checkHandRules(root, p)...)
	if len(errs) > 0 {
		return errs
	}
	rules, err := renderRules(p)
	if err != nil {
		return []error{err}
	}
	am, err := renderAlertmanager(p)
	if err != nil {
		return []error{err}
	}
	for path, want := range map[string][]byte{sloRules: rules, amConfig: am} {
		full := filepath.Join(root, path)
		if check {
			got, err := os.ReadFile(full) // #nosec G304 -- repo file
			if err != nil || !bytes.Equal(got, want) {
				errs = append(errs, fmt.Errorf("%s is out of date with %s: run go run ./cmd/alert-policy", path, policyPath))
			}
			continue
		}
		if err := os.WriteFile(full, want, 0o644); err != nil { // #nosec G306 -- config read by containers
			errs = append(errs, err)
		}
	}
	return errs
}

func main() {
	root := flag.String("root", ".", "repository root")
	check := flag.Bool("check", false, "fail if the generated files are stale or the policy is invalid")
	runbooks := flag.Bool("runbooks", false, "only check that every alert's runbook_url names a file in the repo")
	syncWellFlag := flag.Bool("sync-well", false, "write the policy's SLOs into the Well's slo_definitions (needs WELL_DB and WELL_PASSWORD)")
	flag.Parse()
	if *runbooks {
		errs := checkRunbookURLs(*root)
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "[FAIL] runbook_url:", e)
		}
		if len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "[FAIL] %d alert(s) without a runbook in the repo (S-WELL-SPRING Appendix A6)\n", len(errs))
			os.Exit(1)
		}
		fmt.Println("[PASS] runbook_url: every alert names a runbook in the repo")
		return
	}
	if *syncWellFlag {
		os.Exit(runSyncWell(*root))
	}
	errs := run(*root, *check)
	well := *check && os.Getenv("WELL_DB") != ""
	if *check && len(errs) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		errs = runWellCheck(ctx, *root)
		cancel()
	}
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "[FAIL]", e)
	}
	if len(errs) > 0 {
		os.Exit(1)
	}
	if *check {
		fmt.Println("[PASS] alert policy: generated files current, every hand-written rule is urgency none")
		if well {
			fmt.Println("[PASS] alert policy: the Well's slo_definitions match the policy")
		}
	}
}

// runSyncWell is -sync-well; it returns the exit code.
func runSyncWell(root string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := syncWellEnv(ctx, root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[FAIL] sync-well:", err)
		return 1
	}
	fmt.Printf("[PASS] sync-well: inserted %v, updated %v, deleted %v, %d unchanged\n",
		res.Inserted, res.Updated, res.Deleted, len(res.Unchanged))
	return 0
}
