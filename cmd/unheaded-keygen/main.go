// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// unheaded-keygen creates, rotates and retires a service's Ed25519 key pair
// (ADR-103 §2, S-IRON-GATE Phase 2).
//
//	unheaded-keygen [-dir D] [-trust-dir T] [-owner U] <service>
//	unheaded-keygen -rotate [-owner U] <service>
//	unheaded-keygen -retire <kid> <service>
//	unheaded-keygen -print-kid <service>
//
// <service> is the identity a service passes to auth.LoadServiceAuthConfig
// (wotan, unheaded-daemon, dashboard-backend, ...): it is the token's "sub",
// the key file's name and the name the trust directory binds the key to.
//
// The private key is <dir>/<service>.key (0400); the public half is
// <dir>/<service>.pub (0444) and is copied to <trust-dir>/<service>.pub
// (0444), which every verifier reads. Rotation writes a new key, keeps the
// old public half trusted as <trust-dir>/<service>.<oldkid>.pub, and leaves
// it there until -retire removes it (after one token lifetime, so tokens
// already minted with the old key still verify). The private key is never
// printed.
//
// -algo mldsa65 makes an ML-DSA-65 signing key instead (S-IRON-GATE Phase 9):
// Heimdall's drift key and the signed-prefix keys (HEALTH_SIGNING_KEY_FILE,
// AUDIT_SIGNING_KEY_FILE). <dir>/<name>.key is the 32-byte gungnir seed
// (0400, -owner as above), <dir>/<name>.pub the packed public key (0444);
// nothing goes in the trust directory. It prints the key id and the public
// key in base64 for Wotan's topics.signed_prefixes.
package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"unheaded/pkg/auth"
	"unheaded/pkg/cmdline"
)

const (
	// keyDirMode lets each service open its own key by path without being
	// able to list the others (0750 root:root would lock every service out
	// of its own key). Each key file is 0400 and owned by its service.
	keyDirMode   os.FileMode = 0o711
	trustDirMode os.FileMode = 0o755
	pubMode      os.FileMode = 0o444

	exitOK      = 0
	exitError   = 1
	exitRefused = 2
)

// geteuid is a variable so tests can play a non-root caller.
var geteuid = os.Geteuid

const usageText = `Usage:
  unheaded-keygen [-dir D] [-trust-dir T] [-owner U] <service>   create a key pair
  unheaded-keygen -rotate [-owner U] <service>                   new key, old public half kept
  unheaded-keygen -retire <kid> <service>                        drop a rotated-out public key
  unheaded-keygen -print-kid <service>                           print the current key id
  unheaded-keygen -algo mldsa65 [-dir D] [-owner U] <name>       ML-DSA-65 signing key (drift, signed topics)
  unheaded-keygen -algo mldsa65 -print-kid <name>                print its key id and base64 public key

<service> is the service's auth identity (the name it passes to
auth.LoadServiceAuthConfig): wotan, unheaded-daemon, dashboard-backend, ...
An mldsa65 <name> is the key's file name (heimdall-drift, akira-health).

Flags:
`

const exitText = `
Exit status:
  0  done
  1  I/O or key file error
  2  usage error or refused (key exists, nothing to rotate, unknown or
     current kid, -owner without root)
`

type options struct {
	dir, trustDir, owner, retire string
	rotate, printKid             bool
	name                         string
	algo                         string
}

