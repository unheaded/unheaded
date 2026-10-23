// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Host facts: what collect-host writes to compliance/crypto/hosts/<host>.json
// and scan merges. Hostnames only, never addresses; WireGuard PSKs are
// reduced to set/none before they leave the host and again on parse.

const kindHostSSHD = "host-sshd"
const kindHostWG = "host-wireguard"
const kindHostUnreachable = "host-unreachable"

// HostFacts is one host's read-only crypto facts.
type HostFacts struct {
	Host        string     `json:"host"`
	Unreachable bool       `json:"unreachable,omitempty"`
	SSHD        *SSHDFacts `json:"sshd,omitempty"`
	OpenSSL     string     `json:"openssl,omitempty"`
	WireGuard   *WGFacts   `json:"wireguard,omitempty"`
	Postgres    string     `json:"postgres,omitempty"`
}

// SSHDFacts is the effective sshd configuration (sshd -T).
type SSHDFacts struct {
	Version           string   `json:"version"`
	Kex               []string `json:"kex"`
	HostKeyAlgorithms []string `json:"hostkey_algorithms,omitempty"`
	Ciphers           []string `json:"ciphers,omitempty"`
}

// WGFacts is WireGuard state without key material.
type WGFacts struct {
	Interfaces   []string `json:"interfaces"`
	Peers        []WGPeer `json:"peers"`
	PSKFileDates []string `json:"psk_file_dates,omitempty"`
}

// WGPeer is one peer: its interface and whether a PSK is set. Peers are
// listed in wg's order; no public key is recorded.
type WGPeer struct {
	Interface string `json:"interface"`
	PSK       string `json:"psk"` // "set" or "none"
}

// Runner runs one read-only shell command on a host.
type Runner interface {
	Run(ctx context.Context, cmd string) (string, error)
}

type localRunner struct{}

func (localRunner) Run(ctx context.Context, cmd string) (string, error) {
	out, err := exec.CommandContext(ctx, "bash", "-c", cmd).Output() // #nosec G204 -- fixed read-only command strings from hostCommands
	return string(out), err
}

type sshRunner struct{ target string }

