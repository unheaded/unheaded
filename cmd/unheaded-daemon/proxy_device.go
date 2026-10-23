// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"unheaded/pkg/ports"
)

// A service's stable endpoint is an LXD proxy device named unheaded-proxy on
// the one instance that is "active" for it (S-REAL-STEEL D-04): blue-green
// moves the device from blue to green. Cuirass creates, changes and removes
// that device only; every other device on the instance is left exactly as
// it was.

const proxyDeviceName = "unheaded-proxy"

// ProxySpec is the instance's unheaded-proxy device: host side Listen,
// container side Connect, both "tcp:<ip>:<port>".
type ProxySpec struct {
	Listen  string `json:"listen"`
	Connect string `json:"connect"`
}

var errProxyNotAllowed = errors.New("proxy device not allowed")

// splitTCP parses "tcp:<ip literal>:<port>" with a single decimal port.
func splitTCP(s string) (net.IP, int, error) {
	rest, ok := strings.CutPrefix(s, "tcp:")
	if !ok {
		return nil, 0, fmt.Errorf("%q: only tcp:<ip>:<port>", s)
	}
	host, portStr, err := net.SplitHostPort(rest)
	if err != nil {
		return nil, 0, fmt.Errorf("%q: %w", s, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, 0, fmt.Errorf("%q: host must be an IP literal", s)
	}
	for _, r := range portStr {
		if r < '0' || r > '9' {
			return nil, 0, fmt.Errorf("%q: port must be one decimal number", s)
		}
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || len(portStr) > 5 {
		return nil, 0, fmt.Errorf("%q: bad port", s)
	}
	return ip, port, nil
}

// validate: Listen is 127.0.0.1, or one of the operator's lab addresses
// (CUIRASS_PROXY_LISTEN_ADDRS), on a port in the user-apps band
// 26000-26666; Connect is 127.0.0.1 inside the container, any port 1-65535.
// Nothing listens on every interface, on a low port, or forwards out of the
// container.
func (p *ProxySpec) validate(labAddrs []string) error {
	lip, lport, err := splitTCP(p.Listen)
	if err != nil {
		return fmt.Errorf("%w: proxy.listen %v", errProxyNotAllowed, err)
	}
	allowed := lip.Equal(net.IPv4(127, 0, 0, 1))
	for _, a := range labAddrs {
		if ip := net.ParseIP(a); ip != nil && !ip.IsUnspecified() && ip.Equal(lip) {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("%w: proxy.listen %s: address must be 127.0.0.1 or a configured lab address", errProxyNotAllowed, p.Listen)
	}
	if lport < ports.CustomerStart || lport > ports.CustomerEnd {
		return fmt.Errorf("%w: proxy.listen %s: port must be in %d-%d", errProxyNotAllowed, p.Listen, ports.CustomerStart, ports.CustomerEnd)
	}
	cip, cport, err := splitTCP(p.Connect)
	if err != nil {
		return fmt.Errorf("%w: proxy.connect %v", errProxyNotAllowed, err)
	}
	if !cip.Equal(net.IPv4(127, 0, 0, 1)) {
		return fmt.Errorf("%w: proxy.connect %s: must be 127.0.0.1 inside the container", errProxyNotAllowed, p.Connect)
	}
	if cport < 1 || cport > 65535 {
		return fmt.Errorf("%w: proxy.connect %s: port out of range", errProxyNotAllowed, p.Connect)
	}
	return nil
}

// device is the LXD device for p.
func (p *ProxySpec) device() map[string]string {
	return map[string]string{"type": "proxy", "listen": p.Listen, "connect": p.Connect, "bind": "host"}
}

func proxyEqual(a, b *ProxySpec) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// proxyFromDevices surfaces our device only; other proxies are not ours.
func proxyFromDevices(devs map[string]map[string]string) *ProxySpec {
	d, ok := devs[proxyDeviceName]
	if !ok || d["type"] != "proxy" {
		return nil
	}
	return &ProxySpec{Listen: d["listen"], Connect: d["connect"]}
}

// instancePutFields are the writable fields of an LXD instance (InstancePut):
// a read-modify-write carries these over and nothing else.
var instancePutFields = []string{"architecture", "config", "devices", "ephemeral", "profiles", "stateful", "description"}

var errLXDPrecondition = errors.New("lxd: instance changed meanwhile (ETag mismatch)")

// UpdateProxy sets (p != nil) or removes (p == nil) the unheaded-proxy device
// on a managed instance by read-modify-write with the instance's ETag in
// If-Match, so a concurrent change is never overwritten: a 412 re-reads and
// tries once more, then gives up. Every other device and field is sent back
// as LXD returned it.
func (c *lxdREST) UpdateProxy(ctx context.Context, name string, p *ProxySpec) error {
	if err := validLXDName(name); err != nil {
		return err
	}
	if p != nil {
		if err := p.validate(c.proxyListen); err != nil {
			return err
		}
	}
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		if err = c.putProxy(ctx, name, p); !errors.Is(err, errLXDPrecondition) {
			return err
		}
	}
	return err
}

func (c *lxdREST) putProxy(ctx context.Context, name string, p *ProxySpec) error {
	r, hdr, err := c.doH(ctx, http.MethodGet, "/1.0/instances/"+name, nil, nil, nil)
	if err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(r.Metadata, &raw); err != nil {
		return fmt.Errorf("lxd instance %s: %w", name, err)
	}
	var cfg map[string]string
	if err := json.Unmarshal(raw["config"], &cfg); err != nil || cfg[ManagedLabel] != "true" {
		return fmt.Errorf("%s: %w", name, errLXDNotManaged)
	}
	devices := map[string]json.RawMessage{}
	if len(raw["devices"]) > 0 && string(raw["devices"]) != "null" {
		if err := json.Unmarshal(raw["devices"], &devices); err != nil {
			return fmt.Errorf("lxd instance %s devices: %w", name, err)
		}
	}
	if p == nil {
		delete(devices, proxyDeviceName)
	} else {
		dev, err := json.Marshal(p.device())
		if err != nil {
			return err
		}
		devices[proxyDeviceName] = dev
	}
	body := map[string]json.RawMessage{}
	for _, k := range instancePutFields {
		if v, ok := raw[k]; ok {
			body[k] = v
		}
	}
	devJSON, err := json.Marshal(devices)
	if err != nil {
		return err
	}
	body["devices"] = devJSON
	reqHdr := http.Header{}
	if etag := hdr.Get("ETag"); etag != "" {
		reqHdr.Set("If-Match", etag)
	}
	pr, _, err := c.doH(ctx, http.MethodPut, "/1.0/instances/"+name, nil, body, reqHdr)
	if err != nil {
		return err
	}
	return c.wait(ctx, pr)
}
