// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTextfileLine(t *testing.T) {
	for in, want := range map[string]string{
		"unheaded_well_backup_last_success_timestamp_seconds 1792078339": `unheaded_well_backup_last_success_timestamp_seconds{host="west"} 1792078339`,
		`x_total{db="a,b",k="v"} 3`:                                      `x_total{host="west",db="a,b",k="v"} 3`,
		`x{host="east"} 1.5e3`:                                           `x{host="east"} 1.5e3`,
		`x{} NaN`:                                                        `x{host="west"} NaN`,
		"# HELP x Some help.":                                            "# HELP x Some help.",
		"# TYPE x gauge":                                                 "# TYPE x gauge",
		`esc{msg="a \"quoted\", comma"} -2`:                              `esc{host="west",msg="a \"quoted\", comma"} -2`,
	} {
		got, ok := textfileLine(in, "west")
		if !ok || got != want {
			t.Errorf("%q -> %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{
		"rm -rf /",
		"x 1 2",
		`x{a=b} 1`,
		"x{a=\"1\"}} 1",
		"# random comment",
		"9x 1",
		"x value",
		`x{a="1"} 1 # trailing`,
	} {
		if got, ok := textfileLine(bad, "west"); ok {
			t.Errorf("accepted %q as %q", bad, got)
		}
	}
}

func TestCollectTextfiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("well_backup.prom", "# TYPE a gauge\na 1\nnot a metric\n")
	write(".well_backup.prom.tmp", "half 1\n")
	write("notes.txt", "b 2\n")
	if err := os.Mkdir(filepath.Join(dir, "sub.prom"), 0o700); err != nil {
		t.Fatal(err)
	}
	files, ok := collectTextfiles(dir, "west")
	if !ok || len(files) != 1 || files[0].Name != "well_backup.prom" || files[0].Rejected != 1 || len(files[0].Lines) != 2 {
		t.Fatalf("got %+v, %v", files, ok)
	}
	out := promTextTextfiles("west", files, ok)
	for _, want := range []string{
		"huginn_textfile_ok{host=\"west\"} 1\n",
		"a{host=\"west\"} 1\n",
		"huginn_textfile_rejected_lines{host=\"west\",file=\"well_backup.prom\"} 1\n",
		"huginn_textfile_mtime_seconds{host=\"west\",file=\"well_backup.prom\"} ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "half") || strings.Contains(out, "b{") {
		t.Errorf("read a temp or non-.prom file:\n%s", out)
	}
	if _, ok := collectTextfiles(filepath.Join(dir, "missing"), "west"); ok {
		t.Error("missing directory reported ok")
	}
}
