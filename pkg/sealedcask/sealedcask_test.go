// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package sealedcask

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/sealedcask/casktest"
)

func keys(k ...*casktest.Key) []*mldsa65.PublicKey {
	out := make([]*mldsa65.PublicKey, 0, len(k))
	for _, x := range k {
		out = append(out, x.Pub)
	}
	return out
}

func verifyBytes(b []byte, pubs []*mldsa65.PublicKey) (*Result, error) {
	return Verify(bytes.NewReader(b), pubs, DefaultLimits)
}

func TestPurposeMatchesCasktest(t *testing.T) {
	if Purpose != casktest.Purpose {
		t.Fatalf("Purpose %q, casktest.Purpose %q", Purpose, casktest.Purpose)
	}
}

func TestVerifyGoodCask(t *testing.T) {
	cask, k := casktest.Good(t)
	res, err := verifyBytes(cask, keys(k))
	if err != nil {
		t.Fatalf("good cask refused: %v", err)
	}
	if res.Files != len(casktest.Files()) || res.Root != casktest.Root || res.KeyID == "" {
		t.Fatalf("result %+v", res)
	}
}

func TestVerifyFlippedByteInAnyFile(t *testing.T) {
	for name := range casktest.Files() {
		t.Run(name, func(t *testing.T) {
			k := casktest.NewKey(t)
			files := casktest.Files()
			m := casktest.Manifest(files)
			sig := k.Seal(t, m, Purpose)
			b := append([]byte(nil), files[name]...)
			b[0] ^= 0x01
			files[name] = b
			_, err := verifyBytes(casktest.Tar(t, casktest.Entries(files, m, sig)), keys(k))
			if !errors.Is(err, ErrMismatch) {
				t.Fatalf("flipped byte in %s: err %v, want ErrMismatch", name, err)
			}
		})
	}
}

func TestVerifyUnsignedCask(t *testing.T) {
	k := casktest.NewKey(t)
	files := casktest.Files()
	m := casktest.Manifest(files)
	_, err := verifyBytes(casktest.Tar(t, casktest.Entries(files, m, nil)), keys(k))
	if !errors.Is(err, ErrUnsigned) || !strings.Contains(err.Error(), "UNSIGNED CASK") {
		t.Fatalf("err %v, want ErrUnsigned naming UNSIGNED CASK", err)
	}
}

func TestVerifyWrongKey(t *testing.T) {
	cask, _ := casktest.Good(t)
	other := casktest.NewKey(t)
	if _, err := verifyBytes(cask, keys(other)); !errors.Is(err, ErrSignature) {
		t.Fatalf("err %v, want ErrSignature", err)
	}
	if _, err := verifyBytes(cask, nil); err == nil {
		t.Fatal("no trusted keys: accepted")
	}
}

func TestVerifyRotationOverlap(t *testing.T) {
	cask, newKey := casktest.Good(t)
	old := casktest.NewKey(t)
	res, err := verifyBytes(cask, keys(old, newKey))
	if err != nil {
		t.Fatalf("two trusted keys, signed by the second: %v", err)
	}
	if res.KeyID != newKeyID(newKey) {
		t.Fatalf("KeyID %q", res.KeyID)
	}
}

func newKeyID(k *casktest.Key) string {
	r, _ := Sign(casktest.Manifest(map[string][]byte{"x": []byte("x")}), k.Signer)
	seal := string(r)
	i := strings.Index(seal, `"key_id":"`)
	return seal[i+10 : i+26]
}

func TestVerifyWrongPurpose(t *testing.T) {
	k := casktest.NewKey(t)
	files := casktest.Files()
	m := casktest.Manifest(files)
	sig := k.Seal(t, m, "manifest")
	if _, err := verifyBytes(casktest.Tar(t, casktest.Entries(files, m, sig)), keys(k)); !errors.Is(err, ErrSignature) {
		t.Fatalf("seal for another purpose: err %v", err)
	}
}

func TestVerifyManifestEditedAfterSigning(t *testing.T) {
	k := casktest.NewKey(t)
	files := casktest.Files()
	m := casktest.Manifest(files)
	sig := k.Seal(t, m, Purpose)
	files["bin/evil"] = []byte("backdoor")
	m2 := casktest.Manifest(files)
	if _, err := verifyBytes(casktest.Tar(t, casktest.Entries(files, m2, sig)), keys(k)); !errors.Is(err, ErrSignature) {
		t.Fatalf("re-written manifest: err %v", err)
	}
}

