// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the GNU General Public License v3.0 or later.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// Command cert-gen generates TLS certificates for the Unheaded service mesh.
//
// Usage:
//
//	cert-gen -out ./certs                       # Generate CA + all default service certs
//	cert-gen -out ./certs -services timeguru    # Generate CA + specific service cert
//	cert-gen -out ./certs -services "timeguru,captain,architect"
//	cert-gen -out ./certs -layout per-service -reuse-ca   # the compose layout
//
// Generates (flat, the default):
//
//	<out>/ca.pem           CA certificate
//	<out>/ca-key.pem       CA private key
//	<out>/<service>.pem    Service certificate
//	<out>/<service>-key.pem Service private key
//
// With -layout per-service (S-IRON-GATE Phase 8: each container mounts only
// its own directory, so only its own key):
//
//	<out>/ca.pem, <out>/ca-key.pem
//	<out>/<service>/{<service>.pem,<service>-key.pem,ca.pem}
//
// -reuse-ca keeps an existing <out>/ca.pem + ca-key.pem (every service keeps
// trusting the same CA on a re-run); without it a new CA is made.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unheaded/pkg/cmdline"
	unheadedtls "unheaded/pkg/tls"
)

// defaultServices is the set of first-party services that listen TLS
// (compose service names, which are the names callers dial) plus the
// operator CLI's client certificate. IPs, where set, are LXD bridge
// addresses (lxdbr0 10.10.10.0/24) added as SANs so certificates are valid
// over the bridge. localhost / 127.0.0.1 / ::1 SANs are added
// automatically by pkg/tls.IssueCert; DNS lists extra names (the auth
// identity when it differs from the compose name).
var defaultServices = []serviceSpec{
	{Name: "timeguru", IP: "10.10.10.20"},
	{Name: "captain", IP: "10.10.10.21"},
	{Name: "architect", IP: "10.10.10.22"},
	{Name: "micromanager", IP: "10.10.10.23"},
	{Name: "monad", IP: "10.10.10.24"},
	{Name: "sophia", IP: "10.10.10.25"},
	{Name: "dashboard-backend", IP: "10.10.10.26"},
	{Name: "kanban-app", IP: "10.10.10.27"},
	{Name: "wotan", IP: "10.10.10.10"},
	{Name: "gateway", IP: "10.10.10.100"},
	{Name: "akira"},
	{Name: "cuirass", DNS: []string{"unheaded-daemon"}},
	{Name: "auth", DNS: []string{"unheaded-auth"}},
	{Name: "sword"},
	{Name: "unheaded-cli"},
}

type serviceSpec struct {
	Name string
	IP   string
	DNS  []string
}

type options struct {
	out      string
	services string
	ttl      time.Duration
	layout   string
	reuseCA  bool
}

