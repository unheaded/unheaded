# Build — Mímir

Three Go binaries, all from in-tree source. Reproducible from a clean clone.

## Requirements

- Go 1.21+
- `make` (for sealed-cask path; not strictly needed for plain build)
- For UPC trigger path (`gjallarhorn-sender`): a Linux host with `CAP_NET_RAW`

## Standalone binaries

```bash
# from repo root
go build -o bin/heimdall-daemon       ./cmd/heimdall-daemon/
go build -o bin/gjallarhorn-sender    ./cmd/gjallarhorn-sender/
go build -o bin/gjallarhorn-listener  ./cmd/gjallarhorn-listener/

# verify
./bin/heimdall-daemon -help
./bin/gjallarhorn-sender -help
./bin/gjallarhorn-listener -help
```

These three binaries plus the Ansible role at `tomb/ansible/roles/heimdall/`
constitute the Mímir distribution. No additional source extraction needed.

## Sealed Cask (signed deterministic artifact, per ADR-010)

```bash
./scripts/build-sealed-cask.sh \
    --name mimir \
    --version "$(git rev-parse --short HEAD)" \
    --include "bin/heimdall-daemon" \
    --include "bin/gjallarhorn-sender" \
    --include "bin/gjallarhorn-listener" \
    --include "tomb/ansible/roles/heimdall/" \
    --include "configs/heimdall.yaml" \
    --include "runbooks/security/drift-response.yaml"

# Verify the binding rune (SHA256 chain) on the produced artifact:
./scripts/verify-binding-rune.sh dist/mimir-*.cask
```

The cask is the deterministic, signed artifact community adopters install.
Source-based build (above) is for development; cask build is for release.

## Smoke after build

```bash
# Boot the daemon in dry-run mode (does NOT touch the filesystem)
./bin/heimdall-daemon -dry-run -baseline /tmp/mimir-test-baseline

# In another terminal: trigger a manual scan via UPC packet (Linux + CAP_NET_RAW)
./bin/gjallarhorn-sender -target ::1 -reason "smoke test"

# Or via UDP trigger (no CAP_NET_RAW needed — for non-UPC environments)
./bin/gjallarhorn-listener -port 18888 &
echo '{"reason":"smoke","at":"now"}' | nc -u -q1 localhost 18888
```

A clean smoke produces:
- daemon log entries: `scan_started`, `scan_complete`, `0 drift events`
- no exit, no error
- if drift had been detected: `drift_detected` event published to Wotan
  (or stdout if `--no-wotan`)

## Cross-compile (for adopter platforms)

```bash
# Linux/amd64 (typical adopter host)
GOOS=linux GOARCH=amd64 go build -o dist/mimir/heimdall-daemon-linux-amd64 \
    ./cmd/heimdall-daemon/

# Linux/arm64 (Pi cluster, Graviton)
GOOS=linux GOARCH=arm64 go build -o dist/mimir/heimdall-daemon-linux-arm64 \
    ./cmd/heimdall-daemon/

# macOS (operator workstation)
GOOS=darwin GOARCH=arm64 go build -o dist/mimir/heimdall-daemon-darwin-arm64 \
    ./cmd/heimdall-daemon/
```

## What the build does NOT include

- `wotan` itself — Mímir publishes to Wotan if available; if not, it logs to
  stdout. Wotan is its own tool (see `cmd/tools/anamnesis-lite/` for the
  Wotan-shipping bundle, or `helm/unheaded/` for the cluster deployment).
- The PostgreSQL schema for `zhen_actions` audit (Mímir's drift events can
  be stored in PG via that pipeline; that's an integration the adopter
  enables, not a hard dependency).
- Provisioning scripts for the host being watched — Mímir watches; it
  doesn't deploy. Use the existing `tomb/ansible/` content for that.

## Verification this BUILD.md is current

```bash
# All three commands MUST succeed:
go build -o /tmp/mimir-build-test ./cmd/heimdall-daemon/      && rm /tmp/mimir-build-test
go build -o /tmp/mimir-build-test ./cmd/gjallarhorn-sender/   && rm /tmp/mimir-build-test
go build -o /tmp/mimir-build-test ./cmd/gjallarhorn-listener/ && rm /tmp/mimir-build-test

# Result: zero output, exit 0. If any fail, this BUILD.md is stale relative
# to the source tree — file an issue.
```
