// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	pqtls "unheaded/pkg/tls"
)

// CuirassResolver looks LXD instances up in Cuirass's actual state (GET,
// no key): managed flag, labels, address. The project is Cuirass's own
// (GET /api/v1/info lxd_project): a restricted Cuirass sees one project only.
type CuirassResolver struct {
	base   string
	client *http.Client
}

// NewCuirassResolver resolves through the Cuirass at baseURL.
func NewCuirassResolver(baseURL string) (*CuirassResolver, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("cuirass resolver: bad URL %q", baseURL)
	}
	// https to Cuirass trusts the deployment CA under UNHEADED_TLS_ENABLED
	// (S-IRON-GATE Phase 8); never through a proxy.
	tr, err := pqtls.TransportFromEnv()
	if err != nil {
		return nil, fmt.Errorf("cuirass resolver: %w", err)
	}
	tr.Proxy = nil
	return &CuirassResolver{
		base: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout:       5 * time.Second,
			Transport:     tr,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *CuirassResolver) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cuirass %s answered %d", path, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

// Instance implements Resolver.
func (c *CuirassResolver) Instance(ctx context.Context, name string) (*InstanceInfo, error) {
	var info struct {
		Backend string `json:"lxd_backend"`
		Project string `json:"lxd_project"`
	}
	if err := c.get(ctx, "/api/v1/info", &info); err != nil {
		return nil, err
	}
	if info.Project == "" {
		return nil, errors.New("cuirass reports no LXD project")
	}
	var actual map[string]struct {
		Status   string            `json:"status"`
		IP       string            `json:"ip"`
		Managed  bool              `json:"managed"`
		Labels   map[string]string `json:"labels"`
		Health   string            `json:"health"`
		HostVeth string            `json:"host_veth"`
	}
	if err := c.get(ctx, "/api/v1/state/actual", &actual); err != nil {
		return nil, err
	}
	a, ok := actual[name]
	if !ok {
		return nil, ErrInstanceNotFound
	}
	return &InstanceInfo{Name: name, Status: a.Status, IP: a.IP, Project: info.Project, Managed: a.Managed,
		Labels: a.Labels, Health: a.Health, HostVeth: a.HostVeth}, nil
}
