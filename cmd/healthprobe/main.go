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
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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
	return utls.BaseURL("127.0.0.1:"+strconv.Itoa(p)) + "/health", nil
}

func run(args []string, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: healthprobe <port|url>")
		return 2
	}
	url, err := target(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "healthprobe:", err)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintln(stderr, "healthprobe:", err)
		return 2
	}
	resp, err := client.Do(req)
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
