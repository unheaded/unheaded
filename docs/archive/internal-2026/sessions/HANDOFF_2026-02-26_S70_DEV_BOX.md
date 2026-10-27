# S70 Dev Box Handoff

**Date**: 2026-02-26
**For**: Claude Code agent on dev machine (has Go 1.24, NixOS, bare metal access)
**Repo**: ~/tmp/unheaded (or wherever you have it — pull from main)
**HEAD**: 1fcc5fd — confirm with `git log --oneline -3`
**Session chain**: S66 firewall → S67-S69 observability/Suricata/routing → **S70 (this)**

---

## MANDATORY: Pull First

```bash
cd ~/tmp/unheaded
git pull origin main
git log --oneline -6
# Expected HEAD: 1fcc5fd chore(license): purge BSL from all live files
```

---

## Architecture Reference (memorize these)

```
host-a  Forge    10.20.255.1  fd00:dead:beef::1  AS65001  FRR + OPNsense
host-b  Outpost  10.20.255.2  fd00:dead:beef::2  AS65002  BIRD + IPFire
WireGuard: fd00:dead:beef::/48  MTU=1380
Doom Range ports: 16666-26666
Go module: unheaded
CRITICAL: IPv6 HbH HOPOPT nexthdr 0x00 MUST NEVER be stripped at any layer
```

---

## PART A — Bare Metal Validation (S70 live)

These require the dev machine. Do them first. They gate everything downstream.

### A1. Go build and test

```bash
cd ~/tmp/unheaded

# Full build
go build ./...

# Core packages with race detector
go test ./pkg/metrics/... -race -count=1 -v
go test ./pkg/anamnesis/... -race -count=1 -v
go test ./cmd/routing-health/... -race -count=1 -v

# Full suite
go test ./... -race -count=1 -timeout 120s 2>&1 | tail -20
```

Any failures: fix before proceeding. New packages from S67-S69 have not been
compiled on real Go toolchain yet.

### A2. Nix flake lock

```bash
cd ~/tmp/unheaded/nixos
nix flake update
git add flake.lock
git commit -m "chore(nix): generate real flake.lock from nix flake update"
```

### A3. NixOS syntax check

```bash
cd ~/tmp/unheaded/nixos
# Parse all modules
for f in modules/*.nix modules/services/*.nix tests/*.nix; do
  nix-instantiate --parse "$f" > /dev/null 2>&1 && echo "OK: $f" || echo "FAIL: $f"
done
```

Fix any FAIL before proceeding to nixos-rebuild.

### A4. NixOS rebuild test (host-a)

```bash
# Test only — does not activate
sudo nixos-rebuild test --flake ~/tmp/unheaded/nixos#host-a 2>&1 | tail -30
```

If host-a is not the current machine, use dry-run:

```bash
nixos-rebuild dry-run --flake ~/tmp/unheaded/nixos#host-a 2>&1 | tail -30
```

### A5. Observability stack smoke test

```bash
cd ~/tmp/unheaded

# Start monitoring stack (no root required)
docker compose -f monitoring/docker-compose.yml up -d

# Wait for health
sleep 15
docker compose -f monitoring/docker-compose.yml ps

# Verify endpoints
curl -s http://localhost:9090/-/healthy     # Prometheus
curl -s http://localhost:3000/api/health    # Grafana
curl -s http://localhost:3100/ready         # Loki
curl -s http://localhost:8428/health        # VictoriaMetrics
```

### A6. Routing health smoke test

```bash
cd ~/tmp/unheaded
go build ./cmd/routing-health/
./routing-health &
sleep 2
curl -s http://localhost:8080/health | python3 -m json.tool
curl -s http://localhost:8080/ready
curl -s http://localhost:8080/metrics | grep routing_health
kill %1
```

### A7. YAML lint (monitoring configs)

```bash
cd ~/tmp/unheaded
for f in monitoring/**/*.yml monitoring/**/*.yaml routing/**/*.conf nixos/modules/*.nix; do
  [ -f "$f" ] || continue
  case "$f" in
    *.yml|*.yaml) python3 -c "import yaml,sys; yaml.safe_load(open('$f'))" 2>&1 && echo "OK: $f" || echo "FAIL: $f" ;;
    *.nix) nix-instantiate --parse "$f" > /dev/null 2>&1 && echo "OK: $f" || echo "FAIL: $f" ;;
  esac
done
```

