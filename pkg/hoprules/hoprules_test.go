// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package hoprules

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/protocol/bpfschema"
)

// write is one fake map write, copied so a later change cannot rewrite it.
type write struct {
	m          string
	key, value []byte
}

// fakeMaps records writes and fails the one numbered failAt, counting from 1.
type fakeMaps struct {
	writes []write
	failAt int
}

var errFake = errors.New("fake map failure")

func (f *fakeMaps) Update(m string, key, value []byte) error {
	if f.failAt != 0 && len(f.writes)+1 == f.failAt {
		return errFake
	}
	f.writes = append(f.writes, write{m, bytes.Clone(key), bytes.Clone(value)})
	return nil
}

func at(d time.Duration) func() time.Duration {
	return func() time.Duration { return d }
}

// testKey is thirty-two bytes no error or dump can hit by chance.
func testKey() []byte {
	k := make([]byte, KeySize)
	for i := range k {
		k[i] = 0xA0 + byte(i)
	}
	return k
}

func keyFile(t *testing.T, data []byte, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hop.key")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

// loaded returns a one-key File ready for Apply.
func loaded(t *testing.T, k HMACKey) *File {
	t.Helper()
	k.KeyFile = keyFile(t, testKey(), 0o400)
	f := &File{HMACKeys: []HMACKey{k}}
	if err := f.Load(); err != nil {
		t.Fatal(err)
	}
	return f
}

func le(t *testing.T, v any) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestParse_ReadsEveryField(t *testing.T) {
	f, err := Parse(strings.NewReader(`
validators:
  - hop: 7
    namespace: 2
    version_min: 1
    version_max: 3
    require_crc: true
    require_hmac: true
    require_retry: true
    max_hop_count: 9
hmac_keys:
  - flow: 0x2211
    namespace: 2
    key_file: /etc/unheaded/hop/a.key
    ttl: 24h
retry:
  - flow: 0x2211
    source: fd00::1:2:3:4
    ttl: 30m
`))
	if err != nil {
		t.Fatal(err)
	}
	wantV := Validator{Hop: 7, Namespace: 2, VersionMin: 1, VersionMax: 3, RequireCRC: true,
		RequireHMAC: true, RequireRetry: true, MaxHopCount: 9}
	if len(f.Validators) != 1 || f.Validators[0] != wantV {
		t.Errorf("validators = %+v, want %+v", f.Validators, wantV)
	}
	if len(f.HMACKeys) != 1 {
		t.Fatalf("hmac_keys = %d entries", len(f.HMACKeys))
	}
	if k := f.HMACKeys[0]; k.Flow != 0x2211 || k.Namespace != 2 || k.KeyFile != "/etc/unheaded/hop/a.key" || k.TTL != 24*time.Hour {
		t.Errorf("hmac key = %+v", k)
	}
	wantR := Retry{Flow: 0x2211, Source: netip.MustParseAddr("fd00::1:2:3:4"), TTL: 30 * time.Minute}
	if len(f.Retry) != 1 || f.Retry[0] != wantR {
		t.Errorf("retry = %+v, want %+v", f.Retry, wantR)
	}
}

func TestParse_Refuses(t *testing.T) {
	key := "hmac_keys:\n  - key_file: /k\n"
	cases := []struct{ name, in, want string }{
		{"empty", "", "empty"},
		{"no entries", "validators: []\n", "no entries"},
		{"unknown top level", "rules: []\n", "field rules not found"},
		{"dict ids are not evaluated", "validators:\n  - allowed_dict_ids: [1]\n", "field allowed_dict_ids not found"},
		{"key inline", "hmac_keys:\n  - key: aa\n    key_file: /k\n", "field key not found"},
		{"namespace over a ring byte", "validators:\n  - namespace: 256\n", "uint8"},
		{"negative hop", "validators:\n  - hop: -1\n", "uint32"},
		{"second document", "validators:\n  - hop: 1\n---\nvalidators:\n  - hop: 2\n", "one YAML document"},
		{"version range upside down", "validators:\n  - version_min: 3\n    version_max: 2\n", "version_min"},
		{"duplicate validator", "validators:\n  - hop: 1\n  - hop: 1\n", "validators[1]: same"},
		{"same hop other namespace is fine, same both is not", "validators:\n  - namespace: 1\n  - namespace: 2\n  - namespace: 1\n", "validators[2]: same"},
		{"duplicate key", key + "  - key_file: /l\n", "hmac_keys[1]: same"},
		{"no key file", "hmac_keys:\n  - flow: 1\n", "key_file"},
		{"relative key file", "hmac_keys:\n  - key_file: a.key\n", "absolute"},
		{"negative ttl", "hmac_keys:\n  - key_file: /k\n    ttl: -1s\n", "ttl"},
		{"part of a second", "hmac_keys:\n  - key_file: /k\n    ttl: 1500ms\n", "whole seconds"},
		{"bare number ttl", "hmac_keys:\n  - key_file: /k\n    ttl: 60\n", "Duration"},
		{"no source", "retry:\n  - flow: 1\n", "IPv6"},
		{"v4 source", "retry:\n  - source: 10.0.0.1\n", "IPv6"},
		{"zoned source", "retry:\n  - source: fe80::1%eth0\n", "zone"},
		{"not an address", "retry:\n  - source: nope\n", "nope"},
		{"same low 48 bits", "retry:\n  - source: fd00::1:2:3:4\n  - source: fd01::1:2:3:4\n", "retry[1]: same"},
		{"too big", "validators:\n" + strings.Repeat("  # padding\n", maxRulesBytes/10), "bytes"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse(strings.NewReader(c.in))
			if err == nil {
				t.Fatalf("parsed: %+v", f)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not name %q", err, c.want)
			}
		})
	}
}