const (
	algoEd25519 = "ed25519"
	algoMLDSA65 = "mldsa65"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("unheaded-keygen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var o options
	fs.StringVar(&o.dir, "dir", auth.DefaultKeyDir, "directory for <service>.key and <service>.pub")
	fs.StringVar(&o.trustDir, "trust-dir", auth.DefaultTrustDir, "trust directory every verifier reads")
	fs.StringVar(&o.owner, "owner", "", "chown the private key to this user name or uid[:gid] (root only)")
	fs.BoolVar(&o.rotate, "rotate", false, "replace an existing key; keep its public half trusted until -retire")
	fs.StringVar(&o.retire, "retire", "", "remove the rotated-out public key with this kid from the trust directory")
	fs.BoolVar(&o.printKid, "print-kid", false, "print the current key id and exit")
	fs.StringVar(&o.algo, "algo", algoEd25519, "ed25519 (service identity key) or mldsa65 (ML-DSA-65 signing key: drift, signed topics)")
	// The flag package prints nothing itself (output discarded, no-op
	// Usage); help goes to stdout on -h, usage errors to stderr in one line.
	fs.Usage = func() {}
	printUsage := func(w io.Writer) {
		fmt.Fprint(w, usageText)
		fs.SetOutput(w)
		fs.PrintDefaults()
		fs.SetOutput(io.Discard)
		fmt.Fprint(w, exitText)
	}
	if len(args) == 1 && cmdline.IsVersionArg(args[0]) {
		cmdline.PrintVersion(stdout, cmdline.Spec{Name: "unheaded-keygen"})
		return exitOK
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stdout)
			return exitOK
		}
		return cmdline.ReportUsageError(stderr, cmdline.Spec{Name: "unheaded-keygen"}, "%s", cmdline.Translate(err, args))
	}
	if fs.NArg() != 1 {
		printUsage(stderr)
		fmt.Fprintln(stderr, "Try 'unheaded-keygen --help' for more information.")
		return exitRefused
	}
	o.name = fs.Arg(0)
	if !auth.ValidKeyName(o.name) {
		fmt.Fprintf(stderr, "unheaded-keygen: %v: %q\n", auth.ErrBadKeyName, o.name)
		return exitRefused
	}
	modes := 0
	for _, on := range []bool{o.rotate, o.retire != "", o.printKid} {
		if on {
			modes++
		}
	}
	if modes > 1 {
		fmt.Fprintln(stderr, "unheaded-keygen: -rotate, -retire and -print-kid are exclusive")
		return exitRefused
	}
	if same, err := sameDir(o.dir, o.trustDir); err != nil || same {
		fmt.Fprintln(stderr, "unheaded-keygen: -dir and -trust-dir must be different directories (private keys never go in trust.d)")
		return exitRefused
	}

	uid, gid := -1, -1
	if o.owner != "" {
		if geteuid() != 0 {
			fmt.Fprintln(stderr, "unheaded-keygen: -owner needs root (it chowns the key); run as root or drop -owner")
			return exitRefused
		}
		var err error
		if uid, gid, err = parseOwner(o.owner); err != nil {
			fmt.Fprintf(stderr, "unheaded-keygen: -owner: %v\n", err)
			return exitRefused
		}
	}

	switch o.algo {
	case algoEd25519:
	case algoMLDSA65:
		switch {
		case o.printKid:
			return printMLDSA(o, stdout, stderr)
		case o.rotate || o.retire != "":
			fmt.Fprintln(stderr, "unheaded-keygen: -algo mldsa65 has no -rotate/-retire: make a new key under a new name, enrol its public key beside the old one in Wotan's signed_prefixes, switch the producer, then remove the old key from the config")
			return exitRefused
		default:
			return createMLDSA(o, uid, gid, stdout, stderr)
		}
	default:
		fmt.Fprintf(stderr, "unheaded-keygen: -algo %q: want ed25519 or mldsa65\n", o.algo)
		return exitRefused
	}

	switch {
	case o.printKid:
		return printKid(o, stdout, stderr)
	case o.retire != "":
		return retire(o, stdout, stderr)
	case o.rotate:
		return rotate(o, uid, gid, stdout, stderr)
	default:
		return create(o, uid, gid, stdout, stderr)
	}
}

// refusal is an error that exits 2 rather than 1.
type refusal struct{ msg string }

func (r refusal) Error() string { return r.msg }

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "unheaded-keygen: %v\n", err)
	var r refusal
	if errors.As(err, &r) {
		return exitRefused
	}
	return exitError
}

