// SPDX-License-Identifier: GPL-3.0-or-later
// Package signing provides ML-DSA-65 message signing and verification
// for Wotan topic messages. ADR-043 hard condition #2: all config.* topic
// messages MUST be ML-DSA-65 signed before the Mímir's Law spike begins.
package signing

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/crypto/pqc"
)

const Algorithm = "ml-dsa-65"

var (
	ErrUnsignedConfigTopic = errors.New("signing: config.* topics require ML-DSA-65 signature")
	ErrInvalidSignature    = errors.New("signing: signature verification failed")
	ErrWrongAlgorithm      = errors.New("signing: only ml-dsa-65 accepted (no algorithm agility)")
	ErrStale               = errors.New("signing: signed_at is outside the accepted window")
	ErrReplay              = errors.New("signing: this signed message was already accepted (replay)")
)

// MaxSkew is how far a message's signing time may be from Wotan's clock,
// either way. Inside it, a signature is accepted once (TopicVerifier keeps
// what it accepted for that long); outside it, never. A Wotan restart
// forgets what it accepted, so a message signed in the last MaxSkew could
// be replayed once after a restart.
const MaxSkew = 5 * time.Minute

// Signed is a topic message's signature and what a verifier needs with it.
type Signed struct {
	Signature      []byte
	PublicKey      []byte
	Algorithm      string
	SignedAtUnixMs int64
}

// TopicSigner signs Wotan topic messages with ML-DSA-65.
type TopicSigner struct {
	signer *pqc.MLDSASigner
	pubKey []byte
}

// NewTopicSigner creates a signer from an ML-DSA-65 key pair.
func NewTopicSigner(signer *pqc.MLDSASigner) *TopicSigner {
	return &TopicSigner{
		signer: signer,
		pubKey: signer.PublicKey(),
	}
}

// Sign signs a topic message now.
func (ts *TopicSigner) Sign(topic, senderID string, payload []byte) (Signed, error) {
	return ts.SignAt(topic, senderID, payload, time.Now())
}

// SignAt signs a topic message as of at.
func (ts *TopicSigner) SignAt(topic, senderID string, payload []byte, at time.Time) (Signed, error) {
	ms := at.UnixMilli()
	sig, err := ts.signer.Sign(canonicalize(topic, senderID, ms, payload))
	if err != nil {
		return Signed{}, err
	}
	return Signed{Signature: sig, PublicKey: ts.pubKey, Algorithm: Algorithm, SignedAtUnixMs: ms}, nil
}

// TopicVerifier verifies ML-DSA-65 signatures on Wotan topic messages
// against a fixed set of trusted public keys.
//
// It used to verify against the public key the publisher sent in the same
// request. A signature then proves only that the sender holds *some* key:
// anyone could generate a pair, sign a config.* message and attach their
// own public key. The key in the request must now be one of trusted, the
// keys the operator configured (configs/wotan.yaml topics.signing_keys).
// With none configured, every signed publish is refused.
type TopicVerifier struct {
	verifier *pqc.MLDSAVerifier
	trusted  [][]byte
	now      func() time.Time

	mu   sync.Mutex
	seen map[[sha256.Size]byte]int64 // accepted signature -> forget after (Unix ms)
}

// ErrNoTrustedKeys refuses signed publishes when no signing key is trusted.
var ErrNoTrustedKeys = errors.New("signing: no trusted signing keys configured (topics.signing_keys)")

// ErrUntrustedKey refuses a signature made with a key Wotan does not trust.
var ErrUntrustedKey = errors.New("signing: public key is not a trusted signing key")

// NewTopicVerifier creates a verifier for ML-DSA-65.
func NewTopicVerifier(trusted ...[]byte) (*TopicVerifier, error) {
	v, err := pqc.NewMLDSAVerifier(pqc.MLDSA65)
	if err != nil {
		return nil, err
	}
	keys := make([][]byte, 0, len(trusted))
	for _, k := range trusted {
		keys = append(keys, bytes.Clone(k))
	}
	return &TopicVerifier{verifier: v, trusted: keys, now: time.Now, seen: map[[sha256.Size]byte]int64{}}, nil
}

// Verify checks a topic message signature: algorithm, a trusted key, the
// signing time within MaxSkew of now, the signature over topic, sender,
// signing time and payload, and that this signature has not been accepted
// within the window. It records nothing: call Accept once the message is
// actually taken, so a publish refused for another reason (sender not
// approved yet) can be retried with the same signature.
func (tv *TopicVerifier) Verify(topic, senderID string, payload []byte, sg Signed) error {
	if sg.Algorithm != Algorithm {
		return ErrWrongAlgorithm
	}
	if len(tv.trusted) == 0 {
		return ErrNoTrustedKeys
	}
	if !slices.ContainsFunc(tv.trusted, func(k []byte) bool { return bytes.Equal(k, sg.PublicKey) }) {
		return ErrUntrustedKey
	}
	now := tv.now().UnixMilli()
	skew := MaxSkew.Milliseconds()
	tv.forget(now)
	if sg.SignedAtUnixMs < now-skew || sg.SignedAtUnixMs > now+skew {
		return ErrStale
	}
	ok, err := tv.verifier.Verify(canonicalize(topic, senderID, sg.SignedAtUnixMs, payload), sg.Signature, sg.PublicKey)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidSignature
	}
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if _, dup := tv.seen[sha256.Sum256(sg.Signature)]; dup {
		return ErrReplay
	}
	return nil
}

// Accept records a verified signature as used, atomically: of two copies
// of one message racing past Verify, only the first is accepted. Only
// verified signatures reach it, so the cache is bounded by what trusted
// signers send in one window.
func (tv *TopicVerifier) Accept(sg Signed) error {
	id := sha256.Sum256(sg.Signature)
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if _, dup := tv.seen[id]; dup {
		return ErrReplay
	}
	tv.seen[id] = sg.SignedAtUnixMs + MaxSkew.Milliseconds()
	return nil
}

// forget drops accepted signatures whose window has passed.
func (tv *TopicVerifier) forget(now int64) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	for k, until := range tv.seen {
		if until < now {
			delete(tv.seen, k)
		}
	}
}

// RequiresSignature returns true if the given topic requires ML-DSA-65 signing.
// Per ADR-043: all config.* topics must be signed.
func RequiresSignature(topic string) bool {
	return strings.HasPrefix(topic, "config.")
}

// canonicalize is what a signature covers: SHA-256("wotan/topic-sig/v2" ||
// 0 || topic || 0 || sender_id || 0 || signed_at (8 bytes, big-endian, Unix
// ms) || payload). v1 had no time, so a signed message could be replayed.
func canonicalize(topic, senderID string, signedAtMs int64, payload []byte) []byte {
	h := sha256.New()
	h.Write([]byte("wotan/topic-sig/v2"))
	h.Write([]byte{0})
	h.Write([]byte(topic))
	h.Write([]byte{0})
	h.Write([]byte(senderID))
	h.Write([]byte{0})
	_ = binary.Write(h, binary.BigEndian, signedAtMs)
	h.Write(payload)
	return h.Sum(nil)
}
