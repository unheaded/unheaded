<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->

# Cryptography in the repository: facts for the export notification decision (2026-11-12)

**Status:** read-only audit by an AI agent (Claude, `unheaded-barrister` skill
loaded), 2026-11-12, of branch `develop` at `e5721cf2`. It is not legal advice,
it draws no conclusion about what the regulation requires, and no person has
checked it yet.

Question audited (Stevie, 2026-11-12): whether UPC, Wotan or anything else in
the project is a novel implementation that could be "non-standard
cryptography" for 15 CFR 742.15(b), before the email of
`docs/compliance/export-control-notice.md` (S-E1-LIVE Steps 18-24) is decided.

## Summary

No cipher, hash, signature or key-agreement **algorithm** of the project's own
invention was found in the tree or in its history. Every primitive is a
published one (ML-KEM, ML-DSA, SLH-DSA, Ed25519, X25519, ECDSA P-256, AES-GCM,
ChaCha20-Poly1305 and its XChaCha variant, HKDF, HMAC, SHA-2, SHA-3, BLAKE3,
Argon2id) and nearly all come from libraries (Go standard library,
`golang.org/x/crypto`, `cloudflare/circl`, rustls with aws-lc-rs or ring, the
`pqcrypto` and RustCrypto crates). UPC, MBC and the cartridge system are a
bytecode format and virtual machine with no cryptography of their own; they
use SHA-256 digests and the Gungnir ML-DSA-65 seal. Wotan signs and verifies
with ML-DSA-65 from circl over a project-defined message layout. What the
project does define itself are **constructions and protocols built from those
primitives**. Three of them encrypt data or establish keys: an "age-style"
file encryption format that is not the published age format
(`pkg/secrets/encryption/age.go`), a hybrid ML-KEM-768 + X25519 key
encapsulation with an AES-256-GCM envelope (`crates/zhend/src/crypto/`), and a
KEM tunnel key schedule that belongs to the PQC authentication Internet-Draft
(`pkg/crypto/pqc/kem_tunnel.go`, `services/sophia/hkdf_derive.go`). The rest
are signature and integrity formats. Two published algorithms are implemented
by hand in the tree instead of taken from a library: SHA-256
(`crates/upc-api/src/surface_hash.rs`) and HKDF-SHA-256
(`services/sophia/hkdf_derive.go`). The project's Internet-Drafts are
individual submissions: published, but not adopted or approved by any
standards body. Several of the project-defined constructions are described
nowhere except this public repository. Whether any of that meets the
regulation's definition is for a person to decide from the regulation's text.

## How rows are classified

The bucket column uses these codes. The rule for telling P from D is this
audit's own, chosen so the table is consistent; it is not a rule from the
regulation.

- **S**: standard algorithm from a standard library. The project calls the
  library and defines nothing around it beyond configuration.
- **P**: standard algorithms in a project-defined protocol that is published
  as an Internet-Draft or in this public repository. Used where the project
  defines only which bytes are signed or checked and how they are framed.
- **D**: algorithm or construction defined here. Used where the project
  defines how keys are derived or combined or how data is encrypted, or where
  it implements a primitive itself. Each D row says which of those it is.
- **N**: not cryptography. Listed because it was asked about or its name
  suggests otherwise.

"Callers" means non-test code in this repository that reaches the
construction. "Repo only" means the construction is described in this
repository (code, ADR or wiki) and in no Internet-Draft.

## Classification table

