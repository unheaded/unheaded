// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

// Package cmdline gives every Kingdom binary the same command-line manners
// as GNU coreutils: `--help` prints a usage line, a one-line purpose, the
// options, and examples on stdout and exits 0; `--version` prints one line;
// a bad option, a missing argument, or a stray operand prints ONE line on
// stderr followed by "Try 'NAME --help' for more information." and exits 2.
// Nothing dumps the full usage on error.
//
// It wraps the standard library's flag package; it does not replace it.
// Callers keep their flag definitions and swap flag.Parse() for
// cmdline.Parse(spec).
//
//	func main() {
//		once := flag.Bool("once", false, "run one sweep and exit")
//		cmdline.Parse(cmdline.Spec{
//			Name:     "akira",
//			Purpose:  "Kingdom health daemon (ADR-029).",
//			Examples: []string{"akira --once"},
//		})
//		...
//	}
package cmdline

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
)

// Version is the build version for every binary in the module. Set it at
// link time:
//
//	go build -ldflags "-X unheaded/pkg/cmdline.Version=$(git describe --tags --always --dirty)"
//
// When unset, --version falls back to the module's VCS revision from the
// build info, then to "dev".
var Version string

// Unlimited as MaxArgs accepts any number of operands.
const Unlimited = -1

// ExitUsage is the exit status for a command-line mistake (GNU getopt and
// Go's flag package both use 2).
const ExitUsage = 2

// Spec describes a program for help, version and error output.
type Spec struct {
	// Name of the program as shown in messages. Defaults to the base name of
	// os.Args[0].
	Name string
	// Purpose is one sentence printed under the usage line.
	Purpose string
	// Operands is the operand syntax appended to the usage line, for example
	// "<port|url>" or "FILE...". Empty means the program takes none.
	Operands string
	// MinArgs and MaxArgs bound the operand count. The zero value allows no
	// operands at all, which is what most Kingdom daemons want. Use
	// Unlimited for MaxArgs to accept any number.
	MinArgs, MaxArgs int
	// Commands lists subcommands, printed under "Commands:" when non-empty.
	Commands []Command
	// Examples are printed verbatim under "Examples:", one per line.
	Examples []string
	// Details is free text (environment variables, exit codes, pointers)
	// printed after the options.
	Details string
	// Version overrides the package-level Version for this program.
	Version string
}

// Command is one subcommand of a multi-command binary.
type Command struct {
	Name    string
	Purpose string
}

// Parse parses os.Args[1:] into flag.CommandLine with the Kingdom's
// conventions and exits the process on --help, --version or a usage error.
// On success it returns and flag.Args() holds the operands.
func Parse(spec Spec) {
	if code, exit := ParseSet(flag.CommandLine, spec, os.Args[1:], os.Stdout, os.Stderr); exit {
		os.Exit(code)
	}
}

// ParseSet is the testable core of Parse. It parses args into fs and
// returns (exitCode, true) when the process should exit — after printing
// help or version to stdout (code 0), or a one-line error to stderr (code
// 2) — and (0, false) when the program should carry on.
//
// fs may have been created with any ErrorHandling; it is switched to
// ContinueOnError and its default usage output is suppressed.
func ParseSet(fs *flag.FlagSet, spec Spec, args []string, stdout, stderr io.Writer) (int, bool) {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	spec = spec.withDefaults()
	if fs == nil {
		return failTo(stderr, spec, "internal error: nil flag set"), true
	}

	fs.Init(spec.Name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	var showVersion, showVersionShort bool
	if fs.Lookup("version") == nil {
		fs.BoolVar(&showVersion, "version", false, "output version information and exit")
	}
	if fs.Lookup("V") == nil {
		fs.BoolVar(&showVersionShort, "V", false, "output version information and exit")
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			Usage(stdout, fs, spec)
			return 0, true
		}
		return failTo(stderr, spec, "%s", Translate(err, args)), true
	}

	if boolFlagSet(fs, "help") || boolFlagSet(fs, "h") {
		Usage(stdout, fs, spec)
		return 0, true
	}
	if showVersion || showVersionShort || boolFlagSet(fs, "version") || boolFlagSet(fs, "V") {
		fmt.Fprintf(stdout, "%s (unheaded) %s\n", spec.Name, spec.version())
		return 0, true
	}

	n := fs.NArg()
	if n < spec.MinArgs {
		return failTo(stderr, spec, "missing operand"), true
	}
	if spec.MaxArgs != Unlimited && n > spec.MaxArgs {
		return failTo(stderr, spec, "extra operand '%s'", fs.Arg(spec.MaxArgs)), true
	}
	return 0, false
}