func create(o options, uid, gid int, stdout, stderr io.Writer) int {
	keyPath := filepath.Join(o.dir, o.name+".key")
	pubPath := filepath.Join(o.dir, o.name+".pub")
	if _, err := os.Lstat(keyPath); err == nil {
		return fail(stderr, refusal{fmt.Sprintf("key exists: %s (use -rotate to replace it)", keyPath)})
	}
	if err := ensureDir(o.dir, keyDirMode); err != nil {
		return fail(stderr, err)
	}
	if err := ensureDir(o.trustDir, trustDirMode); err != nil {
		return fail(stderr, err)
	}
	// A .pub without its .key is left from an earlier key; it is derived
	// data, so it goes.
	if err := os.Remove(pubPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(stderr, err)
	}
	kid, err := auth.GenerateKeyPair(o.dir, o.name)
	if err != nil {
		return fail(stderr, err)
	}
	if uid >= 0 {
		if err := os.Chown(keyPath, uid, gid); err != nil {
			return fail(stderr, fmt.Errorf("chown %s: %w", keyPath, err))
		}
	}
	trustPath := filepath.Join(o.trustDir, o.name+".pub")
	if err := copyPub(pubPath, trustPath); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "%s kid %s\n  key   %s (0400)\n  trust %s\n", o.name, kid, keyPath, trustPath)
	return exitOK
}

func rotate(o options, uid, gid int, stdout, stderr io.Writer) int {
	keyPath := filepath.Join(o.dir, o.name+".key")
	st, err := os.Stat(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		return fail(stderr, refusal{fmt.Sprintf("no key to rotate: %s (create one first)", keyPath)})
	} else if err != nil {
		return fail(stderr, err)
	}
	oldPriv, oldKid, err := auth.LoadPrivateKey(keyPath)
	if err != nil {
		return fail(stderr, err)
	}
	// Root rotating without -owner keeps the key's current owner, or the
	// service could no longer read its own key.
	if uid < 0 && geteuid() == 0 {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(sys.Uid), int(sys.Gid)
		}
	}
	if err := ensureDir(o.trustDir, trustDirMode); err != nil {
		return fail(stderr, err)
	}

	tmp, err := os.MkdirTemp(o.dir, ".rotate-")
	if err != nil {
		return fail(stderr, err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	newKid, err := auth.GenerateKeyPair(tmp, o.name)
	if err != nil {
		return fail(stderr, err)
	}
	if uid >= 0 {
		if err := os.Chown(filepath.Join(tmp, o.name+".key"), uid, gid); err != nil {
			return fail(stderr, fmt.Errorf("chown new key: %w", err))
		}
	}

	// Trust first (old key stays trusted under its own file, new key under
	// <service>.pub), then the key itself, so no moment exists where the
	// service signs with a key no verifier holds.
	oldPub, ok := oldPriv.Public().(ed25519.PublicKey)
	if !ok {
		return fail(stderr, fmt.Errorf("%s: key is not Ed25519", o.name))
	}
	oldPEM, err := pubPEM(oldPub)
	if err != nil {
		return fail(stderr, err)
	}
	keptPath := filepath.Join(o.trustDir, o.name+"."+oldKid+".pub")
	if err := writeAtomic(keptPath, oldPEM, pubMode); err != nil {
		return fail(stderr, err)
	}
	if err := copyPub(filepath.Join(tmp, o.name+".pub"), filepath.Join(o.trustDir, o.name+".pub")); err != nil {
		return fail(stderr, err)
	}
	if err := os.Rename(filepath.Join(tmp, o.name+".pub"), filepath.Join(o.dir, o.name+".pub")); err != nil {
		return fail(stderr, err)
	}
	if err := os.Rename(filepath.Join(tmp, o.name+".key"), keyPath); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "%s kid %s -> %s\n  old public key kept trusted: %s\n  restart the service, then after one token lifetime (15 min):\n    unheaded-keygen -retire %s %s\n",
		o.name, oldKid, newKid, keptPath, oldKid, o.name)
	return exitOK
}

