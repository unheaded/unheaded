// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// readPasswordTTY prompts on w and reads one line from /dev/tty with echo
// off, restoring the terminal afterwards.
func readPasswordTTY(w io.Writer, prompt string) (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", errors.New("no terminal to prompt for the password")
	}
	defer tty.Close()
	fd := int(tty.Fd()) // #nosec G115 -- a file descriptor fits an int
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return "", errors.New("stdin terminal does not support no-echo")
	}
	noEcho := *old
	noEcho.Lflag &^= unix.ECHO
	noEcho.Lflag |= unix.ICANON | unix.ISIG
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &noEcho); err != nil {
		return "", err
	}
	defer func() { _ = unix.IoctlSetTermios(fd, unix.TCSETS, old) }()
	fmt.Fprint(w, prompt)
	line, err := bufio.NewReader(io.LimitReader(tty, maxPasswordLen+2)).ReadString('\n')
	fmt.Fprintln(w)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