// Fail prints "NAME: MESSAGE" and the --help hint to stderr and exits 2.
// Use it for a usage mistake the flag package cannot see (a bad operand,
// two options that exclude each other).
func Fail(spec Spec, format string, a ...any) {
	os.Exit(failTo(os.Stderr, spec.withDefaults(), format, a...))
}

// UnknownCommand prints "NAME: unknown command 'X'" and the --help hint to
// stderr and exits 2.
func UnknownCommand(spec Spec, cmd string) {
	os.Exit(unknownCommandTo(os.Stderr, spec.withDefaults(), cmd))
}

// VersionString returns the version --version would print for this spec:
// Spec.Version, else the package Version, else the build's VCS revision,
// else "dev".
func (s Spec) VersionString() string { return s.version() }

// Usage writes the full help text for fs to w.
func Usage(w io.Writer, fs *flag.FlagSet, spec Spec) {
	spec = spec.withDefaults()
	line := "Usage: " + spec.Name
	if fs != nil && countUserFlags(fs) > 0 {
		line += " [OPTION]..."
	}
	if spec.Operands != "" {
		line += " " + spec.Operands
	}
	fmt.Fprintln(w, line)
	if spec.Purpose != "" {
		for _, l := range wrap(spec.Purpose, 78) {
			fmt.Fprintln(w, l)
		}
	}

	if len(spec.Commands) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Commands:")
		width := 0
		for _, c := range spec.Commands {
			if len(c.Name) > width {
				width = len(c.Name)
			}
		}
		for _, c := range spec.Commands {
			fmt.Fprintf(w, "  %-*s  %s\n", width, c.Name, c.Purpose)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	if fs != nil {
		fs.VisitAll(func(f *flag.Flag) {
			switch f.Name {
			case "help", "h", "version", "V":
				return
			}
			printFlag(w, f)
		})
	}
	printOption(w, "-h, --help", "display this help and exit")
	printOption(w, "-V, --version", "output version information and exit")

	if spec.Details != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, strings.TrimRight(spec.Details, "\n"))
	}
	if len(spec.Examples) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Examples:")
		for _, e := range spec.Examples {
			fmt.Fprintf(w, "  %s\n", e)
		}
	}
}

// ---------------------------------------------------------------------------

func (s Spec) withDefaults() Spec {
	if s.Name == "" {
		if len(os.Args) > 0 {
			s.Name = filepath.Base(os.Args[0])
		} else {
			s.Name = "unheaded"
		}
	}
	return s
}

func (s Spec) version() string {
	if s.Version != "" {
		return s.Version
	}
	if Version != "" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi != nil {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
		var rev, dirty string
		for _, kv := range bi.Settings {
			switch kv.Key {
			case "vcs.revision":
				rev = kv.Value
			case "vcs.modified":
				if kv.Value == "true" {
					dirty = "-dirty"
				}
			}
		}
		if rev != "" {
			if len(rev) > 12 {
				rev = rev[:12]
			}
			return rev + dirty
		}
	}
	return "dev"
}

func failTo(w io.Writer, spec Spec, format string, a ...any) int {
	fmt.Fprintf(w, "%s: %s\n", spec.Name, fmt.Sprintf(format, a...))
	fmt.Fprintf(w, "Try '%s --help' for more information.\n", spec.Name)
	return ExitUsage
}

func unknownCommandTo(w io.Writer, spec Spec, cmd string) int {
	return failTo(w, spec, "unknown command '%s'", cmd)
}

