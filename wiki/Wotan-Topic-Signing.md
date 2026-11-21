# Wotan Topic Signing — ML-DSA-65 on `config.*` and `drift.*`

ADR-043 hard condition #2: all messages on `config.*` topics MUST be
ML-DSA-65 signed before [[Mímir's Law|Mimirs-Law]] spike begins.
Implemented as a prerequisite sprint on `main` before the spike branch.

> **Status**: SHIPPED 2026-04-11 (commit `5e0d0400`); **fixed 2026-10-19**: until then the signature was checked against the public key the publisher sent in the same request, so anyone could sign with a key of their own and pass, and the HTTP publish path took `config.*` without any signature. Now Wotan verifies only against the keys in `configs/wotan.yaml` → `topics.signing_keys` (base64 ML-DSA-65 public keys; empty = every `config.*` publish refused), and both HTTP write paths refuse `config.*`.
>
> **2026-10-23 (ADR-105, S-QUANTUM-ARMOR Phase 12)**: `drift.*` is signed too, each signed prefix by its own keys (`topics.signed_prefixes`), and Wotan now delivers the signature with every signed event so subscribers can re-verify it. Before, Wotan checked `config.*` signatures and then dropped them: no subscriber ever received one. The canonical form moved to the importable `pkg/wotansig` unchanged (golden vector).

## What Got Wired

| File | Change |
|---|---|
| `services/wotan/proto/topic.proto` | Added `signature`, `public_key`, `algorithm` fields to `TopicPublishRequest` and `TopicEvent` |
| `services/wotan/internal/signing/signing.go` | NEW — `TopicSigner` + `TopicVerifier` wrapping `pkg/crypto/pqc/ml_dsa.go` |
| `services/wotan/internal/signing/signing_test.go` | NEW — tests covering round-trip, tamper rejection, algorithm rejection, sender rejection, requirement matching, untrusted keys, replay |
| `services/wotan/internal/grpc/topic_service.go` | `PublishTopic()` now enforces ML-DSA-65 signature on any topic matching `config.*` |
| `services/wotan/proto/topic.pb.go` | Regenerated proto bindings |
| `pkg/wotansig/` | 2026-10-23: canonical form, `TopicSigner`, stateless `VerifyEvent` for subscribers (importable; `internal/signing` delegates) |
| `services/wotan/internal/signing/policy.go` | 2026-10-23: per-prefix trusted keys, longest prefix wins |
| `TopicEvent.signed_at_unix_ms` (field 12) | 2026-10-23: with `signature`, `public_key`, `algorithm` (9-11), now populated on every signed event |

## Canonical Form

Sign the SHA-256 of `"wotan/topic-sig/v2" ‖ 0 ‖ topic ‖ 0 ‖ sender_id ‖ 0 ‖ signed_at (int64 big-endian, Unix ms) ‖ payload` (v2, 2026-10-19; v1 had no time).
Deterministic, includes sender to prevent identity spoofing.

The sender is the publish request's `sender_id`, or, for a publisher that
sent none, the identity Wotan verified from its EdDSA service token (ADR-103
§6, S-IRON-GATE Phase 7: `topics.auto_approve` lists verified identities, a
publish under another identity's id is refused, and since FND-043
`topics.permissions` bounds which topics each identity may publish and read). With Wotan's auth off, the
legacy `display_name` is used instead (`topics.legacy_display_names`). A signed
event's `sender_id` is that same string.

## Topic Policy

Built-in signed prefixes: `config.` and `drift.`. More can be added in
`topics.signed_prefixes`. Each prefix trusts only its own keys:

```yaml
topics:
  signing_keys: []            # config.*
  signed_prefixes:
    - prefix: "drift."        # Heimdall drift alerts, drift.detected.<node_id>
      keys: []                # base64 ML-DSA-65 public keys; empty = closed
```

A `config.*` key verifies nothing under `drift.` and a drift key verifies
nothing under `config.` (ADR-043 #4 key separation). The longest matching
prefix decides. Wotan refuses to start on a broken entry (no trailing dot,
wildcard, duplicate, a `config.` prefix, a key that is not a base64 ML-DSA-65
public key, an unknown field, or the too-broad `alerts.`). HTTP refuses every
signed prefix. A prefix also covers its stem: `alerts.critical.` signs the
topic `alerts.critical` itself.

### Signed families (S-IRON-GATE Phase 9, M-053)

| Prefix | Producer | Key |
|---|---|---|
| `drift.` | `heimdall-daemon` | `--drift-sign-key`, one per node |
| `audit.` | none yet | `AUDIT_SIGNING_KEY_FILE` |
| `alerts.critical.` | alert-mailer (each critical Alertmanager notification, firing and resolved, and a snapshot every 5 min; every service on the bus subscribes through `pkg/critalert`) | `ALERTS_SIGNING_KEY_FILE` |
| `system.health.consensus.`, `system.health.remediate.` | Akira | `HEALTH_SIGNING_KEY_FILE` |

Enrolling the `alerts.critical.` key (ADR-147 Phase 8) is configuration on the
host: two key files for alert-mailer, their public halves on Wotan's side, one
line of `monitoring/.env`. The steps are in
[Enrolling alert-mailer on `alerts.critical`](#enrolling-alert-mailer-on-alertscritical)
below.

### A host's copy of the topic config

A deployment's public keys live on the host, not in the repository (ruled
2026-11-17, plan 2 Step 317 option B). The host runs a copy of
`configs/wotan.yaml` that differs in the `keys:` lines only, and
`scripts/wotan-host-config.py` writes it:

| File | What | Default on west |
|---|---|---|
| `--keys` | the enrolled public keys, one `<prefix> <base64 key>` a line; public, may be world-readable | `/etc/unheaded/wotan/signing-keys.conf` |
| `--out` | the copy Wotan's container mounts at `/etc/wotan/wotan.yaml` | `/etc/unheaded/wotan/wotan.yaml` |
| `--source` | the repository's file | `configs/wotan.yaml` |

```bash
sudo scripts/wotan-host-config.py --keys /etc/unheaded/wotan/signing-keys.conf --out /etc/unheaded/wotan/wotan.yaml
scripts/wotan-host-config.py --keys /etc/unheaded/wotan/signing-keys.conf --out /etc/unheaded/wotan/wotan.yaml --check   # exit 4 on drift
```

Before restarting Wotan on a new copy, load it with Wotan's own parser (a
bad key is fatal at start):

```bash
WOTAN_HOST_CONFIG=/etc/unheaded/wotan/wotan.yaml go test ./services/wotan/internal/api -run TestHostWotanConfig -v
```

Run it after enrolling or rotating a key, and after any change to
`configs/wotan.yaml` (a new identity in `auto_approve` or `permissions` does
not reach the host until the copy is written again). Then restart Wotan: it
refuses a change of signing keys on SIGHUP, because a reload would reset
replay protection. The copy is written in place, since the container mounts
that one file. The compose mount is in the host's own
`docker-compose.override.yml`:
`- /etc/unheaded/wotan/wotan.yaml:/etc/wotan/wotan.yaml:ro` under `wotan:`.

`configs/wotan.yaml` lists each with `keys: []`: enrolled per deployment, never
in the checked-in copy. Producers never send these unsigned: every unsigned
publish path in `pkg/wotan-client` refuses a topic under
`wotansig.SignedTopicPrefixes` with `ErrSignatureRequired`, and
`Client.PublishWithSigner` signs when a key is set. Without
`HEALTH_SIGNING_KEY_FILE`, Akira publishes its reports but not consensus or
remediation (`akira_signed_publish_refused_total`); the consensus is still on
`GET /api/v1/status`. `system.health.reports` (every service votes there) and
the rest of `alerts.*` and `logs.*` stay unsigned. Keys come from
`unheaded-keygen -algo mldsa65 -owner <account> <name>`, which prints the
base64 public key to enrol.

### Enrolling alert-mailer on `alerts.critical`

Off until a host does this (ADR-147 Phase 8). Nothing in the repository
changes: two key files, the two public halves on Wotan's side, one line of
`monitoring/.env`.

alert-mailer runs in the monitoring stack (`monitoring/docker-compose.yml`), a
different compose project from Wotan's, so it reaches Wotan at the host address
Wotan's ports are published on for that network (`UNHEADED_OVERLAY_BIND` in the
main stack; `[fd00:dead:beef::1]` on west): 18000 for the join over HTTPS,
18001 for the signed publish over gRPC. Both are TLS 1.3. Wotan's certificate
does not carry that address, so the mailer verifies it against the name
`wotan`, which every Wotan certificate carries (`WOTAN_TLS_SERVER_NAME`,
[[Transport Cascade|Transport-Cascade]]). The chain must still end at the CA.

**What goes where.** `<secrets>` is the host directory `ALERT_MAILER_SECRETS_DIR`
names, the one that holds `smtp-password` (`/etc/unheaded/alert-mailer` on
west: 0700, uid 16710, the uid the container runs as). The container reads it
read-only at `/run/secrets/alert-mailer`; the file names are fixed.

| File on the host | What | Its public half |
|---|---|---|
| `<secrets>/alert-mailer.key` | the Ed25519 service key of identity `alert-mailer`, 0400, uid 16710. Without it Wotan answers 401 | `alert-mailer.pub` in Wotan's trust directory `<trust>`. Compose mounts the repository's `configs/trust.d/dev` at `/etc/unheaded/trust.d`; a Wotan host unit reads `/etc/unheaded/trust.d`. Wotan re-reads the directory every 30 s: no restart |
| `<secrets>/alerts-critical.key` | the ML-DSA-65 seed, 0400, uid 16710. Without it every message is refused and counted, never sent unsigned | one line `alerts.critical. <base64>` in the host's `signing-keys.conf` (previous section). Wotan takes a signing key only at start |
| `configs/certs/dev/ca.pem` | the CA that signed Wotan's certificate (public). It is the file the stack's Prometheus already mounts; for another CA set `ALERT_MAILER_WOTAN_CA_FILE` | |

Wotan's side of the identity is in the repository: `alert-mailer` is in
`AUTH_ALLOW_CALLERS` (compose and `deploy/systemd/unheaded-wotan.service`), in
`topics.auto_approve`, and in `topics.permissions` with
`publish: ["alerts.critical"]` (`configs/wotan.yaml`;
`TestRepoWotanConfig_AlertMailerPublishesAlertsCritical` holds it there). A
host's copy of the topic config must be written from that file.

**Steps**, on the host:

```bash
# 1. The service key. The public half lands in <trust>; Wotan sees it within 30 s.
sudo unheaded-keygen -dir <secrets> -trust-dir <trust> -owner 16710 alert-mailer

# 2. The signing key. This prints its base64 public key
#    (again later: unheaded-keygen -algo mldsa65 -dir <secrets> -print-kid alerts-critical).
sudo unheaded-keygen -algo mldsa65 -dir <secrets> -owner 16710 alerts-critical
#    A key already made in another directory is moved there, not copied, and
#    keeps owner 16710 and mode 0400:
#      sudo mv /etc/unheaded/keys/alerts-critical.key <secrets>/alerts-critical.key

# 3. Enrol the public key (previous section): add "alerts.critical. <base64>"
#    to signing-keys.conf, write the host's copy, check it, then restart Wotan.
sudo scripts/wotan-host-config.py --keys /etc/unheaded/wotan/signing-keys.conf --out /etc/unheaded/wotan/wotan.yaml
WOTAN_HOST_CONFIG=/etc/unheaded/wotan/wotan.yaml go test ./services/wotan/internal/api -run TestHostWotanConfig -v

# 4. monitoring/.env: the one line that switches it on (the host address, https).
#      ALERT_MAILER_WOTAN_ADDR=https://[fd00:dead:beef::1]:18000

# 5. Recreate the mailer, from an image built from a tree that has this page.
sudo docker compose -f monitoring/docker-compose.yml up -d --build alert-mailer
```

The other `monitoring/.env` variables have defaults that fit the compose stack
(TLS on, the dev CA, server name `wotan`, gRPC on the same host at 18001); the
table is in [[Alerting]]. Order: Wotan first, then the mailer. The other order
heals by itself: the mailer tries the join again at each publish until it
succeeds, sends a snapshot every 5 minutes and re-probes gRPC every 30 s;
`alert_mailer_critical_publish_errors_total` rises until then. Mail is never affected: the bus is told after the mail
decision and a failed publish is only counted.

**It works when**, within a minute of the mailer's start:

- its log has `alerts.critical: publishing signed, as alert-mailer` with
  `"service_key":true`, then `alerts.critical: first message accepted by Wotan`
  (`sudo docker logs unheaded-alert-mailer 2>&1 | grep alerts.critical`);
- `curl -s 127.0.0.1:19130/metrics | grep alert_mailer_critical` shows
  `alert_mailer_critical_published_total` at 1 or more and one higher every
  5 minutes, with `alert_mailer_critical_publish_refused_total`,
  `alert_mailer_critical_publish_errors_total` and
  `alert_mailer_critical_rebuild_errors_total` flat;
- every service on the bus reports `unheaded_critical_alerts_state{state="ok"} 1`
  on its `/metrics` (or `firing` while a critical alert fires) where it read
  `unknown` before, and `critical_alerts` in `/health` says the same;
- Wotan has the topic: `rooms` in its `/health` went up by one, and
  `GET /api/v1/topics` (with a service token) lists `alerts.critical` with a
  rising `message_count`.

**When it does not**, the mailer's log says which piece is missing:

| Log line or symptom | Missing |
|---|---|
| `alerts.critical: not published` with `ALERTS_SIGNING_KEY_FILE` in the error at start, `..._publish_refused_total` rising | `<secrets>/alerts-critical.key` is absent, not readable by uid 16710, or readable by group or others |
| `no service key at "/run/secrets/alert-mailer/alert-mailer.key"` at start and `"service_key":false`, then publish errors (401) | `<secrets>/alert-mailer.key` |
| publish errors (401 or 403) with `"service_key":true` | `alert-mailer.pub` is not in the trust directory Wotan reads, or `alert-mailer` is not in that Wotan's `AUTH_ALLOW_CALLERS` |
| `x509: certificate is valid for ..., not ...` | the certificate does not carry the name in `ALERT_MAILER_WOTAN_TLS_SERVER_NAME` (default `wotan`) |
| `x509: certificate signed by unknown authority` | `ALERT_MAILER_WOTAN_CA_FILE` is not the CA that signed Wotan's certificate |
| `alerts.critical: not published` with `UNHEADED_TLS_CA_CERT` in the error | the CA file is absent on the host (docker then makes a directory at that path: remove it, put the file there, recreate the container) |
| `service token withheld on a plaintext link` | `ALERT_MAILER_WOTAN_TLS` is set to something other than `true` |
| publish errors with `PermissionDenied` | the public key is not under `alerts.critical.` in the config Wotan runs, or Wotan was not restarted after it was enrolled |
| publish errors with a refused or timed-out connection | Wotan's 18000 and 18001 are not published on that address, or the host firewall does not let the monitoring network reach them |

To rotate the signing key: make a new one under another name
(`... -algo mldsa65 ... alerts-critical-next`), enrol its public key beside the
old one and restart Wotan, rename it over `<secrets>/alerts-critical.key`,
recreate the mailer, then take the old public key out of `signing-keys.conf`.
The service key: `unheaded-keygen -dir <secrets> -trust-dir <trust> -owner 16710 -rotate alert-mailer`,
then recreate the mailer. A development stack gets the service key from
`sudo scripts/gen-dev-keys.sh` (in `configs/keys/dev/alert-mailer/`, which is
then the `<secrets>` directory); the script prints the remaining steps.

## Subscriber verification

Every signed `TopicEvent` carries `signature`, `public_key`, `algorithm` and
`signed_at_unix_ms`. A subscriber that trusts a key re-verifies with nothing
else, so it does not have to trust Wotan to relay faithfully:

```go
sg := wotansig.Signed{Signature: ev.Signature, PublicKey: ev.PublicKey,
    Algorithm: ev.Algorithm, SignedAtUnixMs: ev.SignedAtUnixMs}
err := wotansig.VerifyEvent(ev.Topic, ev.SenderId, ev.Payload, sg, trustedDriftKeys)
```

Through `pkg/wotan-client`: `msg.VerifySignature(trusted)`. Publishers use
`PublishSigned(ctx, topic, payload, signer)` (gRPC only). `VerifyEvent` has no
clock and no replay cache: freshness and replay are Wotan's checks at publish
time.

## Replication keeps signatures (FND-020)

A signed message is persisted with its signature (`store.Message.Signed`), so
the WAL holds it. When a primary streams its WAL to a standby (`CatchUp`),
each `WALEntry` carries `signature`, `public_key`, `algorithm`,
`signed_at_unix_ms` and `sender_id` (fields 7-11, additive). The standby runs
`cluster.VerifyEntry` before applying anything, against its own signing
policy (the same `topics.signing_keys` / `signed_prefixes` it enforces on
publish):

- unsigned topic: applied as is;
- signed topic, no signature: rejected, `reason="unsigned"` (an old primary
  does not get to decide that a topic is unsigned);
- untrusted key, wrong algorithm, bad signature: `reason="signature"`;
- fields that differ from the signature stored in the entry's data:
  `reason="mismatch"`; data that is not a stored message: `reason="decode"`.

Rejections are counted in `unheaded_wotan_replication_rejected_total{reason}`,
logged with topic and sequence, and skipped. The standby checks key and
signature only (`TopicVerifier.VerifyStored`): no clock window and no replay
cache, because a stored message is old by design and the primary checked
freshness when it accepted it. Cluster mode itself still refuses to start
(ADR-064), so this is proven in-process (`internal/cluster/signature_test.go`)
and not yet live. The Postgres store keeps no signature (no column).

## Heimdall drift events

`heimdall-daemon --wotan-grpc ADDR --drift-sign-key SEED` publishes each drift
alert to `drift.detected.<node_id>` (node ID normalized to `[a-z0-9-]`) as
display name `heimdall`, signed with a key that must differ from the manifest
keys (`--sign-key`, `--pubkey`). Body: `node_id, path, kind, expected_sha256,
actual_sha256, detected_at, manifest_name, manifest_version`. Without
`--wotan-grpc` it prints alerts only. The unit is
`deploy/systemd/unheaded-heimdall.service` (account `unheaded-heimdall`, uid
16736, CAP_DAC_READ_SEARCH only) and the enrolment steps are
`runbooks/security/heimdall-enrol-drift-key.yaml` (draft). Running it on west
and east, and where each node's drift key is made, is S-IRON-GATE E2 for
Stevie.

## Trusted keys

`config.*` is accepted only over gRPC, signed by a key listed in `topics.signing_keys`. The `public_key` in the request is used only to pick which trusted key to check; a key not on the list is `ErrUntrustedKey` (PermissionDenied). Replays: the request carries `signed_at_unix_ms`, which the signature covers. Wotan refuses it more than 5 minutes from its own clock and refuses a signature it has already accepted within that window (a restart forgets them, so a message signed in the last 5 minutes could be replayed once after one).

## Algorithm Lock

Hard condition: `algorithm == "ml-dsa-65"` ONLY. No algorithm agility.
Blocks downgrade attacks. Testing covers explicit rejection of
`hmac-sha256` substitution.

## Reuse

Sits on top of existing `pkg/crypto/pqc/ml_dsa.go` (`github.com/cloudflare/circl`
v1.6.5 in `go.mod`; not vendored). Zero new external dependencies.

`configs/wotan.yaml` ships `signing_keys: []` and every signed prefix with
`keys: []`, so out of the box every publish under them is refused.

---

> **Source:** [services/wotan/internal/signing/](../services/wotan/internal/signing/) · [pkg/wotansig/](../pkg/wotansig/) · [cmd/heimdall-daemon/drift.go](../cmd/heimdall-daemon/drift.go) · [pkg/crypto/pqc/ml_dsa.go](../pkg/crypto/pqc/ml_dsa.go) · [ADR-043](../docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md)
