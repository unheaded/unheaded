// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// weights publish|fetch: a cartridge's DATA section (the scorer's int8
// weights) travels over Wotan from one node to another, sealed with Gungnir
// ML-DSA-65 by the publisher and verified by the fetcher against a pinned
// public key (S-ASCEND-2 Step 221, the DISTRIBUTED leg).
//
// Topic: upc.cart.weights.<model>, under upc.cart.* (RFC-EDITOR-REQUESTS-S5
// item 3; the weights sub-topic is provisional). One message per publish;
// fetch takes the newest.
//
// Wire format (the Wotan message payload, JSON):
//
//	{"type": "upc.cart.weights/v1",
//	 "manifest": "<base64 of the manifest bytes>",
//	 "seal": <GungnirSeal, the same JSON as a .cart.seal file>}
//
// The seal is over the manifest bytes exactly as carried (no
// re-canonicalisation). The manifest is JSON:
//
//	{"v": 1, "model": "scorer", "data_sha256": "<hex>", "data_words": 133,
//	 "image_sha256": "<hex: the source .cart's embedded sha256>",
//	 "data": "<base64 of the DATA section, little-endian words>"}
//
// fetch checks, in order: envelope shape and type; the seal against the
// pinned key (algorithm, expiry, payload hash, ML-DSA-65 signature); the
// manifest's version and that its model is the one asked for (a sealed
// envelope replayed onto another model's topic is refused); data length a
// whole number of words, at most one slot, equal to data_words; the data's
// sha256 equal to data_sha256. Only then is <out>/<model>.data.bin written
// (0600, through a temp file and rename).

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/cartridge"
	"unheaded/pkg/gungnir"
	wotanClient "unheaded/pkg/wotan-client"
)

const (
	weightsType    = "upc.cart.weights/v1"
	weightsVersion = 1
	// weightsTopicPrefix + model is the topic (RFC-EDITOR-REQUESTS-S5 item 3,
	// upc.cart.*; provisional).
	weightsTopicPrefix = "upc.cart.weights."
	// weightsIdentity is the Wotan display name and service identity this
	// tool joins as (wotanClient.InitServiceIdentity; key
	// /etc/unheaded/keys/upc-cartridgectl.key or AUTH_KEY_FILE). A Wotan
	// must list it in topics.auto_approve for publish to be accepted.
	weightsIdentity = "upc-cartridgectl"
	// maxWeightsPayload bounds an envelope read from Wotan: one full slot of
	// DATA (4 KiB) base64'd twice plus a seal is well under it.
	maxWeightsPayload = 64 << 10
	// weightsMaxPages bounds the walk to the newest message (1000 per page;
	// Wotan keeps 10,000 per topic by default).
	weightsMaxPages = 64
	weightsPage     = 1000
)

var (
	ErrNoData          = errors.New("cartridge has no DATA section")
	ErrModel           = errors.New("model name must match [a-z0-9][a-z0-9_-]{0,62} (one topic segment, one file name)")
	ErrWeightsEnvelope = errors.New("not a upc.cart.weights/v1 envelope")
	ErrWeightsSeal     = errors.New("seal does not verify")
	ErrWeightsModel    = errors.New("sealed manifest is for another model")
	ErrWeightsSHA      = errors.New("DATA does not match the sealed manifest")
	ErrNoWeights       = errors.New("no message on the topic")
)

var modelRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// ValidModel refuses a model name that is not one topic segment and one
// plain file name.
func ValidModel(model string) error {
	if !modelRE.MatchString(model) {
		return fmt.Errorf("%w: %q", ErrModel, model)
	}
	return nil
}

// WeightsTopic is the Wotan topic a model's weights are published on.
func WeightsTopic(model string) string { return weightsTopicPrefix + model }

type weightsManifest struct {
	V           int    `json:"v"`
	Model       string `json:"model"`
	DataSHA256  string `json:"data_sha256"`
	DataWords   int    `json:"data_words"`
	ImageSHA256 string `json:"image_sha256,omitempty"`
	Data        []byte `json:"data"`
}

