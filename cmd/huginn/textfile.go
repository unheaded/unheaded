// SPDX-License-Identifier: GPL-3.0-or-later
//
// textfile.go — metrics that other processes leave in a directory, the
// node_exporter textfile pattern (ERDA 1.4). Root-run jobs that huginn
// cannot observe itself (The Well's backup, restore drill and offsite copy)
// write <name>.prom there on success; huginn forwards their samples with
// its host label, plus each file's age.
//
// Only lines that look like the Prometheus text format pass: # HELP/# TYPE
// comments and `name{labels} value`. Anything else is dropped and counted,
// so a stray file cannot inject arbitrary text into the push.
//
// Emitted metrics (all labelled host=):
//   <whatever the files hold>
//   huginn_textfile_mtime_seconds{file}   when the file was last written
//   huginn_textfile_rejected_lines{file}  lines dropped as malformed
//   huginn_textfile_ok                    1 if the directory was read

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	textfileMaxFiles = 64
	textfileMaxBytes = 64 << 10
)

var (
	textfileSample  = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^{}]*\})?[ \t]+([-+]?([0-9]*\.?[0-9]+([eE][-+]?[0-9]+)?|Inf|NaN))$`)
	textfileComment = regexp.MustCompile(`^# (HELP|TYPE) [a-zA-Z_:][a-zA-Z0-9_:]*( .*)?$`)
	textfileLabel   = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*="([^"\\]|\\.)*"$`)
)

// TextFile is one .prom file's accepted lines.
type TextFile struct {
	Name     string
	MTime    int64
	Lines    []string // samples with host added, and comments
	Rejected int
}

// collectTextfiles reads every regular *.prom file in dir (not recursive).
// ok is false when the directory cannot be read.
func collectTextfiles(dir, host string) (files []TextFile, ok bool) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	for _, e := range ents {
		if len(files) >= textfileMaxFiles {
			break
		}
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".prom") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name())) // #nosec G304 -- a name listed in the configured directory, regular files only
		if err != nil {
			continue
		}
		tf := TextFile{Name: e.Name(), MTime: info.ModTime().Unix()}
		sc := bufio.NewScanner(io.LimitReader(f, textfileMaxBytes))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			if out, good := textfileLine(line, host); good {
				tf.Lines = append(tf.Lines, out)
			} else {
				tf.Rejected++
			}
		}
		_ = f.Close()
		files = append(files, tf)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, true
}

// textfileLine validates one line and adds host to a sample's labels
// (unless the sample already has a host label).
func textfileLine(line, host string) (string, bool) {
	if strings.HasPrefix(line, "#") {
		return line, textfileComment.MatchString(line)
	}
	m := textfileSample.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	name, labels, value := m[1], strings.TrimSuffix(strings.TrimPrefix(m[2], "{"), "}"), m[3]
	var parts []string
	if strings.TrimSpace(labels) != "" {
		for _, p := range splitLabels(labels) {
			p = strings.TrimSpace(p)
			if !textfileLabel.MatchString(p) {
				return "", false
			}
			if strings.HasPrefix(p, "host=") {
				host = ""
			}
			parts = append(parts, p)
		}
	}
	if host != "" {
		parts = append([]string{fmt.Sprintf("host=%q", host)}, parts...)
	}
	return fmt.Sprintf("%s{%s} %s", name, strings.Join(parts, ","), value), true
}

// splitLabels splits on commas outside quoted values.
func splitLabels(s string) []string {
	var out []string
	var b strings.Builder
	inQuote, esc := false, false
	for _, r := range s {
		switch {
		case esc:
			esc = false
		case r == '\\' && inQuote:
			esc = true
		case r == '"':
			inQuote = !inQuote
		case r == ',' && !inQuote:
			out = append(out, b.String())
			b.Reset()
			continue
		}
		b.WriteRune(r)
	}
	if strings.TrimSpace(b.String()) != "" {
		out = append(out, b.String())
	}
	return out
}

func promTextTextfiles(host string, files []TextFile, ok bool) string {
	var b strings.Builder
	okv := 0
	if ok {
		okv = 1
	}
	fmt.Fprintf(&b, "huginn_textfile_ok{host=%q} %d\n", host, okv)
	for _, f := range files {
		for _, l := range f.Lines {
			b.WriteString(l)
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "huginn_textfile_mtime_seconds{host=%q,file=%q} %d\n", host, f.Name, f.MTime)
		fmt.Fprintf(&b, "huginn_textfile_rejected_lines{host=%q,file=%q} %d\n", host, f.Name, f.Rejected)
	}
	return b.String()
}
