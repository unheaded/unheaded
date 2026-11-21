// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package ebpf

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cebpf "github.com/cilium/ebpf"
)

func kernelName(s string) [16]byte {
	var n [16]byte
	copy(n[:15], s)
	return n
}

// The kernel keeps 15 bytes of a map's name and a NUL.
func TestPinNameMatches(t *testing.T) {
	cases := []struct {
		kernel, want string
		ok           bool
	}{
		{"HMAC_KEYS", "HMAC_KEYS", true},
		{"HOP_VALIDATORS", "HOP_VALIDATORS", true},
		{"A_NAME_OF_SIXTEE", "A_NAME_OF_SIXTEEN", true}, // both cut at 15
		{"HMAC_KEYS", "HMAC_KEY", false},
		{"HMAC_KEY", "HMAC_KEYS", false},
		{"RETRY_TOKENS", "HMAC_KEYS", false},
		{"", "HMAC_KEYS", false}, // a kernel or loader that set no name
		{"", "", false},
	}
	for _, c := range cases {
		if got := pinNameMatches(kernelName(c.kernel), c.want); got != c.ok {
			t.Errorf("pinNameMatches(%q, %q) = %v, want %v", c.kernel, c.want, got, c.ok)
		}
	}
}

// fakePinFile stands in for a pin: pinTrustError reads owners and modes, which
// an ordinary file has too, so this half needs no bpffs and no root.
func fakePinFile(t *testing.T, dirMode, fileMode os.FileMode) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "hop-ebpf")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "HMAC_KEYS")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, fileMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, dirMode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	return p
}

func TestPinTrustError(t *testing.T) {
	me := os.Geteuid()
	cases := []struct {
		name              string
		dirMode, fileMode os.FileMode
		euid              int
		reason            string // empty: trusted
	}{
		{"the caller's own pin, as bpffs makes it", 0o700, 0o600, me, ""},
		{"a 0755 directory", 0o755, 0o600, me, ""},
		{"a world-writable pin", 0o700, 0o666, me, "group- or world-writable"},
		{"a group-writable pin", 0o700, 0o660, me, "group- or world-writable"},
		{"a world-writable directory", 0o777, 0o600, me, "group- or world-writable"},
		{"a group-writable directory", 0o770, 0o600, me, "group- or world-writable"},
		{"a sticky world-writable directory", 0o777 | os.ModeSticky, 0o600, me, "group- or world-writable"},
		{"another user's pin and directory", 0o700, 0o600, me + 1, "owned by uid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if os.Geteuid() == 0 && c.euid != me {
				t.Skip("root's files are trusted for every caller")
			}
			p := fakePinFile(t, c.dirMode, c.fileMode)
			err := pinTrustError(p, c.euid)
			if c.reason == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, ErrPinUntrusted) {
				t.Fatalf("err = %v, want ErrPinUntrusted", err)
			}
			if !strings.Contains(err.Error(), c.reason) || !strings.Contains(err.Error(), filepath.Dir(p)) {
				t.Errorf("refusal %q does not name the path and %q", err, c.reason)
			}
		})
	}
}

func TestPinTrustError_RefusesLinks(t *testing.T) {
	p := fakePinFile(t, 0o700, 0o600)
	dir := filepath.Dir(p)

	link := filepath.Join(dir, "LINK")
	if err := os.Symlink(p, link); err != nil {
		t.Fatal(err)
	}
	if err := pinTrustError(link, os.Geteuid()); !errors.Is(err, ErrPinUntrusted) || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("symlinked pin: err = %v", err)
	}

	dirLink := filepath.Join(filepath.Dir(dir), "dir-link")
	if err := os.Symlink(dir, dirLink); err != nil {
		t.Fatal(err)
	}
	if err := pinTrustError(filepath.Join(dirLink, "HMAC_KEYS"), os.Geteuid()); !errors.Is(err, ErrPinUntrusted) || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("symlinked pin directory: err = %v", err)
	}
}

func TestPinTrustError_Missing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pins")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(dir, "no-dir", "HMAC_KEYS"), filepath.Join(dir, "HMAC_KEYS")} {
		if err := pinTrustError(p, os.Geteuid()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: err = %v, want os.ErrNotExist", p, err)
		}
	}
}

// The trust check comes before the open: a file that is not a pin and is
// world-writable is refused as untrusted, not as "not a BPF object".
func TestOpenPinnedMapNamed_TrustComesFirst(t *testing.T) {
	p := fakePinFile(t, 0o700, 0o666)
	if _, err := OpenPinnedMapNamed(p, "HMAC_KEYS"); !errors.Is(err, ErrPinUntrusted) {
		t.Fatalf("err = %v, want ErrPinUntrusted", err)
	}
}

func TestOpenPinnedMapNamed_NeedsAName(t *testing.T) {
	if _, err := OpenPinnedMapNamed(fakePinFile(t, 0o700, 0o600), ""); !errors.Is(err, ErrPinName) {
		t.Fatalf("err = %v, want ErrPinName", err)
	}
}

// pinNamedTestMap pins a map the kernel knows as name. Root only.
func pinNamedTestMap(t *testing.T, name string) string {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	cm, err := cebpf.NewMap(&cebpf.MapSpec{Name: name, Type: cebpf.Hash, KeySize: 4, ValueSize: 8, MaxEntries: 8})
	if err != nil {
		t.Fatalf("create map: %v", err)
	}
	t.Cleanup(func() { _ = cm.Close() })
	dir, err := os.MkdirTemp("/sys/fs/bpf", "row41-")
	if err != nil {
		t.Fatalf("mkdir in bpffs: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, name)
	if err := cm.Pin(path); err != nil {
		t.Fatalf("pin: %v", err)
	}
	return path
}

func TestOpenPinnedMapNamed_RoundTrip(t *testing.T) {
	m, err := OpenPinnedMapNamed(pinNamedTestMap(t, "HMAC_KEYS"), "HMAC_KEYS")
	if err != nil {
		t.Fatalf("OpenPinnedMapNamed: %v", err)
	}
	defer m.Close()
	if err := m.Update([]byte{1, 0, 0, 0}, []byte{1, 2, 3, 4, 5, 6, 7, 8}); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

// A stale or wrong pin of the right sizes under the expected file name: the
// kernel's own name for the map gives it away.
func TestOpenPinnedMapNamed_RefusesAnotherMapUnderTheName(t *testing.T) {
	path := pinNamedTestMap(t, "SOMETHING_ELSE")
	wrong := filepath.Join(filepath.Dir(path), "HMAC_KEYS")
	if err := os.Rename(path, wrong); err != nil {
		t.Fatalf("rename pin: %v", err)
	}
	_, err := OpenPinnedMapNamed(wrong, "HMAC_KEYS")
	if !errors.Is(err, ErrPinName) {
		t.Fatalf("err = %v, want ErrPinName", err)
	}
	for _, s := range []string{wrong, "SOMETHING_ELSE", "HMAC_KEYS"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("refusal %q does not say %q", err, s)
		}
	}
}