// A secret pasted where a path goes must not come back in the refusal.
func TestParse_RelativeKeyFileIsNotEchoed(t *testing.T) {
	_, err := Parse(strings.NewReader("hmac_keys:\n  - key_file: 00112233445566778899\n"))
	if err == nil || strings.Contains(err.Error(), "00112233445566778899") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoad_ModesAndSizes(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		mode    os.FileMode
		wantErr error
	}{
		{"0400", testKey(), 0o400, nil},
		{"0600", testKey(), 0o600, nil},
		{"0440", testKey(), 0o440, ErrKeyFileMode},
		{"0640", testKey(), 0o640, ErrKeyFileMode},
		{"0644", testKey(), 0o644, ErrKeyFileMode},
		{"0604", testKey(), 0o604, ErrKeyFileMode},
		{"0700", testKey(), 0o700, ErrKeyFileMode},
		{"0500", testKey(), 0o500, ErrKeyFileMode},
		{"31 bytes", testKey()[:31], 0o400, ErrKeyFileSize},
		{"33 bytes", append(testKey(), 0), 0o400, ErrKeyFileSize},
		{"empty", nil, 0o400, ErrKeyFileSize},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &File{HMACKeys: []HMACKey{{KeyFile: keyFile(t, c.data, c.mode)}}}
			err := f.Load()
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if err != nil {
				if f.HMACKeys[0].key != nil {
					t.Error("a refused file left a key behind")
				}
				return
			}
			if k := f.HMACKeys[0].key(); !bytes.Equal(k[:], testKey()) {
				t.Errorf("key = %x", k)
			}
		})
	}
}

func TestLoad_RefusesWhatIsNotARegularFile(t *testing.T) {
	real := keyFile(t, testKey(), 0o400)
	link := filepath.Join(t.TempDir(), "link.key")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]string{"symlink": link, "directory": t.TempDir(), "missing": real + ".gone"} {
		t.Run(name, func(t *testing.T) {
			f := &File{HMACKeys: []HMACKey{{KeyFile: p}}}
			if err := f.Load(); err == nil {
				t.Fatal("loaded")
			}
		})
	}
}

// The second entry fails; its error names that entry.
func TestLoad_NamesTheEntry(t *testing.T) {
	f := &File{HMACKeys: []HMACKey{
		{KeyFile: keyFile(t, testKey(), 0o400)},
		{Flow: 1, KeyFile: keyFile(t, testKey(), 0o644)},
	}}
	if err := f.Load(); err == nil || !strings.Contains(err.Error(), "hmac_keys[1]") {
		t.Fatalf("err = %v", err)
	}
}

// fmt walks unexported fields, and a bad verb such as %s follows a pointer
// into them: a key held by value or by pointer prints. Only an address may.
func TestKeyBytesAreInNoDump(t *testing.T) {
	f := loaded(t, HMACKey{Flow: 1})
	k := testKey()
	needles := []string{fmt.Sprintf("%x", k), fmt.Sprintf("%d", k), fmt.Sprintf("%v", k), string(k)}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x"} {
		for _, target := range []any{f, *f, f.HMACKeys, f.HMACKeys[0]} {
			out := fmt.Sprintf(verb, target)
			for _, n := range needles {
				if strings.Contains(out, n) {
					t.Fatalf("%s of %T prints the key: %s", verb, target, out)
				}
			}
			if strings.Contains(out, "160") || strings.Contains(out, "a0a1") {
				t.Fatalf("%s of %T prints key bytes: %s", verb, target, out)
			}
		}
	}
}

