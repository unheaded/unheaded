// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// fileConfig is zhen-claude.conf: `key = value` lines, `#` comments.
// Strict: an unknown key, a repeated key or a bad value is an error.
// Flags given on the command line override it.
type fileConfig struct {
	LocalOnly bool // local_only: the prompt never leaves this host
}

const configMaxBytes = 64 << 10

// defaultConfigPath is $XDG_CONFIG_HOME/unheaded/zhen-claude.conf, else
// ~/.config/unheaded/zhen-claude.conf.
func defaultConfigPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "unheaded", "zhen-claude.conf"), nil
}

// loadConfig reads path. A missing file is an empty config unless
// required (the path was given with -config).
func loadConfig(path string, required bool) (fileConfig, error) {
	var c fileConfig
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) && !required {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if !fi.Mode().IsRegular() {
		return c, fmt.Errorf("%s: not a regular file", path)
	}
	if fi.Size() > configMaxBytes {
		return c, fmt.Errorf("%s: larger than %d bytes", path, configMaxBytes)
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- the operator's own config path
	if err != nil {
		return c, err
	}
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if !ok || key == "" {
			return c, fmt.Errorf("%s:%d: want key = value", path, n)
		}
		if seen[key] {
			return c, fmt.Errorf("%s:%d: %s set twice", path, n, key)
		}
		seen[key] = true
		switch key {
		case "local_only":
			b, err := strconv.ParseBool(val)
			if err != nil {
				return c, fmt.Errorf("%s:%d: local_only %q: want true or false", path, n, val)
			}
			c.LocalOnly = b
		default:
			return c, fmt.Errorf("%s:%d: unknown key %q", path, n, key)
		}
	}
	return c, sc.Err()
}
