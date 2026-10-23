// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// Asset kinds the Go scanner emits.
const (
	kindTLSConfig = "tls-config"  // a crypto/tls Config literal
	kindTLSGroups = "tls-groups"  // CurvePreferences set outside a literal
	kindTLSFloor  = "tls-floor"   // MinVersion set outside a tls.Config literal
	kindTLSCap    = "tls-cap"     // MaxVersion below 1.3 set outside a literal
	kindGRPCCreds = "grpc-tls"    // grpc credentials built from files/certs
	kindGodebug   = "godebug"     // a tlsmlkem/tlssecpmlkem kill switch
	kindSigImport = "sig-import"  // a signature algorithm package
	kindKEMImport = "kem-import"  // a KEM implementation package
	kindKexImport = "kex-import"  // a classical key-agreement package
	kindSSHImport = "ssh-library" // golang.org/x/crypto/ssh
)

const (
	pkgTLS   = "crypto/tls"
	pkgCreds = "google.golang.org/grpc/credentials"
	pkgSSH   = "golang.org/x/crypto/ssh"
	pkgPQC   = "unheaded/pkg/crypto/pqc"
)

// Go's TLS key-exchange identifiers (crypto/tls CurveID constants).
var hybridGroups = map[string]bool{
	"X25519MLKEM768":     true,
	"SecP256r1MLKEM768":  true,
	"SecP384r1MLKEM1024": true,
}

var pqOnlyGroups = map[string]bool{"MLKEM1024": true}

var classicalGroups = map[string]bool{
	"X25519": true, "CurveP256": true, "CurveP384": true, "CurveP521": true,
	"X25519Kyber768Draft00": true, // removed in Go 1.24; harmless if it lingers, and not ML-KEM
}

func isHybridGroup(g string) bool { return hybridGroups[g] }

// tlsVersions maps crypto/tls version constants to a sortable label.
var tlsVersions = map[string]string{
	"VersionSSL30": "0.3", "VersionTLS10": "1.0", "VersionTLS11": "1.1",
	"VersionTLS12": "1.2", "VersionTLS13": "1.3",
}

// dynamic marks a version or group list the scanner cannot evaluate.
const dynamic = "dyn"

// below13 reports a known version label below TLS 1.3.
func below13(v string) bool { return v != "" && v != dynamic && v < "1.3" }

// classifyTLS gives the pq-status of a TLS configuration from its floor,
// cap and group list. min/max are labels from tlsVersions, "" for unset
// (Go's default floor is TLS 1.2) or dynamic. groups nil or empty means
// Go's default list (hybrids first, classical fallback, Go >= 1.24).
func classifyTLS(minV, maxV string, groups []string, dynGroups bool) string {
	if below13(maxV) {
		return pqClassical // a session capped below 1.3 has no ML-KEM
	}
	if dynGroups {
		return pqUnknown
	}
	if len(groups) > 0 {
		hyb, pq := 0, 0
		for _, g := range groups {
			switch {
			case hybridGroups[g]:
				hyb++
			case pqOnlyGroups[g]:
				pq++
			}
		}
		switch {
		case hyb == len(groups):
			return pqHybridStrict
		case hyb == 0 && pq > 0:
			return pqOnly
		case hyb == 0:
			return pqClassical
		case minV == "1.3":
			return pqHybrid
		default:
			return pqHybridOpp
		}
	}
	switch minV {
	case "1.3":
		return pqHybrid
	case dynamic:
		return pqUnknown
	default:
		return pqHybridOpp
	}
}

// thirdPartyPeers are clients whose far end is not ours: the server picks
// the group, so their floors stay at 1.2 (ADR-105 decision 2).
var thirdPartyPeers = []struct{ prefix, why string }{
	{"cmd/alert-mailer/", "SMTP relay chooses the group"},
	{"pkg/alerting/notify/", "mail/webhook server chooses the group"},
	{"pkg/audit/export/", "Splunk HEC chooses the group"},
	{"pkg/threatfeed/", "CISA KEV endpoint chooses the group"},
	{"pkg/loadbalancer/health.go", "user backends choose the group"},
	{"cmd/unheaded-daemon/lxd_rest.go", "the LXD API server chooses the group"},
}