### A8. Suricata smoke test (host-b only)

```bash
# Non-destructive — checks build capability only, does NOT start daemon
bash scripts/suricata/smoke-test.sh
```

### A9. Routing selector dry-run

```bash
# Validate only — does NOT switch active config
bash -n scripts/routing/select-routing.sh
# Dry-run each option (reads frr configs):
for opt in bgp-evpn ospf isis mpls; do
  echo "=== $opt ===" && vtysh --dryrun -e "$(cat routing/${opt}/frr-${opt}.conf 2>/dev/null | head -5)" 2>&1 | head -3 || true
done
```

---

## PART B — Code Fixes (no bare metal required)

Do these after Part A. All have known locations from security review TODO.md.

Priority order: P0 → P1 → P2.

### B1. [P0] Verify Captain /tmp fix (TODO.md #14)

```bash
grep -rn "os.TempDir\|/tmp\|ioutil.TempDir\|os.MkdirTemp" services/captain/ cmd/captain/ 2>/dev/null
```

If any unguarded `/tmp` writes remain, fix to use configurable data dir
(env var `CAPTAIN_DATA_DIR`, default `/var/lib/unheaded/captain`).

### B2. [P0] SBOM in CI (TODO.md #13)

`.github/workflows/ci.yml` exists but has no SBOM generation. Add a job:

```yaml
  sbom:
    name: SBOM Generation
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: anchore/sbom-action@v0
        with:
          format: spdx-json
          output-file: sbom.spdx.json
      - uses: actions/upload-artifact@v4
        with:
          name: sbom
          path: sbom.spdx.json
```

Also add `go-licenses` check:

```yaml
      - name: License check
        run: |
          go install github.com/google/go-licenses@latest
          go-licenses check ./... --allowed_licenses=MIT,Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC
```

### B3. [P1] getOrCreateGRPCClient race fix (TODO.md #25)

File: `pkg/wotan-client/client.go:615`

Current pattern has RLock→RUnlock→Lock window. Replace with `sync.Once`:

```go
// Replace grpcClient *GRPCClient field + mu sync.RWMutex pattern with:
type Client struct {
    // ...existing fields...
    grpcOnce   sync.Once
    grpcClient *GRPCClient
    grpcErr    error
}

func (c *Client) getOrCreateGRPCClient() (*GRPCClient, error) {
    c.grpcOnce.Do(func() {
        c.grpcClient, c.grpcErr = NewGRPCClient(c.grpcAddr)
    })
    return c.grpcClient, c.grpcErr
}
```

Note: if the address needs to change after construction, keep the mutex but fix
the window by checking again inside the write lock (current code already does
this — the TODO.md concern was about fragility, not a live bug). `sync.Once`
is the cleaner fix if address is immutable post-construction.

Write test: `TestGetOrCreateGRPCClient_ConcurrentInit` — 50 goroutines calling
simultaneously, assert exactly one NewGRPCClient call and no races.

```bash
go test ./pkg/wotan-client/... -race -count=1 -run TestGetOrCreate
```

### B4. [P1] Wotan nil → degraded mode (TODO.md #19)

Search for patterns where `wotan = nil` silently drops messages:

```bash
grep -rn "wotan.*nil\|wotanClient.*nil\|wotanInstance.*nil" --include="*.go" . | grep -v "_test\|.git"
```

For each call site, replace silent drop with degraded-mode log:

```go
// Before (silent):
if w != nil {
    w.Publish(topic, msg)
}

// After (degraded mode):
if w == nil {
    log.Warn().Str("topic", topic).Msg("wotan unavailable — message dropped (degraded mode)")
    metrics.IncrCounter("wotan_dropped_messages_total", 1)
    return
}
w.Publish(topic, msg)
```

### B5. [P1] gRPC TLS skeleton (TODO.md #26)

File: `pkg/wotan-client/client.go` (and any other gRPC dial sites)

```bash
grep -rn "grpc.Dial\|grpc.NewClient\|grpc.WithInsecure\|credentials.NewTLS" --include="*.go" . | grep -v ".git\|_test"
```

