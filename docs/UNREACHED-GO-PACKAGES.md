<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Go packages no binary imports (2026-10-23)

Input for [ADR-090](adr/ADR-090-total-source-sweep-slop-removal.md); the library
counterpart of [LIVE-PATHS.md](LIVE-PATHS.md), which covers binaries. Not a
deletion list: keeping an experiment is a fine decision, it should just be one.

On develop at 2026-10-23: **148 of 300** packages in the root module are imported by
no `main` package (tests do not count). Regenerate:

```bash
mains=$(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...)
comm -23 <(go list ./... | sort -u) <(go list -deps $mains | grep '^unheaded/' | sort -u)
```

Known to fake success, so a future caller would be misled (found 2026-10-23):

- `pkg/lxd`: `RealClient` returns successful Operations without talking to LXD.
  Its last importer, unheaded-cli, now goes through Cuirass (`f95b0764`); Cuirass
  has its own client (`cmd/unheaded-daemon/lxd_rest.go`).
- `pkg/certs/acme`: account URL `"placeholder"`, nil certificate, revoke is a no-op.
- `cmd/unheaded-daemon/internal/{lxd,state,config,ebpf}`: the daemon's original
  scaffolding; `main.go` uses none of it.

## The list

- `cmd/dashboard-backend/jstest`
- `cmd/unheaded-daemon/internal/config`
- `cmd/unheaded-daemon/internal/ebpf`
- `cmd/unheaded-daemon/internal/lxd`
- `cmd/unheaded-daemon/internal/state`
- `deploy/nftables`
- `internal/bpfmap`
- `internal/dashboard`
- `internal/ebpf/collector`
- `nix/tests`
- `pkg/alerting`
- `pkg/alerting/manager`
- `pkg/alerting/notify`
- `pkg/alerting/rules`
- `pkg/audit/export`
- `pkg/audit/logger`
- `pkg/audit/query`
- `pkg/audit/storage`
- `pkg/baremetal`
- `pkg/baremetal/image`
- `pkg/baremetal/inventory`
- `pkg/baremetal/ipmi`
- `pkg/baremetal/pxe`
- `pkg/certs`
- `pkg/certs/acme`
- `pkg/certs/ca`
- `pkg/certs/issue`
- `pkg/certs/rotation`
- `pkg/certs/store`
- `pkg/compliance`
- `pkg/compliance/audit`
- `pkg/compliance/controls`
- `pkg/compliance/report`
- `pkg/compliance/standards`
- `pkg/config`
- `pkg/config/merge`
- `pkg/config/sources`
- `pkg/config/template`
- `pkg/config/watch`
- `pkg/container`
- `pkg/deploy`
- `pkg/deploy/artifact`
- `pkg/deploy/pipeline`
- `pkg/deploy/rollback`
- `pkg/deploy/strategy`
- `pkg/dns`
- `pkg/ebpf/maploader`
- `pkg/eventbus`
- `pkg/events`
- `pkg/http`
- `pkg/loadbalancer`
- `pkg/lxd`
- `pkg/mesh`
- `pkg/mesh/discovery`
- `pkg/mesh/loadbalance`
- `pkg/mesh/loadbalancer`
- `pkg/mesh/mtls`
- `pkg/mesh/observe`
- `pkg/mesh/policy`
- `pkg/mesh/proxy`
- `pkg/mesh/resilience`
- `pkg/mesh/routing`
- `pkg/metrics/metricstest`
- `pkg/monad`
- `pkg/netlink`
- `pkg/network`
- `pkg/nix`
- `pkg/pqc_policy`
- `pkg/protocol/amplification`
- `pkg/protocol/bpfschema`
- `pkg/protocol/dos`
- `pkg/protocol/encoding`
- `pkg/protocol/errors`
- `pkg/protocol/flowtype`
- `pkg/protocol/fuzz`
- `pkg/protocol/integrity`
- `pkg/protocol/intermediary`
- `pkg/protocol/lifecycle`
- `pkg/protocol/migration`
- `pkg/protocol/prefetch`
- `pkg/protocol/registry`
- `pkg/protocol/sequence`
- `pkg/protocol/settings`
- `pkg/protocol/sophiasync`
- `pkg/protocol/tlv`
- `pkg/runtime`
- `pkg/scheduler`
- `pkg/secrets/encryption`
- `pkg/secrets/rotation`
- `pkg/secrets/store`
- `pkg/service`
- `pkg/state`
- `pkg/storage/cache`
- `pkg/storage/kv`
- `pkg/storage/object`
- `pkg/telemetry`
- `pkg/testing`
- `pkg/trace`
- `pkg/tracing`
- `pkg/uids`
- `pkg/validation`
- `pkg/waf`
- `pkg/waf/detection`
- `pkg/waf/inspection`
- `pkg/waf/ip`
- `pkg/waf/ratelimit`
- `pkg/waf/response`
- `pkg/waf/rules`
- `pkg/worker`
- `pkg/wotan-client/mock`
- `scripts/host`
- `scripts/lib`
- `services/anamnesis`
- ~~`services/anomaly`~~: reached by `cmd/anomaly` since S5 Phase 15 (2026-10-23)
- `services/canary`
- `services/cape`
- `services/cloak`
- `services/compliance`
- `services/cuirass`
- `services/firewall`
- `services/gauntlets`
- `services/hauberk`
- `services/kenoma`
- `services/keymgmt`
- `services/keymgmt/handler`
- `services/keymgmt/lifecycle`
- `services/keymgmt/store`
- `services/loadbalancer`
- `services/nfv`
- `services/pauldrons`
- `services/pleroma`
- `services/qos`
- `services/sabatons`
- `services/selfheal`
- `services/sword`
- `services/tassets`
- `services/telemetry`
- `services/vambraces`
- `services/version`
- `services/wotan/internal/circuitbreaker`
- `services/wotan/internal/compute`
- `tests/doom-memwrite`
- `tests/e2e`
- `tests/integration`
- `tests/pqc`
- `tests/s77`
- `tests/security/doom`
- `tomb/lich/harnesses`
