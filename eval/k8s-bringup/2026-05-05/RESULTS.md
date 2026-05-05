# WAVE17 K8s Bring-up — Results (2026-05-05)

## Final state: 9/9 SERVICES RUNNING

All 9 Unheaded services run cleanly on a 3-node kind cluster via
`helm upgrade --install unheaded`:

| Service | Status |
|---|---|
| architect | ✓ 1/1 Running |
| captain | ✓ 1/1 Running |
| dashboard-backend | ✓ 1/1 Running |
| kanban-app | ✓ 1/1 Running |
| micromanager | ✓ 1/1 Running |
| monad | ✓ 1/1 Running |
| sophia | ✓ 1/1 Running |
| timeguru | ✓ 1/1 Running (× 2 replicas per chart values) |
| wotan | ✓ 1/1 Running |

10 pods total (timeguru runs 2 replicas per chart default).

```
$ kubectl exec -n unheaded $(kubectl get pod -n unheaded -l app.kubernetes.io/name=wotan -o name | head -1) \
    -- wget -qO- http://localhost:18000/health
{"rooms":0,"service":"wotan","status":"healthy","timestamp":"2026-05-05T06:28:01...","total_members":0,"version":"0.1.0"}
```

## What had to be fixed mid-run

### Fix 1: K8s service-link env vars were breaking monad

K8s injects `<SERVICENAME>_PORT=tcp://<clusterip>:<port>` env vars for every
service in the namespace. Monad's `cmd/monad/main.go:73` reads `MONAD_PORT` as
a port-number; K8s' value of `"tcp://10.96.17.1:19004"` produced a malformed
listen address.

**Fix**: added `enableServiceLinks: false` to both pod templates
(`helm/unheaded/templates/services.yaml` line 43, `wotan.yaml` line 21).
Services discover peers via stable DNS names instead. After fix: monad listens
on `:19004` correctly.

### Fix 2: Chart had no way to mount writable volumes

`securityContext.readOnlyRootFilesystem: true` (correct hardening posture
per CLAUDE.md) prevented captain + timeguru from creating their state
directories. Chart provided no volume override surface.

**Fix**: extended `helm/unheaded/templates/services.yaml` with `{{- with
$svc.volumes }}` + `{{- with $svc.volumeMounts }}` blocks. `values-local.yaml`
adds `emptyDir` mounts for captain (`/var/lib/unheaded`) and timeguru
(`/app/data`). Both pods now Running.

### Adjacent fix: image registry override

Default chart references `ghcr.io/unheaded/<service>:0.1.0-alpha`. Locally
loaded kind images are under `docker.io/library/`. `values-local.yaml`
overrides `global.image.registry: docker.io/library` and per-service
repository names to `unheaded-dev-<service>`.

## Reproducer

```bash
# from a fresh clone or after `kind delete cluster --name unheaded`:
./deploy/k8s/kind/bring-up.sh

# after ~90s:
kubectl get pods -n unheaded
# expect 10/10 Running

# health probe:
kubectl exec -n unheaded $(kubectl get pod -n unheaded -l app.kubernetes.io/name=wotan -o name | head -1) \
    -- wget -qO- http://localhost:18000/health
```

## What this enables

This is the substrate ADR-064 (Wotan active/active 3-node cluster) was
deferring on. The chart now successfully deploys all services to a 3-node
kind cluster; the next ADR-064 phase can iterate from this baseline:
- Phase 1: convert wotan Deployment → StatefulSet (3 replicas, headless service)
- Phase 2: introduce hashicorp/raft for cluster membership + topic-leader election
- Phase 3: quorum-acked publish + LICH-014 split-brain campaign
- Phase 4: Akira integration + ADR-035 cutover

Until then: existing bare-metal active-passive deployment continues
unchanged. The kind bench is for development + ADR-064 incremental work.

## Files added / modified this run

```
NEW:
  deploy/k8s/kind/cluster-config.yaml
  deploy/k8s/kind/values-local.yaml
  deploy/k8s/kind/bring-up.sh
  docs/battle-plans/WAVE17-OVERNIGHT-K8S-SUBSTRATE.md
  eval/k8s-bringup/2026-05-05/RESULTS.md (this file)

MODIFIED:
  helm/unheaded/templates/services.yaml   (enableServiceLinks: false +
                                            volumes/volumeMounts support)
  helm/unheaded/templates/wotan.yaml      (enableServiceLinks: false)
```

## Stack untouched

The bare-metal Unheaded stack on the host (postgres :5432, llama-server :8081,
vor :9876, wotan :18000, shield :19009, dashboard-backend :20000,
kanban-app :20001, wiki-server :20002, zhen_app :20103, zhen-agentd :20105)
is unchanged. The kind cluster runs in parallel inside docker without
affecting the bare-metal services.