| # | What it is (purpose) | Where | Primitives and their standard | Implemented by | Composition or protocol the project's own? | Published where | Bucket |
|---|---|---|---|---|---|---|---|
| 1 | MBC bytecode, cartridge VM, eBPF interpreter (compute; no cryptography) | `crates/monad-mbc/src/`, `ebpf/cartridge-ebpf/src/main.rs`, `ebpf/monad-common/src/cart.rs`, `pkg/cartridge/` | none | n/a | The ISA and image format are the project's | `draft-bellis-unheaded-mbc-isa` (-00 on datatracker, expired; -01 in `ietf-submission/`); ADR-108 | N |
| 2 | Cartridge state hash (replay comparison) | `ebpf/monad-common/src/cart.rs:955-979` | FNV-1a 32-bit (published, non-cryptographic) | written in tree | no | ADR-108 | N |
| 3 | Cartridge image digest (integrity of a `.cart` file) | `crates/upc-cartridge/src/lib.rs:156`, `:247` | SHA-256 (FIPS 180-4) | `sha2` 0.10.9 | Header layout is the project's | Repo only | P |
| 4 | Cartridge seal and sealed weights (who may load a cartridge) | `cmd/upc-cartridgectl/prepare.go:57-74`, `main_linux.go:209-232`, `weights.go:149-192` | ML-DSA-65 (FIPS 204), SHA-256 | circl v1.6.5 through `pkg/gungnir` | Uses row 6 unchanged | Repo only | P |
| 5 | UPC syscall-surface id and RV2MBC table digest (loader gate) | `crates/upc-api/src/surface_hash.rs:21-101`, `crates/upc-api/src/rv2mbc_sha.rs:26` | SHA-256 (FIPS 180-4) with a domain prefix | **written in tree** (no hashing crate in the eBPF workspace) | The algorithm is the standard's; the implementation is the project's | Repo only (ADR-080) | D (published algorithm, implementation written here; a hash, not a cipher) |
| 6 | Gungnir seal v2 (signature with expiry over a payload digest) | `pkg/gungnir/gungnir.go:77`, `:83-95`, `:113`, `:141` | ML-DSA-65 with FIPS 204 context string `unheaded/gungnir/seal/v2`; SHA-256 | circl v1.6.5; Go stdlib | Signed-message layout (length-prefixed fields) is the project's | Repo only (ADR-043) | P |
| 7 | Gungnir evidence seal (purpose-bound signature) | `pkg/gungnir/evidence.go:43`, `:77`, `:122`, `:154` | ML-DSA-65 with context `unheaded/gungnir/evidence/v1`; SHA-256 | circl v1.6.5 | Message layout and purpose field are the project's | Repo only | P |
| 8 | Sealed cask Binding Rune (signed `sha256sum` manifest of a tarball) | `pkg/sealedcask/sealedcask.go:9-14`, `:403-457`; `cmd/cask-verify/main.go`; `scripts/build-sealed-cask.sh:131-134` | SHA-256; ML-DSA-65 (row 7, purpose `sealed-cask-v1`) | coreutils `sha256sum`; circl | Archive layout is the project's | Repo only (ADR-010) | P |
| 9 | Runbook manifest seal | `pkg/champion/runbookseal.go:49-58`, `:141`; `pkg/champion/runbookmanifest.go:191`; `scripts/runbook-manifest.sh` | SHA-256; ML-DSA-65 (row 7, purpose `runbook-manifest`) | coreutils; circl | no, beyond row 7 | Repo only | P |
| 10 | Gjallarhorn trigger packet | `pkg/gjallarhorn/types.go:6-12`, `:69-71` | none; the packet is unauthenticated by design | n/a | Packet layout is the project's | Repo only (ADR-043) | N |
| 11 | Wotan topic signature (who published a message) | `pkg/wotansig/wotansig.go:67-81`, `:110-124`; `pkg/crypto/pqc/ml_dsa.go:47`, `:125` | SHA-256 over a domain-separated tuple (`wotan/topic-sig/v2`), then ML-DSA-65 with empty context | Go stdlib; circl v1.6.5 | Canonical form is the project's | Repo only (the string `topic-sig` is in no draft) | P |
| 12 | Wotan replay window, clock-skew check, stored-message check on replication | `services/wotan/internal/signing/signing.go:103-143`; `services/wotan/internal/cluster/signature.go:117` | SHA-256 of the signature as a replay-cache key; row 11 | Go stdlib | Policy is the project's | Repo only | P |
| 13 | Sophia dictionary delta signature | `pkg/protocol/sophiasync/signing.go:34-36`, `:117`, `:170`, `:199` | SHA-256 over `sophia/delta-sig/v1` tuple, then ML-DSA-65 | Go stdlib; circl | Canonical form is the project's. No first-party service calls it (`signing.go:57-58`) | The sophia-dictionary draft requires signed deltas; the canonical form is repo only | P |
| 14 | Wotan WAL record checksum | `pkg/storage/wal/segment.go:498`, `wal.go:464` | CRC-32 (IEEE 802.3), error detection | Go `hash/crc32` | no | n/a | N |
| 15 | Monad register checksum | `pkg/protocol/encoding/encoding.go:211`; `ebpf/monad-common/src/lib.rs:782` | CRC-16/CCITT-FALSE (polynomial 0x1021), error detection | written in tree | no | protocol-foundation draft; the PQC draft section 18.6 says it is not cryptographic | N |
| 16 | Exponent encoding, varint, TLV | `pkg/protocol/encoding/encoding.go:64-310` | none (number encoding) | written in tree | yes | protocol-foundation draft | N |
| 17 | Inverse Mask | `cmd/protocol-api/monad.go:277-279`; `docs/protocol/INVERSE_MASK.md` | none (bitwise NOT of an IPv6 address, an addressing idea) | written in tree | yes | Repo only | N |
| 18 | Monad `ENCRYPT` flag | `pkg/monad/flags.go:11`; `ebpf/monad-common/src/lib.rs:158-159` | none; marks a payload carried over TLS | n/a | n/a | protocol-foundation draft | N |
| 19 | hop-ebpf "HMAC" and retry checks | `ebpf/hop-ebpf/src/main.rs:130-146`, `:597-606`; `docs/adr/ADR-125-hop-checks.md:20` | none computed: only presence and expiry of a map entry are read; the wire format has no tag | n/a | n/a | ADR-125 | N |
| 20 | Monad header tag and per-flow secret (reference package) | `pkg/protocol/integrity/hmac.go:32-66`, `:70-78` | HMAC-SHA-256 (RFC 2104, FIPS 198-1) truncated to 8 bytes; flow secret = HMAC-SHA-256(node secret, flow label, source, destination). The comment says HKDF; the code is one HMAC call | Go stdlib | The derivation and the truncated tag are the project's. **No callers** | Repo only; protocol-foundation leaves the HMAC field undefined (ADR-125) | D (key derivation defined here; authentication only) |
| 21 | Flow migration and Shield retry tokens (reference package) | `pkg/protocol/migration/migration.go:24-45`, `:61-102` | HMAC-SHA-256 truncated to 16 bytes | Go stdlib | Token layout is the project's. **No callers**; `ValidateToken` does not compare the tag | Repo only | P |
| 22 | PQC packet authentication, "signature by reference" (12-byte SigRef, KeyRef, HashPfx, SeqNum; compliance tiers; 2-of-3 multi-signature) | `ebpf/pqc-common/src/lib.rs`; `services/shield/pqc_verify.go:119-209`; `pkg/crypto/pqc/hash_pfx.go:14-19`, `security.go:29-39`; `pkg/sophia/pqc_maps.go`, `pqc_control.go`; `services/keymgmt/` | ML-DSA (FIPS 204), SLH-DSA (FIPS 205), SHA-256 prefix and binding digests | circl v1.6.5; Go stdlib | The protocol is the project's. Per the draft's own status paragraph (`ietf-submission/draft-bellis-unheaded-pqc-authentication-01.md:254-262`) no wire-level verification exists; `shield-ebpf/src/main.rs:754-758`. `NewPQCVerifier` has **no callers** | `draft-bellis-unheaded-pqc-authentication` (-00 on datatracker 2026-03-19, expired; -01 not uploaded) | P (authentication) |
| 23 | KEM tunnel key schedule ("Shield-to-Shield tunnel keys") | `pkg/crypto/pqc/kem_tunnel.go:46-88`, `:138-187`, `:234-243`; `services/shield/kem_tunnel.go` (state only) | ML-KEM-512/768/1024 (FIPS 203); HKDF-SHA-256 (RFC 5869) with labels `unheaded-pqc-signing`, `unheaded-pqc-tunnel-<context>` | circl v1.6.5; `golang.org/x/crypto/hkdf` | Key schedule and labels are the project's. Derives signing, authentication and IV keys; **nothing encrypts with them**. Callers: benchmarks in `tests/pqc/` only | PQC draft section 16 names ML-KEM and HKDF-SHA256; the labels are repo only | D (key establishment and derivation defined here) |
| 24 | Sophia tunnel key and nonce derivation | `services/sophia/hkdf_derive.go:15-58`, `:62-81` | HKDF-SHA-256 (RFC 5869) | **written in tree** on Go `crypto/hmac` | Labels `unheaded-pqc-tunnel-v1`, `unheaded-pqc-nonce-v1` are the project's. **No callers** outside its test | Repo only | D (published algorithm, implementation written here; derivation labels defined here) |
| 25 | PQC algorithm wrappers and registry | `pkg/crypto/pqc/ml_dsa.go`, `ml_kem.go`, `slh_dsa.go:101-102`, `registry.go` | ML-DSA-44/65/87, ML-KEM-512/768/1024, SLH-DSA (FIPS 203, 204, 205) | circl v1.6.5 | no | n/a | S |
| 26 | HQC (and, in history, FN-DSA) | `pkg/crypto/pqc/hqc.go` | none: a stub that returns "not available" | n/a | n/a | n/a | N |
| 27 | TLS 1.3 hybrid key exchange profiles | `pkg/tls/pq.go:25-27`, `:41-63` | TLS 1.3 (RFC 8446); groups X25519MLKEM768, SecP256r1MLKEM768, SecP384r1MLKEM1024 (IETF TLS working group document `draft-ietf-tls-ecdhe-mlkem`; its current status was not checked) | Go `crypto/tls`, toolchain go1.27.3 | no: the project only sets `CurvePreferences`. No hybrid combiner or handshake of the project's own exists in `pkg/tls` | n/a | S |
| 28 | zhend QUIC transport | `crates/zhend/src/api/quic.rs:411-417` | QUIC with TLS 1.3 (RFC 9000, 9001); X25519MLKEM768 first | quinn 0.11.12, rustls 0.23.46, aws-lc-rs 1.18.2 | no | n/a | S |
| 29 | **zhend hybrid KEM** (key establishment) | `crates/zhend/src/crypto/kem.rs:104-142`, `:145-176`, `:179-204` | ML-KEM-768 (FIPS 203), X25519 (RFC 7748), HKDF-SHA-256 (RFC 5869): key = HKDF(salt `zhen-hybrid-kem-v1`, IKM = ML-KEM secret then X25519 secret, info `fragment-encryption`) | `pqcrypto-mlkem` 0.1.1 (PQClean bindings), `x25519-dalek` 2.0.1, `hkdf` 0.12.4, `sha2` 0.10.9 | **The combiner and the 1120-byte ciphertext layout are the project's.** It is not the TLS hybrid group and not X-Wing. Callers: row 30 only | Repo only (ADR-021) | D (key combination defined here) |
| 30 | **zhend sealed fragment envelope** (encryption) | `crates/zhend/src/crypto/envelope.rs:59-83`, `:91-123` | AES-256-GCM (FIPS 197, NIST SP 800-38D) keyed by row 29, random 96-bit nonce | `aes-gcm` 0.10.3 | Envelope format is the project's. Compiled by default (`pq` feature). **No callers** outside the module's tests | Repo only (ADR-021) | D (encryption format defined here) |
| 31 | zhend peer identity and gossip signatures | `crates/zhend/src/crypto/sign.rs:45-80`, `:95-125`; `crates/zhend/src/qi/gossip.rs` | ML-DSA-65 (FIPS 204) over `zhen-peer-identity-v1:` message | `pqcrypto-mldsa` 0.1.2 | Message layout is the project's | Repo only | P |
| 32 | zhend fragment id | `crates/zhend/Cargo.toml:14` | BLAKE3 (published by its authors; no standards-body document known to this audit) | `blake3` 1.8.7 | no | n/a | S (see "must read": BLAKE3 is published but its standards status needs checking) |
| 33 | **"Age-style" recipient encryption** (encryption) | `pkg/secrets/encryption/age.go:180-264`, `:267-359` | X25519 (RFC 7748); HKDF (RFC 5869) over **SHA3-256** (FIPS 202); ChaCha20-Poly1305 (RFC 8439) with an all-zero nonce to wrap a 16-byte file key; XChaCha20-Poly1305 (`draft-irtf-cfrg-xchacha`, an expired research-group draft, not an RFC) for the payload with the header as associated data | `golang.org/x/crypto` v0.58.1-0.20261009181718-2f1f834330b2; Go stdlib `crypto/sha3` | **The construction is the project's.** It writes the label `age-encryption.org/v1` but differs from the published age v1 format (see below). **No callers**: nothing constructs an `AgeEncryptor` outside tests | Repo only | D (encryption format defined here) |
| 34 | Password and symmetric "simple" encryptor | `pkg/secrets/encryption/age.go:381-394`, `:397-432` | XChaCha20-Poly1305; key from a password by HKDF-SHA3-256 with a fixed default salt | `golang.org/x/crypto` | Password derivation is the project's (HKDF, not a password hash). **No callers** | Repo only | D (key derivation defined here) |
| 35 | Secret envelope encryption (DEK wrapped by KEK) | `pkg/secrets/encryption/envelope.go:92-144`, `:147-218`, `:426-433` | XChaCha20-Poly1305 for data and for the wrapped key; KEK from a master key by HKDF-SHA3-256 (info `kek-derivation`) | `golang.org/x/crypto` | JSON envelope and KEK derivation are the project's. **No callers** (`cmd/unheaded-cli/cmd/secret.go:478-483`: the CLI has no secrets store) | Repo only | D (encryption format and key derivation defined here) |
| 36 | Service tokens and user JWTs | `pkg/auth/jwt_eddsa.go:62-82`, `:129`; `pkg/auth/service_token_eddsa.go`; `pkg/auth/jwt.go:234`, `:281`; `services/gateway/middleware/auth.go:156`, `:193`; `crates/svc-token/src/lib.rs:33`, `:251` | JWS and JWT (RFC 7515, 7519) with EdDSA (RFC 8037, Ed25519 RFC 8032) and HS256 (HMAC-SHA-256) | Go stdlib; `ring` 0.17.14 | Token serialisation is written in tree to the RFCs; claims are the project's | IETF RFCs | S |
| 37 | Key ids, JWKS, key files | `pkg/auth/keys.go:80-92`, `:164-170`; `pkg/gungnir/keys.go:29-30`, `:88`; `cmd/unheaded-keygen/mldsa.go:33-37`, `main.go` | Ed25519 and ML-DSA-65 key generation from random seeds; key id = truncated SHA-256 of the public key | Go stdlib; circl | File layout is the project's. No key derivation beyond the libraries' seed expansion | Repo only (ADR-103) | S |
| 38 | Password storage | `cmd/unheaded-auth/users.go:75-79`, `:137-138` | Argon2id (RFC 9106), PHC string format | `golang.org/x/crypto/argon2` | no | n/a | S |
| 39 | BFF session cookie, API keys, approved tool-call hash | `pkg/bff/session.go:39-40`; `pkg/auth/apikey.go:151`, `:220`; `pkg/champion/confirmcall.go:130`, `:183` | Random value from `crypto/rand`; SHA-256 as lookup key; constant-time compare. The cookie is an opaque random id, not encrypted or signed | Go stdlib | no | n/a | S |
| 40 | Internal CA and certificates | `pkg/tls/ca.go:67`, `:92`, `:210`, `:234`; `cmd/cert-gen/` | X.509 (RFC 5280), ECDSA P-256 (FIPS 186-5) | Go stdlib | no | n/a | S |
| 41 | Audit and custody hash chains | `pkg/audit/audit.go:134`; `cmd/yggdrasil-evidence/custody.go:52-55`, `:71`, `:111` | SHA-256 chaining; custody entries also ML-DSA-65 (row 7) | Go stdlib; circl | Record layout is the project's | Repo only | P |
| 42 | WebSocket handshake | `cmd/dashboard-backend/internal/websocket/server.go:486` | SHA-1 as RFC 6455 fixes it; not a security function | Go stdlib | no | RFC 6455 | S |
| 43 | Scripts that drive system tools | `scripts/wireguard/`, `scripts/apt-repo.sh`, release and key-ceremony scripts | WireGuard, GnuPG, OpenSSL, cryptsetup, cosign, age, sops as installed programs | the system's tools | no cryptography implemented | n/a | S |