// Translate turns a flag package parse error into coreutils phrasing
// ("unrecognized option '--x'", "option '--n' requires an argument",
// "invalid argument 'abc' for '--port'"). args are the arguments that were
// parsed, used to echo the option the way the user spelled it. Callers that
// run their own flag.FlagSet can use it to keep the Kingdom's error shape.
// The flag package has no typed errors for these, so the prefixes below are
// matched as text; they have been stable since Go 1.0.
func Translate(err error, args []string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "flag provided but not defined: -"):
		name := strings.TrimPrefix(msg, "flag provided but not defined: -")
		return fmt.Sprintf("unrecognized option '%s'", originalToken(args, name))
	case strings.HasPrefix(msg, "flag needs an argument: -"):
		name := strings.TrimPrefix(msg, "flag needs an argument: -")
		return fmt.Sprintf("option '%s' requires an argument", originalToken(args, name))
	case strings.HasPrefix(msg, "invalid value ") || strings.HasPrefix(msg, "invalid boolean value "):
		// invalid value "abc" for flag -port: parse error
		// invalid boolean value "x" for -once: ...
		rest := msg[strings.Index(msg, "value ")+len("value "):]
		val, after, ok := cutQuoted(rest)
		if !ok {
			return msg
		}
		after = strings.TrimPrefix(after, " for flag -")
		after = strings.TrimPrefix(after, " for -")
		name, reason, _ := strings.Cut(after, ":")
		out := fmt.Sprintf("invalid argument '%s' for '%s'", val, originalToken(args, name))
		if reason = strings.TrimSpace(reason); reason != "" && reason != "parse error" {
			out += ": " + reason
		}
		return out
	case strings.HasPrefix(msg, "bad flag syntax: "):
		return fmt.Sprintf("unrecognized option '%s'", strings.TrimPrefix(msg, "bad flag syntax: "))
	}
	return msg
}

// cutQuoted splits `"val" rest` into val and rest.
func cutQuoted(s string) (val, rest string, ok bool) {
	if !strings.HasPrefix(s, `"`) {
		return "", s, false
	}
	end := strings.Index(s[1:], `"`)
	if end < 0 {
		return "", s, false
	}
	return s[1 : 1+end], s[2+end:], true
}

// originalToken finds how the user actually spelled option name in args
// (-name, --name, --name=value), so the error echoes their dashes.
func originalToken(args []string, name string) string {
	for _, a := range args {
		if a == "--" {
			break
		}
		for _, p := range []string{"--", "-"} {
			if strings.HasPrefix(a, p) {
				body := a[len(p):]
				if body == name || strings.HasPrefix(body, name+"=") {
					return p + name
				}
			}
		}
	}
	return "-" + name
}

func boolFlagSet(fs *flag.FlagSet, name string) bool {
	f := fs.Lookup(name)
	if f == nil {
		return false
	}
	if b, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok || !b.IsBoolFlag() {
		return false
	}
	return f.Value.String() == "true"
}

func countUserFlags(fs *flag.FlagSet) int {
	n := 0
	fs.VisitAll(func(f *flag.Flag) {
		switch f.Name {
		case "version", "V":
		default:
			n++
		}
	})
	return n
}

const (
	optColumn = 29 // description starts here
	lineWidth = 80
)

func printFlag(w io.Writer, f *flag.Flag) {
	name, usage := flag.UnquoteUsage(f)
	left := "--" + f.Name
	if len(f.Name) == 1 {
		left = "-" + f.Name
	}
	if name != "" {
		left += " " + placeholder(name)
	}
	if !isZeroDefault(f) {
		def := f.DefValue
		if def == "" || strings.ContainsAny(def, " \t") {
			def = fmt.Sprintf("%q", def)
		}
		usage += fmt.Sprintf(" (default %s)", def)
	}
	printOption(w, left, usage)
}

// printOption prints one option row: left column padded to optColumn, the
// description word-wrapped with a hanging indent. A long left column pushes
// the description to the next line, like coreutils.
func printOption(w io.Writer, left, desc string) {
	indent := strings.Repeat(" ", optColumn)
	lines := wrap(desc, lineWidth-optColumn)
	if len(lines) == 0 {
		lines = []string{""}
	}
	// Long-only options are indented past the "-x, " slot so that the
	// "--" of every option lines up, as in coreutils --help.
	pad := 2
	if strings.HasPrefix(left, "--") {
		pad = 6
	}
	head := strings.Repeat(" ", pad) + left
	if len(head)+2 > optColumn {
		fmt.Fprintln(w, head)
		for _, l := range lines {
			fmt.Fprintln(w, indent+l)
		}
		return
	}
	fmt.Fprintf(w, "%-*s%s\n", optColumn, head, lines[0])
	for _, l := range lines[1:] {
		fmt.Fprintln(w, indent+l)
	}
}

