# `deploy/`

Deployment artifacts for the Phase D-A agent runtime.

## Layout

```
deploy/
├── systemd/
│   ├── zhen-agentd.service           — drop-in unit
│   └── zhen-agentd.env.example       — example /etc/zhen-agentd.env
├── docker/
│   └── zhen-agentd.Dockerfile        — multi-stage build → distroless
└── README.md                          — this file
```

## systemd

```bash
sudo install -m 0644 deploy/systemd/zhen-agentd.service /etc/systemd/system/
sudo install -m 0600 -o root deploy/systemd/zhen-agentd.env.example /etc/zhen-agentd.env
sudo $EDITOR /etc/zhen-agentd.env       # set AUTH_API_KEYS, WELL_DSN, ...
sudo install -d -o zhen -g zhen /var/zhen/projects/default
sudo systemctl daemon-reload
sudo systemctl enable --now zhen-agentd
sudo journalctl -u zhen-agentd -f
```

The unit hardens defense-in-depth on top of the in-process gate:
`NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, seccomp filter,
empty capability set, `ReadWritePaths=/var/zhen/projects` (must include
your `-allowed-roots`).

## Docker

```bash
make zhen-agentd-docker
docker run --rm -p 20105:20105 \
    -e AUTH_ENABLED=false \
    -e VOR_URL=http://host.docker.internal:9876 \
    -e LLAMA_URL=http://host.docker.internal:8081 \
    zhen-agentd:dev
```

The base image is `gcr.io/distroless/static-debian12:nonroot` (~18 MB).
CGO disabled. Production deploys behind an nginx/HAProxy sidecar for TLS
termination per Unheaded's Port-Authority pattern.

## Production deploy checklist

Before turning on `AUTH_ENABLED=true` and exposing beyond loopback:

- [ ] `WELL_DSN` set with `sslmode=verify-full` (not `disable`)
- [ ] `AUTH_API_KEYS` is a long random string (≥32 hex chars), per-environment
- [ ] `-rate-limit` set to a sane value (5-25 rps depending on traffic)
- [ ] `-allowed-roots` enumerates ONLY the project roots tenants are allowed to target
- [ ] PostgreSQL `unheaded_app` database has a `zhen` user with INSERT/UPDATE/SELECT on `zhen_actions`
- [ ] Reverse-proxy in front terminates TLS, sets `X-Forwarded-For`, and only forwards from trusted IPs
- [ ] `journalctl -u zhen-agentd` is in the central log pipeline
- [ ] `zhen_agentd_*` Prometheus metrics scraped (target: `127.0.0.1:20105/metrics`)

## Endpoints exposed

| Path | Auth | Rate-limit | Purpose |
|---|---|---|---|
| `GET /health` | bypass | bypass | liveness |
| `GET /ready` | bypass | bypass | readiness (vor + llama-server) |
| `GET /metrics` | bypass | bypass | Prometheus exposition |
| `GET /api/v1/openapi.json` | bypass | applies | OpenAPI 3.0 spec |
| `POST /api/v1/agent/ask` | enforced | applies | run the agent loop |
| `POST /api/v1/agent/confirm` | enforced | applies | redeem pending-confirm token |

OpenAPI spec is also served as a static file under `cmd/zhen-agentd/openapi.json` for offline use / API gateway discovery.
