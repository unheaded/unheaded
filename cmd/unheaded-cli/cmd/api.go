// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/ports"
)

// The CLI reads and changes the Kingdom only through its services' HTTP
// APIs: Cuirass (the context's endpoint) for containers, Akira for health.
// A command with no API behind it fails; it never prints sample data.

const apiMaxResponse = 8 << 20

var apiClient = &http.Client{Timeout: 10 * time.Second}

var (
	defaultAkira     = fmt.Sprintf("http://localhost:%d", ports.Akira)
	defaultDashboard = fmt.Sprintf("http://localhost:%d", ports.DashboardBackend)
)

// cluster returns the context chosen by --context, else the current one.
func (ctx *Context) cluster() (*Cluster, error) {
	name := ctx.Context
	if name == "" {
		name = ctx.Config.CurrentContext
	}
	c, ok := ctx.Config.Contexts[name]
	if !ok {
		return nil, fmt.Errorf("context not found: %s", name)
	}
	// config list shows UNHEADED_ENDPOINT as an override; honour it.
	if e := os.Getenv("UNHEADED_ENDPOINT"); e != "" {
		c.Endpoint = e
	}
	// Config files from before these fields existed.
	if c.Akira == "" {
		c.Akira = defaultAkira
	}
	if c.Dashboard == "" {
		c.Dashboard = defaultDashboard
	}
	return &c, nil
}

// serviceURL joins an http(s) base URL and a path.
func serviceURL(base, path string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("invalid endpoint %q (want http[s]://host:port)", base)
	}
	return strings.TrimRight(base, "/") + path, nil
}

// cuirassAudience is Cuirass's auth identity, the "aud" of tokens for it
// (its key and trust.d name is unheaded-daemon, S-IRON-GATE Phase 2).
const cuirassAudience = "unheaded-daemon"

// cliKeyName is the CLI's own identity: its key's trust.d name and the
// "sub" of the tokens it mints.
const cliKeyName = "unheaded-cli"

// cliHostKeyFile is where a provisioned host keeps the CLI key.
var cliHostKeyFile = "/etc/unheaded/keys/unheaded-cli.key"

// callCuirass is callJSON for Cuirass: it authenticates as the CLI with a
// service token minted for Cuirass (S-IRON-GATE Step 166), unless
// UNHEADED_TOKEN is set.
func callCuirass(method, target string, body, out any) error {
	return callJSONAs(cuirassAudience, method, target, body, out)
}

// cliKeyFile is the CLI's key: AUTH_KEY_FILE, then
// ~/.config/unheaded/cli.key, then the host key; "" when none exists.
func cliKeyFile() string {
	if f := os.Getenv("AUTH_KEY_FILE"); f != "" {
		return f
	}
	var candidates []string
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "unheaded", "cli.key"))
	}
	candidates = append(candidates, cliHostKeyFile)
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// authorization picks the credential for a call to aud: UNHEADED_TOKEN
// (a uhk_ API key or a token) when set; else, for a service audience, a
// token minted from the CLI's key: AUTH_KEY_FILE, then
// ~/.config/unheaded/cli.key, then the host key. An AUTH_KEY_FILE that
// cannot be used is an error, never a silent anonymous call; with no key
// anywhere the call goes without one and the service answers for itself.
func authorization(aud string) (string, error) {
	if tok := os.Getenv("UNHEADED_TOKEN"); tok != "" {
		return "Bearer " + tok, nil
	}
	if aud == "" {
		return "", nil
	}
	keyFile := cliKeyFile()
	if keyFile == "" {
		return "", nil
	}
	m, err := auth.LoadServiceTokenMinter(cliKeyName, keyFile)
	if err != nil {
		return "", fmt.Errorf("CLI key %s: %w", keyFile, err)
	}
	return m.AuthorizationHeader(aud)
}

// callJSON sends body (if any) as JSON and decodes a 2xx response into out
// (if any). UNHEADED_TOKEN, when set, goes as a bearer token.
func callJSON(method, target string, body, out any) error {
	return callJSONAs("", method, target, body, out)
}

// callJSONAs is callJSON authenticating for audience aud (see authorization).
func callJSONAs(aud, method, target string, body, out any) error {
	authz, err := authorization(aud)
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, target, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	resp, err := apiClient.Do(req)
	if err != nil {
		return err // *url.Error already names the method and URL
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, apiMaxResponse+1))
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, target, err)
	}
	if len(data) > apiMaxResponse {
		return fmt.Errorf("%s %s: response over %d bytes", method, target, apiMaxResponse)
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return &apiError{Status: resp.StatusCode, Msg: e.Error}
		}
		return &apiError{Status: resp.StatusCode, Msg: strings.TrimSpace(string(data))}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s %s: %w", method, target, err)
	}
	return nil
}

type apiError struct {
	Status int
	Msg    string
}

func (e *apiError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Msg) }

func isHTTPStatus(err error, status int) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.Status == status
}

// errNoAPI is what a command returns when nothing in the Kingdom serves it
// yet, instead of pretending.
func errNoAPI(what, instead string) error {
	return fmt.Errorf("%s: not available, no Unheaded service provides it yet; %s", what, instead)
}

// noAPICommand keeps a command in help and completion but says plainly that
// nothing serves it yet, and what to use instead.
func noAPICommand(path, short, usage, instead string, aliases ...string) *Command {
	name := path[strings.LastIndex(path, " ")+1:]
	return &Command{
		Name: name, Short: short + " (not available)", Usage: usage, Aliases: aliases,
		RunFunc: func(*Context, []string) error { return errNoAPI("unheaded "+path, instead) },
	}
}
