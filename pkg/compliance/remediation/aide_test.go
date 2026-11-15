// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package remediation

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func fixByID(t *testing.T, m *Model, id string) Fix {
	t.Helper()
	for _, f := range m.Fixes {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("no fix %s in the model", id)
	return Fix{}
}

func fileText(t *testing.T, a Action) string {
	t.Helper()
	if a.File == nil {
		t.Fatalf("not a file action: %+v", a)
	}
	s, err := a.File.Render("west", Facts{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// neverWatched are the trees DE1 keeps out of AIDE for good ("AIDE, quiet
// by design", rule 1): each is a ! line, so no later include reaches them.
var neverWatched = []string{"/var/lib", "/var/log", "/var/cache", "/var/tmp", "/tmp", "/home", "/root", "/nix/store",
	"/opt/rocm", "/usr/local/lib/python3", "/usr/local/lib/ollama", "/usr/lib/python3/dist-packages/__pycache__",
	"/var/snap", "/snap", "/var/zhen", "/etc/aide"}

var watched = []string{"/usr/bin", "/usr/sbin", "/usr/lib", "/usr/local/bin", "/usr/local/sbin", "/etc", "/boot",
	"/lib/systemd/system", "/etc/systemd/system", "/opt/unheaded", "/usr/share/unheaded", "/usr/local/lib/unheaded"}

// aide-install is DE1's design, action by action and in this order.
func TestAideInstallIsQuietByDesign(t *testing.T) {
	f := fixByID(t, repoModel(t), "aide-install")
	a := f.Actions
	if len(a) != 10 {
		t.Fatalf("%d actions, want 10", len(a))
	}

	// 1. The package, without what it recommends (aide-common: the daily
	// timer, the cron script, aideinit and a mail transport agent).
	if a[0].Package == nil || a[0].Package.Name != "aide" || !a[0].Package.NoRecommends {
		t.Errorf("action 1 = %+v, want package aide without recommends", a[0])
	}

	// 2. Our own config: an include list, standing alone.
	if a[1].File == nil || a[1].File.Path != "/etc/aide/aide-unheaded.conf" || a[1].File.Mode != "0600" ||
		a[1].File.Owner != "root" || a[1].File.Group != "root" {
		t.Fatalf("action 2 = %+v, want /etc/aide/aide-unheaded.conf 0600 root:root", a[1].File)
	}
	conf := fileText(t, a[1])
	if strings.Contains(conf, "@@") {
		t.Error("the config uses an @@ directive: it must not include the package's aide.conf.d")
	}
	lines := map[string]bool{}
	var rule string
	for _, l := range strings.Split(conf, "\n") {
		l = strings.TrimSpace(l)
		lines[l] = true
		if name, def, ok := strings.Cut(l, " = "); ok && name == "UH" {
			rule = def
		}
	}
	for _, p := range neverWatched {
		if !lines["!"+p] {
			t.Errorf("the config has no line %q", "!"+p)
		}
	}
	for _, p := range watched {
		if !lines[p+" UH"] {
			t.Errorf("the config does not watch %s", p)
		}
	}
	for l := range lines {
		// AIDE 0.19 refuses a selection line that does not start with a
		// slash ("unexpected character: '!'" for "!.*/x", exit 17).
		if strings.HasPrefix(l, "!") && !strings.HasPrefix(l, "!/") {
			t.Errorf("negative line %q does not start with !/: aide refuses the whole config", l)
		}
		if strings.HasPrefix(l, "/") && !strings.HasSuffix(l, " UH") {
			t.Errorf("selection line %q does not use the UH rule", l)
		}
		if strings.HasPrefix(l, "/") {
			if p := strings.TrimSuffix(l, " UH"); !sliceHas(watched, p) {
				t.Errorf("the config watches %s, which DE1's list does not name", p)
			}
		}
	}
	got := strings.Split(rule, "+")
	sort.Strings(got)
	want := []string{"acl", "c", "g", "i", "m", "n", "p", "s", "sha512", "u", "xattrs"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("rule UH = %q, want exactly %v (no atime: reading a file is never a change)", rule, want)
	}
	// The report: entries and their flags only (unheaded-aide reads no more,
	// and a large diff stays small), as JSON, and the format line before the
	// URL it applies to.
	if f, u := strings.Index(conf, "\nreport_format=json\n"), strings.Index(conf, "\nreport_url=stdout\n"); f < 0 || u < f {
		t.Error("report_format=json must come before report_url=stdout: aide applies it to the URLs after it")
	}
	if !lines["!/.*/__pycache__"] {
		t.Error("the config does not keep every __pycache__ out")
	}
	for _, need := range []string{"database_in=file:/var/lib/aide/aide.db", "database_out=file:/var/lib/aide/aide.db.new",
		"report_level=list_entries", "report_format=json", "report_url=stdout"} {
		if !lines[need] {
			t.Errorf("the config lacks %q", need)
		}
	}
	for _, bad := range []string{"mailto", "MAILTO", "report_url=syslog", "report_url=file"} {
		if strings.Contains(conf, bad) {
			t.Errorf("the config contains %q: one report, on stdout, to the wrapper", bad)
		}
	}

	// 3. Our units, idle by every knob systemd has.
	if a[2].File == nil || a[2].File.Path != "/etc/systemd/system/unheaded-aide-check.service" || a[2].File.Mode != "0644" {
		t.Fatalf("action 3 = %+v, want the service unit", a[2].File)
	}
	svc := "\n" + fileText(t, a[2])
	for _, want := range []string{"Nice=19", "IOSchedulingClass=idle", "CPUSchedulingPolicy=idle", "IOWeight=10", "IOAccounting=yes",
		"CPUAccounting=yes", "MemoryMax=512M", "TimeoutStartSec=3h", "Type=oneshot", "User=unheaded-aide",
		"ExecStart=/usr/local/bin/unheaded-aide check --textfile-dir /var/lib/unheaded/textfile",
		"ConditionPathExists=/var/lib/aide/aide.db"} {
		if !strings.Contains(svc, "\n"+want+"\n") {
			t.Errorf("the service unit lacks %q", want)
		}
	}
	for _, bad := range []string{"--init", "--update", "aideinit", "User=root", "mail"} {
		if strings.Contains(svc, bad) {
			t.Errorf("the service unit contains %q", bad)
		}
	}
	if a[3].File == nil || a[3].File.Path != "/etc/systemd/system/unheaded-aide-check.timer" || a[3].File.Mode != "0644" {
		t.Fatalf("action 4 = %+v, want the timer unit", a[3].File)
	}
	tmr := "\n" + fileText(t, a[3])
	for _, want := range []string{"OnCalendar=weekly", "RandomizedDelaySec=6h", "Persistent=true", "WantedBy=timers.target"} {
		if !strings.Contains(tmr, "\n"+want+"\n") {
			t.Errorf("the timer unit lacks %q", want)
		}
	}
	if strings.Count(tmr, "OnCalendar=") != 1 {
		t.Error("the timer has more than one OnCalendar: weekly is the only run")
	}

	// 4-6. systemd reads them; the package's own paths are off.
	if a[4].Run != "systemctl daemon-reload" {
		t.Errorf("action 5 = %+v, want daemon-reload", a[4])
	}
	if s := a[5].Service; s == nil || s.Name != "dailyaidecheck.timer" || s.Masked == nil || !*s.Masked || s.Enabled != nil || s.Active != nil {
		t.Errorf("action 6 = %+v, want the package's timer masked (mask alone: it may not be installed)", a[5].Service)
	}
	if a[6].Divert != "/etc/cron.daily/dailyaidecheck" || a[7].Divert != "/etc/cron.daily/aide" {
		t.Errorf("actions 7-8 = %q %q, want the package's cron script under both of its names diverted", a[6].Divert, a[7].Divert)
	}

	// 7-8. The config parses, then our timer is on.
	if a[8].Validate != ValidateAideConfig {
		t.Errorf("action 9 = %+v, want the config check", a[8])
	}
	if s := a[9].Service; s == nil || s.Name != "unheaded-aide-check.timer" || s.Enabled == nil || !*s.Enabled || s.Active == nil || !*s.Active {
		t.Errorf("action 10 = %+v, want our timer enabled and started", a[9].Service)
	}

	// The fix never builds or updates a database: that is Stevie's, after a
	// review (rule 5).
	for i, act := range a {
		for _, s := range []string{act.Run, act.Validate} {
			if strings.Contains(s, "init") || strings.Contains(s, "update") {
				t.Errorf("action %d runs %q", i+1, s)
			}
		}
	}
	if !strings.Contains(f.Note, "DE1") {
		t.Error("the fix note does not quote DE1")
	}
	for _, c := range f.Closes {
		if strings.HasSuffix(c, ":aide-installed") {
			t.Error("the fix claims to close aide-installed, which needs the database only an attended init builds")
		}
		// A diff waits for a person: no fix closes that.
		if strings.HasSuffix(c, ":aide-unreviewed") {
			t.Error("the fix claims to close aide-unreviewed, which only a review closes")
		}
	}
	// What it does establish: our timer, and nothing else that starts AIDE.
	for _, want := range []string{"host-probe:*:aide-timer", "host-probe:*:aide-quiet"} {
		if !sliceHas(f.Closes, want) {
			t.Errorf("the fix does not close %s", want)
		}
	}
}

func sliceHas(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// No allowlisted command builds or updates an AIDE database, and the model
// refuses one that tries.
func TestAllowlistsRefuseAideInit(t *testing.T) {
	for _, list := range []map[string]bool{AllowedRun, AllowedValidate, AllowedPrecheck} {
		for cmd := range list {
			if strings.Contains(cmd, "aideinit") || strings.Contains(cmd, "--init") || strings.Contains(cmd, "--update") ||
				strings.Contains(cmd, " -i") || strings.Contains(cmd, " -u") {
				t.Errorf("allowlisted: %q", cmd)
			}
		}
	}
	for _, cmd := range []string{"aideinit -y -f", "aideinit", "aide --init", "aide -i", "aide --update", "aide -u",
		"aide --config=/etc/aide/aide-unheaded.conf --init", "unheaded-aide init", "unheaded-aide promote"} {
		for _, kind := range []string{"run", "validate"} {
			if _, err := Parse([]byte(minimal + "      - " + kind + ": \"" + cmd + "\"\n")); !errors.Is(err, ErrInvalidModel) {
				t.Errorf("%s %q: got %v, want ErrInvalidModel", kind, cmd, err)
			}
		}
	}
}

func TestParseDivert(t *testing.T) {
	if _, err := Parse([]byte(minimal + "      - divert: /etc/cron.daily/dailyaidecheck\n")); err != nil {
		t.Fatal(err)
	}
	for name, a := range map[string]string{
		"outside /etc":        "      - divert: /usr/bin/aide\n",
		"dotdot":              "      - divert: /etc/cron.daily/../shadow\n",
		"relative":            "      - divert: cron.daily/aide\n",
		"a space":             "      - divert: \"/etc/cron.daily/a b\"\n",
		"an option":           "      - divert: \"/etc/cron.daily/x --remove\"\n",
		"divert and a run":    "      - divert: /etc/cron.daily/aide\n        run: \"systemctl daemon-reload\"\n",
		"/etc itself, a tree": "      - divert: /etc/\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(minimal + a)); !errors.Is(err, ErrInvalidModel) {
				t.Fatalf("got %v, want ErrInvalidModel", err)
			}
		})
	}
}

// The units the fix writes are the ones in deploy/systemd, which the unit
// gates read: one text, two places, held equal here.
func TestAideUnitsMatchDeploy(t *testing.T) {
	f := fixByID(t, repoModel(t), "aide-install")
	for i, name := range map[int]string{2: "unheaded-aide-check.service", 3: "unheaded-aide-check.timer"} {
		raw, err := os.ReadFile(filepath.Join(repoRoot(t), "deploy/systemd", name))
		if err != nil {
			t.Fatal(err)
		}
		if got := fileText(t, f.Actions[i]); got != string(raw) {
			t.Errorf("deploy/systemd/%s differs from the text aide-install writes", name)
		}
	}
}
