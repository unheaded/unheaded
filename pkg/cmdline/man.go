// SPDX-License-Identifier: GPL-3.0-or-later

package cmdline

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// Man writes the program's man page (roff, section 1) to w from the same
// Spec and flags that --help prints, so the two cannot drift. Every binary
// that parses with ParseSet answers a hidden --man with it; scripts/gen-man.sh
// turns those into man/NAME.1.
//
// The date comes from SOURCE_DATE_EPOCH when set (reproducible builds),
// else today. All text is escaped, so a description cannot start a roff
// request.
func Man(w io.Writer, fs *flag.FlagSet, spec Spec) {
	spec = spec.withDefaults()
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }

	p(".TH %s 1 %q %q %q\n", strings.ToUpper(roffEscape(spec.Name)), manDate(),
		"unheaded "+spec.version(), "Unheaded Kingdom")

	p(".SH NAME\n")
	first, _, _ := strings.Cut(spec.Purpose, "\n")
	if first != "" {
		p("%s \\- %s\n", roffEscape(spec.Name), roffEscape(first))
	} else {
		p("%s\n", roffEscape(spec.Name))
	}

	p(".SH SYNOPSIS\n.B %s\n", roffEscape(spec.Name))
	if fs != nil && countUserFlags(fs) > 0 {
		p("[\\fIOPTION\\fR]...\n")
	}
	if spec.Operands != "" {
		p("%s\n", roffText(spec.Operands))
	}

	if spec.Purpose != "" {
		p(".SH DESCRIPTION\n%s\n", roffText(spec.Purpose))
	}

	if len(spec.Commands) > 0 {
		p(".SH COMMANDS\n")
		for _, c := range spec.Commands {
			p(".TP\n.B %s\n%s\n", roffEscape(c.Name), roffText(c.Purpose))
		}
	}

	p(".SH OPTIONS\n")
	if fs != nil {
		fs.VisitAll(func(f *flag.Flag) {
			if hiddenFlag(f.Name) {
				return
			}
			left, desc := flagRow(f)
			manOption(w, left, defaultFromTilde(desc))
		})
	}
	manOption(w, "-h, --help", "display this help and exit")
	manOption(w, "-V, --version", "output version information and exit")

	if d := strings.TrimRight(spec.Details, "\n"); d != "" {
		if strings.HasPrefix(d, "Exit codes") {
			p(".SH EXIT STATUS\n%s\n", roffText(d))
		} else {
			p(".SH NOTES\n%s\n", roffText(d))
		}
	}

	if len(spec.Examples) > 0 {
		p(".SH EXAMPLES\n.EX\n")
		for _, e := range spec.Examples {
			p("%s\n", roffText(e))
		}
		p(".EE\n")
	}

	p(".SH SEE ALSO\n.BR unheaded (1)\n")
}

// defaultFromTilde writes a default under the caller's home from ~. A page is
// generated on one host and read on all of them; --help keeps the real path.
func defaultFromTilde(desc string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return desc
	}
	return strings.ReplaceAll(desc, "(default "+home+"/", "(default ~/")
}

// manOption is one .TP entry: the option in bold with its placeholder in
// italics ("--config FILE" -> \-\-config \fIFILE\fR), then the description.
func manOption(w io.Writer, left, desc string) {
	opt, arg, hasArg := strings.Cut(left, " ")
	if strings.HasPrefix(left, "-h, ") || strings.HasPrefix(left, "-V, ") {
		opt, hasArg = left, false
	}
	head := roffEscape(opt)
	if hasArg {
		head += " \\fI" + roffEscape(arg) + "\\fR"
	}
	fmt.Fprintf(w, ".TP\n.B %s\n%s\n", head, roffText(desc))
}

// manDate is SOURCE_DATE_EPOCH as a UTC date, or today.
func manDate() string {
	if v := os.Getenv("SOURCE_DATE_EPOCH"); v != "" {
		if sec, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Unix(sec, 0).UTC().Format("2006-01-02")
		}
	}
	return time.Now().UTC().Format("2006-01-02")
}

// roffEscape makes s safe inside a roff line: backslashes become \e and
// hyphens \- (so they render as minus signs and copy as ASCII).
func roffEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\e`)
	return strings.ReplaceAll(s, "-", `\-`)
}

// roffText escapes multi-line text and guards every line that would
// otherwise start a roff request (a dot or an apostrophe).
func roffText(s string) string {
	lines := strings.Split(roffEscape(s), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, ".") || strings.HasPrefix(l, "'") {
			lines[i] = `\&` + l
		}
	}
	return strings.Join(lines, "\n")
}
