// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// unheaded-baseline is the host baseline agent (ADR-098), audit mode only.
//
// It evaluates this host's entries of the shipped baseline with the same
// collectors the central compliance pull uses, and reports every deviation
// with what enforcing mode would have done about it (the audit-mode record,
// like SELinux logging permissive=1 denials). It changes nothing on the
// host: this build has no enforcing code path, and a config asking for
// enforcing is reported as a critical event and run as audit.
//
// Outputs: /var/lib/unheaded/baseline/report.json, one log line per
// deviation (journald via the unit's stdout), and with -textfile-dir a
// node-exporter textfile. Exit 0 when the checks ran (deviations are data),
// 1 when the host could not be evaluated.
//
//	unheaded-baseline                      # as root, from the systemd timer
//	unheaded-baseline -hostname east -textfile-dir /var/lib/node_exporter/textfile
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"unheaded/pkg/compliance/crosswalk"
	"unheaded/pkg/compliance/crosswalk/collect"
)

const (
	modeAudit     = "audit"
	modeEnforcing = "enforcing"
	maxFileBytes  = 256 << 10
)

type options struct {
	baselinePath string
	configPath   string
	stateDir     string
	textfileDir  string
	hostname     string
	procSys      string
	trustedUID   int // owner the baseline must have; root in production
	now          func() time.Time
	sshdDump     func(ctx context.Context, host string) (string, error)
	runProbes    func(ctx context.Context, host, script string) (string, error)
}

type check struct {
	Key          string `json:"key"`
	Verdict      string `json:"verdict"` // pass | fail | unevaluated
	Detail       string `json:"detail,omitempty"`
	WouldEnforce string `json:"would_enforce,omitempty"`
}

type report struct {
	Version        int       `json:"version"`
	Host           string    `json:"host"`
	ModeConfig     string    `json:"mode_config"`
	ModeEffective  string    `json:"mode_effective"`
	ConfigError    string    `json:"config_error,omitempty"`
	ObservedAt     time.Time `json:"observed_at"`
	BaselineSHA256 string    `json:"baseline_sha256"`
	Evaluated      int       `json:"evaluated"`
	Deviations     int       `json:"deviations"`
	Unevaluated    int       `json:"unevaluated"`
	Checks         []check   `json:"checks"`
}

func main() {
	var o options
	flag.StringVar(&o.baselinePath, "baseline", "/usr/share/unheaded/baseline/baseline.yaml", "baseline file (root-owned)")
	flag.StringVar(&o.configPath, "config", "/etc/unheaded/baseline.conf", "mode file: mode=audit|enforcing")
	flag.StringVar(&o.stateDir, "state-dir", "/var/lib/unheaded/baseline", "where report.json is written")
	flag.StringVar(&o.textfileDir, "textfile-dir", "", "node-exporter textfile directory (optional)")
	flag.StringVar(&o.hostname, "hostname", "", "host name in the baseline (default: os.Hostname)")
	timeout := flag.Duration("timeout", 2*time.Minute, "give up after this long")
	flag.Parse()
	o.trustedUID = 0

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if _, err := run(ctx, o, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "unheaded-baseline:", err)
		os.Exit(1)
	}
}

var hostRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func run(ctx context.Context, o options, logw io.Writer) (*report, error) {
	now := time.Now
	if o.now != nil {
		now = o.now
	}
	host := o.hostname
	if host == "" {
		h, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		host = strings.ToLower(strings.SplitN(h, ".", 2)[0])
	}
	if !hostRe.MatchString(host) {
		return nil, fmt.Errorf("hostname %q is not a baseline host name", host)
	}

	if err := checkTrust(o.baselinePath, o.trustedUID); err != nil {
		return nil, err
	}
	raw, err := readBounded(o.baselinePath)
	if err != nil {
		return nil, err
	}
	base, err := collect.ParseBaseline(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", o.baselinePath, err)
	}
	sources := base.ForHost(host)
	if len(sources) == 0 {
		return nil, fmt.Errorf("no baseline checks for host %q in %s", host, o.baselinePath)
	}
	sum := sha256.Sum256(raw)

	rep := &report{Version: 1, Host: host, ModeEffective: modeAudit, ObservedAt: now().UTC(),
		BaselineSHA256: hex.EncodeToString(sum[:])}
	rep.ModeConfig, rep.ConfigError = readMode(o.configPath)
	switch {
	case rep.ConfigError != "":
		logf(logw, "CRITICAL host=%s config %s: %s; running audit", host, o.configPath, rep.ConfigError)
	case rep.ModeConfig == modeEnforcing:
		logf(logw, "CRITICAL enforcing requested host=%s: not available in this build (ADR-098 phase 4); running audit", host)
	}

	recs, err := evaluate(ctx, o, host, sources, now)
	if err != nil {
		return nil, err
	}
	latest := map[string]crosswalk.Record{}
	for _, r := range recs {
		latest[r.Source.Key()] = r
	}
	for _, s := range sources {
		c := check{Key: s.Key(), Verdict: "unevaluated"}
		if r, ok := latest[s.Key()]; ok {
			c.Verdict, c.Detail = string(r.Verdict), r.Detail
		}
		switch c.Verdict {
		case string(crosswalk.VerdictFail):
			rep.Evaluated++
			rep.Deviations++
			c.WouldEnforce = wouldEnforce(s)
			logf(logw, "DEVIATION host=%s mode=%s key=%q detail=%q would_enforce=%q",
				host, rep.ModeEffective, c.Key, c.Detail, c.WouldEnforce)
		case string(crosswalk.VerdictPass):
			rep.Evaluated++
		default:
			rep.Unevaluated++
			logf(logw, "UNEVALUATED host=%s key=%q", host, c.Key)
		}
		rep.Checks = append(rep.Checks, c)
	}
	if rep.Evaluated == 0 {
		return nil, fmt.Errorf("no check could be evaluated on %s (%d in the baseline)", host, len(sources))
	}
	logf(logw, "SUMMARY host=%s mode_config=%s mode_effective=%s evaluated=%d deviations=%d unevaluated=%d",
		host, rep.ModeConfig, rep.ModeEffective, rep.Evaluated, rep.Deviations, rep.Unevaluated)

	body, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeAtomic(o.stateDir, "report.json", append(body, '\n')); err != nil {
		return nil, err
	}
	if o.textfileDir != "" {
		if err := writeAtomic(o.textfileDir, "unheaded_baseline.prom", textfile(rep)); err != nil {
			return nil, err
		}
	}
	return rep, nil
}

// evaluate runs the shared collectors for this host only. Each evaluator is
// wrapped to refuse any other host: the agent never reaches over the network.
func evaluate(ctx context.Context, o options, host string, sources []crosswalk.Source, now func() time.Time) ([]crosswalk.Record, error) {
	refuse := func(h string) error { return fmt.Errorf("agent evaluates only %s, not %s", host, h) }
	dump := o.sshdDump
	if dump == nil {
		dump = func(ctx context.Context, h string) (string, error) { return collect.SSHDDump(ctx, h, true) }
	}
	runProbes := o.runProbes
	if runProbes == nil {
		runProbes = func(ctx context.Context, h, script string) (string, error) {
			return collect.RunScript(ctx, h, script, true)
		}
	}
	collectors := []interface {
		Collect(context.Context, []crosswalk.Source) ([]crosswalk.Record, error)
	}{
		&collect.HostSysctl{ProcSys: o.procSys, Hostname: host, Now: now,
			Remote: func(_ context.Context, h string, _ []string) (map[string]string, error) { return nil, refuse(h) }},
		&collect.HostSSHD{Hostname: host, Now: now, Dump: func(ctx context.Context, h string) (string, error) {
			if h != host {
				return "", refuse(h)
			}
			return dump(ctx, h)
		}},
		&collect.HostProbes{Hostname: host, Now: now, Run: func(ctx context.Context, h, script string) (string, error) {
			if h != host {
				return "", refuse(h)
			}
			return runProbes(ctx, h, script)
		}},
	}
	var recs []crosswalk.Record
	for _, c := range collectors {
		r, err := c.Collect(ctx, sources)
		if err != nil {
			return nil, err
		}
		recs = append(recs, r...)
	}
	return recs, nil
}

// probeActions says what enforcing mode would do for each baseline probe
// (ADR-098 section 2). Descriptions only: nothing here is executed.
var probeActions = map[string]string{
	"firewall-inbound-deny":       "load nft table inet unheaded (last operator-confirmed ruleset)",
	"auditd-running":              "systemctl unmask auditd; systemctl enable --now auditd",
	"ntp-synchronized":            "systemctl enable --now systemd-timesyncd",
	"unattended-upgrades-enabled": `set APT::Periodic::Unattended-Upgrade "1" in apt.conf.d`,
	"apparmor-enabled":            "none: needs a reboot with apparmor=1 (alert only)",
}

