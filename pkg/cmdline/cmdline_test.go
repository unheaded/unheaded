// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package cmdline

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"strings"
	"testing"
	"time"
)

func newSet(t *testing.T) *flag.FlagSet {
	t.Helper()
	fs := flag.NewFlagSet("demo", flag.ExitOnError) // ParseSet must override the handling
	fs.String("config", "", "path to the config `FILE`")
	fs.Int("port", 19100, "HTTP API port")
	fs.Bool("once", false, "run one sweep and exit")
	fs.Duration("timeout", 4*time.Second, "probe timeout")
	fs.String("empty", "", "a string whose zero default must not print")
	return fs
}

func spec() Spec {
	return Spec{
		Name:     "demo",
		Purpose:  "Demonstrate the Kingdom's command-line conventions.",
		Examples: []string{"demo --once", "demo --config /etc/unheaded/demo.yaml"},
		Version:  "1.2.3",
	}
}

func parse(t *testing.T, fs *flag.FlagSet, sp Spec, args ...string) (code int, exit bool, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code, exit = ParseSet(fs, sp, args, &o, &e)
	return code, exit, o.String(), e.String()
}

func TestParseSet_HappyPath_Continues(t *testing.T) {
	fs := newSet(t)
	code, exit, out, errOut := parse(t, fs, spec(), "--once", "-port=1", "--config", "x.yaml")
	if exit {
		t.Fatalf("exit=%v code=%d out=%q err=%q", exit, code, out, errOut)
	}
	if out != "" || errOut != "" {
		t.Errorf("unexpected output out=%q err=%q", out, errOut)
	}
	if fs.Lookup("port").Value.String() != "1" {
		t.Error("flag not parsed")
	}
}

func TestParseSet_Help_StdoutExit0(t *testing.T) {
	for _, h := range []string{"-h", "--help", "-help"} {
		t.Run(h, func(t *testing.T) {
			fs := newSet(t)
			code, exit, out, errOut := parse(t, fs, spec(), h)
			if !exit || code != 0 {
				t.Fatalf("exit=%v code=%d", exit, code)
			}
			if errOut != "" {
				t.Errorf("help must go to stdout, got stderr %q", errOut)
			}
			for _, want := range []string{
				"Usage: demo [OPTION]...\n",
				"Demonstrate the Kingdom's command-line conventions.\n",
				"--config FILE",
				"--port N",
				"(default 19100)",
				"--timeout DURATION",
				"(default 4s)",
				"--once",
				"-h, --help",
				"--version",
				"Examples:\n  demo --once\n  demo --config /etc/unheaded/demo.yaml\n",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("help missing %q\n---\n%s", want, out)
				}
			}
			if strings.Contains(out, `(default "")`) || strings.Contains(out, "(default false)") {
				t.Errorf("zero defaults must not print\n---\n%s", out)
			}
		})
	}
}

func TestParseSet_HelpFlagNamedHelp_WhenCallerDefinedIt(t *testing.T) {
	fs := newSet(t)
	fs.Bool("help", false, "caller-defined help")
	code, exit, out, _ := parse(t, fs, spec(), "--help")
	if !exit || code != 0 || !strings.Contains(out, "Usage: demo") {
		t.Fatalf("caller-defined --help must still print usage: exit=%v code=%d out=%q", exit, code, out)
	}
}

func TestParseSet_Version(t *testing.T) {
	for _, v := range []string{"--version", "-version", "-V"} {
		t.Run(v, func(t *testing.T) {
			fs := newSet(t)
			code, exit, out, errOut := parse(t, fs, spec(), v)
			if !exit || code != 0 {
				t.Fatalf("exit=%v code=%d err=%q", exit, code, errOut)
			}
			if out != "demo (unheaded) 1.2.3\n" {
				t.Errorf("version line = %q", out)
			}
		})
	}
}

func TestParseSet_Version_FallsBackToPackageVar(t *testing.T) {
	old := Version
	Version = "9.9.9"
	t.Cleanup(func() { Version = old })
	sp := spec()
	sp.Version = ""
	fs := newSet(t)
	_, _, out, _ := parse(t, fs, sp, "--version")
	if out != "demo (unheaded) 9.9.9\n" {
		t.Errorf("version line = %q", out)
	}
}