func placeholder(name string) string {
	switch name {
	case "string":
		return "STRING"
	case "int", "int64", "uint", "uint64":
		return "N"
	case "float", "float64":
		return "NUM"
	case "duration":
		return "DURATION"
	case "value":
		return "VALUE"
	}
	return name
}

// isZeroDefault mirrors the flag package's unexported isZeroValue: build a
// fresh Value of the same type and compare its String() to DefValue.
func isZeroDefault(f *flag.Flag) (zero bool) {
	defer func() {
		if recover() != nil {
			zero = f.DefValue == "" || f.DefValue == "0" || f.DefValue == "false"
		}
	}()
	t := reflect.TypeOf(f.Value)
	var z reflect.Value
	if t.Kind() == reflect.Pointer {
		z = reflect.New(t.Elem())
	} else {
		z = reflect.Zero(t)
	}
	v, ok := z.Interface().(flag.Value)
	if !ok {
		return f.DefValue == "" || f.DefValue == "0" || f.DefValue == "false"
	}
	return v.String() == f.DefValue
}

// wrap breaks s into lines no longer than width, on spaces. Words longer
// than width stand alone.
func wrap(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := words[0]
	for _, wd := range words[1:] {
		if len(cur)+1+len(wd) > width {
			lines = append(lines, cur)
			cur = wd
			continue
		}
		cur += " " + wd
	}
	return append(lines, cur)
}

// ---------------------------------------------------------------------------
// Helpers for programs that drive their own flag.FlagSets (subcommand tools).
// They keep the same output shape as Parse without taking over dispatch.

// HandleParseError is for a FlagSet the caller parsed itself (give it
// SetOutput(io.Discard) so the flag package prints nothing of its own). On
// flag.ErrHelp it writes the usage for fs and spec to stdout and returns 0;
// on any other error it writes the translated one-line error and the --help
// hint to stderr and returns ExitUsage.
func HandleParseError(err error, fs *flag.FlagSet, spec Spec, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	spec = spec.withDefaults()
	if err == nil {
		return 0
	}
	if errors.Is(err, flag.ErrHelp) {
		Usage(stdout, fs, spec)
		return 0
	}
	return failTo(stderr, spec, "%s", Translate(err, args))
}

// ReportUsageError writes "NAME: MESSAGE" and the --help hint to w and
// returns ExitUsage, for mistakes the flag package cannot see.
func ReportUsageError(w io.Writer, spec Spec, format string, a ...any) int {
	if w == nil {
		w = io.Discard
	}
	return failTo(w, spec.withDefaults(), format, a...)
}

// ReportUnknownCommand writes "NAME: unknown command 'CMD'" and the --help
// hint to w and returns ExitUsage.
func ReportUnknownCommand(w io.Writer, spec Spec, cmd string) int {
	if w == nil {
		w = io.Discard
	}
	return unknownCommandTo(w, spec.withDefaults(), cmd)
}

// PrintVersion writes the one-line version for spec to w.
func PrintVersion(w io.Writer, spec Spec) {
	if w == nil {
		w = io.Discard
	}
	spec = spec.withDefaults()
	fmt.Fprintf(w, "%s (unheaded) %s\n", spec.Name, spec.version())
}

// IsVersionArg reports whether a single leading argument asks for the
// version, for dispatchers that switch on args[0].
func IsVersionArg(arg string) bool {
	return arg == "--version" || arg == "-version" || arg == "-V"
}

// IsHelpArg reports whether a single leading argument asks for help, for
// dispatchers that switch on args[0].
func IsHelpArg(arg string) bool {
	return arg == "--help" || arg == "-help" || arg == "-h" || arg == "help"
}