func main() {
	var o options
	flag.StringVar(&o.out, "out", "./certs", "Output directory for certificates")
	flag.StringVar(&o.services, "services", "", "Comma-separated service names (default: all)")
	flag.DurationVar(&o.ttl, "ttl", 24*time.Hour, "Certificate TTL")
	flag.StringVar(&o.layout, "layout", "flat", "flat (<out>/<svc>.pem) or per-service (<out>/<svc>/<svc>.pem + ca.pem)")
	flag.BoolVar(&o.reuseCA, "reuse-ca", false, "keep an existing <out>/ca.pem + ca-key.pem instead of making a new CA")
	cmdline.Parse(cmdline.Spec{
		Name:     "cert-gen",
		Purpose:  "Generate TLS certificates for the Unheaded service mesh.",
		Examples: []string{"cert-gen -out /etc/unheaded/certs"},
	})
	if err := generate(o, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// loadOrNewCA returns the CA in dir when reuse is set and both files are
// there, else a new one (which is then written).
func loadOrNewCA(dir string, reuse bool) (ca *unheadedtls.CA, reused bool, err error) {
	caPath := filepath.Join(dir, "ca.pem")
	keyPath := filepath.Join(dir, "ca-key.pem")
	if reuse {
		certPEM, cerr := os.ReadFile(caPath) // #nosec G304 -- operator-chosen output directory
		keyPEM, kerr := os.ReadFile(keyPath) // #nosec G304 -- operator-chosen output directory
		switch {
		case cerr == nil && kerr == nil:
			ca, err := unheadedtls.LoadCA(certPEM, keyPEM)
			if err != nil {
				return nil, false, fmt.Errorf("reuse CA in %s: %w", dir, err)
			}
			return ca, true, nil
		case errors.Is(cerr, fs.ErrNotExist) && errors.Is(kerr, fs.ErrNotExist):
			// first run: make one below
		default:
			return nil, false, fmt.Errorf("reuse CA in %s: need both ca.pem and ca-key.pem (cert: %v, key: %v)", dir, cerr, kerr)
		}
	}
	ca, err = unheadedtls.NewCA()
	if err != nil {
		return nil, false, fmt.Errorf("NewCA: %w", err)
	}
	if err := os.WriteFile(caPath, ca.CertPEM(), 0644); err != nil { // #nosec G306 -- 0644 — X.509 certificates are public material; private keys alongside are 0600
		return nil, false, fmt.Errorf("write %s: %w", caPath, err)
	}
	if err := os.WriteFile(keyPath, ca.KeyPEM(), 0600); err != nil {
		return nil, false, fmt.Errorf("write %s: %w", keyPath, err)
	}
	return ca, false, nil
}

func selectServices(list string) []serviceSpec {
	if list == "" {
		return defaultServices
	}
	var specs []serviceSpec
	for _, name := range strings.Split(list, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		spec := serviceSpec{Name: name}
		for _, d := range defaultServices {
			if d.Name == name {
				spec = d
			}
		}
		specs = append(specs, spec)
	}
	return specs
}

func generate(o options, stdout io.Writer) error {
	if o.layout != "flat" && o.layout != "per-service" {
		return fmt.Errorf("-layout %q: want flat or per-service", o.layout)
	}
	if err := os.MkdirAll(o.out, 0750); err != nil {
		return fmt.Errorf("mkdir %s: %w", o.out, err)
	}
	ca, reused, err := loadOrNewCA(o.out, o.reuseCA)
	if err != nil {
		return err
	}
	if reused {
		fmt.Fprintf(stdout, "CA (kept): %s\n", filepath.Join(o.out, "ca.pem"))
	} else {
		fmt.Fprintf(stdout, "CA certificate: %s\n", filepath.Join(o.out, "ca.pem"))
		fmt.Fprintf(stdout, "CA private key: %s\n", filepath.Join(o.out, "ca-key.pem"))
	}

	specs := selectServices(o.services)
	for _, svc := range specs {
		if strings.ContainsAny(svc.Name, `/\`) || svc.Name == "." || svc.Name == ".." {
			return fmt.Errorf("service name %q is not a file name", svc.Name)
		}
		var ips []net.IP
		if ip := net.ParseIP(svc.IP); ip != nil {
			ips = append(ips, ip)
		}
		cert, err := ca.IssueCert(unheadedtls.CertRequest{
			ServiceName: svc.Name,
			DNSNames:    svc.DNS,
			IPAddresses: ips,
			TTL:         o.ttl,
		})
		if err != nil {
			return fmt.Errorf("IssueCert(%s): %w", svc.Name, err)
		}
		dir := o.out
		if o.layout == "per-service" {
			dir = filepath.Join(o.out, svc.Name)
			if err := os.MkdirAll(dir, 0750); err != nil {
				return fmt.Errorf("mkdir %s: %w", dir, err)
			}
			caCopy := filepath.Join(dir, "ca.pem")
			if err := os.WriteFile(caCopy, ca.CertPEM(), 0644); err != nil { // #nosec G306 -- 0644 — the CA certificate is public material
				return fmt.Errorf("write %s: %w", caCopy, err)
			}
		}
		certPath := filepath.Join(dir, svc.Name+".pem")
		keyPath := filepath.Join(dir, svc.Name+"-key.pem")
		if err := os.WriteFile(certPath, cert.CertPEM, 0644); err != nil { // #nosec G306 -- 0644 — X.509 certificates are public material; private keys alongside are 0600
			return fmt.Errorf("write %s: %w", certPath, err)
		}
		if err := os.WriteFile(keyPath, cert.KeyPEM, 0600); err != nil {
			return fmt.Errorf("write %s: %w", keyPath, err)
		}
		fmt.Fprintf(stdout, "  %s: cert=%s key=%s (TTL=%s)\n", svc.Name, certPath, keyPath, o.ttl)
	}

	fmt.Fprintf(stdout, "\nGenerated %d service certificates in %s\n", len(specs), o.out)
	return nil
}

func lookupDefaultIP(name string) string {
	for _, svc := range defaultServices {
		if svc.Name == name {
			return svc.IP
		}
	}
	return ""
}