func TestParseSet_Version_NeverEmpty(t *testing.T) {
	old := Version
	Version = ""
	t.Cleanup(func() { Version = old })
	sp := spec()
	sp.Version = ""
	fs := newSet(t)
	_, _, out, _ := parse(t, fs, sp, "--version")
	if !strings.HasPrefix(out, "demo (unheaded) ") || strings.TrimSpace(strings.TrimPrefix(out, "demo (unheaded) ")) == "" {
		t.Errorf("version must fall back to something: %q", out)
	}
}

func TestParseSet_VersionFlag_NotRegisteredTwice(t *testing.T) {
	fs := newSet(t)
	fs.Bool("version", false, "caller's own")
	fs.Bool("V", false, "caller's own short")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked redefining flag: %v", r)
		}
	}()
	_, exit, _, _ := parse(t, fs, spec(), "--once")
	if exit {
		t.Fatal("should continue")
	}
}

func TestParseSet_UnknownOption(t *testing.T) {
	cases := []struct{ arg, want string }{
		{"--bogus", "demo: unrecognized option '--bogus'\n"},
		{"-bogus", "demo: unrecognized option '-bogus'\n"},
		{"--bogus=1", "demo: unrecognized option '--bogus'\n"},
		{"-?", "demo: unrecognized option '-?'\n"},
	}
	for _, c := range cases {
		t.Run(c.arg, func(t *testing.T) {
			fs := newSet(t)
			code, exit, out, errOut := parse(t, fs, spec(), c.arg)
			if !exit || code != 2 {
				t.Fatalf("exit=%v code=%d", exit, code)
			}
			if out != "" {
				t.Errorf("nothing on stdout, got %q", out)
			}
			if errOut != c.want+"Try 'demo --help' for more information.\n" {
				t.Errorf("stderr = %q", errOut)
			}
			if strings.Contains(errOut, "Usage:") {
				t.Error("must not dump full usage on error")
			}
		})
	}
}