## Constructions defined in this repository

Ordered by how directly each one encrypts data or establishes keys. Signature
and integrity formats come after.

### 1. "Age-style" file encryption (`pkg/secrets/encryption/age.go`), table row 33

What it does: for each recipient it makes an ephemeral X25519 key pair,
derives a wrap key with HKDF over SHA3-256 (salt = ephemeral public key then
recipient public key, info `age-encryption.org/v1/X25519`), wraps a random
16-byte file key with ChaCha20-Poly1305 under an all-zero nonce, derives a
payload key with HKDF-SHA3-256 (info `payload`), and encrypts the payload in
one XChaCha20-Poly1305 call with the text header as associated data
(`age.go:186-263`).

Why it is cryptography: it encrypts data for confidentiality with a key
agreement.

Why it is the project's own: the type comment calls it "age-compatible"
(`age.go:32-34`) and it writes age's version line, but by this audit's
knowledge of the public age v1 specification it is a different construction:
age uses HKDF over SHA-256, authenticates the header with an HMAC, encrypts
the payload in 64 KiB chunks (STREAM) under a key salted by a 16-byte nonce,
and encodes keys in Bech32. This code uses SHA3-256, has no header MAC,
encrypts in one shot, and encodes keys as case-folded base64
(`age.go:119-134`, `:149-171`), with a SHA3 hash of the string as fallback
when decoding fails. A person should confirm the comparison against the
specification at age-encryption.org/v1. The real age library
(`filippo.io/age`) was never a dependency (History, search 5).

