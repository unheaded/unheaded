// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/gungnir"
	wotanClient "unheaded/pkg/wotan-client"
)

// imageWithData builds a valid tick-stage image: one HALT word of ROM, then
// data as its DATA section (len(data) must be a multiple of 4).
func imageWithData(t *testing.T, data []byte) []byte {
	t.Helper()
	if len(data)%4 != 0 {
		t.Fatalf("data length %d is not a multiple of 4", len(data))
	}
	le := binary.LittleEndian
	b := []byte{'M', 'B', 'C', 'C', 0, 1}
	b = le.AppendUint16(b, 64)                       // max_insn
	b = le.AppendUint16(b, 1)                        // rom_words
	b = le.AppendUint16(b, uint16(len(data)/4))      // data_words
	b = le.AppendUint16(b, 0)                        // entry
	b = le.AppendUint16(b, 16)                       // loop_bound
	b = le.AppendUint16(b, 0)                        // flags
	b = le.AppendUint16(b, 0)                        // reserved
	body := append(le.AppendUint32(nil, 0), data...) // ROM then DATA
	h := sha256.New()
	h.Write(b[:20])
	h.Write(body)
	b = append(b, h.Sum(nil)...)
	return append(b, body...)
}

func scorerData(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../cartridges/scorer.data.bin")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sealed(t *testing.T, s *gungnir.Signer, model string, data []byte) []byte {
	t.Helper()
	img := imageWithData(t, data)
	p, err := SealWeightsFromImage(img, model, s, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWeightsDataSection_IsTheDATABytes(t *testing.T) {
	data := scorerData(t)
	got, imgSHA, err := WeightsDataSection(imageWithData(t, data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("DATA section differs from scorer.data.bin")
	}
	if imgSHA == ([32]byte{}) {
		t.Fatal("image sha256 not returned")
	}
	if _, _, err := WeightsDataSection([]byte("not a cart")); err == nil {
		t.Fatal("a non-image was accepted")
	}
	if _, _, err := WeightsDataSection(imageWithData(t, nil)); !errors.Is(err, ErrNoData) {
		t.Fatalf("image without DATA: %v, want ErrNoData", err)
	}
}

func TestWeightsModelName(t *testing.T) {
	for _, ok := range []string{"scorer", "scorer-v2", "a", "s_1"} {
		if err := ValidModel(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Scorer", "a.b", "../x", "a/b", "*", "#", "-x", strings.Repeat("a", 64)} {
		if err := ValidModel(bad); !errors.Is(err, ErrModel) {
			t.Errorf("%q: %v, want ErrModel", bad, err)
		}
	}
	if got := WeightsTopic("scorer"); got != "upc.cart.weights.scorer" {
		t.Fatalf("topic %q", got)
	}
}

func TestWeightsSeal_Good(t *testing.T) {
	s, pub := testKey(t, 1)
	data := scorerData(t)
	w, err := OpenWeights(sealed(t, s, "scorer", data), "scorer", pub)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if !bytes.Equal(w.Data, data) || w.SHA256 != hex.EncodeToString(sum[:]) || w.Model != "scorer" || w.KeyID != gungnir.KeyID(pub) {
		t.Fatalf("opened %+v", w)
	}
}

func TestWeightsSeal_TamperedData(t *testing.T) {
	s, pub := testKey(t, 1)
	payload := sealed(t, s, "scorer", scorerData(t))
	var env map[string]json.RawMessage
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatal(err)
	}
	var mb64 string
	if err := json.Unmarshal(env["manifest"], &mb64); err != nil {
		t.Fatal(err)
	}
	manifest, _ := base64.StdEncoding.DecodeString(mb64)
	var m map[string]any
	if err := json.Unmarshal(manifest, &m); err != nil {
		t.Fatal(err)
	}
	data, _ := base64.StdEncoding.DecodeString(m["data"].(string))
	data[7] ^= 0x01 // one weight bit
	m["data"] = base64.StdEncoding.EncodeToString(data)
	manifest, _ = json.Marshal(m)
	env["manifest"], _ = json.Marshal(base64.StdEncoding.EncodeToString(manifest))
	tampered, _ := json.Marshal(env)
	if _, err := OpenWeights(tampered, "scorer", pub); !errors.Is(err, ErrWeightsSeal) {
		t.Fatalf("tampered data: %v, want ErrWeightsSeal", err)
	}
}

func TestWeightsSeal_WrongKey(t *testing.T) {
	s, _ := testKey(t, 1)
	_, other := testKey(t, 2)
	if _, err := OpenWeights(sealed(t, s, "scorer", scorerData(t)), "scorer", other); !errors.Is(err, ErrWeightsSeal) {
		t.Fatalf("wrong key: %v, want ErrWeightsSeal", err)
	}
}

func TestWeightsSeal_WrongModel(t *testing.T) {
	s, pub := testKey(t, 1)
	if _, err := OpenWeights(sealed(t, s, "sampler", scorerData(t)), "scorer", pub); !errors.Is(err, ErrWeightsModel) {
		t.Fatalf("wrong model: %v, want ErrWeightsModel", err)
	}
}

// A seal that verifies over a manifest whose data does not hash to its
// data_sha256 (a buggy or hostile signer) is still refused.
func TestWeightsSeal_SHAMismatch(t *testing.T) {
	s, pub := testKey(t, 1)
	data := scorerData(t)
	m := weightsManifest{V: weightsVersion, Model: "scorer", DataSHA256: strings.Repeat("00", 32), DataWords: len(data) / 4, Data: data}
	mb, _ := json.Marshal(m)
	payload, err := sealManifest(mb, s, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWeights(payload, "scorer", pub); !errors.Is(err, ErrWeightsSHA) {
		t.Fatalf("sha mismatch: %v, want ErrWeightsSHA", err)
	}
}

func TestWeightsSeal_Expired(t *testing.T) {
	s, pub := testKey(t, 1)
	data := scorerData(t)
	payload, err := SealWeightsFromImage(imageWithData(t, data), "scorer", s, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2100 * time.Millisecond)
	if _, err := OpenWeights(payload, "scorer", pub); !errors.Is(err, gungnir.ErrSealExpired) {
		t.Fatalf("expired: %v, want ErrSealExpired", err)
	}
}

func TestWeightsSeal_Garbage(t *testing.T) {
	_, pub := testKey(t, 1)
	for _, p := range []string{"", "{}", "not json", `{"type":"other/v1"}`, `{"type":"upc.cart.weights/v1","manifest":"!!","seal":{}}`} {
		if _, err := OpenWeights([]byte(p), "scorer", pub); !errors.Is(err, ErrWeightsEnvelope) && !errors.Is(err, ErrWeightsSeal) {
			t.Errorf("%q: %v, want a refusal", p, err)
		}
	}
	if _, err := OpenWeights([]byte(`{}`), "scorer", nil); err == nil {
		t.Error("nil pubkey accepted")
	}
}

// fakeWotan speaks the slice of Wotan's REST API weights uses: subscribe
// (auto-approved or pending), publish into a per-topic log with global
// seqs, and GET messages paged by after_seq and limit, oldest first.
type fakeWotan struct {
	mu      sync.Mutex
	seq     int64
	msgs    map[string][]wotanClient.Message
	pending bool
	maxPage int
	joins   []string
}

func newFakeWotan(t *testing.T) (*fakeWotan, string) {
	f := &fakeWotan{msgs: map[string][]wotanClient.Message{}, maxPage: 1000}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func (f *fakeWotan) put(topic, payload string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	f.msgs[topic] = append(f.msgs[topic], wotanClient.Message{MessageID: fmt.Sprint(f.seq), Topic: topic, Seq: f.seq, Payload: payload})
}

func (f *fakeWotan) serve(w http.ResponseWriter, r *http.Request) {
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/topics/")
	i := strings.LastIndex(rest, "/")
	if !ok || i <= 0 {
		http.NotFound(w, r)
		return
	}
	topic, action := rest[:i], rest[i+1:]
	w.Header().Set("Content-Type", "application/json")
	switch {
	case action == "subscribe" && r.Method == http.MethodPost:
		var req struct {
			DisplayName string `json:"display_name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.joins = append(f.joins, req.DisplayName)
		status := "approved"
		if f.pending {
			status = "pending"
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"subscriber": map[string]any{
			"subscriber_id": "00000000-0000-0000-0000-000000000001", "topic": topic, "display_name": req.DisplayName, "status": status,
		}})
	case action == "publish" && r.Method == http.MethodPost:
		var req struct {
			Payload string `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Payload == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.put(topic, req.Payload)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"message_id": "x", "topic": topic})
	case action == "messages" && r.Method == http.MethodGet:
		after, _ := strconv.ParseInt(r.URL.Query().Get("after_seq"), 10, 64)
		limit := f.maxPage
		if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v < limit {
			limit = v
		}
		f.mu.Lock()
		out := []wotanClient.Message{}
		var last int64
		for _, m := range f.msgs[topic] {
			last = m.Seq
			if m.Seq > after && len(out) < limit {
				out = append(out, m)
			}
		}
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"messages": out, "last_seq": last})
	default:
		http.NotFound(w, r)
	}
}

// keyFiles writes a 0600 seed and its packed public key.
func keyFiles(t *testing.T, seedByte byte) (seedPath, pubPath string) {
	t.Helper()
	dir := t.TempDir()
	var seed [mldsa65.SeedSize]byte
	for i := range seed {
		seed[i] = seedByte
	}
	pub, _ := mldsa65.NewKeyFromSeed(&seed)
	seedPath, pubPath = filepath.Join(dir, "lab.seed"), filepath.Join(dir, "lab.pub")
	if err := os.WriteFile(seedPath, seed[:], 0o600); err != nil {
		t.Fatal(err)
	}
	pb, _ := pub.MarshalBinary()
	if err := os.WriteFile(pubPath, pb, 0o644); err != nil {
		t.Fatal(err)
	}
	return seedPath, pubPath
}

func writeCart(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, imageWithData(t, data), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func runWeights(args ...string) (string, error) {
	var out bytes.Buffer
	err := cmdWeights(context.Background(), args, nil, &out)
	return out.String(), err
}

func TestWeightsCLI_PublishFetchRoundTrip(t *testing.T) {
	f, url := newFakeWotan(t)
	seed, pub := keyFiles(t, 3)
	data := scorerData(t)
	cart := writeCart(t, "scorer.cart", data)

	// Older weights first: fetch must take the newest message.
	if _, err := runWeights("publish", "--wotan", url, "--cart", writeCart(t, "scorer.cart", make([]byte, 8)), "--sign-key", seed); err != nil {
		t.Fatal(err)
	}
	out, err := runWeights("publish", "--wotan", url, "--cart", cart, "--sign-key", seed)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	if !strings.Contains(out, "upc.cart.weights.scorer") || !strings.Contains(out, sha) {
		t.Fatalf("publish output %q", out)
	}
	if len(f.msgs["upc.cart.weights.scorer"]) != 2 || f.joins[0] != weightsIdentity {
		t.Fatalf("published %d messages, joins %v", len(f.msgs["upc.cart.weights.scorer"]), f.joins)
	}

	dir := filepath.Join(t.TempDir(), "nodeB")
	out, err = runWeights("fetch", "--wotan", url, "--model", "scorer", "--pubkey", pub, "--out", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "seal verified") || !strings.Contains(out, sha) {
		t.Fatalf("fetch output %q", out)
	}
	path := filepath.Join(dir, "scorer.data.bin")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("fetched DATA differs from scorer.data.bin")
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", fi.Mode().Perm())
	}
}

// The newest message is found across pages.
func TestWeightsCLI_FetchPagesToNewest(t *testing.T) {
	f, url := newFakeWotan(t)
	f.maxPage = 2
	seed, pub := keyFiles(t, 3)
	for i := 0; i < 4; i++ {
		f.put("upc.cart.weights.scorer", "noise")
	}
	data := scorerData(t)
	if _, err := runWeights("publish", "--wotan", url, "--cart", writeCart(t, "x.cart", data), "--sign-key", seed, "--model", "scorer"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := runWeights("fetch", "--wotan", url, "--model", "scorer", "--pubkey", pub, "--out", dir); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "scorer.data.bin")); !bytes.Equal(got, data) {
		t.Fatal("did not fetch the newest message")
	}
}

func TestWeightsCLI_FetchRefusals(t *testing.T) {
	f, url := newFakeWotan(t)
	seed, pub := keyFiles(t, 3)
	_, otherPub := keyFiles(t, 4)
	dir := t.TempDir()

	if _, err := runWeights("fetch", "--wotan", url, "--model", "scorer", "--pubkey", pub, "--out", dir); !errors.Is(err, ErrNoWeights) {
		t.Fatalf("empty topic: %v, want ErrNoWeights", err)
	}
	if _, err := runWeights("publish", "--wotan", url, "--cart", writeCart(t, "scorer.cart", scorerData(t)), "--sign-key", seed); err != nil {
		t.Fatal(err)
	}
	if _, err := runWeights("fetch", "--wotan", url, "--model", "scorer", "--pubkey", otherPub, "--out", dir); !errors.Is(err, ErrWeightsSeal) {
		t.Fatalf("wrong key: %v, want ErrWeightsSeal", err)
	}
	// A sealed scorer envelope replayed onto another model's topic.
	f.put("upc.cart.weights.sampler", f.msgs["upc.cart.weights.scorer"][0].Payload)
	if _, err := runWeights("fetch", "--wotan", url, "--model", "sampler", "--pubkey", pub, "--out", dir); !errors.Is(err, ErrWeightsModel) {
		t.Fatalf("wrong model: %v, want ErrWeightsModel", err)
	}
	// Junk published after the good message: the newest is refused, not skipped.
	f.put("upc.cart.weights.scorer", "junk")
	if _, err := runWeights("fetch", "--wotan", url, "--model", "scorer", "--pubkey", pub, "--out", dir); !errors.Is(err, ErrWeightsEnvelope) {
		t.Fatalf("junk newest: %v, want ErrWeightsEnvelope", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "scorer.data.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a refused fetch wrote a file")
	}
	if _, err := runWeights("fetch", "--wotan", url, "--model", "../x", "--pubkey", pub, "--out", dir); !errors.Is(err, ErrModel) {
		t.Fatalf("bad model name: %v, want ErrModel", err)
	}
}

func TestWeightsCLI_PublishRefusals(t *testing.T) {
	f, url := newFakeWotan(t)
	seed, _ := keyFiles(t, 3)
	if _, err := runWeights("publish", "--wotan", url, "--cart", writeCart(t, "empty.cart", nil), "--sign-key", seed); !errors.Is(err, ErrNoData) {
		t.Fatalf("no DATA: %v, want ErrNoData", err)
	}
	if err := os.Chmod(seed, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runWeights("publish", "--wotan", url, "--cart", writeCart(t, "scorer.cart", scorerData(t)), "--sign-key", seed); !errors.Is(err, gungnir.ErrKeyFilePerms) {
		t.Fatalf("readable seed: %v, want ErrKeyFilePerms", err)
	}
	if err := os.Chmod(seed, 0o600); err != nil {
		t.Fatal(err)
	}
	f.pending = true
	if _, err := runWeights("publish", "--wotan", url, "--cart", writeCart(t, "scorer.cart", scorerData(t)), "--sign-key", seed); err == nil || !strings.Contains(err.Error(), "auto_approve") {
		t.Fatalf("pending subscription: %v, want an auto_approve hint", err)
	}
	if _, err := runWeights("publish", "--cart", "x", "--sign-key", seed); err == nil {
		t.Fatal("missing --wotan accepted")
	}
	if _, err := runWeights("bogus"); err == nil {
		t.Fatal("unknown subcommand accepted")
	}
}
