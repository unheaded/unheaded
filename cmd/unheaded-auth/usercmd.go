// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// readPassword reads a password without echo from the terminal; a
// variable so tests never need a tty.
var readPassword = readPasswordTTY

// runUser implements `unheaded-auth user add|list|del`.
func runUser(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: unheaded-auth user add|list|del ...")
		return 2
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("unheaded-auth user "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	usersFile := fs.String("users-file", defaultUsersFile, "users file")
	var roles *string
	var pwStdin *bool
	if sub == "add" {
		roles = fs.String("roles", "", "comma-separated roles: admin, operator, viewer, observer, guest (required)")
		pwStdin = fs.Bool("password-stdin", false, "read the password from the first line of stdin instead of the terminal")
	}
	pos, err := parseInterspersed(fs, rest)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	switch sub {
	case "add":
		if len(pos) != 1 {
			fmt.Fprintln(stderr, "usage: unheaded-auth user add <name> --roles r1,r2 [--password-stdin]")
			return 2
		}
		return userAdd(*usersFile, pos[0], *roles, *pwStdin, stdin, stdout, stderr)
	case "list":
		if len(pos) != 0 {
			fmt.Fprintln(stderr, "usage: unheaded-auth user list")
			return 2
		}
		return userList(*usersFile, stdout, stderr)
	case "del":
		if len(pos) != 1 {
			fmt.Fprintln(stderr, "usage: unheaded-auth user del <name>")
			return 2
		}
		return userDel(*usersFile, pos[0], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unheaded-auth user: unknown subcommand %q (add, list, del)\n", sub)
		return 2
	}
}

// parseInterspersed lets flags follow positional arguments
// (`user add bob --roles viewer`), which the flag package alone stops at.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func userAdd(path, name, roleList string, pwStdin bool, stdin io.Reader, stdout, stderr io.Writer) int {
	if !validUsername(name) {
		fmt.Fprintf(stderr, "unheaded-auth: bad username %q (lower-case letters, digits, . _ - @; at most %d)\n", name, maxUsernameLen)
		return 2
	}
	roles, err := validateRoles(splitList(roleList))
	if err != nil {
		fmt.Fprintf(stderr, "unheaded-auth: --roles: %v\n", err)
		return 2
	}
	uf, err := readUsersFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "unheaded-auth: %v\n", err)
		return 1
	}
	for _, u := range uf.Users {
		if u.Username == name {
			fmt.Fprintf(stderr, "unheaded-auth: user %q exists (user del first to replace it)\n", name)
			return 2
		}
	}
	var pw string
	if pwStdin {
		line, err := bufio.NewReader(io.LimitReader(stdin, maxPasswordLen+2)).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintf(stderr, "unheaded-auth: read password: %v\n", err)
			return 1
		}
		pw = strings.TrimRight(line, "\r\n")
	} else {
		if pw, err = readPassword(stderr, "Password for "+name+": "); err != nil {
			fmt.Fprintf(stderr, "unheaded-auth: %v (use --password-stdin in scripts)\n", err)
			return 1
		}
		again, err := readPassword(stderr, "Again: ")
		if err != nil {
			fmt.Fprintf(stderr, "unheaded-auth: %v\n", err)
			return 1
		}
		if again != pw {
			fmt.Fprintln(stderr, "unheaded-auth: passwords differ")
			return 2
		}
	}
	if pw == "" {
		fmt.Fprintln(stderr, "unheaded-auth: empty password refused")
		return 2
	}
	if len(pw) > maxPasswordLen {
		fmt.Fprintf(stderr, "unheaded-auth: password longer than %d bytes\n", maxPasswordLen)
		return 2
	}
	h, err := hashPassword(pw, defaultParams)
	if err != nil {
		fmt.Fprintf(stderr, "unheaded-auth: %v\n", err)
		return 1
	}
	uf.Users = append(uf.Users, userRecord{Username: name, Hash: h, Roles: roles})
	if err := writeUsersFile(path, uf); err != nil {
		fmt.Fprintf(stderr, "unheaded-auth: write %s: %v\n", path, err)
		return 1
	}
	fmt.Fprintf(stdout, "added %s (%s) to %s; reload the issuer (SIGHUP) to use it\n", name, strings.Join(roles, ","), path)
	if os.Geteuid() == 0 {
		fmt.Fprintf(stdout, "  the issuer reads this file as its own account: chown unheaded-auth:unheaded-auth %s if root created it\n", path)
	}
	return 0
}

func userList(path string, stdout, stderr io.Writer) int {
	uf, err := readUsersFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "unheaded-auth: %v\n", err)
		return 1
	}
	for _, u := range uf.Users {
		fmt.Fprintf(stdout, "%s\t%s\n", u.Username, strings.Join(u.Roles, ","))
	}
	return 0
}

func userDel(path, name string, stdout, stderr io.Writer) int {
	uf, err := readUsersFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "unheaded-auth: %v\n", err)
		return 1
	}
	kept := uf.Users[:0]
	found := false
	for _, u := range uf.Users {
		if u.Username == name {
			found = true
			continue
		}
		kept = append(kept, u)
	}
	if !found {
		fmt.Fprintf(stderr, "unheaded-auth: no user %q\n", name)
		return 2
	}
	uf.Users = kept
	if err := writeUsersFile(path, uf); err != nil {
		fmt.Fprintf(stderr, "unheaded-auth: write %s: %v\n", path, err)
		return 1
	}
	fmt.Fprintf(stdout, "removed %s from %s; reload the issuer (SIGHUP). Tokens already issued stay valid until they expire (15 min).\n", name, path)
	return 0
}
