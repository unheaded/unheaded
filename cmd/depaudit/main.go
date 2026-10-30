// SPDX-License-Identifier: GPL-3.0-or-later

// Command depaudit reports how old each Go module dependency's latest
// release is (S-LOW-FRUIT Phase 10). An unmaintained module never gets a
// security advisory, so age is a risk class of its own; docs/policy/
// dependency-watch.md records a decision for every direct module past the
// limit, and scripts/check-dep-staleness.sh holds that line.
//
// It reads `go list -m -json all` on stdin and asks the module proxy for
// each module's latest version and its release time. Only the standard
// library is used.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"unheaded/pkg/cmdline"
)

// module is the part of `go list -m -json` output depaudit uses.
type module struct {
	Path     string
	Version  string
	Main     bool
	Indirect bool
	Replace  *struct{ Path, Version string }
}

// latest is the module proxy's @latest answer.
type latest struct {
	Version string
	Time    time.Time
}

// row is one line of the report.
type row struct {
	Path, Pinned, Latest string
	Released             time.Time
	Months               int
	Direct, Stale        bool
	Err                  string
}

func main() {
	maxAge := flag.Int("max-age-months", 24, "a module whose latest release is older than `N` months is STALE")
	all := flag.Bool("all", false, "report indirect modules too (default: direct only)")
	failStale := flag.Bool("fail-on-stale-direct", false, "exit 1 when a direct module is STALE")
	proxy := flag.String("proxy", "https://proxy.golang.org", "module proxy base `URL`")
	cmdline.Parse(cmdline.Spec{
		Name:    "depaudit",
		Purpose: "Report the latest-release age of every Go module dependency, from `go list -m -json all` on stdin.",
		Examples: []string{
			"go list -m -json all | depaudit",
			"go list -m -json all | depaudit --max-age-months 36 --fail-on-stale-direct",
		},
		Details: "Exit codes: 0 report printed, 1 a direct module is STALE (with --fail-on-stale-direct)\nor a proxy lookup failed, 2 usage error.",
	})

	mods, err := readModules(os.Stdin)
	if err != nil {
		cmdline.Fail(cmdline.Spec{Name: "depaudit"}, "%v", err)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	rows := audit(mods, *all, *maxAge, time.Now(), func(path string) (latest, error) {
		return fetchLatest(client, *proxy, path)
	})
	printReport(os.Stdout, rows, *maxAge)

	bad := false
	for _, r := range rows {
		if r.Err != "" || (*failStale && r.Direct && r.Stale) {
			bad = true
		}
	}
	if bad {
		os.Exit(1)
	}
}

// readModules decodes the stream of JSON objects `go list -m -json` prints.
func readModules(r io.Reader) ([]module, error) {
	dec := json.NewDecoder(bufio.NewReader(r))
	var mods []module
	for {
		var m module
		err := dec.Decode(&m)
		if errors.Is(err, io.EOF) {
			return mods, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reading go list -m -json output: %w", err)
		}
		mods = append(mods, m)
	}
}

// audit builds the report. Main modules and modules replaced by a local
// path are skipped (nothing to fetch); indirect ones only with all.
func audit(mods []module, all bool, maxAge int, now time.Time, get func(string) (latest, error)) []row {
	var rows []row
	for _, m := range mods {
		if m.Main || (m.Indirect && !all) {
			continue
		}
		if m.Replace != nil && m.Replace.Version == "" {
			continue // replaced by a directory: ours
		}
		r := row{Path: m.Path, Pinned: m.Version, Direct: !m.Indirect}
		l, err := get(m.Path)
		if err != nil {
			r.Err = err.Error()
			rows = append(rows, r)
			continue
		}
		r.Latest, r.Released = l.Version, l.Time
		r.Months = monthsBetween(l.Time, now)
		r.Stale = r.Months > maxAge
		rows = append(rows, r)
	}
	return rows
}

// monthsBetween counts whole months from a to b (30.44-day months).
func monthsBetween(a, b time.Time) int {
	if b.Before(a) {
		return 0
	}
	return int(b.Sub(a).Hours() / (24 * 30.436875))
}

// escapePath applies the module proxy's case encoding: each upper-case
// letter becomes '!' followed by its lower-case form.
func escapePath(p string) string {
	var b strings.Builder
	for _, c := range p {
		if unicode.IsUpper(c) {
			b.WriteByte('!')
			b.WriteRune(unicode.ToLower(c))
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

// fetchLatest asks the proxy for a module's latest version and time.
func fetchLatest(c *http.Client, base, path string) (latest, error) {
	u := strings.TrimRight(base, "/") + "/" + escapePath(path) + "/@latest"
	resp, err := c.Get(u) // #nosec G107 -- proxy base from a flag, path from go list
	if err != nil {
		return latest{}, fmt.Errorf("%s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return latest{}, fmt.Errorf("%s: proxy answered %s", path, resp.Status)
	}
	var l latest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&l); err != nil {
		return latest{}, fmt.Errorf("%s: %w", path, err)
	}
	if l.Version == "" || l.Time.IsZero() {
		return latest{}, fmt.Errorf("%s: proxy answer has no version or time", path)
	}
	return l, nil
}

func printReport(w io.Writer, rows []row, maxAge int) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "MODULE\tPINNED\tLATEST\tRELEASED\tMONTHS\tKIND\tSTATUS")
	for _, r := range rows {
		kind := "direct"
		if !r.Direct {
			kind = "indirect"
		}
		if r.Err != "" {
			fmt.Fprintf(tw, "%s\t%s\t-\t-\t-\t%s\tERROR %s\n", r.Path, r.Pinned, kind, r.Err)
			continue
		}
		status := "ok"
		if r.Stale {
			status = fmt.Sprintf("STALE (>%d months)", maxAge)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n", r.Path, r.Pinned, r.Latest, r.Released.Format("2006-01-02"), r.Months, kind, status)
	}
	_ = tw.Flush() // stdout; nothing useful to do if it fails
}