func TestVerifyExtraAndMissingFiles(t *testing.T) {
	k := casktest.NewKey(t)
	files := casktest.Files()
	m := casktest.Manifest(files)
	sig := k.Seal(t, m, Purpose)

	extra := casktest.Entries(files, m, sig)
	extra = append(extra, casktest.Entry{Name: casktest.Root + "/bin/extra", Body: []byte("x")})
	if _, err := verifyBytes(casktest.Tar(t, extra), keys(k)); !errors.Is(err, ErrMismatch) {
		t.Fatalf("extra file: err %v", err)
	}

	var missing []casktest.Entry
	for _, e := range casktest.Entries(files, m, sig) {
		if !strings.HasSuffix(e.Name, "configs/wotan.yaml") {
			missing = append(missing, e)
		}
	}
	if _, err := verifyBytes(casktest.Tar(t, missing), keys(k)); !errors.Is(err, ErrMismatch) {
		t.Fatalf("missing file: err %v", err)
	}
}

// hostile returns a good cask's entries with extra appended.
func hostile(t *testing.T, extra ...casktest.Entry) ([]byte, *casktest.Key) {
	k := casktest.NewKey(t)
	files := casktest.Files()
	m := casktest.Manifest(files)
	es := casktest.Entries(files, m, k.Seal(t, m, Purpose))
	return casktest.Tar(t, append(es, extra...)), k
}

func TestUnsafeMembersRefused(t *testing.T) {
	R := casktest.Root
	cases := map[string][]casktest.Entry{
		"dotdot":              {{Name: R + "/../escape"}},
		"dotdot inner":        {{Name: R + "/bin/../../escape"}},
		"absolute":            {{Name: "/etc/passwd"}},
		"absolute under root": {{Name: "/" + R + "/bin/x"}},
		"dot prefix":          {{Name: "./" + R + "/bin/x"}},
		"double slash":        {{Name: R + "//bin/x"}},
		"backslash":           {{Name: R + "/bin\\x"}},
		"symlink escaping":    {{Name: R + "/bin/sh", Type: tar.TypeSymlink, Linkname: "../../../../bin/sh"}},
		"symlink absolute":    {{Name: R + "/bin/sh", Type: tar.TypeSymlink, Linkname: "/bin/sh"}},
		"symlink inside":      {{Name: R + "/bin/alias", Type: tar.TypeSymlink, Linkname: "wotan"}},
		"hardlink escaping":   {{Name: R + "/bin/h", Type: tar.TypeLink, Linkname: "/etc/shadow"}},
		"hardlink inside":     {{Name: R + "/bin/h", Type: tar.TypeLink, Linkname: R + "/bin/wotan"}},
		"char device":         {{Name: R + "/dev/null", Type: tar.TypeChar}},
		"block device":        {{Name: R + "/dev/sda", Type: tar.TypeBlock}},
		"fifo":                {{Name: R + "/bin/fifo", Type: tar.TypeFifo}},
		"second root":         {{Name: "other-root/x"}},
		"duplicate":           {{Name: R + "/bin/wotan", Body: []byte("\x7fELF pretend binary")}},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			cask, k := hostile(t, extra...)
			_, err := verifyBytes(cask, keys(k))
			if !errors.Is(err, ErrEntry) {
				t.Fatalf("err %v, want ErrEntry", err)
			}
		})
	}
}

func TestBadRootNames(t *testing.T) {
	for _, root := range []string{"cask", "sealed-cask-", "sealed-cask-a b", "..", "sealed-cask-x/../.."} {
		cask := casktest.Tar(t, []casktest.Entry{{Name: root + "/BINDING_RUNE.sha256", Body: []byte("x")}})
		if _, err := verifyBytes(cask, nil); !errors.Is(err, ErrEntry) {
			t.Errorf("root %q: err %v", root, err)
		}
	}
	top := casktest.Tar(t, []casktest.Entry{{Name: "BINDING_RUNE.sha256", Body: []byte("x")}})
	if _, err := verifyBytes(top, nil); !errors.Is(err, ErrEntry) {
		t.Errorf("top-level file: err %v", err)
	}
}

