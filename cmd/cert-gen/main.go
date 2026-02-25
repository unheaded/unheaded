// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the Business Source License 1.1 (BSL 1.1).
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
//
// Generates:
//
//	<out>/ca.pem           CA certificate
//	<out>/ca-key.pem       CA private key
//	<out>/<service>.pem    Service certificate
//	<out>/<service>-key.pem Service private key
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	unheadedtls "unheaded/pkg/tls"
)

// defaultServices is the set of Unheaded Alpha services.
// IPs are LXD bridge addresses (lxdbr0 10.10.10.0/24) added as cert SANs so
// certificates are valid when services are reached over the bridge network.
// localhost / 127.0.0.1 / ::1 SANs are added automatically by pkg/tls.IssueCert.
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
}

type serviceSpec struct {
	Name string
	IP   string
}

func main() {
	outDir := flag.String("out", "./certs", "Output directory for certificates")
	serviceList := flag.String("services", "", "Comma-separated service names (default: all)")
	ttl := flag.Duration("ttl", 24*time.Hour, "Certificate TTL")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: mkdir %s: %v\n", *outDir, err)
		os.Exit(1)
	}

	// Generate or load CA.
	ca, err := unheadedtls.NewCA()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: NewCA: %v\n", err)
		os.Exit(1)
	}

	caPath := filepath.Join(*outDir, "ca.pem")
	caKeyPath := filepath.Join(*outDir, "ca-key.pem")

	if err := os.WriteFile(caPath, ca.CertPEM(), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: write %s: %v\n", caPath, err)
		os.Exit(1)
	}
	if err := os.WriteFile(caKeyPath, ca.KeyPEM(), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "error: write %s: %v\n", caKeyPath, err)
		os.Exit(1)
	}
	fmt.Printf("CA certificate: %s\n", caPath)
	fmt.Printf("CA private key: %s\n", caKeyPath)

	// Determine which services to generate certs for.
	specs := defaultServices
	if *serviceList != "" {
		specs = nil
		for _, name := range strings.Split(*serviceList, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			ip := lookupDefaultIP(name)
			specs = append(specs, serviceSpec{Name: name, IP: ip})
		}
	}

	// Generate per-service certs.
	for _, svc := range specs {
		var ips []net.IP
		if svc.IP != "" {
			if ip := net.ParseIP(svc.IP); ip != nil {
				ips = append(ips, ip)
			}
		}

		cert, err := ca.IssueCert(unheadedtls.CertRequest{
			ServiceName: svc.Name,
			IPAddresses: ips,
			TTL:         *ttl,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: IssueCert(%s): %v\n", svc.Name, err)
			os.Exit(1)
		}

		certPath := filepath.Join(*outDir, svc.Name+".pem")
		keyPath := filepath.Join(*outDir, svc.Name+"-key.pem")

		if err := os.WriteFile(certPath, cert.CertPEM, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error: write %s: %v\n", certPath, err)
			os.Exit(1)
		}
		if err := os.WriteFile(keyPath, cert.KeyPEM, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "error: write %s: %v\n", keyPath, err)
			os.Exit(1)
		}
		fmt.Printf("  %s: cert=%s key=%s (TTL=%s)\n", svc.Name, certPath, keyPath, *ttl)
	}

	fmt.Printf("\nGenerated %d service certificates in %s\n", len(specs), *outDir)
}

func lookupDefaultIP(name string) string {
	for _, svc := range defaultServices {
		if svc.Name == name {
			return svc.IP
		}
	}
	return ""
}