func retire(o options, stdout, stderr io.Writer) int {
	current := ""
	if pub, err := readPub(filepath.Join(o.trustDir, o.name+".pub")); err == nil {
		current = auth.KeyID(pub)
	}
	if o.retire == current {
		return fail(stderr, refusal{fmt.Sprintf("%s is %s's current key; rotate first", o.retire, o.name)})
	}
	entries, err := os.ReadDir(o.trustDir)
	if err != nil {
		return fail(stderr, err)
	}
	for _, e := range entries {
		fn := e.Name()
		// Only this service's rotated-out keys: "<service>.<x>.pub".
		if !strings.HasPrefix(fn, o.name+".") || !strings.HasSuffix(fn, ".pub") || fn == o.name+".pub" {
			continue
		}
		pub, err := readPub(filepath.Join(o.trustDir, fn))
		if err != nil || auth.KeyID(pub) != o.retire {
			continue
		}
		p := filepath.Join(o.trustDir, fn)
		if err := os.Remove(p); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "%s kid %s retired (removed %s); verifiers drop it on their next trust reload\n", o.name, o.retire, p)
		return exitOK
	}
	return fail(stderr, refusal{fmt.Sprintf("no rotated-out key %s for %s in %s", o.retire, o.name, o.trustDir)})
}

func printKid(o options, stdout, stderr io.Writer) int {
	var lastErr error
	for _, p := range []string{filepath.Join(o.dir, o.name+".pub"), filepath.Join(o.trustDir, o.name+".pub")} {
		pub, err := readPub(p)
		if err != nil {
			lastErr = err
			continue
		}
		fmt.Fprintln(stdout, auth.KeyID(pub))
		return exitOK
	}
	return fail(stderr, lastErr)
}

// parseOwner resolves "uid", "uid:gid" or a user name (with its primary
// group). Root (uid or gid 0) is refused: a service key owned by root is
// one the service cannot read.
func parseOwner(s string) (uid, gid int, err error) {
	if s == "" {
		return 0, 0, errors.New("empty owner")
	}
	num := func(v string) (int, error) {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("bad id %q (want a positive number)", v)
		}
		return n, nil
	}
	if u, g, ok := strings.Cut(s, ":"); ok {
		if uid, err = num(u); err != nil {
			return 0, 0, err
		}
		if gid, err = num(g); err != nil {
			return 0, 0, err
		}
		return uid, gid, nil
	}
	if s[0] >= '0' && s[0] <= '9' || s[0] == '-' {
		if uid, err = num(s); err != nil {
			return 0, 0, err
		}
		return uid, uid, nil
	}
	u, err := user.Lookup(s)
	if err != nil {
		return 0, 0, err
	}
	if uid, err = num(u.Uid); err != nil {
		return 0, 0, fmt.Errorf("user %s: %w", s, err)
	}
	if gid, err = num(u.Gid); err != nil {
		return 0, 0, fmt.Errorf("user %s: %w", s, err)
	}
	return uid, gid, nil
}

func sameDir(a, b string) (bool, error) {
	aa, err := filepath.Abs(a)
	if err != nil {
		return false, err
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false, err
	}
	if aa == bb {
		return true, nil
	}
	sa, errA := os.Stat(aa)
	sb, errB := os.Stat(bb)
	if errA == nil && errB == nil {
		return os.SameFile(sa, sb), nil
	}
	return false, nil
}

// ensureDir creates dir with exactly mode when it does not exist; an
// existing directory keeps the mode its operator gave it.
func ensureDir(dir string, mode os.FileMode) error {
	st, err := os.Stat(dir)
	if err == nil {
		if !st.IsDir() {
			return fmt.Errorf("%s is not a directory", dir)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(dir, mode); err != nil {
		return err
	}
	return os.Chmod(dir, mode)
}

func readPub(path string) (ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- operator-chosen key or trust directory, name validated
	if err != nil {
		return nil, err
	}
	return auth.ParsePublicKeyPEM(raw)
}

func pubPEM(pub ed25519.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

// copyPub checks src is one Ed25519 public key and installs it at dst
// (0444), replacing what was there.
func copyPub(src, dst string) error {
	raw, err := os.ReadFile(src) // #nosec G304 -- file this tool just wrote
	if err != nil {
		return err
	}
	if _, err := auth.ParsePublicKeyPEM(raw); err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	return writeAtomic(dst, raw, pubMode)
}

// writeAtomic writes data to a temp file beside path and renames it over
// path, so a reader never sees a half-written key.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".keygen-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // gone already after a successful rename
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