type weightsEnvelope struct {
	Type     string               `json:"type"`
	Manifest string               `json:"manifest"`
	Seal     *gungnir.GungnirSeal `json:"seal"`
}

// Weights is a verified DATA blob.
type Weights struct {
	Model       string
	SHA256      string // hex, of Data
	ImageSHA256 string // the source image's embedded sha256 (provenance, as sealed)
	KeyID       string
	ExpiresAt   time.Time
	Data        []byte
}

// WeightsDataSection decodes raw as a .cart image (every check
// cartridge.DecodeImage makes, embedded sha256 included) and returns its
// DATA section as the little-endian bytes it was built from, plus the
// image's embedded sha256.
func WeightsDataSection(raw []byte) ([]byte, [32]byte, error) {
	img, err := cartridge.DecodeImage(raw)
	if err != nil {
		return nil, [32]byte{}, err
	}
	if len(img.Data) == 0 {
		return nil, img.SHA256, ErrNoData
	}
	data := make([]byte, 0, 4*len(img.Data))
	for _, w := range img.Data {
		data = binary.LittleEndian.AppendUint32(data, w)
	}
	return data, img.SHA256, nil
}

// SealWeightsFromImage builds the sealed envelope for raw's DATA section.
func SealWeightsFromImage(raw []byte, model string, s *gungnir.Signer, ttl time.Duration) ([]byte, error) {
	if err := ValidModel(model); err != nil {
		return nil, err
	}
	data, imgSHA, err := WeightsDataSection(raw)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	m := weightsManifest{
		V: weightsVersion, Model: model, DataSHA256: hex.EncodeToString(sum[:]),
		DataWords: len(data) / 4, ImageSHA256: hex.EncodeToString(imgSHA[:]), Data: data,
	}
	mb, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return sealManifest(mb, s, ttl)
}

// sealManifest seals manifest bytes as they are and wraps them.
func sealManifest(manifest []byte, s *gungnir.Signer, ttl time.Duration) ([]byte, error) {
	seal, err := s.SignFor(manifest, ttl)
	if err != nil {
		return nil, err
	}
	return json.Marshal(weightsEnvelope{Type: weightsType, Manifest: base64.StdEncoding.EncodeToString(manifest), Seal: seal})
}

func strictUnmarshal(b []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data")
	}
	return nil
}

// OpenWeights verifies a sealed envelope against pub and returns its DATA
// only if every check passes (see the file comment for the order).
func OpenWeights(payload []byte, model string, pub *mldsa65.PublicKey) (*Weights, error) {
	if pub == nil {
		return nil, ErrNoKey
	}
	if len(payload) > maxWeightsPayload {
		return nil, fmt.Errorf("%w: %d bytes, more than %d", ErrWeightsEnvelope, len(payload), maxWeightsPayload)
	}
	var env weightsEnvelope
	if err := strictUnmarshal(payload, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWeightsEnvelope, err)
	}
	if env.Type != weightsType || env.Seal == nil || env.Manifest == "" {
		return nil, fmt.Errorf("%w: type %q", ErrWeightsEnvelope, env.Type)
	}
	manifest, err := base64.StdEncoding.DecodeString(env.Manifest)
	if err != nil {
		return nil, fmt.Errorf("%w: manifest: %v", ErrWeightsEnvelope, err)
	}
	if err := gungnir.Verify(manifest, env.Seal, pub); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWeightsSeal, err)
	}
	// Signed by the pinned key from here on; still checked, not trusted.
	var m weightsManifest
	if err := strictUnmarshal(manifest, &m); err != nil {
		return nil, fmt.Errorf("%w: manifest: %v", ErrWeightsEnvelope, err)
	}
	if m.V != weightsVersion {
		return nil, fmt.Errorf("%w: manifest version %d", ErrWeightsEnvelope, m.V)
	}
	if m.Model != model {
		return nil, fmt.Errorf("%w: sealed for %q, fetching %q", ErrWeightsModel, m.Model, model)
	}
	if len(m.Data) == 0 || len(m.Data)%4 != 0 || len(m.Data)/4 > cartridge.SlotWords || len(m.Data)/4 != m.DataWords {
		return nil, fmt.Errorf("%w: %d bytes for data_words %d (max %d words)", ErrWeightsSHA, len(m.Data), m.DataWords, cartridge.SlotWords)
	}
	sum := sha256.Sum256(m.Data)
	if got := hex.EncodeToString(sum[:]); got != m.DataSHA256 {
		return nil, fmt.Errorf("%w: data sha256 %s, manifest says %s", ErrWeightsSHA, got, m.DataSHA256)
	}
	return &Weights{
		Model: m.Model, SHA256: m.DataSHA256, ImageSHA256: m.ImageSHA256, KeyID: env.Seal.KeyId,
		ExpiresAt: time.Unix(env.Seal.ExpiresAtUnix, 0).UTC(), Data: m.Data,
	}, nil
}

