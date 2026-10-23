// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/logger"
)

// lxdREST is an LXDClient over LXD's REST API, standard library only: the
// unix socket, or HTTPS with a client certificate and a pinned server
// certificate. Deploy the HTTPS one. A certificate restricted to one
// project cannot even see instances outside it (west's pihole); the socket
// is root-equivalent and sees everything.
//
// Every start, stop, restart and delete re-reads the instance and refuses
// one without ManagedLabel=true, whatever the caller planned.
type lxdREST struct {
	http    *http.Client
	base    string // "http://lxd" over the socket
	project string // "" = the server's default project
}

// LXDRESTConfig selects and authenticates the connection. Socket and URL
// are exclusive; URL needs all three PEM paths.
type LXDRESTConfig struct {
	Socket     string
	URL        string
	ClientCert string
	ClientKey  string
	ServerCert string // pinned: the connection fails unless it is this exact certificate
	Project    string
}

const (
	lxdMaxResponse = 32 << 20
	lxdWaitSlice   = 30 * time.Second
)

var (
	errLXDNotFound   = errors.New("lxd: not found")
	errLXDNotManaged = errors.New("lxd: instance is not Unheaded's (no " + ManagedLabel + "=true); refusing")

	// LXD's own instance-name rule.
	lxdNameRe = regexp.MustCompile(`^[A-Za-z]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	// Label keys become user.unheaded.label.<key> config keys.
	lxdLabelKeyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

	// Image remotes Cuirass knows, as in `lxc launch <remote>:<alias>`. An
	// image with no remote is a local alias.
	lxdImageRemotes = map[string]string{
		"images": "https://images.lxd.canonical.com",
		"ubuntu": "https://cloud-images.ubuntu.com/releases",
	}
)

func validLXDName(name string) error {
	if !lxdNameRe.MatchString(name) {
		return fmt.Errorf("lxd: invalid instance name %q (letters, digits, hyphens; starts with a letter; at most 63)", name)
	}
	return nil
}

func newLXDREST(cfg LXDRESTConfig) (*lxdREST, error) {
	if cfg.Project != "" {
		if err := validLXDName(cfg.Project); err != nil {
			return nil, fmt.Errorf("lxd project: %w", err)
		}
	}
	switch {
	case cfg.Socket != "" && cfg.URL != "":
		return nil, errors.New("lxd: set a socket or a URL, not both")
	case cfg.Socket != "":
		socket := cfg.Socket
		tr := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		}
		return &lxdREST{http: &http.Client{Transport: tr}, base: "http://lxd", project: cfg.Project}, nil
	case cfg.URL != "":
		u, err := url.Parse(cfg.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || (u.Path != "" && u.Path != "/") {
			return nil, fmt.Errorf("lxd: URL must be https://host[:port], got %q", cfg.URL)
		}
		if cfg.ClientCert == "" || cfg.ClientKey == "" || cfg.ServerCert == "" {
			return nil, errors.New("lxd: an HTTPS URL needs a client cert, client key and pinned server cert")
		}
		cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("lxd client cert: %w", err)
		}
		pinned, err := readPEMCert(cfg.ServerCert)
		if err != nil {
			return nil, fmt.Errorf("lxd server cert: %w", err)
		}
		tr := &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{cert},
			// LXD's server certificate is self-signed; like `lxc`, trust is
			// the exact certificate, checked in VerifyConnection.
			InsecureSkipVerify: true, // #nosec G402 -- chain verification is replaced by an exact certificate pin in VerifyConnection
			VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.PeerCertificates) == 0 || !bytes.Equal(cs.PeerCertificates[0].Raw, pinned) {
					return errors.New("lxd: server certificate does not match the pinned one")
				}
				return nil
			},
		}}
		return &lxdREST{http: &http.Client{Transport: tr}, base: "https://" + u.Host, project: cfg.Project}, nil
	default:
		return nil, errors.New("lxd: no socket or URL")
	}
}

func readPEMCert(path string) ([]byte, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-configured path
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%s: no PEM CERTIFICATE block", path)
	}
	return block.Bytes, nil
}

type lxdResponse struct {
	Type       string          `json:"type"` // sync, async, error
	StatusCode int             `json:"status_code"`
	ErrorCode  int             `json:"error_code"`
	Error      string          `json:"error"`
	Operation  string          `json:"operation"`
	Metadata   json.RawMessage `json:"metadata"`
}

// do sends one request. The project goes on every request, so a project
// that does not exist fails loudly instead of falling back to default.
func (c *lxdREST) do(ctx context.Context, method, path string, query url.Values, body any) (*lxdResponse, error) {
	if query == nil {
		query = url.Values{}
	}
	if c.project != "" {
		query.Set("project", c.project)
	}
	target := c.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lxd %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, lxdMaxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("lxd %s %s: %w", method, path, err)
	}
	if len(data) > lxdMaxResponse {
		return nil, fmt.Errorf("lxd %s %s: response over %d bytes", method, path, lxdMaxResponse)
	}
	var r lxdResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("lxd %s %s: HTTP %d, not an LXD response: %w", method, path, resp.StatusCode, err)
	}
	if r.Type == "error" || resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusNotFound || r.ErrorCode == http.StatusNotFound {
			return nil, fmt.Errorf("lxd %s %s: %w: %s", method, path, errLXDNotFound, r.Error)
		}
		return nil, fmt.Errorf("lxd %s %s: HTTP %d: %s", method, path, resp.StatusCode, r.Error)
	}
	return &r, nil
}

// wait blocks until an async operation finishes, in slices so ctx is
// honoured; sync responses are already done.
func (c *lxdREST) wait(ctx context.Context, r *lxdResponse) error {
	if r.Type != "async" {
		return nil
	}
	op := r.Operation
	id := strings.TrimPrefix(op, "/1.0/operations/")
	if id == op || id == "" || strings.ContainsAny(id, "/?#") {
		return fmt.Errorf("lxd: unexpected operation %q", op)
	}
	for {
		slice := lxdWaitSlice
		if dl, ok := ctx.Deadline(); ok {
			slice = min(slice, time.Until(dl))
		}
		secs := max(int(slice/time.Second), 1)
		wr, err := c.do(ctx, http.MethodGet, "/1.0/operations/"+id+"/wait", url.Values{"timeout": {strconv.Itoa(secs)}}, nil)
		if err != nil {
			return err
		}
		var st struct {
			StatusCode int    `json:"status_code"`
			Err        string `json:"err"`
		}
		if err := json.Unmarshal(wr.Metadata, &st); err != nil {
			return fmt.Errorf("lxd operation %s: %w", id, err)
		}
		switch {
		case st.StatusCode == http.StatusOK:
			return nil
		case st.StatusCode >= 400:
			return fmt.Errorf("lxd operation %s failed (%d): %s", id, st.StatusCode, st.Err)
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("lxd operation %s: %w", id, err)
		}
	}
}

type lxdInstance struct {
	Name   string            `json:"name"`
	Status string            `json:"status"`
	Config map[string]string `json:"config"`
	State  *struct {
		Network map[string]struct {
			Addresses []struct {
				Family  string `json:"family"`
				Address string `json:"address"`
				Scope   string `json:"scope"`
			} `json:"addresses"`
		} `json:"network"`
	} `json:"state"`
}

func (in *lxdInstance) managed() bool { return in.Config[ManagedLabel] == "true" }

// toState maps an instance to Cuirass's view. LXD has no health check, so
// health is "unknown"; the address is the first global IPv4, else IPv6,
// in interface-name order.
func (in *lxdInstance) toState() *ContainerState {
	s := &ContainerState{ID: in.Name, Status: strings.ToLower(in.Status), Health: "unknown", Managed: in.managed()}
	if in.State == nil {
		return s
	}
	ifaces := make([]string, 0, len(in.State.Network))
	for name := range in.State.Network {
		ifaces = append(ifaces, name)
	}
	sort.Strings(ifaces)
	for _, family := range []string{"inet", "inet6"} {
		for _, name := range ifaces {
			for _, a := range in.State.Network[name].Addresses {
				if a.Family == family && a.Scope == "global" {
					s.IP = a.Address
					return s
				}
			}
		}
	}
	return s
}

// ListContainers returns every instance in the project, managed or not:
// an unmanaged one must still be seen to refuse a name collision.
func (c *lxdREST) ListContainers(ctx context.Context) (map[string]*ContainerState, error) {
	r, err := c.do(ctx, http.MethodGet, "/1.0/instances", url.Values{"recursion": {"2"}}, nil)
	if err != nil {
		return nil, err
	}
	var list []lxdInstance
	if err := json.Unmarshal(r.Metadata, &list); err != nil {
		return nil, fmt.Errorf("lxd instances: %w", err)
	}
	out := make(map[string]*ContainerState, len(list))
	for i := range list {
		out[list[i].Name] = list[i].toState()
	}
	return out, nil
}

func (c *lxdREST) getInstance(ctx context.Context, name string) (*lxdInstance, error) {
	if err := validLXDName(name); err != nil {
		return nil, err
	}
	r, err := c.do(ctx, http.MethodGet, "/1.0/instances/"+name, nil, nil)
	if err != nil {
		return nil, err
	}
	var in lxdInstance
	if err := json.Unmarshal(r.Metadata, &in); err != nil {
		return nil, fmt.Errorf("lxd instance %s: %w", name, err)
	}
	return &in, nil
}

// requireManaged is the client's own safe-mode check, independent of the
// reconciler's plan.
func (c *lxdREST) requireManaged(ctx context.Context, name string) (*lxdInstance, error) {
	in, err := c.getInstance(ctx, name)
	if err != nil {
		return nil, err
	}
	if !in.managed() {
		return nil, fmt.Errorf("%s: %w", name, errLXDNotManaged)
	}
	return in, nil
}

// imageSource turns "remote:alias" or a local "alias" into an LXD source.
func imageSource(image string) (map[string]string, error) {
	if image == "" || len(image) > 255 || strings.ContainsAny(image, " \t\r\n") {
		return nil, fmt.Errorf("lxd: invalid image %q", image)
	}
	remote, alias, hasRemote := strings.Cut(image, ":")
	if !hasRemote {
		return map[string]string{"type": "image", "alias": image}, nil
	}
	server, ok := lxdImageRemotes[remote]
	if !ok || alias == "" {
		return nil, fmt.Errorf("lxd: unknown image remote in %q (known: images, ubuntu)", image)
	}
	return map[string]string{"type": "image", "alias": alias, "server": server, "protocol": "simplestreams"}, nil
}

// CreateContainer creates a stopped container carrying ManagedLabel. CPU is
// a core count and Memory bytes; zero leaves LXD's default.
func (c *lxdREST) CreateContainer(ctx context.Context, spec *ContainerSpec) error {
	if err := validLXDName(spec.ID); err != nil {
		return err
	}
	src, err := imageSource(spec.Image)
	if err != nil {
		return err
	}
	if spec.CPU < 0 || spec.Memory < 0 {
		return errors.New("lxd: negative cpu or memory")
	}
	config := map[string]string{}
	for k, v := range spec.Labels {
		if !lxdLabelKeyRe.MatchString(k) {
			return fmt.Errorf("lxd: invalid label key %q", k)
		}
		config["user.unheaded.label."+k] = v
	}
	config[ManagedLabel] = "true"
	if spec.CPU > 0 {
		config["limits.cpu"] = strconv.Itoa(spec.CPU)
	}
	if spec.Memory > 0 {
		config["limits.memory"] = strconv.FormatInt(spec.Memory, 10) + "B"
	}
	r, err := c.do(ctx, http.MethodPost, "/1.0/instances", nil, map[string]any{
		"name": spec.ID, "type": "container", "source": src, "config": config,
	})
	if err != nil {
		return err
	}
	return c.wait(ctx, r)
}

func (c *lxdREST) setState(ctx context.Context, name, action string, force bool) error {
	r, err := c.do(ctx, http.MethodPut, "/1.0/instances/"+name+"/state", nil, map[string]any{
		"action": action, "force": force, "timeout": 30,
	})
	if err != nil {
		return err
	}
	return c.wait(ctx, r)
}

func (c *lxdREST) StartContainer(ctx context.Context, name string) error {
	if _, err := c.requireManaged(ctx, name); err != nil {
		return err
	}
	return c.setState(ctx, name, "start", false)
}

func (c *lxdREST) StopContainer(ctx context.Context, name string, force bool) error {
	if _, err := c.requireManaged(ctx, name); err != nil {
		return err
	}
	return c.setState(ctx, name, "stop", force)
}

func (c *lxdREST) RestartContainer(ctx context.Context, name string) error {
	if _, err := c.requireManaged(ctx, name); err != nil {
		return err
	}
	return c.setState(ctx, name, "restart", false)
}

// DeleteContainer deletes a managed instance; LXD refuses to delete a
// running one, so force stops it first.
func (c *lxdREST) DeleteContainer(ctx context.Context, name string, force bool) error {
	in, err := c.requireManaged(ctx, name)
	if err != nil {
		return err
	}
	if force && !strings.EqualFold(in.Status, "stopped") {
		if err := c.setState(ctx, name, "stop", true); err != nil {
			return fmt.Errorf("stop before delete: %w", err)
		}
	}
	r, err := c.do(ctx, http.MethodDelete, "/1.0/instances/"+name, nil, nil)
	if err != nil {
		return err
	}
	return c.wait(ctx, r)
}

// lxdServerInfo is what Start logs: whether this client is trusted, and
// what it talks to.
type lxdServerInfo struct {
	Auth        string `json:"auth"`
	Environment struct {
		ServerVersion string `json:"server_version"`
		Project       string `json:"project"`
	} `json:"environment"`
}

func (c *lxdREST) serverInfo(ctx context.Context) (*lxdServerInfo, error) {
	r, err := c.do(ctx, http.MethodGet, "/1.0", nil, nil)
	if err != nil {
		return nil, err
	}
	var info lxdServerInfo
	if err := json.Unmarshal(r.Metadata, &info); err != nil {
		return nil, fmt.Errorf("lxd /1.0: %w", err)
	}
	if info.Auth != "trusted" {
		return &info, fmt.Errorf("lxd: this client is %q, not trusted (add its certificate with lxc config trust add)", info.Auth)
	}
	return &info, nil
}

// newLXDClient builds the configured backend. For a real one it checks
// once that LXD answers and trusts this client; a failure is logged, not
// fatal: reconcile skips cycles (cuirass_lxd_up 0) until LXD answers.
func newLXDClient(cfg *Config, log *logger.Logger) (LXDClient, error) {
	rc := LXDRESTConfig{Project: cfg.LXDProject}
	switch cfg.LXDBackend {
	case "", "mock":
		return NewMockLXDClient(log), nil
	case "unix":
		rc.Socket = cfg.LXDSocket
	case "https":
		rc.URL, rc.ClientCert, rc.ClientKey, rc.ServerCert = cfg.LXDURL, cfg.LXDClientCert, cfg.LXDClientKey, cfg.LXDServerCert
	default:
		return nil, fmt.Errorf("unknown LXD_BACKEND %q (mock, unix, https)", cfg.LXDBackend)
	}
	c, err := newLXDREST(rc)
	if err != nil {
		return nil, err
	}
	if cfg.LXDBackend == "unix" {
		log.Warn().Str("socket", rc.Socket).
			Msg("LXD unix socket is root-equivalent and sees every project; prefer LXD_BACKEND=https with a restricted certificate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := c.serverInfo(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("LXD not ready; reconcile waits for it")
		return c, nil
	}
	log.Info().Str("server_version", info.Environment.ServerVersion).Str("project", info.Environment.Project).
		Msg("LXD connected")
	return c, nil
}