// lintExempt paths probe weak crypto on purpose (Phase 4 lint).
var lintExempt = []struct{ prefix, why string }{
	{"cmd/lich-security/", "red-team tool: probes weak TLS on purpose"},
}

func ownerOf(path string) (owner, why string) {
	for _, p := range thirdPartyPeers {
		if strings.HasPrefix(path, p.prefix) {
			return ownerThirdParty, p.why
		}
	}
	return ownerFirstParty, ""
}

// importSpec describes what an imported crypto package means.
type importSpec struct {
	kind, name, primitive, param, status string
	level                                int
	cnsa2                                bool
	detail                               string
}

var goCryptoImports = map[string]importSpec{
	"github.com/cloudflare/circl/sign/mldsa/mldsa44": {kindSigImport, "ML-DSA-44", primSignature, "ML-DSA-44", pqSignature, 2, false, "FIPS 204"},
	"github.com/cloudflare/circl/sign/mldsa/mldsa65": {kindSigImport, "ML-DSA-65", primSignature, "ML-DSA-65", pqSignature, 3, false, "FIPS 204"},
	"github.com/cloudflare/circl/sign/mldsa/mldsa87": {kindSigImport, "ML-DSA-87", primSignature, "ML-DSA-87", pqSignature, 5, true, "FIPS 204"},
	"github.com/cloudflare/circl/sign/slhdsa":        {kindSigImport, "SLH-DSA", primSignature, "SLH-DSA", pqSignature, -1, false, "FIPS 205; level depends on the parameter set"},
	"github.com/cloudflare/circl/kem/mlkem/mlkem512": {kindKEMImport, "ML-KEM-512", primKEM, "ML-KEM-512", pqNA, 1, false, "KEM implementation; key-exchange sites are inventoried separately"},
	"github.com/cloudflare/circl/kem/mlkem/mlkem768": {kindKEMImport, "ML-KEM-768", primKEM, "ML-KEM-768", pqNA, 3, false, "KEM implementation; key-exchange sites are inventoried separately"},
	"github.com/cloudflare/circl/kem/mlkem/mlkem1024": {kindKEMImport, "ML-KEM-1024", primKEM, "ML-KEM-1024", pqNA, 5, true,
		"KEM implementation; key-exchange sites are inventoried separately"},
	"crypto/mlkem":                         {kindKEMImport, "ML-KEM (crypto/mlkem)", primKEM, "ML-KEM-768/1024", pqNA, -1, false, "KEM implementation; key-exchange sites are inventoried separately"},
	"crypto/ed25519":                       {kindSigImport, "Ed25519", primSignature, "Ed25519", pqClassicalSig, 0, false, ""},
	"golang.org/x/crypto/ed25519":          {kindSigImport, "Ed25519", primSignature, "Ed25519", pqClassicalSig, 0, false, ""},
	"crypto/ecdsa":                         {kindSigImport, "ECDSA", primSignature, "ECDSA", pqClassicalSig, 0, false, ""},
	"crypto/rsa":                           {kindSigImport, "RSA", primSignature, "RSA", pqClassicalSig, 0, false, "RSA signatures and/or encryption"},
	"golang.org/x/crypto/curve25519":       {kindKexImport, "X25519", primKeyAgree, "X25519", pqClassical, 0, false, "classical key agreement"},
	"crypto/ecdh":                          {kindKexImport, "ECDH", primKeyAgree, "ECDH", pqClassical, 0, false, "classical key agreement"},
	"golang.org/x/crypto/chacha20poly1305": {}, // symmetric: out of scope (zero kind = skip)
}

// killSwitch matches GODEBUG settings that turn Go's hybrid groups off.
var killSwitch = regexp.MustCompile(`\b(tlsmlkem|tlssecpmlkem)=0\b`)

// scanGo inventories every non-test Go file in the tree.
func scanGo(t *Tree) ([]Asset, error) {
	var out []Asset
	for _, rel := range t.Match(func(f string) bool {
		return strings.HasSuffix(f, ".go") && !strings.HasSuffix(f, "_test.go")
	}) {
		src, err := t.Read(rel)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		assets, err := scanGoFile(rel, src)
		if err != nil {
			return nil, err
		}
		out = append(out, assets...)
	}
	return out, nil
}