Every primitive in it is published. The construction as a whole is described
only in this repository. It has no callers outside its tests: nothing in any
binary constructs an `AgeEncryptor`. It has been in the public history since
the commit of 2026-01-31.

The same package holds the envelope format (row 35) and the password-keyed
encryptor (row 34): XChaCha20-Poly1305 with keys derived by HKDF-SHA3-256.
Also project-defined, also without callers.

### 2. zhend hybrid KEM and sealed fragment (`crates/zhend/src/crypto/kem.rs`, `envelope.rs`), rows 29-30

What it does: `encapsulate` runs ML-KEM-768 against the recipient's ML-KEM
key and an ephemeral X25519 exchange against the recipient's X25519 key,
concatenates the two shared secrets, and derives one 32-byte key with
HKDF-SHA-256 under the salt `zhen-hybrid-kem-v1` and info
`fragment-encryption` (`kem.rs:179-204`). The ciphertext is the ML-KEM
ciphertext followed by the ephemeral X25519 public key. `seal` encrypts a
payload with AES-256-GCM under that key (`envelope.rs:59-83`).

Why it is cryptography: key establishment and encryption for
confidentiality.

Why it is the project's own: the way the two secrets are combined, the labels
and the wire layout are defined in this file and ADR-021. It is not the TLS
hybrid group (that is row 28, done by rustls) and it is not a published
combiner such as X-Wing; the HKDF input does not include the ciphertext or
the public keys. The primitives are from crates.