func TestApply_ExactBytes(t *testing.T) {
	f := loaded(t, HMACKey{Flow: 0x2211, Namespace: 2, TTL: time.Hour})
	f.Validators = []Validator{{Hop: 7, Namespace: 2, VersionMin: 1, VersionMax: 3, RequireCRC: true,
		RequireHMAC: true, RequireRetry: true, MaxHopCount: 9}}
	f.Retry = []Retry{{Flow: 0x2211, Source: netip.MustParseAddr("fd00::1:2:3:4"), TTL: time.Minute}}

	var w fakeMaps
	got, err := Apply(&w, f, at(100*time.Second+999*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Applied{Validators: 1, HMACKeys: 1, Retry: 1}); got != want {
		t.Errorf("applied = %+v, want %+v", got, want)
	}

	var key [KeySize]byte
	copy(key[:], testKey())
	want := []write{
		{bpfschema.MapHMACKeys,
			le(t, bpfschema.HMACKeyKey{FlowID: 0x2211, Namespace: 2}),
			le(t, bpfschema.HMACKeyValue{Key: key, CreatedAt: 100, ExpiresAt: 3700})},
		{bpfschema.MapRetryTokens,
			le(t, bpfschema.RetryTokenKey{FlowID: 0x2211, SrcLow48: [6]byte{0x04, 0x00, 0x03, 0x00, 0x02, 0x00}}),
			le(t, bpfschema.RetryTokenValue{IssuedAt: 100, ExpiresAt: 160})},
		{bpfschema.MapHopValidators,
			le(t, bpfschema.HopValidatorKey{HopID: 7, Namespace: 2}),
			le(t, bpfschema.HopValidatorValue{AllowedVersionMin: 1, AllowedVersionMax: 3, RequireCRC: 1,
				RequireHMAC: 1, MaxHopCount: 9, Flags: 1})},
	}
	if len(w.writes) != len(want) {
		t.Fatalf("%d writes, want %d", len(w.writes), len(want))
	}
	for i := range want {
		if g := w.writes[i]; g.m != want[i].m || !bytes.Equal(g.key, want[i].key) || !bytes.Equal(g.value, want[i].value) {
			t.Errorf("write %d:\n got %s %x %x\nwant %s %x %x", i, g.m, g.key, g.value, want[i].m, want[i].key, want[i].value)
		}
	}
}

// The same three keys as bytes on the wire to the map, written out by hand:
// bpfschema's structs cannot vouch for themselves. These are make_hmac_key,
// make_validator_key and make_retry_key in ebpf/hop-ebpf/src/main.rs.
func TestApply_KeysMatchHopEbpfByHand(t *testing.T) {
	f := loaded(t, HMACKey{Flow: 0x2211, Namespace: 2})
	f.Validators = []Validator{{Hop: 7, Namespace: 2}}
	f.Retry = []Retry{{Flow: 0x2211, Source: netip.MustParseAddr("fd00::1:2:3:4")}}
	var w fakeMaps
	if _, err := Apply(&w, f, at(time.Second)); err != nil {
		t.Fatal(err)
	}
	want := map[string]uint64{
		bpfschema.MapHMACKeys:      0x2211 | 2<<16,
		bpfschema.MapRetryTokens:   0x2211 | 0x0002_0003_0004<<16,
		bpfschema.MapHopValidators: 7 | 2<<32,
	}
	for _, g := range w.writes {
		if got := binary.LittleEndian.Uint64(g.key); got != want[g.m] {
			t.Errorf("%s key = %#x, want %#x", g.m, got, want[g.m])
		}
	}
}

func TestApply_SizesAreTheMaps(t *testing.T) {
	f := loaded(t, HMACKey{})
	f.Validators = []Validator{{}}
	f.Retry = []Retry{{Source: netip.MustParseAddr("fd00::1")}}
	var w fakeMaps
	if _, err := Apply(&w, f, at(time.Second)); err != nil {
		t.Fatal(err)
	}
	want := map[string][2]int{
		bpfschema.MapHMACKeys:      {8, 40},
		bpfschema.MapRetryTokens:   {8, 48},
		bpfschema.MapHopValidators: {8, 32},
	}
	for _, g := range w.writes {
		if s := want[g.m]; len(g.key) != s[0] || len(g.value) != s[1] {
			t.Errorf("%s: key %d value %d, want %v", g.m, len(g.key), len(g.value), s)
		}
	}
}

func TestApply_NoTTLIsNever(t *testing.T) {
	f := loaded(t, HMACKey{})
	f.Retry = []Retry{{Source: netip.MustParseAddr("fd00::1")}}
	var w fakeMaps
	if _, err := Apply(&w, f, at(50*time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint32(w.writes[0].value[36:]); got != 0 {
		t.Errorf("hmac expires_at = %d", got)
	}
	if got := binary.LittleEndian.Uint32(w.writes[1].value[36:]); got != 0 {
		t.Errorf("retry expires_at = %d", got)
	}
}

// In enforce mode a rule that lands before its key drops traffic, so rules
// go last whatever order the file gives.
func TestApply_ValidatorsLast(t *testing.T) {
	f := loaded(t, HMACKey{})
	f.Validators = []Validator{{Hop: 1}, {Hop: 2}}
	f.Retry = []Retry{{Source: netip.MustParseAddr("fd00::1")}}
	var w fakeMaps
	if _, err := Apply(&w, f, at(time.Second)); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range w.writes {
		got = append(got, g.m)
	}
	want := []string{bpfschema.MapHMACKeys, bpfschema.MapRetryTokens, bpfschema.MapHopValidators, bpfschema.MapHopValidators}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("order = %v", got)
	}
}

func TestApply_RefusesBeforeAnyWrite(t *testing.T) {
	v4 := netip.MustParseAddr("10.0.0.1")
	good := func(t *testing.T) *File {
		f := loaded(t, HMACKey{})
		f.Validators = []Validator{{}}
		return f
	}
	cases := []struct {
		name  string
		file  func(*testing.T) *File
		clock func() time.Duration
		want  string
	}{
		{"key never loaded", func(*testing.T) *File {
			return &File{HMACKeys: []HMACKey{{KeyFile: "/k"}}}
		}, at(time.Second), "not loaded"},
		{"second key not loaded", func(t *testing.T) *File {
			f := good(t)
			f.HMACKeys = append(f.HMACKeys, HMACKey{Flow: 1, KeyFile: "/k"})
			return f
		}, at(time.Second), "hmac_keys[1]"},
		{"hand-built file is validated", func(t *testing.T) *File {
			f := good(t)
			f.Retry = []Retry{{Source: v4}}
			return f
		}, at(time.Second), "IPv6"},
		{"no clock", good, nil, "boottime"},
		{"clock before boot", good, at(-time.Second), "boottime"},
		{"clock past a u32", good, at((1 << 32) * time.Second), "boottime"},
		{"expiry past a u32", func(t *testing.T) *File {
			f := good(t)
			f.HMACKeys[0].TTL = (1 << 32) * time.Second
			return f
		}, at(time.Second), "ttl"},
		{"retry expiry past a u32", func(t *testing.T) *File {
			f := good(t)
			f.Retry = []Retry{{Source: netip.MustParseAddr("fd00::1"), TTL: (1 << 32) * time.Second}}
			return f
		}, at(time.Second), "ttl"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var w fakeMaps
			got, err := Apply(&w, c.file(t), c.clock)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if len(w.writes) != 0 || got != (Applied{}) {
				t.Errorf("wrote %d, applied %+v", len(w.writes), got)
			}
		})
	}
}

// A map failure stops the walk and Applied says what landed.
func TestApply_WriterFailure(t *testing.T) {
	f := loaded(t, HMACKey{})
	f.Validators = []Validator{{Hop: 1}, {Hop: 2}}
	w := fakeMaps{failAt: 3}
	got, err := Apply(&w, f, at(time.Second))
	if !errors.Is(err, errFake) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), bpfschema.MapHopValidators) {
		t.Errorf("error names no map: %v", err)
	}
	if want := (Applied{HMACKeys: 1, Validators: 1}); got != want {
		t.Errorf("applied = %+v, want %+v", got, want)
	}
	for _, s := range []string{fmt.Sprintf("%x", testKey()), fmt.Sprintf("%v", testKey())} {
		if strings.Contains(err.Error(), s) {
			t.Error("error prints the key")
		}
	}
}