func TestParseSet_MissingArgument(t *testing.T) {
	fs := newSet(t)
	code, exit, _, errOut := parse(t, fs, spec(), "--config")
	if !exit || code != 2 {
		t.Fatalf("exit=%v code=%d", exit, code)
	}
	want := "demo: option '--config' requires an argument\nTry 'demo --help' for more information.\n"
	if errOut != want {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestParseSet_InvalidValue(t *testing.T) {
	fs := newSet(t)
	code, exit, _, errOut := parse(t, fs, spec(), "--port", "abc")
	if !exit || code != 2 {
		t.Fatalf("exit=%v code=%d", exit, code)
	}
	if !strings.HasPrefix(errOut, "demo: invalid argument 'abc' for '--port'") || !strings.HasSuffix(errOut, "Try 'demo --help' for more information.\n") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestParseSet_Operands(t *testing.T) {
	t.Run("none allowed by default", func(t *testing.T) {
		fs := newSet(t)
		code, exit, _, errOut := parse(t, fs, spec(), "stray")
		if !exit || code != 2 || errOut != "demo: extra operand 'stray'\nTry 'demo --help' for more information.\n" {
			t.Errorf("exit=%v code=%d stderr=%q", exit, code, errOut)
		}
	})
	t.Run("required missing", func(t *testing.T) {
		fs := newSet(t)
		sp := spec()
		sp.Operands = "<port|url>"
		sp.MinArgs, sp.MaxArgs = 1, 1
		code, exit, _, errOut := parse(t, fs, sp)
		if !exit || code != 2 || errOut != "demo: missing operand\nTry 'demo --help' for more information.\n" {
			t.Errorf("exit=%v code=%d stderr=%q", exit, code, errOut)
		}
	})
	t.Run("required present", func(t *testing.T) {
		fs := newSet(t)
		sp := spec()
		sp.Operands = "<port|url>"
		sp.MinArgs, sp.MaxArgs = 1, 1
		_, exit, _, _ := parse(t, fs, sp, "8080")
		if exit {
			t.Error("should continue")
		}
		if fs.Arg(0) != "8080" {
			t.Error("operand lost")
		}
	})
	t.Run("too many", func(t *testing.T) {
		fs := newSet(t)
		sp := spec()
		sp.Operands = "<port|url>"
		sp.MinArgs, sp.MaxArgs = 1, 1
		code, _, _, errOut := parse(t, fs, sp, "a", "b")
		if code != 2 || !strings.HasPrefix(errOut, "demo: extra operand 'b'\n") {
			t.Errorf("code=%d stderr=%q", code, errOut)
		}
	})
	t.Run("unlimited", func(t *testing.T) {
		fs := newSet(t)
		sp := spec()
		sp.Operands = "FILE..."
		sp.MinArgs, sp.MaxArgs = 1, Unlimited
		_, exit, _, _ := parse(t, fs, sp, "a", "b", "c")
		if exit {
			t.Error("should continue")
		}
	})
	t.Run("operands in usage line", func(t *testing.T) {
		fs := newSet(t)
		sp := spec()
		sp.Operands = "<port|url>"
		_, _, out, _ := parse(t, fs, sp, "--help")
		if !strings.Contains(out, "Usage: demo [OPTION]... <port|url>\n") {
			t.Errorf("usage line wrong:\n%s", out)
		}
	})
}

func TestParseSet_NoFlagsDefined_StillHasHelpAndVersion(t *testing.T) {
	fs := flag.NewFlagSet("bare", flag.ExitOnError)
	sp := Spec{Name: "bare", Purpose: "A daemon configured only by environment."}
	_, exit, out, _ := parse(t, fs, sp, "--help")
	if !exit || !strings.Contains(out, "Usage: bare\n") || !strings.Contains(out, "--version") {
		t.Errorf("exit=%v out=%q", exit, out)
	}
	code, exit, _, errOut := parse(t, flag.NewFlagSet("bare", flag.ExitOnError), sp, "--nope")
	if !exit || code != 2 || !strings.HasPrefix(errOut, "bare: unrecognized option '--nope'") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestParseSet_DetailsAndCommands(t *testing.T) {
	fs := flag.NewFlagSet("multi", flag.ExitOnError)
	sp := Spec{
		Name:     "multi",
		Operands: "COMMAND [ARG]...",
		MinArgs:  1, MaxArgs: Unlimited,
		Purpose:  "Do several things.",
		Commands: []Command{{"verify", "check a signature"}, {"sign", "produce a signature"}},
		Details:  "Exit codes: 0 ok, 1 refused, 2 usage.",
	}
	_, _, out, _ := parse(t, fs, sp, "--help")
	for _, want := range []string{"Commands:\n", "  verify", "check a signature", "  sign", "Exit codes: 0 ok, 1 refused, 2 usage.\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestParseSet_StopsAtFirstOperand_ForSubcommands(t *testing.T) {
	// A top-level parser for a subcommand binary must not eat the
	// subcommand's own flags.
	fs := flag.NewFlagSet("multi", flag.ExitOnError)
	sp := Spec{Name: "multi", Operands: "COMMAND [ARG]...", MinArgs: 1, MaxArgs: Unlimited}
	_, exit, _, errOut := parse(t, fs, sp, "verify", "--pubkey", "k.pem")
	if exit {
		t.Fatalf("must continue; stderr=%q", errOut)
	}
	if got := strings.Join(fs.Args(), " "); got != "verify --pubkey k.pem" {
		t.Errorf("args = %q", got)
	}
}

func TestFail_FormatsLikeCoreutils(t *testing.T) {
	var e bytes.Buffer
	code := failTo(&e, spec(), "cannot read %q: %v", "x.yaml", io.EOF)
	if code != 2 {
		t.Errorf("code=%d", code)
	}
	if e.String() != "demo: cannot read \"x.yaml\": EOF\nTry 'demo --help' for more information.\n" {
		t.Errorf("stderr=%q", e.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	var e bytes.Buffer
	sp := spec()
	sp.Commands = []Command{{"verify", ""}, {"sign", ""}}
	code := unknownCommandTo(&e, sp, "verfy")
	if code != 2 || e.String() != "demo: unknown command 'verfy'\nTry 'demo --help' for more information.\n" {
		t.Errorf("code=%d stderr=%q", code, e.String())
	}
}

func TestWrap(t *testing.T) {
	got := wrap("one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen", 30)
	for _, line := range got {
		if len(line) > 30 {
			t.Errorf("line too long: %q", line)
		}
	}
	if strings.Join(got, " ") != "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen" {
		t.Errorf("words lost: %q", got)
	}
}

func TestParseSet_NilSafety(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	code, exit := ParseSet(nil, Spec{}, nil, nil, nil)
	if !exit || code != 2 {
		t.Errorf("nil set must fail closed: exit=%v code=%d", exit, code)
	}
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	code, exit = ParseSet(fs, Spec{}, []string{"--help"}, nil, nil) // nil writers tolerated
	if !exit || code != 0 {
		t.Errorf("nil writers: exit=%v code=%d", exit, code)
	}
}

func TestOriginalToken(t *testing.T) {
	cases := []struct {
		args []string
		name string
		want string
	}{
		{[]string{"--bogus"}, "bogus", "--bogus"},
		{[]string{"-bogus"}, "bogus", "-bogus"},
		{[]string{"--bogus=3"}, "bogus", "--bogus"},
		{[]string{"-x", "--bogus", "y"}, "bogus", "--bogus"},
		{[]string{"nothing"}, "bogus", "-bogus"},
	}
	for _, c := range cases {
		if got := originalToken(c.args, c.name); got != c.want {
			t.Errorf("originalToken(%v,%q)=%q want %q", c.args, c.name, got, c.want)
		}
	}
}

func TestTranslate_Exported(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Int("n", 0, "")
	args := []string{"--n"}
	err := fs.Parse(args)
	if got := Translate(err, args); got != "option '--n' requires an argument" {
		t.Errorf("Translate = %q", got)
	}
	if Translate(nil, nil) != "" {
		t.Error("nil error must translate to empty")
	}
	if got := Translate(errors.New("something else"), nil); got != "something else" {
		t.Errorf("unknown errors pass through, got %q", got)
	}
}

func TestVersionString(t *testing.T) {
	if (Spec{Version: "1.0"}).VersionString() != "1.0" {
		t.Error("explicit version wins")
	}
	if (Spec{}).VersionString() == "" {
		t.Error("never empty")
	}
}

func TestHandleParseError(t *testing.T) {
	sp := Spec{Name: "tool verify", Purpose: "Verify a cask."}
	t.Run("help to stdout", func(t *testing.T) {
		fs := flag.NewFlagSet("verify", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.String("cask", "", "cask `FILE`")
		args := []string{"--help"}
		err := fs.Parse(args)
		var o, e bytes.Buffer
		if code := HandleParseError(err, fs, sp, args, &o, &e); code != 0 {
			t.Fatalf("code=%d", code)
		}
		if !strings.Contains(o.String(), "Usage: tool verify [OPTION]...\n") || !strings.Contains(o.String(), "--cask FILE") || e.Len() != 0 {
			t.Errorf("out=%q err=%q", o.String(), e.String())
		}
	})
	t.Run("error to stderr", func(t *testing.T) {
		fs := flag.NewFlagSet("verify", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		args := []string{"--nope"}
		err := fs.Parse(args)
		var o, e bytes.Buffer
		if code := HandleParseError(err, fs, sp, args, &o, &e); code != ExitUsage {
			t.Fatalf("code=%d", code)
		}
		if e.String() != "tool verify: unrecognized option '--nope'\nTry 'tool verify --help' for more information.\n" || o.Len() != 0 {
			t.Errorf("out=%q err=%q", o.String(), e.String())
		}
	})
	t.Run("nil error", func(t *testing.T) {
		if HandleParseError(nil, nil, sp, nil, nil, nil) != 0 {
			t.Error("nil error must be 0")
		}
	})
}

func TestReportHelpers(t *testing.T) {
	var e bytes.Buffer
	if ReportUsageError(&e, Spec{Name: "t"}, "missing %s", "key") != ExitUsage || e.String() != "t: missing key\nTry 't --help' for more information.\n" {
		t.Errorf("ReportUsageError: %q", e.String())
	}
	e.Reset()
	if ReportUnknownCommand(&e, Spec{Name: "t"}, "zap") != ExitUsage || e.String() != "t: unknown command 'zap'\nTry 't --help' for more information.\n" {
		t.Errorf("ReportUnknownCommand: %q", e.String())
	}
	e.Reset()
	PrintVersion(&e, Spec{Name: "t", Version: "3"})
	if e.String() != "t (unheaded) 3\n" {
		t.Errorf("PrintVersion: %q", e.String())
	}
	for _, a := range []string{"--version", "-version", "-V"} {
		if !IsVersionArg(a) {
			t.Errorf("IsVersionArg(%q) false", a)
		}
	}
	for _, a := range []string{"--help", "-help", "-h", "help"} {
		if !IsHelpArg(a) {
			t.Errorf("IsHelpArg(%q) false", a)
		}
	}
	if IsVersionArg("-v") || IsHelpArg("hel") {
		t.Error("false positives")
	}
	// nil writers never panic
	ReportUsageError(nil, Spec{}, "x")
	ReportUnknownCommand(nil, Spec{}, "x")
	PrintVersion(nil, Spec{})
}
