// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// sign, pubkey, verify-signature, verify-iso. Signatures are gungnir evidence
// seals (ADR-105 decision 5): JSON, ML-DSA-65, context
// unheaded/gungnir/evidence/v1, purpose inside the signature, no expiry. The
// one trust anchor is build.pubkey (ADR-105 decision 7).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/gungnir"
)

const (
	defaultKey    = "/etc/yggdrasil/build.key"
	defaultPubkey = "/etc/yggdrasil/build.pubkey"

	purposeManifest = "manifest"
	purposeISO      = "iso-sha256"
	purposeCustody  = "custody-entry"

	fileManifest    = "manifest.yaml"
	fileManifestSig = "manifest.yaml.sig"
	fileISOSha      = "image.iso.sha256"
	fileISOShaSig   = "image.iso.sha256.sig"

	maxSignInput = maxManifestBytes
)

// isoShaLine is sha256sum's output for one file: "<64 hex>  <name>\n".
var isoShaLine = regexp.MustCompile(`^([a-f0-9]{64})  ([A-Za-z0-9._/-]+)\n?$`)

func cmdSign(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("sign", stderr)
	key := fs.String("key", defaultKey, "ML-DSA-65 private key seed (32 bytes, mode 0600)")
	in := fs.String("in", "", "file to sign")
	out := fs.String("out", "", "signature file to write")
	purpose := fs.String("purpose", "", "manifest | iso-sha256 | custody-entry")
	index := fs.Int("custody-index", -1, "with --purpose custody-entry: sign this chain_of_custody entry of the --in manifest (writes hex)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || *in == "" || *out == "" {
		fmt.Fprintln(stderr, "sign: --in, --out and --purpose are required, no positional args")
		return exitUsage
	}
	switch *purpose {
	case purposeManifest, purposeISO:
		if *index != -1 {
			fmt.Fprintln(stderr, "sign: --custody-index only goes with --purpose custody-entry")
			return exitUsage
		}
	case purposeCustody:
		if *index < 0 {
			fmt.Fprintln(stderr, "sign: --purpose custody-entry needs --custody-index N")
			return exitUsage
		}
	default:
		fmt.Fprintf(stderr, "sign: --purpose must be manifest, iso-sha256 or custody-entry (got %q)\n", *purpose)
		return exitUsage
	}
	data, err := readBounded(*in, maxSignInput)
	if err != nil {
		fmt.Fprintf(stderr, "sign: %v\n", err)
		return exitUsage
	}
	signer, err := loadSigner(*key)
	if err != nil {
		fmt.Fprintf(stderr, "sign: %v\n", err)
		return exitFail
	}

	var outBytes []byte
	switch *purpose {
	case purposeCustody:
		hexSig, err := signCustodyEntry(signer, data, *index)
		if err != nil {
			fmt.Fprintf(stderr, "sign: %v\n", err)
			return exitFail
		}
		outBytes = []byte(hexSig + "\n")
	default:
		if *purpose == purposeManifest {
			if _, err := ValidateManifest(data); err != nil {
				fmt.Fprintf(stderr, "sign: refusing to sign a manifest that does not validate:\n%v\n", err)
				return exitFail
			}
		}
		seal, err := signer.SignEvidence(data, *purpose)
		if err != nil {
			fmt.Fprintf(stderr, "sign: %v\n", err)
			return exitFail
		}
		b, err := json.Marshal(seal)
		if err != nil {
			fmt.Fprintf(stderr, "sign: %v\n", err)
			return exitFail
		}
		outBytes = append(b, '\n')
	}
	// A signature is public; 0644 so operators and auditors can read it.
	if err := os.WriteFile(*out, outBytes, 0o644); err != nil { // #nosec G306 G703 -- public signature; operator-named path
		fmt.Fprintf(stderr, "sign: %v\n", err)
		return exitFail
	}
	fmt.Fprintf(stdout, "sign: %s (%s) -> %s\n", *in, *purpose, *out)
	return exitOK
}

