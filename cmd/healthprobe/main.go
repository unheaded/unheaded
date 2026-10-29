// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// healthprobe is the container healthcheck for first-party services
// (S-IRON-GATE Phase 8). busybox wget cannot verify a certificate against
// the deployment CA, so once a service listens TLS only the image needs a
// probe that can:
//
//	healthprobe 19000                  # GET <scheme>://127.0.0.1:19000/health
//	healthprobe https://127.0.0.1:20000/ready
//
// The scheme follows UNHEADED_TLS_ENABLED (the container's own
// environment), and https trusts UNHEADED_TLS_CA_CERT only
// (pkg/tls.ClientFromEnv). Exit 0 on a 200, 1 on anything else, 2 on
// usage errors. It never follows a redirect.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/cmdline"
	utls "unheaded/pkg/tls"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// target turns a port or a URL into the URL to probe.
func target(arg string) (string, error) {
	if strings.Contains(arg, "://") {
		if !strings.HasPrefix(arg, "http://") && !strings.HasPrefix(arg, "https://") {
			return "", fmt.Errorf("unsupported URL %q", arg)
		}
		return arg, nil
	}
	p, err := strconv.Atoi(arg)
	if err != nil || p < 1 || p > 65535 {
		return "", fmt.Errorf("want a port (1-65535) or a URL, got %q", arg)
	}
	scheme := "http"
	if utls.ServesTLS() { // the container's own listener is TLS
		scheme = "https"
	}
	return scheme + "://127.0.0.1:" + strconv.Itoa(p) + "/health", nil
}

func run(args []string, stderr io.Writer) int {
	spec := cmdline.Spec{
		Name:     "healthprobe",
		Purpose:  "Probe a Kingdom service's /health endpoint by port or URL; exit 0 when healthy.",
		Operands: "<port|url>",
		MinArgs:  1,
		MaxArgs:  1,
		Examples: []string{"healthprobe 19100", "healthprobe https://127.0.0.1:19006/health"},
		Details:  "Exit codes: 0 healthy, 1 unhealthy or unreachable, 2 usage.",
	}
	fs := flag.NewFlagSet("healthprobe", flag.ContinueOnError)
	if code, exit := cmdline.ParseSet(fs, spec, args, os.Stdout, stderr); exit {
		return code
	}
	url, err := target(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "healthprobe:", err)
		fmt.Fprintln(stderr, "Try 'healthprobe --help' for more information.")
		return 2
	}
	client, err := utls.ClientFromEnv(4 * time.Second)
	if err != nil {
		fmt.Fprintln(stderr, "healthprobe:", err)
		return 1
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	// #nosec G704 -- url is this container's own health endpoint: a port (127.0.0.1) or an http(s) URL from the healthcheck argv (target)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintln(stderr, "healthprobe:", err)
		return 2
	}
	resp, err := client.Do(req) // #nosec G704 -- see the request above
	if err != nil {
		fmt.Fprintln(stderr, "healthprobe:", err)
		return 1
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "healthprobe: %s: HTTP %d\n", url, resp.StatusCode)
		return 1
	}
	return 0
}