The module is compiled into zhend by default (`crates/zhend/Cargo.toml`,
feature `pq`). `seal` and `unseal` have no callers outside the module's own
tests. From 2026-03-29 to 2026-05-08 the same construction was built on
`pqcrypto-kyber` 0.8 and `pqcrypto-dilithium` 0.5, the pre-standard
algorithms (History, search 6).

### 3. PQC authentication protocol and its KEM tunnel key schedule, rows 22-24

What it is: `draft-bellis-unheaded-pqc-authentication` defines packet
authentication by reference: full ML-DSA or SLH-DSA signatures sit in Sophia
BPF maps and the Monad register carries a 12-byte reference with a 4-byte
SHA-256 prefix of the signature and a sequence number; a 52-byte pseudo-header
is what gets signed (draft section 7); four compliance tiers select
algorithms, up to a 2-of-3 multi-signature. Section 16 adds key establishment
between Shield instances: ML-KEM encapsulation, then per-flow signing keys by
HKDF-SHA256.

In code: the reference structures and checks (row 22) and
`pkg/crypto/pqc/kem_tunnel.go` (row 23), which encapsulates with circl's
ML-KEM and derives a signing key, an authentication key and an IV key with
`x/crypto/hkdf` under project labels. `services/sophia/hkdf_derive.go`
(row 24) is a second, hand-written HKDF-SHA-256 with its own tunnel and nonce
labels.

Why it is cryptography: digital signature verification, and key
establishment and derivation. No code encrypts anything with the derived
keys; the type is described as an "encrypted tunnel"
(`services/shield/kem_tunnel.go:42`) but holds state only.

Why it is the project's own: the protocol is specified by the project. It was
published as an individual Internet-Draft (-00 uploaded 2026-03-19 per
`docs/protocol/RFC-DESK-PASS-2026-11-10.md`, now expired; -01 prepared, not
uploaded). An individual Internet-Draft is published; it is not adopted by a
working group and not approved by any standards body. The draft itself says
the wire-level phases are not implemented, and the callers found are tests
and benchmarks.

### 4. SHA-256 written in the tree (`crates/upc-api/src/surface_hash.rs`), row 5