func TestSizeCaps(t *testing.T) {
	cask, k := casktest.Good(t)
	lim := DefaultLimits
	lim.MaxMember = 8
	if _, err := Verify(bytes.NewReader(cask), keys(k), lim); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("member cap: %v", err)
	}
	lim = DefaultLimits
	lim.MaxTotal = 40
	if _, err := Verify(bytes.NewReader(cask), keys(k), lim); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("total cap: %v", err)
	}
	lim = DefaultLimits
	lim.MaxEntries = 3
	if _, err := Verify(bytes.NewReader(cask), keys(k), lim); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("entry cap: %v", err)
	}
	lim = DefaultLimits
	lim.MaxManifest = 10
	if _, err := Verify(bytes.NewReader(cask), keys(k), lim); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("manifest cap: %v", err)
	}
	if DefaultLimits.MaxMember != 2<<30 || DefaultLimits.MaxTotal != 8<<30 {
		t.Fatalf("default caps %+v, want 2 GiB per member, 8 GiB total", DefaultLimits)
	}
}

func TestTrailingDataAndNotGzip(t *testing.T) {
	cask, k := casktest.Good(t)
	if _, err := verifyBytes(append(append([]byte(nil), cask...), 'x'), keys(k)); !errors.Is(err, ErrFormat) {
		t.Fatalf("appended byte: err %v", err)
	}
	if _, err := verifyBytes([]byte("not a cask"), keys(k)); !errors.Is(err, ErrFormat) {
		t.Fatalf("not gzip: err %v", err)
	}
	if _, err := verifyBytes(cask[:len(cask)/2], keys(k)); err == nil {
		t.Fatal("truncated cask accepted")
	}
}

func TestParseManifest(t *testing.T) {
	h := strings.Repeat("ab", 32)
	good := h + "  ./bin/x\n" + h + "  ./configs/y.yaml\n"
	m, err := ParseManifest([]byte(good))
	if err != nil || len(m.Files) != 2 {
		t.Fatalf("good manifest: %v %+v", err, m)
	}
	if _, err := ParseManifest([]byte(h + " *./bin/x\n")); err != nil {
		t.Fatalf("binary-mode line: %v", err)
	}
	bad := map[string]string{
		"empty":           "",
		"upper hex":       strings.ToUpper(h) + "  ./bin/x\n",
		"short hash":      h[:62] + "  ./bin/x\n",
		"one space":       h + " ./bin/x\n",
		"escaped name":    "\\" + h + "  ./bin/x\\ny\n",
		"no dot slash":    h + "  bin/x\n",
		"dotdot":          h + "  ./bin/../../x\n",
		"absolute":        h + "  /etc/passwd\n",
		"duplicate":       h + "  ./bin/x\n" + h + "  ./bin/x\n",
		"lists the rune":  h + "  ./BINDING_RUNE.sha256\n",
		"lists the sig":   h + "  ./BINDING_RUNE.sig\n",
		"no final eol":    h + "  ./bin/x",
		"blank line":      h + "  ./bin/x\n\n",
		"crlf":            h + "  ./bin/x\r\n",
		"not clean":       h + "  ./bin//x\n",
		"file and parent": h + "  ./bin\n" + h + "  ./bin/x\n",
	}
	for name, in := range bad {
		if _, err := ParseManifest([]byte(in)); !errors.Is(err, ErrManifest) {
			t.Errorf("%s: err %v, want ErrManifest", name, err)
		}
	}
}

func TestSignRefusesBadManifest(t *testing.T) {
	k := casktest.NewKey(t)
	if _, err := Sign([]byte("not a manifest\n"), k.Signer); !errors.Is(err, ErrManifest) {
		t.Fatalf("err %v", err)
	}
	out, err := Sign(casktest.Manifest(casktest.Files()), k.Signer)
	if err != nil || !bytes.HasSuffix(out, []byte("\n")) || !bytes.Contains(out, []byte(Purpose)) {
		t.Fatalf("sign: %v %q", err, out)
	}
}

func TestExtractGoodCask(t *testing.T) {
	cask, k := casktest.Good(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "cask.tar.gz")
	if err := os.WriteFile(src, cask, 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")
	if _, err := ExtractFile(src, dest, keys(k), DefaultLimits); err != nil {
		t.Fatalf("extract: %v", err)
	}
	for name, body := range casktest.Files() {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || !bytes.Equal(got, body) {
			t.Fatalf("%s: %v %q", name, err, got)
		}
	}
	fi, err := os.Stat(filepath.Join(dest, "bin/wotan"))
	if err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("bin/wotan mode %v %v", fi.Mode(), err)
	}
	fi, err = os.Stat(filepath.Join(dest, "configs/wotan.yaml"))
	if err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("configs mode %v %v", fi.Mode(), err)
	}
	if _, err := os.Stat(filepath.Join(dest, "BINDING_RUNE.sha256")); err != nil {
		t.Fatalf("rune not extracted: %v", err)
	}
	if fi, _ := os.Stat(dest); fi.Mode().Perm() != 0o755 {
		t.Fatalf("dest mode %v", fi.Mode())
	}
}

