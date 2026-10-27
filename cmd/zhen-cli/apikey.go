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
	"strings"
)

// defaultConfPath is $XDG_CONFIG_HOME/unheaded/zhen-cli.conf, normally
// ~/.config/unheaded/zhen-cli.conf.
func defaultConfPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "unheaded", "zhen-cli.conf")
}

// loadAPIKey finds zhen-agentd's API key: ZHEN_API_KEY, then the older
// ZHEN_AGENTD_API_KEY, then `api_key = <key>` in confPath. The file must
// be 0600 (owner only): one that group or others can read is refused
// even when the environment has the key, so a leaked key is noticed
// instead of silently kept on disk. No file and no env: no key (the
// daemon then answers 401 unless it runs -no-auth on loopback).
func loadAPIKey(getenv func(string) string, confPath string) (string, error) {
	var fileKey string
	if confPath != "" {
		k, err := readConfKey(confPath)
		if err != nil {
			return "", err
		}
		fileKey = k
	}
	for _, name := range []string{"ZHEN_API_KEY", "ZHEN_AGENTD_API_KEY"} {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			return v, nil
		}
	}
	return fileKey, nil
}

func readConfKey(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- the user's own config path
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("%s is mode %04o: it holds an API key and must be 0600 (chmod 600 %s)", path, fi.Mode().Perm(), path)
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == "api_key" {
			if v = strings.TrimSpace(v); v != "" {
				return v, nil
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return "", fmt.Errorf("%s has no api_key = <key> line", path)
}