func init() {
	// Generated probe families (collect/cisprobes.go).
	for name := range collect.Probes {
		switch {
		case strings.HasPrefix(name, "kmod-") && strings.HasSuffix(name, "-disabled"):
			mod := strings.TrimSuffix(strings.TrimPrefix(name, "kmod-"), "-disabled")
			probeActions[name] = fmt.Sprintf("write /etc/modprobe.d/%s.conf (install %s /bin/false; blacklist %s); unload it if loaded and unused", mod, mod, mod)
		case strings.HasPrefix(name, "svc-"):
			probeActions[name] = "systemctl disable --now and mask the units; the package stays (removal is the operator's decision)"
		case strings.HasPrefix(name, "pkg-"):
			probeActions[name] = "none (alert only): removing a package can take dependents with it; the operator runs apt purge"
		case name == "cron-active":
			probeActions[name] = "systemctl enable --now cron"
		case name == "cron-allow-restricted", name == "at-restricted":
			probeActions[name] = "create the .allow file (root, mode 0640, CIS group) listing root; remove cron.deny"
		case name == "mta-local-only":
			probeActions[name] = "none (alert only): set the MTA to loopback-only (postfix inet_interfaces = loopback-only) and restart it"
		case strings.HasPrefix(name, "acct-"):
			probeActions[name] = "none (alert only): account database changes need a human; the detail names each offender"
		case name == "apparmor-installed", name == "sudo-installed":
			probeActions[name] = "none (alert only): the operator installs the package"
		case name == "apparmor-profiles-enforced":
			probeActions[name] = "none (alert only): moving a profile to enforce can break the confined program"
		case name == "grub-password":
			probeActions[name] = "none (alert only): a boot loader password is set by the operator, who must keep it"
		case strings.HasPrefix(name, "sudo-"):
			probeActions[name] = "write /etc/sudoers.d/00-unheaded (Defaults use_pty, logfile, timestamp_timeout=15), visudo -c first; remove !authenticate lines"
		case strings.HasPrefix(name, "mount-"):
			probeActions[name] = "none (alert only): mount options and partitions change through fstab and a remount or reboot, which the operator does"
		case strings.HasPrefix(name, "perm-"):
			probeActions[name] = "chown to root (and the CIS group) and chmod to the CIS mode; the detail names each file"
		}
	}
}

var (
	sysctlRef = regexp.MustCompile(`^[a-z0-9-]+:([a-z0-9_.-]+)(>=|<=|=)(-?[0-9]+)$`)
	sshdRef   = regexp.MustCompile(`^[a-z0-9-]+:([a-z0-9]+)(>=|<=|=)(.+)$`)
)

func wouldEnforce(s crosswalk.Source) string {
	switch s.Kind {
	case crosswalk.KindHostSysctl:
		if m := sysctlRef.FindStringSubmatch(s.Ref); m != nil {
			return fmt.Sprintf("sysctl -w %s=%s (and persist in sysctl.d)", m[1], m[3])
		}
	case crosswalk.KindHostSSHD:
		if m := sshdRef.FindStringSubmatch(s.Ref); m != nil {
			return fmt.Sprintf("set %s %s in sshd_config.d/00-unheaded.conf; sshd -t; reload (never on failure)", sshdKeyword(m[1]), m[3])
		}
	case crosswalk.KindHostProbe:
		if _, name, ok := strings.Cut(s.Ref, ":"); ok {
			if a, ok := probeActions[name]; ok {
				return a
			}
		}
	}
	return "none defined (alert only)"
}

// sshdKeyword restores sshd_config capitalisation for the keys the baseline
// uses; `sshd -T` reports them lower-case.
func sshdKeyword(k string) string {
	known := map[string]string{
		"passwordauthentication": "PasswordAuthentication", "kbdinteractiveauthentication": "KbdInteractiveAuthentication",
		"permitemptypasswords": "PermitEmptyPasswords", "permitrootlogin": "PermitRootLogin",
		"x11forwarding": "X11Forwarding", "maxauthtries": "MaxAuthTries",
		"clientaliveinterval": "ClientAliveInterval", "clientalivecountmax": "ClientAliveCountMax",
	}
	if v, ok := known[k]; ok {
		return v
	}
	return k
}

