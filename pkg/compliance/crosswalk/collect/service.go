// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package collect

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

// HostServices reads each first-party service a host-service ref names
// (ADR-097, amendment of 2026-11-15; FND-037): is it running, is its auth
// setting on, and does a request with no credentials get 401. One script
// per host says what runs; the request goes from the collector.
//
// A container's or a unit's environment holds passwords and tokens. The
// script's reader prints the auth keys only (the AUTH_ENABLED entry, and
// for a token variable the word "set" or "empty"), and parseService keeps
// those keys only, so no other value can reach a record, an error or a log
// line even if a reader printed more.
type HostServices struct {
	Hostname string // default os.Hostname()
	// Run executes script with sh on host (locally for the collector's own
	// host, else over ssh). Default: RunScript.
	Run func(ctx context.Context, host, script string) (string, error)
	// Table is the service table. Default: crosswalk.HostServices.
	Table map[string]crosswalk.HostService
	Now   func() time.Time
}

const (
	serviceProbeTimeout = 5 * time.Second
	serviceBodyLimit    = 64 << 10
	maxCABytes          = 1 << 20
)

var (
	serviceRefRe = regexp.MustCompile(`^([a-z0-9][a-z0-9-]{0,62}):([a-z0-9][a-z0-9-]{0,62})$`)
	// A name the script may quote: table constants only, checked again here
	// so a table edit cannot put shell syntax into a script.
	serviceNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,62}$`)
	tokenVarRe    = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)
	imageIDRe     = regexp.MustCompile(`^sha256:[0-9a-f]{12,64}$`)
	// A Debian package version starts with a digit.
	debVersionRe = regexp.MustCompile(`^[0-9][A-Za-z0-9.+~:-]{0,63}$`)
)

// serviceDockerPrelude finds a docker the collector may use: its own, or
// sudo's. Neither: every container section says "unreachable".
const serviceDockerPrelude = "if docker version --format x >/dev/null 2>&1; then D=docker; " +
	"elif sudo -n docker version --format x >/dev/null 2>&1; then D='sudo -n docker'; else D=; fi\n"

// containerFormat is the docker inspect template for one service: state,
// image, addresses, and of the environment only the auth key. For a token
// variable it prints whether the value is empty, never the value.
func containerFormat(svc crosswalk.HostService) string {
	key := `{{if eq $k "AUTH_ENABLED"}}env={{.}}{{"\n"}}{{end}}`
	if svc.AuthVar != "" {
		key = fmt.Sprintf(`{{if eq $k %q}}token={{if gt (len .) %d}}set{{else}}empty{{end}}{{"\n"}}{{end}}`,
			svc.AuthVar, len(svc.AuthVar)+1)
	}
	return `kind=container{{"\n"}}running={{.State.Running}}{{"\n"}}image={{.Image}}{{"\n"}}` +
		`{{range .NetworkSettings.Networks}}addr={{.IPAddress}}{{"\n"}}{{end}}` +
		`{{range .Config.Env}}{{$k := index (split . "=") 0}}` + key + `{{end}}`
}

// serviceSection is the sh that reads one service on its host.
func serviceSection(svc crosswalk.HostService, run crosswalk.ServiceRun) (string, error) {
	for _, v := range []string{run.Container, run.Unit, run.Package} {
		if v != "" && !serviceNameRe.MatchString(v) {
			return "", fmt.Errorf("service table: %q is not a plain name", v)
		}
	}
	if svc.AuthVar != "" && !tokenVarRe.MatchString(svc.AuthVar) {
		return "", fmt.Errorf("service table: token variable %q", svc.AuthVar)
	}
	if run.Container != "" {
		return fmt.Sprintf(`if [ -n "$D" ]; then $D container inspect --format %s %s 2>/dev/null || echo absent; else echo unreachable; fi`,
			shQuote(containerFormat(svc)), shQuote(run.Container)), nil
	}
	if run.Unit == "" || run.Package == "" {
		return "", fmt.Errorf("service table: a run needs a container, or a unit and its package")
	}
	keys := `-e 's/^\(AUTH_ENABLED=.*\)$/env=\1/p'`
	if svc.AuthVar != "" {
		keys = fmt.Sprintf(`-e 's/^%[1]s=..*$/token=set/p' -e 's/^%[1]s=$/token=empty/p'`, svc.AuthVar)
	}
	unit := shQuote(run.Unit + ".service")
	return `st=$(systemctl show -p ActiveState --value ` + unit + `) || st=; ` +
		`if [ -z "$st" ]; then echo unreachable; else echo kind=unit; echo "running=$st"; ` +
		`echo "image=$(dpkg-query -W -f='${Version}' ` + shQuote(run.Package) + ` 2>/dev/null)"; ` +
		`pid=$(systemctl show -p MainPID --value ` + unit + `); ` +
		`if [ "${pid:-0}" -gt 0 ] 2>/dev/null; then ` +
		`if sudo -n test -r "/proc/$pid/environ"; then sudo -n cat "/proc/$pid/environ" | tr '\0' '\n' | sed -n ` + keys + `; ` +
		`else echo environ=unreadable; fi; fi; fi`, nil
}

// serviceState is what one section said, auth keys only.
type serviceState struct {
	unreachable bool // docker or systemd did not answer, or the environment could not be read
	absent      bool // no such container
	kind        string
	running     bool
	version     string // image id or package version, "" when it is not one
	addrs       []string
	authSeen    int
	authTrue    bool
	token       string // "set", "empty" or ""
}

// parseService reads a section. It keeps the keys it knows and drops every
// other line unread: nothing else is stored, so nothing else can be
// reported.
func parseService(sec string) serviceState {
	var st serviceState
	for _, line := range strings.Split(sec, "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case "unreachable", "environ=unreadable":
			st.unreachable = true
			continue
		case "absent":
			st.absent = true
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "kind":
			if st.kind == "" && (v == "container" || v == "unit") {
				st.kind = v
			}
		case "running":
			st.running = v == "true" || v == "active"
		case "image":
			if imageIDRe.MatchString(v) {
				st.version = v[:len("sha256:")+12]
			} else if debVersionRe.MatchString(v) {
				st.version = v
			}
		case "addr":
			if a, err := netip.ParseAddr(v); err == nil {
				st.addrs = append(st.addrs, a.String())
			}
		case "env":
			if name, val, ok := strings.Cut(v, "="); ok && name == "AUTH_ENABLED" {
				st.authSeen++
				// pkg/auth turns authentication on for the exact string
				// "true" and nothing else (pkg/auth/setup.go).
				st.authTrue = val == "true"
			}
		case "token":
			if v == "set" || v == "empty" {
				st.token = v
			}
		}
	}
	return st
}

// probeService sends one request with no credentials and returns the
// status. An error means nothing verifiable answered.
func probeService(ctx context.Context, svc crosswalk.HostService, run crosswalk.ServiceRun, addr string) (int, error) {
	// The path is a constant of the service table; os.Root keeps the read
	// inside its directory whatever the name is.
	dir, err := os.OpenRoot(filepath.Dir(run.CA))
	if err != nil {
		return 0, fmt.Errorf("CA file: %w", err)
	}
	f, err := dir.Open(filepath.Base(run.CA))
	_ = dir.Close()
	if err != nil {
		return 0, fmt.Errorf("CA file: %w", err)
	}
	pemBytes, err := io.ReadAll(io.LimitReader(f, maxCABytes))
	_ = f.Close()
	if err != nil {
		return 0, fmt.Errorf("CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return 0, fmt.Errorf("CA file holds no certificate")
	}
	client := &http.Client{
		Timeout: serviceProbeTimeout,
		Transport: &http.Transport{
			TLSClientConfig:       &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13, ServerName: run.TLSName},
			TLSHandshakeTimeout:   serviceProbeTimeout,
			ResponseHeaderTimeout: serviceProbeTimeout,
			DisableKeepAlives:     true,
		},
		// The answer to this request is the evidence, not where it points.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	ctx, cancel := context.WithTimeout(ctx, serviceProbeTimeout)
	defer cancel()
	url := "https://" + net.JoinHostPort(addr, strconv.Itoa(svc.Port)) + svc.Route
	req, err := http.NewRequestWithContext(ctx, svc.Method, url, http.NoBody)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, serviceBodyLimit))
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

// Collect returns one record per host-service source that could be read.
func (h *HostServices) Collect(ctx context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	table := h.Table
	if table == nil {
		table = crosswalk.HostServices
	}
	local := h.Hostname
	if local == "" {
		local, _ = os.Hostname()
	}
	run := h.Run
	if run == nil {
		run = func(ctx context.Context, host, script string) (string, error) {
			return RunScript(ctx, host, script, host == local)
		}
	}
	type check struct {
		src  crosswalk.Source
		name string
		svc  crosswalk.HostService
		run  crosswalk.ServiceRun
	}
	byHost := map[string][]check{}
	var hosts []string
	for _, s := range sources {
		if s.Kind != crosswalk.KindHostService {
			continue
		}
		m := serviceRefRe.FindStringSubmatch(s.Ref)
		if m == nil {
			return nil, fmt.Errorf("host-service ref %q must be <host>:<service>", s.Ref)
		}
		svc, ok := table[m[2]]
		if !ok || svc.Exempt != "" {
			return nil, fmt.Errorf("host-service ref %q: %q is not a service to read", s.Ref, m[2])
		}
		r, ok := svc.Runs[m[1]]
		if !ok {
			return nil, fmt.Errorf("host-service ref %q: %s does not run on %s", s.Ref, m[2], m[1])
		}
		switch svc.Method {
		case http.MethodGet, http.MethodPost, http.MethodDelete:
		default:
			return nil, fmt.Errorf("host-service ref %q: method %q", s.Ref, svc.Method)
		}
		if _, ok := byHost[m[1]]; !ok {
			hosts = append(hosts, m[1])
		}
		byHost[m[1]] = append(byHost[m[1]], check{s, m[2], svc, r})
	}
	var recs []crosswalk.Record
	for _, host := range hosts {
		checks := byHost[host]
		sort.SliceStable(checks, func(i, j int) bool { return checks[i].name < checks[j].name })
		tag, err := sectionTag()
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		b.WriteString(probePath)
		b.WriteString(serviceDockerPrelude)
		seen := map[string]bool{}
		for _, c := range checks {
			if seen[c.name] {
				continue
			}
			seen[c.name] = true
			sec, err := serviceSection(c.svc, c.run)
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(&b, "echo @@%s %s; { %s; } </dev/null 2>/dev/null\n", tag, c.name, sec)
		}
		b.WriteString("exit 0\n")
		out, err := run(ctx, host, b.String())
		if err != nil {
			continue // unreachable: no verdicts
		}
		sections, forged := splitSections(out, tag)
		for _, c := range checks {
			sec, ok := sections[c.name]
			if !ok || forged[c.name] {
				continue
			}
			pass, detail, assessed := judgeService(ctx, c.name, c.svc, c.run, parseService(sec))
			if !assessed {
				continue
			}
			v := crosswalk.VerdictFail
			if pass {
				v = crosswalk.VerdictPass
			}
			recs = append(recs, crosswalk.Record{Source: c.src, Verdict: v, ObservedAt: now(), Detail: host + ": " + detail})
		}
	}
	return recs, nil
}

// judgeService turns one service's state, and if that is in order one
// request, into a verdict. assessed is false when nothing could be read.
func judgeService(ctx context.Context, name string, svc crosswalk.HostService, run crosswalk.ServiceRun, st serviceState) (pass bool, detail string, assessed bool) {
	what := "container " + run.Container
	wantKind := "container"
	if run.Container == "" {
		what, wantKind = "unit "+run.Unit, "unit"
	}
	if st.unreachable {
		return false, "", false
	}
	if st.absent {
		return false, fmt.Sprintf("%s is not running (no %s)", name, what), true
	}
	if st.kind != wantKind {
		return false, "", false // not the reading this entry asks for
	}
	if !st.running {
		return false, fmt.Sprintf("%s is not running (%s)", name, what), true
	}
	version := st.version
	if version == "" {
		version = "version not read"
	}
	if run.Container != "" {
		version = "image " + version
	} else {
		version = run.Package + " " + version
	}

	var auth string
	if svc.AuthVar != "" {
		switch st.token {
		case "set":
			auth = svc.AuthVar + " is set"
		case "empty":
			return false, fmt.Sprintf("%s running (%s): %s is empty, so it has no token to check", name, version, svc.AuthVar), true
		default:
			return false, fmt.Sprintf("%s running (%s): %s is not set", name, version, svc.AuthVar), true
		}
	} else {
		switch {
		case st.authSeen == 0:
			return false, fmt.Sprintf("%s running (%s): AUTH_ENABLED is not set, so pkg/auth admits every request", name, version), true
		case st.authSeen > 1:
			return false, fmt.Sprintf("%s running (%s): AUTH_ENABLED is set more than once", name, version), true
		case !st.authTrue:
			return false, fmt.Sprintf(`%s running (%s): AUTH_ENABLED is not "true", so pkg/auth admits every request`, name, version), true
		}
		auth = "AUTH_ENABLED=true"
	}

	addrs := st.addrs
	if run.Addr != "" {
		addrs = []string{run.Addr}
	}
	for _, addr := range addrs {
		status, err := probeService(ctx, svc, run, addr)
		if err != nil {
			continue
		}
		req := svc.Method + " " + svc.Route
		switch {
		case status == http.StatusUnauthorized:
			return true, fmt.Sprintf("%s running (%s), %s, %s without credentials answers 401", name, version, auth, req), true
		case status >= 200 && status < 300:
			return false, fmt.Sprintf("%s running (%s), %s, but it answers without a login: %s without credentials answers %d", name, version, auth, req, status), true
		default:
			return false, fmt.Sprintf("%s running (%s), %s, but %s without credentials answers %d, not 401", name, version, auth, req, status), true
		}
	}
	return false, "", false // nothing verifiable answered: the 401 was not seen
}