// latestMessage walks topic to its newest message.
func latestMessage(ctx context.Context, c *wotanClient.Client, topic string) (*wotanClient.Message, error) {
	var newest *wotanClient.Message
	var after int64
	for page := 0; page < weightsMaxPages; page++ {
		msgs, err := c.GetMessages(ctx, topic, after, weightsPage)
		if err != nil {
			return nil, fmt.Errorf("get %s: %w", topic, err)
		}
		if len(msgs) == 0 {
			if newest == nil {
				return nil, fmt.Errorf("%w %s", ErrNoWeights, topic)
			}
			return newest, nil
		}
		for _, m := range msgs {
			if m == nil || m.Seq <= after {
				return nil, fmt.Errorf("get %s: seq %d not after %d", topic, seqOf(m), after)
			}
			after = m.Seq
			if !m.Deleted {
				newest = m
			}
		}
	}
	return nil, fmt.Errorf("get %s: more than %d pages", topic, weightsMaxPages)
}

func seqOf(m *wotanClient.Message) int64 {
	if m == nil {
		return -1
	}
	return m.Seq
}

// writeWeights writes data to dir/<model>.data.bin, 0600, by rename.
func writeWeights(dir, model string, data []byte) (string, error) {
	if err := ValidModel(model); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "."+model+".data.bin.*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // a no-op after the rename
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	out := filepath.Join(dir, model+".data.bin")
	if err := os.Rename(tmp, out); err != nil {
		return "", err
	}
	return out, nil
}

const weightsUsage = `usage:
  upc-cartridgectl weights publish --wotan URL --cart FILE.cart --sign-key SEED [--model NAME] [--ttl D]
  upc-cartridgectl weights fetch   --wotan URL --model NAME --pubkey PUB --out DIR`

// cmdWeights runs `weights publish|fetch`. defaultPub is the global
// --pubkey, used by fetch when its own --pubkey is not given.
func cmdWeights(ctx context.Context, args []string, defaultPub *mldsa65.PublicKey, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(weightsUsage)
	}
	switch args[0] {
	case "publish":
		return weightsPublish(ctx, args[1:], out)
	case "fetch":
		return weightsFetch(ctx, args[1:], defaultPub, out)
	default:
		return fmt.Errorf("unknown weights command %q\n%s", args[0], weightsUsage)
	}
}

func weightsFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("weights "+name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func usageErr(err error, missing string) error {
	if err == nil {
		err = errors.New(missing)
	}
	return fmt.Errorf("%w\n%s", err, weightsUsage)
}

func weightsPublish(ctx context.Context, args []string, out io.Writer) error {
	fs := weightsFlags("publish")
	wotan := fs.String("wotan", "", "Wotan HTTP base URL (http://host:port)")
	cartPath := fs.String("cart", "", ".cart image whose DATA section is published")
	key := fs.String("sign-key", "", "ML-DSA-65 private key seed (0600)")
	model := fs.String("model", "", "model name (default: the .cart file name without .cart)")
	ttl := fs.Duration("ttl", 7*24*time.Hour, "seal lifetime (<= 31 days)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *wotan == "" || *cartPath == "" || *key == "" {
		return usageErr(err, "publish needs --wotan, --cart and --sign-key")
	}
	if *model == "" {
		*model = strings.TrimSuffix(filepath.Base(*cartPath), ".cart")
	}
	if err := ValidModel(*model); err != nil {
		return err
	}
	priv, err := gungnir.LoadSeedFile(*key)
	if err != nil {
		return err
	}
	s, err := gungnir.NewSignerWithID(priv)
	if err != nil {
		return err
	}
	raw, err := readFileCapped(*cartPath)
	if err != nil {
		return err
	}
	payload, err := SealWeightsFromImage(raw, *model, s, *ttl)
	if err != nil {
		return err
	}
	c, err := wotanClient.NewClient(*wotan)
	if err != nil {
		return err
	}
	defer c.Close()
	topic := WeightsTopic(*model)
	sub, err := c.Subscribe(ctx, topic, weightsIdentity)
	if err != nil {
		return fmt.Errorf("join %s: %w", topic, err)
	}
	if sub.Status != "approved" {
		return fmt.Errorf("join %s: subscription %s: Wotan must list %q in topics.auto_approve (with AUTH_ENABLED=true: as a service identity, key from unheaded-keygen %s, and in AUTH_ALLOW_CALLERS)", topic, sub.Status, weightsIdentity, weightsIdentity)
	}
	if err := c.Publish(ctx, topic, payload); err != nil {
		return fmt.Errorf("publish %s (not delivered): %w", topic, err)
	}
	data, _, _ := WeightsDataSection(raw) // cannot fail: SealWeightsFromImage decoded the same bytes
	sum := sha256.Sum256(data)
	keyID := "-"
	if pub, ok := priv.Public().(*mldsa65.PublicKey); ok {
		keyID = gungnir.KeyID(pub)
	}
	fmt.Fprintf(out, "published %s: model=%s data_sha256=%s words=%d key=%s (seal ml-dsa-65, %d-byte envelope)\n",
		topic, *model, hex.EncodeToString(sum[:]), len(data)/4, keyID, len(payload))
	return nil
}

func weightsFetch(ctx context.Context, args []string, defaultPub *mldsa65.PublicKey, out io.Writer) error {
	fs := weightsFlags("fetch")
	wotan := fs.String("wotan", "", "Wotan HTTP base URL (http://host:port)")
	model := fs.String("model", "", "model name (topic upc.cart.weights.<model>)")
	pubPath := fs.String("pubkey", "", "pinned ML-DSA-65 public key the seal must verify against")
	dir := fs.String("out", "", "directory for <model>.data.bin (made 0700 if missing)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *wotan == "" || *model == "" || *dir == "" {
		return usageErr(err, "fetch needs --wotan, --model and --out")
	}
	if err := ValidModel(*model); err != nil {
		return err
	}
	pub := defaultPub
	if *pubPath != "" {
		p, err := gungnir.LoadPublicKeyFile(*pubPath)
		if err != nil {
			return err
		}
		pub = p
	}
	if pub == nil {
		return ErrNoKey
	}
	c, err := wotanClient.NewClient(*wotan)
	if err != nil {
		return err
	}
	defer c.Close()
	topic := WeightsTopic(*model)
	msg, err := latestMessage(ctx, c, topic)
	if err != nil {
		return err
	}
	w, err := OpenWeights([]byte(msg.Payload), *model, pub)
	if err != nil {
		return fmt.Errorf("%s seq %d: %w", topic, msg.Seq, err)
	}
	path, err := writeWeights(*dir, *model, w.Data)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "fetched %s seq %d: seal verified (ml-dsa-65, key %s, expires %s), data_sha256=%s, wrote %s (%d bytes)\n",
		topic, msg.Seq, w.KeyID, w.ExpiresAt.Format(time.RFC3339), w.SHA256, path, len(w.Data))
	return nil
}