type goFile struct {
	rel     string
	fset    *token.FileSet
	tlsName string // local name of crypto/tls, "" if not imported
	creds   string // local name of grpc/credentials
	ssh     string // local name of x/crypto/ssh
	out     []Asset
}

func scanGoFile(rel string, src []byte) ([]Asset, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", rel, err)
	}
	g := &goFile{rel: rel, fset: fset}
	g.imports(f)
	g.godebug(f)
	if g.tlsName != "" {
		consumed := g.tlsLiterals(f)
		g.tlsSettings(f, consumed)
	}
	if g.creds != "" {
		g.grpcCreds(f)
	}
	return g.out, nil
}

func (g *goFile) line(p token.Pos) int { return g.fset.Position(p).Line }

func (g *goFile) add(a Asset) {
	a.Path = g.rel
	a.Scanner = "go"
	if a.LinkState == "" {
		a.LinkState = linkEnabled
	}
	if a.Owner == "" {
		owner, why := ownerOf(g.rel)
		a.Owner = owner
		if why != "" {
			a.Detail = strings.TrimSpace(a.Detail + "; " + why)
			a.Detail = strings.TrimPrefix(a.Detail, "; ")
		}
	}
	g.out = append(g.out, a)
}

func localName(imp *ast.ImportSpec, path string) string {
	if imp.Name != nil {
		return imp.Name.Name
	}
	return path[strings.LastIndex(path, "/")+1:]
}

func (g *goFile) imports(f *ast.File) {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := localName(imp, p)
		switch p {
		case pkgTLS:
			g.tlsName = name
		case pkgCreds:
			g.creds = name
		case pkgSSH:
			g.ssh = name
			g.sshImport(f, imp, name)
			continue
		case pkgPQC:
			g.pqcImport(f, imp, name)
			continue
		}
		spec, ok := goCryptoImports[p]
		if !ok || spec.kind == "" {
			continue
		}
		g.add(Asset{
			Line: g.line(imp.Pos()), Kind: spec.kind, Name: spec.name + " (" + p + ")",
			AssetType: assetAlgorithm, Primitive: spec.primitive, ParamSet: spec.param,
			NISTLevel: spec.level, PQStatus: spec.status, CNSA2: spec.cnsa2, Detail: spec.detail,
			Owner: ownerFirstParty,
		})
	}
}

// sshImport records x/crypto/ssh: a live link only if the file dials or
// accepts; otherwise it is key encoding.
func (g *goFile) sshImport(f *ast.File, imp *ast.ImportSpec, name string) {
	connects := false
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == name {
				switch sel.Sel.Name {
				case "Dial", "NewClientConn", "NewServerConn":
					connects = true
				}
			}
		}
		return !connects
	})
	a := Asset{
		Line: g.line(imp.Pos()), Kind: kindSSHImport, Name: "SSH (golang.org/x/crypto/ssh)",
		AssetType: assetProtocol, Protocol: protoSSH, PQStatus: pqNA,
		Detail: "key encoding only; no connection made here",
	}
	if connects {
		a.PQStatus = pqHybridOpp
		a.Detail = "library default KEX list offers mlkem768x25519-sha256 first; the server decides"
	}
	g.add(a)
}

// pqcImport records the in-tree PQC wrapper by what the file uses from it.
func (g *goFile) pqcImport(f *ast.File, imp *ast.ImportSpec, name string) {
	used := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == name {
				used[sel.Sel.Name] = true
			}
		}
		return true
	})
	a := Asset{
		Line: g.line(imp.Pos()), Kind: kindSigImport, Name: "PQC helpers (" + pkgPQC + ")",
		AssetType: assetAlgorithm, Primitive: primSignature, NISTLevel: -1, PQStatus: pqNA,
		Owner: ownerFirstParty, Detail: "no ML-DSA/ML-KEM/SLH-DSA operation referenced here",
	}
	for _, p := range []struct {
		sel, param string
		level      int
		cnsa       bool
	}{{"MLDSA44", "ML-DSA-44", 2, false}, {"MLDSA65", "ML-DSA-65", 3, false}, {"MLDSA87", "ML-DSA-87", 5, true}} {
		if used[p.sel] {
			a.Name, a.ParamSet, a.NISTLevel, a.CNSA2 = p.param+" ("+pkgPQC+")", p.param, p.level, p.cnsa
			a.PQStatus, a.Detail = pqSignature, "FIPS 204 via the in-tree wrapper"
			g.add(a)
			return
		}
	}
	for sel := range used {
		if strings.HasPrefix(sel, "MLDSA") || strings.HasPrefix(sel, "NewMLDSA") || strings.HasPrefix(sel, "SLHDSA") || strings.HasPrefix(sel, "NewSLHDSA") {
			a.PQStatus, a.Detail = pqSignature, "post-quantum signatures via the in-tree wrapper; parameter set chosen at run time"
			break
		}
	}
	g.add(a)
}