func loadSigner(seedPath string) (*gungnir.Signer, error) {
	priv, err := gungnir.LoadSeedFile(seedPath)
	if err != nil {
		return nil, err
	}
	return gungnir.NewSignerWithID(priv)
}

// cmdPubkey derives build.pubkey from the seed. It generates nothing: where
// the seed comes from and who holds it is the operator's call (ADR-043 #4, E4).
func cmdPubkey(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("pubkey", stderr)
	key := fs.String("key", defaultKey, "ML-DSA-65 private key seed (32 bytes, mode 0600)")
	out := fs.String("out", "", "public key file to write")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || *out == "" {
		fmt.Fprintln(stderr, "pubkey: --out is required, no positional args")
		return exitUsage
	}
	pub, err := gungnir.PublicKeyFromSeed(*key)
	if err != nil {
		fmt.Fprintf(stderr, "pubkey: %v\n", err)
		return exitFail
	}
	if err := os.WriteFile(*out, pub.Bytes(), 0o644); err != nil { // #nosec G306 G703 -- a public key; operator-named path
		fmt.Fprintf(stderr, "pubkey: %v\n", err)
		return exitFail
	}
	fmt.Fprintf(stdout, "pubkey: key id %s -> %s\n", gungnir.KeyID(pub), *out)
	return exitOK
}

// verifySeal checks pack's sigName as pub's evidence seal over payloadName.
func verifySeal(p *Pack, payloadName, sigName, purpose string, pub *mldsa65.PublicKey) error {
	if !p.Has(payloadName) {
		return fmt.Errorf("%s missing from the pack", payloadName)
	}
	if !p.Has(sigName) {
		return fmt.Errorf("%s missing from the pack (unsigned pack: not evidence)", sigName)
	}
	seal, err := gungnir.ParseEvidenceSeal(p.File(sigName))
	if err != nil {
		return fmt.Errorf("%s: %w", sigName, err)
	}
	if err := gungnir.VerifyEvidence(p.File(payloadName), seal, pub, purpose); err != nil {
		return fmt.Errorf("%s: %w", sigName, err)
	}
	return nil
}

// packArgs parses `[--pubkey P] PACK` (plus extra flags registered by the
// caller) and loads both. ok=false means rc is the exit code to return.
type packCmd struct {
	pub  *mldsa65.PublicKey
	pack *Pack
	path string
}

func loadPackAndKey(name, pubkeyPath, packPath string, stderr io.Writer) (*packCmd, int) {
	pub, err := gungnir.LoadPublicKeyFile(pubkeyPath)
	if err != nil {
		fmt.Fprintf(stderr, "%s: public key: %v\n", name, err)
		return nil, exitUsage
	}
	p, rc := loadPack(name, packPath, stderr)
	if p == nil {
		return nil, rc
	}
	return &packCmd{pub: pub, pack: p, path: packPath}, exitOK
}

// loadPack: an unreadable path is a usage error (2); a file that is not a safe
// evidence pack is a verification failure (1).
func loadPack(name, packPath string, stderr io.Writer) (*Pack, int) {
	if _, err := os.Stat(packPath); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return nil, exitUsage
	}
	p, err := ReadPack(packPath)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %s: FAIL: %v\n", name, packPath, err)
		return nil, exitFail
	}
	return p, exitOK
}

func checkSignatures(pc *packCmd) []error {
	var errs []error
	if err := verifySeal(pc.pack, fileManifest, fileManifestSig, purposeManifest, pc.pub); err != nil {
		errs = append(errs, err)
	}
	if err := verifySeal(pc.pack, fileISOSha, fileISOShaSig, purposeISO, pc.pub); err != nil {
		errs = append(errs, err)
	}
	return errs
}