func TestExtractStripsSpecialBits(t *testing.T) {
	k := casktest.NewKey(t)
	files := casktest.Files()
	m := casktest.Manifest(files)
	es := casktest.Entries(files, m, k.Seal(t, m, Purpose))
	for i := range es {
		if strings.HasSuffix(es[i].Name, "bin/wotan") {
			es[i].Mode = 0o6777
		}
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "cask.tar.gz")
	if err := os.WriteFile(src, casktest.Tar(t, es), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")
	if _, err := ExtractFile(src, dest, keys(k), DefaultLimits); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dest, "bin/wotan"))
	if err != nil || fi.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v %v", fi.Mode(), err)
	}
}

func TestExtractRefusesBeforeWriting(t *testing.T) {
	R := casktest.Root
	for name, e := range map[string]casktest.Entry{
		"dotdot":           {Name: R + "/../escape"},
		"symlink escaping": {Name: R + "/bin/sh", Type: tar.TypeSymlink, Linkname: "../../../../tmp"},
	} {
		t.Run(name, func(t *testing.T) {
			cask, k := hostile(t, e)
			dir := t.TempDir()
			src := filepath.Join(dir, "cask.tar.gz")
			if err := os.WriteFile(src, cask, 0o600); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(dir, "out")
			if _, err := ExtractFile(src, dest, keys(k), DefaultLimits); !errors.Is(err, ErrEntry) {
				t.Fatalf("err %v", err)
			}
			if _, err := os.Lstat(dest); !os.IsNotExist(err) {
				t.Fatalf("dest left behind: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(dir, "escape")); !os.IsNotExist(err) {
				t.Fatal("escape written")
			}
		})
	}
}

func TestExtractRefusesUnsignedAndExistingDest(t *testing.T) {
	k := casktest.NewKey(t)
	files := casktest.Files()
	m := casktest.Manifest(files)
	dir := t.TempDir()
	src := filepath.Join(dir, "cask.tar.gz")
	if err := os.WriteFile(src, casktest.Tar(t, casktest.Entries(files, m, nil)), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")
	if _, err := ExtractFile(src, dest, keys(k), DefaultLimits); !errors.Is(err, ErrUnsigned) {
		t.Fatalf("unsigned: %v", err)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest left behind: %v", err)
	}
	good, k2 := casktest.Good(t)
	if err := os.WriteFile(src, good, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractFile(src, dest, keys(k2), DefaultLimits); err == nil {
		t.Fatal("extract into an existing directory accepted")
	}
}

// FuzzVerify: any byte string is hostile input. Verify never panics, never
// accepts without a trusted signature, and anything it accepts names only
// clean paths under one sealed-cask-* root.
func FuzzVerify(f *testing.F) {
	k := casktest.NewKey(f)
	files := casktest.Files()
	m := casktest.Manifest(files)
	f.Add(casktest.Tar(f, casktest.Entries(files, m, k.Seal(f, m, Purpose))))
	f.Add(casktest.Tar(f, casktest.Entries(files, m, nil)))
	f.Add(casktest.Tar(f, []casktest.Entry{{Name: casktest.Root + "/../x"}}))
	f.Add(casktest.Tar(f, []casktest.Entry{{Name: casktest.Root + "/l", Type: tar.TypeSymlink, Linkname: "/etc"}}))
	f.Add([]byte{0x1f, 0x8b})
	f.Add([]byte{})
	lim := Limits{MaxMember: 1 << 16, MaxTotal: 1 << 18, MaxEntries: 256, MaxManifest: 1 << 14}
	pubs := keys(k)
	f.Fuzz(func(t *testing.T, in []byte) {
		res, err := Verify(bytes.NewReader(in), pubs, lim)
		if err != nil {
			if res != nil {
				t.Fatal("error with a result")
			}
			return
		}
		if !rootRe.MatchString(res.Root) || res.KeyID == "" {
			t.Fatalf("accepted %+v", res)
		}
	})
}
