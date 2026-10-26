# Steel Echo: the game-day workload

`cmd/steel-echo` is one static Go binary with four jobs: a tiny HTTP service to deploy and break, a client-side availability meter, and a Monad frame sender and counter for the packet-chaos lab. Stdlib plus `pkg/httputil`, `pkg/metrics` and `pkg/monadpkt`; no new module. S-REAL-STEEL Phase 6 (D-14, D-15) and Phase 13 (D-25).

## `serve`

| | |
|---|---|
| `GET /health` | 200 `ok`, always (so a bad build is bad at `/`, not at its probe) |
| `GET /version` | the `-X main.version` build value |
| `GET /` | hostname and version only; never echoes path, query, headers or body (zero user data access); anything else 404 with a fixed body; POST 405 |
| `GET /metrics` | the house `unheaded_http_requests_total` / `unheaded_http_request_duration_seconds` (`pkg/httputil` middleware), which Sword's canary analysis reads |
| `--addr` | default `:8080` |
| `STEEL_ERROR_RATE` | 0..1, fraction of `/` answered 500 (out of range is refused, not clamped) |
| `STEEL_LATENCY_MS` | added delay on `/` only |

SIGTERM, SIGINT and SIGPWR shut it down cleanly (LXD stop reaches it through OpenRC).

## `load` (the meter)

```
steel-echo load --target http://127.0.0.1:26601/ --rps 20 --duration 5m --out bg.csv
steel-echo load --discover http://127.0.0.1:19006 --svc steel-echo --rps 20 --out gameday.csv
```

| Flag | |
|---|---|
| `--target URL` | fixed target (the stable proxy endpoint) |
| `--discover CUIRASS --svc NAME [--port 8080] [--refresh 2s]` | every refresh period, `GET /api/v1/state/actual` (no key needed) and round-robin over the healthy, running, managed instances labelled `svc=NAME`; traffic share = replica share. Loopback and garbage addresses refused; a failed refresh keeps the last targets; a tick with no healthy target counts as `errconn` |
| `--rps` (1..5000), `--duration` (≤ 24 h), `--timeout` (2 s) | |
| `--out CSV` | one line per second: `ts,sent,ok,err5xx,errconn,p50_ms,p99_ms,versions_seen`; 3xx/4xx count in `sent` only |

Availability = `ok / sent` over a window (`awk` recipe in plan Step 243). Caveat **FND-023**: discover mode follows Cuirass's actual state, which follows its 30 s sync, so a dead replica stays a target until the next sync; measured 0.9343 over a 2 min kill window (Phase 11) and 0.9829 in game day B.

## `monad-send`, `monad-count` (packet-chaos lab, Linux only)

```
sudo steel-echo monad-send --iface steel0-h --dst fd00:5ee1::2 --flow-label 0x5EE1 --count 1000
sudo ip netns exec steel0 steel-echo monad-count --iface steel0-n --seconds 30
```

`monad-send` writes IPv6 frames with a Monad Hop-by-Hop option (`pkg/monadpkt`, CRC pinned to the CCITT-FALSE check value 0x29B1) over AF_PACKET; `--src` (`fd00:5ee1::1`), `--dst-mac` (default `33:33:00:00:00:01`), `--interval` (1 ms), `--src-svc`. `monad-count` reads the first 86 header bytes of each frame (`MSG_TRUNC`, no payload) and prints JSON per flow label: `received`, `chaos_set`, `chaos_clear`, `crc_ok`, `crc_bad`. Game day E and plan Step 346 are built on these two.

## Images

`scripts/steel-echo-image.sh` builds `steel-echo-v1..v4` as LXD images: Alpine 3.22 + OpenRC, the static binary under `supervise-daemon` as user `steel` (group `steel`: the first build had no group and the service would not start, fixed and republished with `--replace`), `/etc/conf.d/steel-echo` holding the knobs. v1, v2, v3 are good builds; **v4 is the deliberately bad one** (`STEEL_ERROR_RATE=0.3`: 30 % of `/` answer 500, `/health` stays green), the canary that must roll back.

| Mode | |
|---|---|
| (default) | dry run: prints every command, contacts no LXD, not even reads |
| `--stage DIR` | builds the four payloads locally into `DIR/<ver>/` |
| `--apply` | per version: `lxc launch images:alpine/3.22 steel-build-<ver>`, push, `rc-update add`, stop, `lxc publish --alias steel-echo-<ver>`, delete the builder. Refuses on an existing alias without `--replace` and on any leftover `steel-build-*`; never touches an instance not named `steel-build-<ver>` |
| `--project` | default `default`: govan builds in the default project, whose image store project `unheaded` shares (`features.images=false`, H16); Cuirass's restricted identity only runs images (D-15) |

On west since 2026-10-26: four aliases, no builders left, pihole untouched. In LXD an instance is `steel-echo-<ver>-<n>` running image `steel-echo-<ver>`, with `health {8080, /health}`, the proxy device on `127.0.0.1:26601`, and labels `svc`, `ver`, `chaos=allowed`; `curl 127.0.0.1:26601/version` says which version holds the endpoint.

---

> **Source:** [cmd/steel-echo](../cmd/steel-echo) · [pkg/monadpkt](../pkg/monadpkt) · [scripts/steel-echo-image.sh](../scripts/steel-echo-image.sh) · [S-REAL-STEEL battle plan](../docs/battle-plans/S-REAL-STEEL-battle-plan.md) (Phases 6, 13)