func report(stdout, stderr io.Writer, name, what string, errs []error) int {
	if len(errs) == 0 {
		fmt.Fprintf(stdout, "%s: %s: OK\n", name, what)
		return exitOK
	}
	for _, e := range errs {
		fmt.Fprintf(stderr, "%s: FAIL: %v\n", name, e)
	}
	return exitFail
}

func cmdVerifySignature(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("verify-signature", stderr)
	pubkey := fs.String("pubkey", defaultPubkey, "ML-DSA-65 build public key")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "verify-signature: expected one positional arg (evidence-pack tarball)")
		return exitUsage
	}
	pc, rc := loadPackAndKey("verify-signature", *pubkey, fs.Arg(0), stderr)
	if pc == nil {
		return rc
	}
	return report(stdout, stderr, "verify-signature", "manifest.yaml and image.iso.sha256 signed by key "+gungnir.KeyID(pc.pub), checkSignatures(pc))
}

// checkISO verifies image.iso.sha256's seal, then compares the ISO's streamed
// SHA-256 with that signed line and with the manifest's image fields.
func checkISO(pc *packCmd, isoPath string, isoSum []byte, isoSize int64) []error {
	if err := verifySeal(pc.pack, fileISOSha, fileISOShaSig, purposeISO, pc.pub); err != nil {
		return []error{err}
	}
	m := isoShaLine.FindSubmatch(pc.pack.File(fileISOSha))
	if m == nil {
		return []error{fmt.Errorf("%s is not one `sha256sum` line", fileISOSha)}
	}
	signedHex, signedName := string(m[1]), string(m[2])
	got := hex.EncodeToString(isoSum)
	var errs []error
	if got != signedHex {
		errs = append(errs, fmt.Errorf("%s: sha256 %s, signed %s says %s", isoPath, got, fileISOSha, signedHex))
	}
	man, err := ValidateManifest(pc.pack.File(fileManifest))
	if err != nil {
		return append(errs, fmt.Errorf("%s does not validate: %v", fileManifest, err))
	}
	if man.Image.IsoSha256 != signedHex {
		errs = append(errs, fmt.Errorf("manifest image.iso_sha256 %s disagrees with signed %s %s", man.Image.IsoSha256, fileISOSha, signedHex))
	}
	if man.Image.IsoPath != signedName {
		errs = append(errs, fmt.Errorf("manifest image.iso_path %q disagrees with signed %s name %q", man.Image.IsoPath, fileISOSha, signedName))
	}
	if man.Image.IsoSizeBytes != isoSize {
		errs = append(errs, fmt.Errorf("%s: %d bytes, manifest image.iso_size_bytes says %d", isoPath, isoSize, man.Image.IsoSizeBytes))
	}
	return errs
}

// hashFile streams a regular file through SHA-256.
func hashFile(path string) ([]byte, int64, error) {
	f, err := os.Open(path) // #nosec G304 -- operator-named ISO, only hashed
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !fi.Mode().IsRegular() {
		return nil, 0, errors.New(path + ": not a regular file")
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return nil, 0, err
	}
	return h.Sum(nil), n, nil
}

func cmdVerifyISO(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("verify-iso", stderr)
	iso := fs.String("iso", "", "the ISO file to check")
	pubkey := fs.String("pubkey", defaultPubkey, "ML-DSA-65 build public key")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *iso == "" || fs.NArg() != 1 {
		fmt.Fprintln(stderr, "verify-iso: --iso and a positional evidence-pack arg are required")
		return exitUsage
	}
	sum, size, err := hashFile(*iso)
	if err != nil {
		fmt.Fprintf(stderr, "verify-iso: %v\n", err)
		return exitUsage
	}
	pc, rc := loadPackAndKey("verify-iso", *pubkey, fs.Arg(0), stderr)
	if pc == nil {
		return rc
	}
	return report(stdout, stderr, "verify-iso", *iso+" matches the signed image.iso.sha256 and the manifest", checkISO(pc, *iso, sum, size))
}