var reSSHTarget = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9._-]+$|^[A-Za-z0-9._-]+$`)

func (r sshRunner) Run(ctx context.Context, cmd string) (string, error) {
	out, err := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "--", r.target, cmd).Output() // #nosec G204 -- target validated by reSSHTarget, cmd from hostCommands
	return string(out), err
}

// The only commands collect-host runs. All read-only; the WireGuard one
// drops the key column on the host, so no key is ever transported.
var hostCommands = struct {
	sshV, sshdT, openssl, wgIfaces, wgPSK, pskDates, postgres string
}{
	sshV:     "ssh -V 2>&1",
	sshdT:    "sudo -n sshd -T 2>/dev/null | grep -iE '^(kexalgorithms|hostkeyalgorithms|ciphers) '",
	openssl:  "openssl version 2>/dev/null",
	wgIfaces: "sudo -n wg show interfaces 2>/dev/null",
	wgPSK:    `sudo -n wg show all preshared-keys 2>/dev/null | awk '{print $1, ($3=="(none)")?"none":"set"}'`,
	pskDates: "sudo -n find /etc/wireguard -maxdepth 1 -name '*preshared*' -printf '%TY-%Tm-%Td\\n' 2>/dev/null",
	postgres: "postgres --version 2>/dev/null || true",
}

var (
	reBase64Key = regexp.MustCompile(`[A-Za-z0-9+/]{42,43}=`)
	reIPv4      = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	reIPv6      = regexp.MustCompile(`(?i)\b(?:fd[0-9a-f]{2}|fe80|2[0-9a-f]{3}):[0-9a-f:]*:[0-9a-f:]*`)
	reHostName  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	reIface     = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)
	reDate      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)
)

// collectHost gathers facts through r. A command that fails is recorded as
// absent, not guessed; if nothing at all answers, the host is unreachable.
func collectHost(ctx context.Context, host string, r Runner) (*HostFacts, error) {
	if !reHostName.MatchString(host) {
		return nil, fmt.Errorf("host %q: want a short lowercase hostname", host)
	}
	f := &HostFacts{Host: host}
	sshV, errV := r.Run(ctx, hostCommands.sshV)
	sshdT, _ := r.Run(ctx, hostCommands.sshdT)
	if errV != nil && strings.TrimSpace(sshV) == "" {
		return &HostFacts{Host: host, Unreachable: true}, nil
	}
	if s := parseSSHD(sshV, sshdT); s != nil {
		f.SSHD = s
	}
	if v, err := r.Run(ctx, hostCommands.openssl); err == nil {
		f.OpenSSL = firstLine(v)
	}
	ifaces, _ := r.Run(ctx, hostCommands.wgIfaces)
	psk, _ := r.Run(ctx, hostCommands.wgPSK)
	dates, _ := r.Run(ctx, hostCommands.pskDates)
	f.WireGuard = parseWG(ifaces, psk, dates)
	if v, err := r.Run(ctx, hostCommands.postgres); err == nil {
		f.Postgres = firstLine(v)
	}
	return f, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(strings.TrimSpace(s), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseSSHD(sshV, sshdT string) *SSHDFacts {
	s := &SSHDFacts{Version: firstLine(sshV)}
	for _, ln := range strings.Split(sshdT, "\n") {
		fields := strings.Fields(ln)
		if len(fields) != 2 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "kexalgorithms":
			s.Kex = splitList(fields[1])
		case "hostkeyalgorithms":
			s.HostKeyAlgorithms = splitList(fields[1])
		case "ciphers":
			s.Ciphers = splitList(fields[1])
		}
	}
	if s.Version == "" && len(s.Kex) == 0 {
		return nil
	}
	return s
}

// parseWG reduces `wg show` output to interface names and set/none per
// peer. It accepts the host-side reduced form ("wg0 set") and, defensively,
// the raw three-column form ("wg0 <peer-pubkey> <psk>"): only the third
// column's (none)-ness survives, never a key.
func parseWG(ifaces, psk, dates string) *WGFacts {
	w := &WGFacts{}
	for _, name := range strings.Fields(ifaces) {
		if reIface.MatchString(name) {
			w.Interfaces = append(w.Interfaces, name)
		}
	}
	for _, ln := range strings.Split(psk, "\n") {
		f := strings.Fields(ln)
		var iface, state string
		switch len(f) {
		case 2:
			iface, state = f[0], f[1]
			if state != "set" && state != "none" {
				continue
			}
		case 3:
			iface, state = f[0], "set"
			if f[2] == "(none)" {
				state = "none"
			}
		default:
			continue
		}
		if !reIface.MatchString(iface) {
			continue
		}
		w.Peers = append(w.Peers, WGPeer{Interface: iface, PSK: state})
	}
	for _, d := range strings.Fields(dates) {
		if reDate.MatchString(d) {
			w.PSKFileDates = append(w.PSKFileDates, d[:10])
		}
	}
	sort.Strings(w.PSKFileDates)
	if len(w.Interfaces) == 0 && len(w.Peers) == 0 {
		return nil
	}
	return w
}

// errLeak means the encoded facts contain something that must never be
// written: a WireGuard-key-shaped string or an IP address.
var errLeak = errors.New("refusing to write host facts")

// encodeHostFacts renders facts as indented JSON, refusing key material
// and addresses as a last line of defence.
func encodeHostFacts(f *HostFacts) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	switch {
	case reBase64Key.Match(b):
		return nil, fmt.Errorf("%w: a 44-character base64 value (key-shaped) is present", errLeak)
	case reIPv4.Match(b), reIPv6.Match(b):
		return nil, fmt.Errorf("%w: an IP address is present (hostnames only)", errLeak)
	}
	return b, nil
}

// writeHostFacts collects and writes one host's facts file.
func writeHostFacts(host string, r Runner, outPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	f, err := collectHost(ctx, host, r)
	if err != nil {
		return err
	}
	b, err := encodeHostFacts(f)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, b, 0o644) // #nosec G306 -- checked-in inventory, no secrets (encodeHostFacts refuses key-shaped data)
}

// scanHosts turns compliance/crypto/hosts/*.json into assets. scan never
// connects anywhere; it only reads what collect-host wrote.
func scanHosts(t *Tree) ([]Asset, error) {
	var out []Asset
	for _, rel := range t.Match(func(f string) bool {
		return strings.HasPrefix(f, "compliance/crypto/hosts/") && path.Ext(f) == ".json"
	}) {
		data, err := t.Read(rel)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		var f HostFacts
		if err := dec.Decode(&f); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		if !reHostName.MatchString(f.Host) {
			return nil, fmt.Errorf("%s: bad host name %q", rel, f.Host)
		}
		base := Asset{Path: rel, Scanner: "host", Owner: ownerFirstParty, LinkState: linkEnabled, AssetType: assetProtocol}
		if f.Unreachable {
			a := base
			a.Kind, a.Name, a.Protocol, a.PQStatus = kindHostUnreachable, "host "+f.Host, protoOther, pqUnknown
			a.Detail = "not assessed: the host did not answer collect-host"
			out = append(out, a)
			continue
		}
		if f.SSHD != nil {
			a := base
			a.Kind, a.Name, a.Protocol = kindHostSSHD, "sshd on "+f.Host, protoSSH
			a.Version, a.Groups, a.PQStatus = f.SSHD.Version, f.SSHD.Kex, classifySSHKex(f.SSHD.Kex)
			a.Detail = "effective sshd -T KexAlgorithms"
			out = append(out, a)
		}
		if f.WireGuard != nil {
			for _, p := range f.WireGuard.Peers {
				a := base
				a.Kind, a.Name, a.Protocol = kindHostWG, "WireGuard "+p.Interface+" peer on "+f.Host, protoOther
				a.PQStatus, a.Detail = pqClassical, "no pre-shared key: X25519 only"
				if p.PSK == "set" {
					a.PQStatus = pqPSKStatic
					a.Detail = "static pre-shared key"
					if len(f.WireGuard.PSKFileDates) > 0 {
						a.Detail += "; PSK file dated " + strings.Join(f.WireGuard.PSKFileDates, ", ")
					}
				}
				out = append(out, a)
			}
		}
	}
	return out, nil
}