A complete SHA-256 (round constants at `:21-30`, compression at `:34-98`),
written so the eBPF build script and the loader share one dependency-free
implementation. It is the algorithm of FIPS 180-4, not a variant; a test
(`crates/upc-api/tests/io_gates.rs:471-485`) checks it against the standard
test vectors (empty input, "abc", one million "a").
It is a hash used for identifiers and integrity, not encryption. It is the
only primitive found whose implementation lives in this repository, besides
the HKDF of row 24 (which is built on the standard library's HMAC), the
CRC-16 of row 15 and the FNV-1a of row 2 (neither cryptographic).

### 5. Monad header tag and flow secret (`pkg/protocol/integrity/hmac.go`), row 20

An 8-byte truncation of HMAC-SHA-256 over an 18-byte Monad header, keyed by a
per-flow secret that is itself one HMAC-SHA-256 call over flow label and
addresses. Message authentication, not encryption. Nothing calls it, the wire
format v0x01 has no field to carry the tag, and ADR-125 records that the
eBPF side checks only that a key entry exists.

### 6. Signature formats: Gungnir, Wotan, Sophia, zhend peers, rows 4, 6-9, 11-13, 31, 41

All use ML-DSA-65 as the library provides it, with no change to the
algorithm. What the project defines is the message that gets signed:
Gungnir's length-prefixed field list under a FIPS 204 context string; Wotan's
and Sophia's SHA-256 of a NUL-separated, domain-prefixed tuple; zhend's
prefixed identity message. These authenticate and check integrity; none
encrypts. They are in use (cartridge loading, sealed casks, runbook manifests,
Heimdall drift events, Wotan topics, evidence custody). They are described in
this repository only: the strings `gungnir`, `topic-sig` and `delta-sig`
appear in none of the drafts in `ietf-submission/`.

### What UPC and MBC are

The MBC instruction set, the cartridge interpreter, the Shim and the
RISC-V-to-MBC translator are a virtual CPU and its tooling. A search of
`crates/monad-mbc/src/`, `ebpf/cartridge-ebpf/src/main.rs` and the opcode
list found no cryptographic instruction and no cryptographic routine. The XOR
that appears is the logic opcode, and the xorshift generators
(`crates/upc-cartridge/src/diff.rs:50-65`, `reference.rs:172`, `scorer.rs:69`)
are test-input generators. UPC's contact with cryptography is rows 2-5.

## Source, libraries and object code

**Source.** The repository contains encryption source code in two senses.
It calls libraries for TLS, signatures and AEAD throughout (rows 25-28,
36-40). It also contains source that itself composes encryption and key
establishment from primitives (rows 23, 29, 30, 33-35), and two hand-written
published primitives (rows 5, 24). No cipher primitive (block cipher, stream
cipher, AEAD mode) is implemented in the tree: `crypto/aes` and
`crypto/cipher` are imported by no Go file now or in history, and no cipher
constants were found.

**Third-party cryptographic libraries linked.**

- Go: standard library `crypto/*` at toolchain go1.27.3 (`go.mod`);
  `golang.org/x/crypto` v0.58.1-0.20261009181718-2f1f834330b2 (argon2,
  chacha20poly1305, curve25519, hkdf, sha3); `github.com/cloudflare/circl`
  v1.6.5 (mldsa44/65/87, mlkem512/768/1024, slhdsa).
- Rust, zhend (`crates/zhend/Cargo.lock`): rustls 0.23.46, aws-lc-rs 1.18.2
  (aws-lc-sys 0.46.0), ring 0.17.14, quinn 0.11.12, pqcrypto-mlkem 0.1.1,
  pqcrypto-mldsa 0.1.2, x25519-dalek 2.0.1, aes-gcm 0.10.3, hkdf 0.12.4,
  sha2 0.10.9, blake3 1.8.7.
- Rust, other crates: ring 0.17.14 (`crates/svc-token`); sha2 0.10.9
  (`crates/upc-cartridge`, `cmd/upc-bootctl`); rustls 0.23.46 with ring
  0.17.14 as transitive dependencies of `cmd/ebpf-collector`.
- System programs driven by scripts, not linked: WireGuard, GnuPG, OpenSSL,
  cryptsetup, cosign, age, sops.

**Object code.** Checked on 2026-11-12 without credentials:

- GitHub Releases: the public API lists none for `unheaded/unheaded`. One tag
  exists, `v0.1.0-alpha` (2026-02-23). `.github/workflows/release.yml` would
  attach binaries and debs to a release on any pushed `v*` tag.
- Container images: `.github/workflows/docker.yml` pushes
  `ghcr.io/unheaded/unheaded/wotan` and `.../kanban` on pushes to `main`,
  `develop` and `staging`. Anonymous pulls are denied and the organisation's
  public package list shows 0 packages. Whether private images exist could
  not be determined from outside.
- Debian packages: `wiki/Releases.md` describes an apt repository served from
  west on private addresses only.
- The git tree tracks no compiled executables. It tracks 8 MBC bytecode
  images, which are programs for the virtual CPU and contain no cryptography.

**Other public locations.** The organisation has other public repositories
(`unheaded-doom`, `unheaded-forge`, `unheaded-k8s`, `DOOM`, `doomgeneric`,
`www`). They were not examined. The ROM HMAC-SHA-256 code left this
repository for `unheaded-doom` on 2026-11-05.

**Against the notice as drafted.** Facts a person may want beside
`docs/compliance/export-control-notice.md`: its product list and email text
name TLS, ML-DSA, SLH-DSA, XChaCha20-Poly1305 and the age-style format, and do
not name the zhend hybrid KEM with AES-256-GCM, the KEM tunnel key schedule,
or the hand-written SHA-256. It describes the age-style format as X25519,
HKDF, ChaCha20-Poly1305 without the SHA3-256 and XChaCha differences from
age. The inventory it calls the full list, `compliance/crypto/cbom.cdx.json`,
has no entry for ChaCha20-Poly1305, HKDF, AES-GCM, x25519-dalek, BLAKE3 or
`surface_hash.rs`.

## History

Twelve searches over all refs (4,738 commits), 2026-11-12.

1. `-S'crypto/aes'` in Go files: no commit.
2. `-G'"crypto/(aes|cipher|des|rc4)"'` in Go files: no commit.
3. `-G'x/crypto/(pbkdf2|scrypt|bcrypt|nacl|salsa20|blowfish|twofish|tea|xtea|cast5|chacha20")'`
   in Go files: no commit.
4. `-S'age-encryption.org/v1'` in Go files: added 2026-01-31 (`5ce8dd90`,
   `b843ae56`), removed and restored on 2026-02-25 (`b99884b6`, `dd04bf0e`),
   present since. `pkg/protocol/integrity/hmac.go` has the same history.
5. `-S'filippo.io/age'`: no commit. The age library was never used.
6. `-S'pqcrypto-kyber'`: zhend arrived on 2026-03-29 (`77f19201`) with
   `pqcrypto-kyber` 0.8 and `pqcrypto-dilithium` 0.5 under the same hybrid
   construction; `cea6f91a` on 2026-05-08 moved it to `pqcrypto-mlkem` and
   `pqcrypto-mldsa`. For those six weeks the public history holds the hybrid
   KEM over pre-standard Kyber and signatures with pre-standard Dilithium
   (both published as NIST submissions; neither is the FIPS text).
7. `-S'X25519Kyber768Draft00'` in Go and Rust: one commit, `7af6633a`
   (2026-10-23), the crypto-inventory scanner's list of names it recognises.
   The draft group was never configured.
8. `-G'go-fn-dsa|fn-dsa-go|/falcon"|pqcrypto-falcon|liboqs'` in `go.mod` and
   Cargo manifests: no commit. The libraries discussed in
   `PQC-DEPENDENCY-LICENSING-ASSESSMENT.md` were never dependencies.
9. Files named for FN-DSA: `pkg/crypto/pqc/fn_dsa.go` existed from 2026-03-05
   (`4ae8e1d1`) to 2026-11-04 (`f56e8c24`). Read at its last version: a stub
   whose signer and verifier return "not available". No FN-DSA or Falcon code
   was ever in the tree.
10. Deleted files with cryptographic names: `cmd/pqc-verifier/` (an HTTP
    wrapper around `services/shield` verification, deleted 2026-11-04,
    `91ff726d`); `internal/doom/hmackey.go` (search 11); metrics and
    dashboard files for PQC counters. No cipher, KDF or obfuscation file.
11. `-S'UPC_HMAC_KEY'`: the UPC Doom ROM carried an HMAC-SHA-256 tag printed
    by `cmd/doom` and `wotan-ctl`. Until `b993efbf` (2026-10-28) a default
    key was compiled in; that commit removed the default. The code left for
    `unheaded-doom` on 2026-11-05 (`ebafbc26`). Standard HMAC; a key value
    that was once in public history.
12. `-G` for primitive constants (SHA-256, ChaCha, MD5, Blowfish),
    `XORKeyStream`, and the words obfuscate, scramble, xor-cipher in code
    files: the hits are `surface_hash.rs` (added 2026-10-28, `37b01b8d`), WAF
    rules that detect obfuscated attack strings (`pkg/waf/detection/`,
    deleted 2026-11-04), and a "scrambled control" in forge training tests.
    No hand-written cipher appears in history.

Also read, not searched: `docs/protocol/INVERSE_MASK.md` and
`cmd/protocol-api/monad.go:277-279` (Inverse Mask is address arithmetic, in
the tree today); ADR-125 line 18, which records that the Shield HMAC and
retry checks "were never built". No separate pickaxe was run for an earlier
Shield HMAC.

## What a person must read in the regulation before deciding

This section names text to read. It does not say what the text means.

1. **15 CFR 742.15(b)**, paragraphs (b)(1) and (b)(2), on the eCFR as of the
   day of reading: what is released, the condition attached, to whom the
   notification goes, and when a new one is needed.
2. **15 CFR 772.1, the definition of "non-standard cryptography"**, word for
   word. Points in it that the facts above touch:
   - it speaks of "cryptographic functionality", "algorithms or protocols":
     rows 22-24, 29-30 and 33-35 are project-defined protocols or
     constructions over published algorithms, not new algorithms;
   - "adopted or approved by a duly recognized international standards body":
     an individual Internet-Draft is neither; XChaCha20 comes from a
     research-group draft that did not become an RFC; BLAKE3 has no
     standards-body document known to this audit; the TLS hybrid groups are
     in a working-group document whose status needs checking;
   - "and have not otherwise been published": several constructions are
     described only in this public repository; the PQC protocol was also
     posted to the IETF datatracker. Whether publication by the author in the
     repository itself counts is the question the notice's item 3 left open.
3. **15 CFR 772.1, the definition of "cryptography"** and any note to it on
   what it does and does not include, and the definitions of the terms used
   for source code and object code of encryption software. Read against the
   split in the table between encryption and key establishment (rows 23,
   27-30, 33-35) and signatures, MACs and hashes (the rest).
4. **Supplement No. 1 to 15 CFR part 774, Category 5 Part 2**: ECCN 5D002 and
   the 5A002.a paragraph it refers to, with their notes, in particular any
   text on authentication, digital signature and data-integrity functions.
   This audit recalls that such text exists but did not read it; do not rely
   on that recollection.
5. **15 CFR 734.3(b)(3) and 734.7** on what "published" means, and
   **15 CFR 734.17** if it applies to posting encryption source code and
   object code on the internet.
6. For object code, should a release or public image ever be published:
   whatever 742.15(b) and 15 CFR 740.17 say about object code compiled from
   publicly available source. No public object code was found on 2026-11-12.
7. **86 FR 16482** (2021-03-29), the rule that narrowed the notification, for
   its preamble's explanation of the "non-standard" limit.
8. The notification is tied to an internet location. The other public
   repositories listed above are separate locations and were not audited.

Questions of fact a person may still want answered before deciding: whether
to keep unreferenced constructions (rows 20, 21, 23, 24, 30, 33-35) in the
tree at all, since code with no callers is still published source; and
whether the pre-standard Kyber period in history (search 6) changes anything,
since history is published too.

## Not examined

- `tomb/` (314 tracked files), `wt/`, `.claude/worktrees/`, `node_modules/`,
  `target/`, `llama.cpp/`, `references/`, `dist/`, `bin/`, `sbom/`,
  `sbom-results/`: excluded by instruction or as build output.
- The other public repositories of the organisation, and the archived WAF
  repository (local only, per project notes).
- JavaScript and TypeScript beyond a search for Web Crypto and Node crypto
  calls (none found); Python beyond its imports (`hashlib`, `hmac`,
  `secrets`, `ssl` from the standard library); Nix files; proto definitions.
- `pkg/secrets/store/vault.go`, `pkg/transport/mtls`, `pkg/mesh`: assumed to
  be TLS and HTTP clients from their imports; not read line by line.
- Every caller of `crypto/tls` (52 files) was not read; the audit relied on
  `pkg/tls/pq.go` and the import census to conclude that no handshake is
  defined in the tree.
- The IETF datatracker was not queried. Draft status is taken from
  `docs/protocol/RFC-DESK-PASS-2026-11-10.md`.
- The standards status of `draft-ietf-tls-ecdhe-mlkem`, XChaCha20 and BLAKE3
  was not checked against current sources.
- The comparison with the age v1 format is from the auditor's knowledge of
  that specification, not from reading it during this audit.
- Whether private GHCR images exist, and the date the repository first became
  public.
- The regulation's text. Nothing in this file was checked against the eCFR.
- Nothing was built or run. "No callers" comes from text search for
  constructor and function names, not from a call-graph tool.

## Update 2026-11-18

Added after the audit; nothing above this heading was changed. Stevie ruled on
2026-11-18 (decision pack of that date, row 31): "I'll send it; delete the
unused code." The two constructions of sections 1 and 2 were deleted in
(this commit). Like the audit, this section is the work of an AI agent, is not
legal advice, and draws no conclusion about what the regulation requires.

**Deleted.**

| Row | What | Files | How "no callers" was checked this time |
|---|---|---|---|
| 33 | "Age-style" recipient encryption | `pkg/secrets/encryption/age.go` (the file), and its 14 tests in `pkg/secrets/encryption/encryption_test.go` | A word search of every Go file for each of `AgeEncryptor`, `AgeIdentity`, `AgeRecipient`, `AgeConfig`, `NewAgeEncryptor`, `GenerateAgeIdentity`, `ParseAgeIdentity`, `ParseAgeRecipient` found them only in `age.go` and that test file. (`store.AgeRecipient` in `pkg/secrets/store/file.go` is a different type, a JSON metadata struct with no cryptography; it stays.) `go list` over `./...`, with and without tests and with other build tags: the package is imported by `pkg/secrets/store` and by tests, and no `main` package reaches it. |
| 29 | zhend hybrid KEM | `crates/zhend/src/crypto/kem.rs` with its 5 unit tests | No `use`, path or re-export of `kem`, `HybridKemKeypair`, `encapsulate` or `decapsulate` in `crates/zhend/src`, `tests/`, `benches/` or `build.rs` outside `kem.rs`, `envelope.rs` and the re-export in `crypto/mod.rs`. No other crate depends on zhend. `cargo check --locked` and `cargo test --locked` pass without the files. |
| 30 | zhend sealed fragment envelope | `crates/zhend/src/crypto/envelope.rs` with its 8 unit tests | The same search for `envelope`, `SealedFragment`, `seal`, `unseal`: one doc comment in `pu/fragment.rs`, now reworded. The `sealed_payload` field of `Fragment` is a byte field that nothing fills; it stays because it is part of the stored fragment layout. |

**Not deleted, because code that compiles uses it.**

- `crates/zhend/src/crypto/sign.rs` and `crypto/mod.rs` (row 31, ML-DSA-65
  peer identity): used by `qi/gossip.rs`, `qi/peer.rs`, `qi/message.rs` and
  `tests/phase4_pq_auth.rs`. The directory `crates/zhend/src/crypto/` now
  holds only these two files.
- The `Encryptor` interface that was declared in `age.go`: it is the type of
  `FileStore`'s encryptor in `pkg/secrets/store/file.go`.
- `SimpleEncryptor` (row 34): the tests of `pkg/secrets` and
  `pkg/secrets/store` build it as their fixture. It, the interface and the
  SHA3-256 adapter moved unchanged to `pkg/secrets/encryption/encryptor.go`.
  Row 34's location is now `encryptor.go:48-98`; it still has no caller
  outside tests.
- Row 35 (`envelope.go`) was not part of the ruling and is untouched.

**Dependencies.** Go: none left; `go mod tidy -diff` is empty, because
`golang.org/x/crypto` still provides `chacha20poly1305`, `hkdf`, `argon2` and
`sha3`. Its `curve25519` package is no longer imported by any Go file. Rust:
`pqcrypto-mlkem`, `x25519-dalek`, `aes-gcm`, `hkdf` and `sha2` left
`crates/zhend/Cargo.toml`, and 27 packages left `crates/zhend/Cargo.lock`
with none added or changed. Across all lock files 18 crate names are gone
from the build (`docs/policy/dependency-baseline.txt`, 536 to 518 packages)
and the Rust component count fell from 352 to 334
(`docs/policy/sbom-count-baseline.txt`). The list under "Third-party
cryptographic libraries linked" above therefore no longer holds for zhend:
of the crates it names, zhend's lock file keeps rustls, aws-lc-rs, ring,
quinn, pqcrypto-mldsa and blake3.

**Crypto inventory.** `compliance/crypto/cbom.cdx.json` was regenerated: 116
components before, 110 after. Gone: the four import entries of `age.go`
(hash, AEAD, key exchange, KDF), zhend's `kem-import` entry and its four
`rust-crypto-dep` entries. Added: three import entries for `encryptor.go`
(hash, AEAD, KDF). One entry for `cmd/alert-mailer/smtp.go` moved two lines;
that drift predates this change.

**What the table above looks like now.** Rows 29, 30 and 33 describe code
that is no longer in the tree at the commit named here; it remains in the
public history (History, searches 4 and 6). Row 24 (`services/sophia/`) had
already left the tree with the Sophia service. Of the rows the last section
listed as unreferenced, 20, 21, 23, 34 and 35 are still in the tree.

**The notice.** `docs/compliance/export-control-notice.md` was updated the
same day: its list no longer names an age-style format, and it now names the
ML-KEM key schedule of row 23 and the SHA-256 of row 5, which the paragraph
"Against the notice as drafted" said were missing. The notification is still
not sent; sending it is Stevie's.