Add TLS option behind env flag for now (don't break mock mode):

```go
// In NewGRPCClient or wherever grpc.Dial is called:
var dialOpts []grpc.DialOption
if os.Getenv("UNHEADED_GRPC_TLS") == "1" {
    tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}
    dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
} else {
    dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
}
```

Document in `docs/network/GRPC_TLS.md`: env var, cert paths, mTLS roadmap.

### B6. [P2] Go module version (TODO.md #28)

```bash
head -5 go.mod
# If go 1.21 or lower:
go mod edit -go=1.24
go mod tidy
git add go.mod go.sum
git commit -m "chore(go): update go directive to 1.24"
```

Current go.mod already says `go 1.24.0` — verify and skip if correct.

### B7. [P2] Remove dead BroadcastJSON

From TODO.md #37: dead method in websocket server. Locate and remove:

```bash
grep -rn "BroadcastJSON" --include="*.go" . | grep -v ".git\|_test"
```

If found: delete the method and any callers. Run tests. Commit.
If not found: already cleaned. Skip.

---

## PART C — Commit Cadence

Commit after each lettered section (A1-A9, B1-B7) completes cleanly.

Format:
```
fix/chore(s70): [what was done]

Co-Authored-By: Claude Code <noreply@anthropic.com>
```

---

## PART D — After All Above Pass

Only if A1-A9 all pass and bare metal is stable:

1. Exchange WireGuard keys between host-a and host-b
2. Run `scripts/firewall/firewall-health-check.sh` — all PASS expected
3. Monad HbH end-to-end validation:
   ```bash
   # Requires Scapy on host with access to both hosts
   python3 -c "
   from scapy.all import *
   from scapy.layers.inet6 import *
   p = IPv6(dst='fd00:dead:beef::2')/IPv6ExtHdrHopByHop(options=[
     HBHOptUnknown(otype=0x3E, optdata=b'\x00'*18)
   ])/UDP(dport=16666)/b'monad-hbh-test'
   r = sr1(p, timeout=2, iface='wg0')
   print('HbH PASS' if r else 'HbH DROP — CRITICAL')
   "
   ```
4. Collect H1-H8 verdicts → `docs/research/VERDICTS.md`

---

## Known Pitfalls

- `nixos/flake.lock` is a stub — MUST run `nix flake update` before nixos-rebuild
- `go test ./...` will skip eBPF tests that need a real kernel (expected)
- `pkg/metrics/lxd.go` and `docker.go` gracefully degrade if sockets missing — do not fail tests
- `cmd/routing-health` reads `/sys/class/net/wg0/operstate` — "unknown" is valid (wg0 is up)
- `pkg/anamnesis/suricata.go` tests use testdata/eve-sample.json fixture — 2 alerts expected
- Any agent writing docs MUST use RFC 2119 language (MUST/SHALL/SHOULD/MAY/NOT) in specs
- License is MIT. Any agent that writes BSL/BUSL anywhere gets reverted immediately
- No marketing copy, taglines, or poetic quotes in README or docs without muck approval

---

## Files Added in S67-S69 (not yet compiled on dev box)

```
pkg/metrics/collector.go + baremetal.go + lxd.go + docker.go + nixos.go + tests
pkg/anamnesis/suricata.go + suricata_test.go + testdata/eve-sample.json
cmd/routing-health/main.go + main_test.go
nixos/flake.nix (flake.lock is stub)
nixos/modules/suricata.nix + frr-ospf.nix + frr-mpls.nix + observability.nix
nixos/tests/ (5 test files)
monitoring/ (full Prometheus/Loki/VictoriaMetrics/Grafana/Alertmanager stack)
routing/ospf/ + routing/isis/ + routing/mpls/ + routing/suricata/rules/
scripts/routing/select-routing.sh + scripts/suricata/smoke-test.sh
docker/suricata/ + docker/routing/ospf/
lxd/containers/suricata.yaml + frr-ospf.yaml
docs/legal/SURICATA_GPL_ISOLATION.md
docs/network/ALTERNATE_ROUTING_OPTIONS.md
```

---

## Definition of Done for S70

- [ ] `go build ./...` clean
- [ ] `go test ./... -race` all pass
- [ ] `nix flake update` run, real flake.lock committed
- [ ] NixOS module parse: 0 failures
- [ ] Observability stack up: Prometheus + Grafana + Loki + VictoriaMetrics healthy
- [ ] routing-health /health returns JSON with all fields
- [ ] SBOM CI job added to .github/workflows/ci.yml
- [ ] getOrCreateGRPCClient race fixed + concurrent test passes
- [ ] Wotan nil paths emit degraded-mode log instead of silent drop
- [ ] gRPC TLS skeleton behind UNHEADED_GRPC_TLS=1 env flag