func (g *goFile) godebug(f *ast.File) {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "//go:debug") && killSwitch.MatchString(c.Text) {
				g.add(Asset{
					Line: g.line(c.Pos()), Kind: kindGodebug, Name: "GODEBUG hybrid kill switch (//go:debug)",
					AssetType: assetProtocol, Protocol: protoTLS, PQStatus: pqClassical,
					Detail: strings.TrimSpace(strings.TrimPrefix(c.Text, "//")),
				})
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil || !killSwitch.MatchString(s) {
			return true
		}
		g.add(Asset{
			Line: g.line(lit.Pos()), Kind: kindGodebug, Name: "GODEBUG hybrid kill switch (string)",
			AssetType: assetProtocol, Protocol: protoTLS, PQStatus: pqClassical,
			Detail: killSwitch.FindString(s),
		})
		return true
	})
}

// isTLSSel reports whether e is tlsName.<Sel> and returns Sel.
func (g *goFile) tlsSel(e ast.Expr) (string, bool) {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != g.tlsName {
		return "", false
	}
	return sel.Sel.Name, true
}

// version evaluates a MinVersion/MaxVersion expression.
func (g *goFile) version(e ast.Expr) string {
	if name, ok := g.tlsSel(e); ok {
		if v, ok := tlsVersions[name]; ok {
			return v
		}
	}
	return dynamic
}

// groups evaluates a CurvePreferences expression: the list, or dyn=true.
func (g *goFile) groups(e ast.Expr) ([]string, bool) {
	if id, ok := e.(*ast.Ident); ok && id.Name == "nil" {
		return nil, false
	}
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		return nil, true
	}
	out := []string{}
	for _, el := range cl.Elts {
		name, ok := g.tlsSel(el)
		if !ok || (!hybridGroups[name] && !pqOnlyGroups[name] && !classicalGroups[name]) {
			return nil, true
		}
		out = append(out, name)
	}
	return out, false
}

// mentionsTLS reports whether e refers to the crypto/tls package anywhere.
func (g *goFile) mentionsTLS(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if x, ok := n.(ast.Expr); ok {
			if _, ok := g.tlsSel(x); ok {
				found = true
			}
		}
		return !found
	})
	return found
}

// tlsLiterals emits one asset per tls.Config literal and returns the
// key/value nodes it consumed so tlsSettings does not count them twice.
func (g *goFile) tlsLiterals(f *ast.File) map[ast.Node]bool {
	consumed := map[ast.Node]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if name, ok := g.tlsSel(cl.Type); !ok || name != "Config" {
			return true
		}
		var minV, maxV string
		var grp []string
		dynGroups := false
		fields := map[string]bool{}
		for _, el := range cl.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			fields[key.Name] = true
			switch key.Name {
			case "MinVersion":
				minV = g.version(kv.Value)
				consumed[kv] = true
			case "MaxVersion":
				maxV = g.version(kv.Value)
				consumed[kv] = true
			case "CurvePreferences":
				grp, dynGroups = g.groups(kv.Value)
				consumed[kv] = true
			}
		}
		role := "TLS config"
		switch {
		case fields["RootCAs"] || fields["ServerName"] || fields["InsecureSkipVerify"]:
			role = "TLS client config"
		case fields["ClientCAs"] || fields["ClientAuth"] || fields["GetCertificate"] || fields["GetConfigForClient"] || fields["Certificates"]:
			role = "TLS server config"
		}
		floor := minV
		if floor == "" {
			floor = "1.2" // Go's default for clients and servers
		}
		status := classifyTLS(minV, maxV, grp, dynGroups)
		g.add(Asset{
			Line: g.line(cl.Pos()), Kind: kindTLSConfig, Name: role, Floor: floor, Cap: maxV,
			AssetType: assetProtocol, Protocol: protoTLS, Version: versionRange(floor, maxV),
			Groups: grp, PQStatus: status, Detail: tlsDetail(minV, maxV, grp, dynGroups),
		})
		return true
	})
	return consumed
}

