// SPDX-License-Identifier: GPL-3.0-or-later

package cmdline

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func manFor(t *testing.T, spec Spec, setup func(fs *flag.FlagSet)) string {
	t.Helper()
	t.Setenv("SOURCE_DATE_EPOCH", "1761696000") // 2025-10-29
	fs := flag.NewFlagSet(spec.Name, flag.ContinueOnError)
	if setup != nil {
		setup(fs)
	}
	var b bytes.Buffer
	Man(&b, fs, spec)
	return b.String()
}

func TestManSections(t *testing.T) {
	out := manFor(t, Spec{
		Name:     "akira",
		Purpose:  "Check every service and publish consensus.",
		Operands: "[TARGET]",
		MaxArgs:  1,
		Commands: []Command{{Name: "run", Purpose: "run the checker"}},
		Examples: []string{"akira --once", "akira --config /etc/unheaded/akira.yaml"},
		Details:  "Exit codes: 0 ok, 1 failure, 2 usage.",
		Version:  "v1.2.3",
	}, func(fs *flag.FlagSet) {
		fs.String("config", "/etc/unheaded/akira.yaml", "config `FILE`")
		fs.Bool("once", false, "run one sweep and exit")
	})
	for _, want := range []string{
		`.TH AKIRA 1 "2025-10-29" "unheaded v1.2.3" "Unheaded Kingdom"`,
		".SH NAME\nakira \\- Check every service and publish consensus.\n",
		".SH SYNOPSIS\n.B akira\n[\\fIOPTION\\fR]...\n[TARGET]\n",
		".SH DESCRIPTION\n",
		".SH COMMANDS\n.TP\n.B run\nrun the checker\n",
		".SH OPTIONS\n",
		".TP\n.B \\-\\-config \\fIFILE\\fR\nconfig FILE (default /etc/unheaded/akira.yaml)\n",
		".TP\n.B \\-\\-once\nrun one sweep and exit\n",
		".TP\n.B \\-h, \\-\\-help\n",
		".TP\n.B \\-V, \\-\\-version\n",
		".SH EXIT STATUS\nExit codes: 0 ok, 1 failure, 2 usage.\n",
		".SH EXAMPLES\n.EX\nakira \\-\\-once\nakira \\-\\-config /etc/unheaded/akira.yaml\n.EE\n",
		".SH SEE ALSO\n.BR unheaded (1)\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\\-\\-man") {
		t.Error("the man page must not list the hidden --man flag")
	}
}

func TestManBareSpec(t *testing.T) {
	out := manFor(t, Spec{Name: "tool", Version: "v1"}, nil)
	for _, absent := range []string{".SH COMMANDS", ".SH EXAMPLES", ".SH EXIT STATUS", "[\\fIOPTION\\fR]"} {
		if strings.Contains(out, absent) {
			t.Errorf("bare spec should not have %q:\n%s", absent, out)
		}
	}
	if !strings.Contains(out, ".SH OPTIONS\n") {
		t.Error("help and version are always documented")
	}
}

func TestManDetailsWithoutExitCodes(t *testing.T) {
	out := manFor(t, Spec{Name: "tool", Version: "v1", Details: "Environment: FOO sets the thing."}, nil)
	if strings.Contains(out, ".SH EXIT STATUS") || !strings.Contains(out, ".SH NOTES\nEnvironment: FOO sets the thing.\n") {
		t.Errorf("non-exit-code details go under NOTES:\n%s", out)
	}
}

func TestManEscapesHostileText(t *testing.T) {
	out := manFor(t, Spec{
		Name:     "tool",
		Purpose:  ".TH injected\n'also",
		Examples: []string{".SH EVIL", `tool --path C:\temp`},
		Version:  "v1",
	}, func(fs *flag.FlagSet) {
		fs.String("x", "", "uses a \\ backslash and -dash")
	})
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, ".TH injected") || strings.HasPrefix(line, ".SH EVIL") || strings.HasPrefix(line, "'also") {
			t.Errorf("unescaped control line %q", line)
		}
	}
	for _, want := range []string{`\&.TH injected`, `\&'also`, `\&.SH EVIL`, `C:\etemp`, `a \e backslash and \-dash`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing escaped %q in:\n%s", want, out)
		}
	}
}

func TestParseSetManFlag(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1761696000")
	fs := flag.NewFlagSet("tool", flag.ContinueOnError)
	var o, e bytes.Buffer
	code, exit := ParseSet(fs, Spec{Name: "tool", Purpose: "Do a thing.", Version: "v1"}, []string{"--man"}, &o, &e)
	if !exit || code != 0 || !strings.HasPrefix(o.String(), ".TH TOOL 1 ") || e.Len() != 0 {
		t.Fatalf("code=%d exit=%v out=%q err=%q", code, exit, o.String(), e.String())
	}
	o.Reset()
	fs2 := flag.NewFlagSet("tool", flag.ContinueOnError)
	ParseSet(fs2, Spec{Name: "tool"}, []string{"--help"}, &o, &e)
	if strings.Contains(o.String(), "--man") {
		t.Errorf("--help must not list --man:\n%s", o.String())
	}
	if strings.Contains(o.String(), "[OPTION]") {
		t.Errorf("a program with no flags of its own has no [OPTION]...:\n%s", o.String())
	}
}

// A page is generated on one host and read on all of them, so a default under
// the generating user's home is written from ~, and two homes give one page.
func TestManWritesHomeDefaultsFromTilde(t *testing.T) {
	page := func(home string) string {
		t.Setenv("HOME", home)
		return manFor(t, Spec{Name: "zhen"}, func(fs *flag.FlagSet) {
			fs.String("state", home+"/.local/state/zhen", "state `DIR`")
			fs.String("sibling", home+"2/state", "not under home")
			fs.String("etc", "/etc/zhen", "system `DIR`")
		})
	}
	out := page("/home/builder")
	for _, want := range []string{
		"state DIR (default ~/.local/state/zhen)",
		"not under home (default /home/builder2/state)",
		"system DIR (default /etc/zhen)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("page lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "/home/builder/") {
		t.Errorf("page carries the builder's home:\n%s", out)
	}
	if other := page("/home/runner"); strings.ReplaceAll(other, "runner", "builder") != out {
		t.Errorf("two homes gave two pages:\n%s\n---\n%s", out, other)
	}
}