// readMode returns the configured mode and, when the file is malformed,
// why. A missing file is audit. Anything unexpected is audit plus an error,
// never a guess: the only other mode is spelled exactly "enforcing".
func readMode(path string) (mode, problem string) {
	raw, err := readBounded(path)
	if errors.Is(err, fs.ErrNotExist) {
		return modeAudit, ""
	}
	if err != nil {
		return modeAudit, err.Error()
	}
	mode = ""
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch {
		case !ok:
			return modeAudit, fmt.Sprintf("line %d: want key=value", n)
		case k != "mode":
			return modeAudit, fmt.Sprintf("line %d: unknown key %q", n, k)
		case mode != "":
			return modeAudit, fmt.Sprintf("line %d: mode set twice", n)
		case v != modeAudit && v != modeEnforcing:
			return modeAudit, fmt.Sprintf("line %d: mode %q is not audit or enforcing", n, v)
		}
		mode = v
	}
	if mode == "" {
		return modeAudit, ""
	}
	return mode, ""
}

// checkTrust refuses a baseline anyone but uid could have written: it must
// be a regular file (not a symlink) owned by uid, not group or world
// writable, in a directory with the same properties (ssh StrictModes).
func checkTrust(path string, uid int) error {
	for i, p := range []string{path, filepath.Dir(path)} {
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if i == 0 && !fi.Mode().IsRegular() {
			return fmt.Errorf("%s: not a regular file", p)
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("%s: cannot read owner", p)
		}
		if int(st.Uid) != uid {
			return fmt.Errorf("%s: owned by uid %d, want %d", p, st.Uid, uid)
		}
		if fi.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("%s: writable by group or others (%v)", p, fi.Mode().Perm())
		}
	}
	return nil
}

func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- operator-supplied path, trust-checked where it matters
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxFileBytes {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, maxFileBytes)
	}
	return b, nil
}

func writeAtomic(dir, name string, body []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- report is not secret
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

func logf(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, format+"\n", args...)
}

func escapeLabel(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

// textfile renders node-exporter textfile metrics for the report.
func textfile(r *report) []byte {
	var b strings.Builder
	gauge := func(name, help string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
	}
	gauge("unheaded_baseline_check", "1 if the baseline check passes, 0 if it deviates (ADR-098)")
	checks := append([]check(nil), r.Checks...)
	sort.Slice(checks, func(i, j int) bool { return checks[i].Key < checks[j].Key })
	for _, c := range checks {
		if c.Verdict == "unevaluated" {
			continue
		}
		v := 0
		if c.Verdict == string(crosswalk.VerdictPass) {
			v = 1
		}
		fmt.Fprintf(&b, "unheaded_baseline_check{key=\"%s\"} %d\n", escapeLabel(c.Key), v)
	}
	gauge("unheaded_baseline_deviations", "Baseline checks deviating at the last run")
	fmt.Fprintf(&b, "unheaded_baseline_deviations %d\n", r.Deviations)
	gauge("unheaded_baseline_unevaluated", "Baseline checks that could not be read at the last run")
	fmt.Fprintf(&b, "unheaded_baseline_unevaluated %d\n", r.Unevaluated)
	gauge("unheaded_baseline_mode_config", "Mode requested by /etc/unheaded/baseline.conf")
	fmt.Fprintf(&b, "unheaded_baseline_mode_config{mode=\"%s\"} 1\n", escapeLabel(r.ModeConfig))
	gauge("unheaded_baseline_config_error", "1 if the mode file is malformed")
	cfgErr := 0
	if r.ConfigError != "" {
		cfgErr = 1
	}
	fmt.Fprintf(&b, "unheaded_baseline_config_error %d\n", cfgErr)
	gauge("unheaded_baseline_enforcing", "1 if enforcing is in effect (never, in this build)")
	b.WriteString("unheaded_baseline_enforcing 0\n")
	gauge("unheaded_baseline_last_run_timestamp_seconds", "When the agent last completed a run")
	fmt.Fprintf(&b, "unheaded_baseline_last_run_timestamp_seconds %s\n", strconv.FormatInt(r.ObservedAt.Unix(), 10))
	return []byte(b.String())
}