func versionRange(floor, maxV string) string {
	if maxV == "" || maxV == dynamic {
		maxV = "1.3"
	}
	if floor == dynamic {
		return "dynamic-" + maxV
	}
	if floor == maxV {
		return floor
	}
	return floor + "-" + maxV
}

func tlsDetail(minV, maxV string, grp []string, dynGroups bool) string {
	var parts []string
	switch minV {
	case "":
		parts = append(parts, "MinVersion unset (Go default TLS 1.2)")
	case dynamic:
		parts = append(parts, "MinVersion set at run time")
	}
	if below13(maxV) {
		parts = append(parts, "MaxVersion below TLS 1.3")
	}
	switch {
	case dynGroups:
		parts = append(parts, "CurvePreferences set at run time")
	case len(grp) == 0:
		parts = append(parts, "Go default groups (X25519MLKEM768 first)")
	}
	return strings.Join(parts, "; ")
}

// tlsSettings emits MinVersion/MaxVersion/CurvePreferences set outside a
// tls.Config literal: in another struct's literal or by assignment.
func (g *goFile) tlsSettings(f *ast.File, consumed map[ast.Node]bool) {
	emit := func(field string, value ast.Expr, pos token.Pos) {
		if !g.mentionsTLS(value) {
			return
		}
		ln := g.line(pos)
		switch field {
		case "MinVersion":
			v := g.version(value)
			g.add(Asset{
				Line: ln, Kind: kindTLSFloor, Name: "TLS floor setting", Floor: v,
				AssetType: assetProtocol, Protocol: protoTLS, Version: versionRange(v, ""),
				PQStatus: classifyTLS(v, "", nil, false), Detail: "MinVersion set outside a tls.Config literal",
			})
		case "MaxVersion":
			v := g.version(value)
			if !below13(v) {
				return
			}
			g.add(Asset{
				Line: ln, Kind: kindTLSCap, Name: "TLS version cap", Cap: v,
				AssetType: assetProtocol, Protocol: protoTLS, Version: "-" + v,
				PQStatus: pqClassical, Detail: "MaxVersion below TLS 1.3: no ML-KEM is possible",
			})
		case "CurvePreferences":
			grp, dyn := g.groups(value)
			g.add(Asset{
				Line: ln, Kind: kindTLSGroups, Name: "TLS group preference",
				AssetType: assetProtocol, Protocol: protoTLS, Groups: grp,
				PQStatus: classifyTLS("", "", grp, dyn), Detail: tlsDetail("1.3", "", grp, dyn),
			})
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.KeyValueExpr:
			if consumed[x] {
				return true
			}
			if key, ok := x.Key.(*ast.Ident); ok {
				emit(key.Name, x.Value, x.Pos())
			}
		case *ast.AssignStmt:
			if len(x.Lhs) != len(x.Rhs) {
				return true
			}
			for i, lhs := range x.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok {
					emit(sel.Sel.Name, x.Rhs[i], x.Pos())
				}
			}
		}
		return true
	})
}

// grpcCreds records grpc credentials built without a tls.Config of ours:
// they take Go's defaults (TLS 1.2 floor, default groups).
func (g *goFile) grpcCreds(f *ast.File) {
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok || x.Name != g.creds {
			return true
		}
		role := ""
		switch sel.Sel.Name {
		case "NewServerTLSFromFile", "NewServerTLSFromCert":
			role = "gRPC server TLS credentials"
		case "NewClientTLSFromFile", "NewClientTLSFromCert":
			role = "gRPC client TLS credentials"
		default:
			return true
		}
		g.add(Asset{
			Line: g.line(call.Pos()), Kind: kindGRPCCreds, Name: role, Floor: "1.2",
			AssetType: assetProtocol, Protocol: protoTLS, Version: "1.2-1.3",
			PQStatus: pqHybridOpp, Detail: "credentials." + sel.Sel.Name + ": Go defaults (TLS 1.2 floor, default groups)",
		})
		return true
	})
}
