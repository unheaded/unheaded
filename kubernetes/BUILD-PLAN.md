<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Unheaded on Kubernetes — The Hard Way

A from-scratch runbook for standing up a Kubernetes control plane by hand and then
deploying the entire Unheaded Kingdom onto it. This mirrors the Docker stack
(`docker-compose.yml` + the four `docker/**/docker-compose.*.yml` files) one-for-one,
but expresses every workload as a hardened, declarative manifest.

It is deliberately written in the spirit of Kelsey Hightower's *Kubernetes The Hard
Way*: you bootstrap the PKI, etcd, and each control-plane component manually so you
understand every moving part. A `kubeadm` shortcut is given at the end for when you
just want a cluster.

> Free to use. Free to share. GPL-3.0.

---

## 0. What you are building

```
                         ┌──────────────── edge ────────────────┐
   internet ──▶ haproxy-edge (LoadBalancer 21080/21443)         │
                         └──▶ gateway (21000/21443) ──┐          │
                                                      ▼          │
   ┌───────────── data plane (ClusterIP, default-deny) ─────────┐
   │  wotan(18000/18001 bus) ◀── monad sophia timeguru architect│
   │  captain micromanager cuirass dashboard-backend kanban-app │
   │  the-well (PostgreSQL StatefulSet, 3 DBs / 7 users)        │
   └────────────────────────────────────────────────────────────┘
   ┌──────── observability (kingdom.tier: observability) ────────┐
   │  prometheus victoriametrics loki grafana                    │
   │  node-exporter(DS) promtail(DS) clickhouse vector           │
   └─────────────────────────────────────────────────────────────┘
   ┌──────── security ────────┐   ┌──── optional overlays ────┐
   │  suricata (IDS DaemonSet) │   │ gpu-vllm   wireguard      │
   └───────────────────────────┘   └───────────────────────────┘
```

Ports are the canonical **Doom Range** (16666–26666) from `pkg/ports/ports.go`. No
port is invented here; every value traces back to the registry or to a well-known
upstream image port (5432 Postgres, 9090 Prometheus, 3000 Grafana, etc.).

---

## 1. Prerequisites

- 3 nodes (or 1 for a lab): `controller-0`, `worker-0`, `worker-1`. Ubuntu 22.04+ /
  any systemd Linux, kernel 6.0+ (eBPF + Suricata want a recent kernel).
- `cfssl` / `cfssljson` (PKI), `etcd`, `kubectl`, the Kubernetes server binaries
  (`kube-apiserver`, `kube-controller-manager`, `kube-scheduler`, `kubelet`,
  `kube-proxy`), and `containerd` + `runc` + CNI plugins.
- A container registry you can push to (default placeholder:
  `ghcr.io/stevenrbellis/unheaded`). **This is a decision point — see §9.**

Build & push the service images first (from the repo root, using the existing
multi-stage `Dockerfile` targets — `wotan`, `timeguru`, `captain`, `architect`,
`micromanager`, `monad`, `sophia`, `cuirass`, `dashboard-backend`, `kanban-app`, plus
`gateway`):

```bash
# example — adapt registry/tag to your environment
for t in wotan timeguru captain architect micromanager monad sophia \
         cuirass dashboard-backend kanban-app gateway; do
  docker build --target "$t" -t ghcr.io/stevenrbellis/unheaded/$t:dev .
  docker push ghcr.io/stevenrbellis/unheaded/$t:dev
done
```

---

## 2. PKI — the trust root (cfssl)

Kubernetes is a mutual-TLS mesh of components. Generate a CA and one cert per
identity. This mirrors the "crypto slow/robust at the edge" doctrine: take the time
to get certs right.

```bash
mkdir -p pki && cd pki
# 2.1 Certificate Authority
cfssl gencert -initca ca-csr.json | cfssljson -bare ca
# 2.2 One cert each for:
#     admin, kube-controller-manager, kube-scheduler, kube-proxy,
#     each kubelet (system:node:<host>), and the api-server itself.
#     The api-server SAN list MUST include: 10.32.0.1 (service CIDR .1),
#     the controller IP, 127.0.0.1, kubernetes.default(.svc.cluster.local).
# 2.3 service-account key pair (kube-controller-manager signs SA tokens):
openssl genrsa -out service-account.key 4096
openssl req -new -x509 -key service-account.key -out service-account.crt -days 3650
```

Distribute: `ca.pem` to every node; component cert+key to its owner; build
`kubeconfig` files for admin / controller-manager / scheduler / kube-proxy / each
kubelet with `kubectl config set-cluster/set-credentials/set-context`.

Generate the data-encryption key for Secrets at rest:

```bash
head -c 32 /dev/urandom | base64   # → put in EncryptionConfig (aescbc) for the apiserver
```

---

## 3. etcd — cluster state

```bash
# On controller-0 (repeat/cluster for HA):
etcd \
  --name controller-0 \
  --cert-file=pki/kubernetes.pem --key-file=pki/kubernetes-key.pem \
  --trusted-ca-file=pki/ca.pem --client-cert-auth \
  --peer-cert-file=pki/kubernetes.pem --peer-key-file=pki/kubernetes-key.pem \
  --peer-trusted-ca-file=pki/ca.pem --peer-client-cert-auth \
  --listen-client-urls https://127.0.0.1:2379 \
  --advertise-client-urls https://127.0.0.1:2379 \
  --data-dir=/var/lib/etcd
# verify:
etcdctl --endpoints=https://127.0.0.1:2379 \
  --cacert=pki/ca.pem --cert=pki/kubernetes.pem --key=pki/kubernetes-key.pem \
  member list
```

---

## 4. Control plane (controller-0)

Run each as a systemd unit. Key flags:

**kube-apiserver**
```
--etcd-servers=https://127.0.0.1:2379 --etcd-cafile/certfile/keyfile=...
--client-ca-file=pki/ca.pem
--tls-cert-file=pki/kubernetes.pem --tls-private-key-file=pki/kubernetes-key.pem
--service-account-key-file=pki/service-account.crt
--service-account-signing-key-file=pki/service-account.key
--service-account-issuer=https://kubernetes.default.svc.cluster.local
--encryption-provider-config=encryption-config.yaml
--authorization-mode=Node,RBAC
--enable-admission-plugins=NodeRestriction,PodSecurity   # PSA: restricted (see §7)
--service-cluster-ip-range=10.32.0.0/24
--service-node-port-range=3000-32767    # ◀── REQUIRED so dev NodePorts can use
                                         #     20000/20001/21000/21443/3000
```
> The widened `--service-node-port-range` is what lets the `dev` overlay publish
> services on their real Doom-Range ports instead of the 30000–32767 default.

**kube-controller-manager**
```
--cluster-signing-cert-file=pki/ca.pem --cluster-signing-key-file=pki/ca-key.pem
--service-account-private-key-file=pki/service-account.key
--root-ca-file=pki/ca.pem --use-service-account-credentials=true
--cluster-cidr=10.200.0.0/16 --service-cluster-ip-range=10.32.0.0/24
```

**kube-scheduler** — point at its kubeconfig; defaults are fine.

Verify:
```bash
kubectl get componentstatuses   # etcd / scheduler / controller-manager Healthy
kubectl version
```

RBAC: create the `system:kube-apiserver-to-kubelet` ClusterRole + binding so the
apiserver can reach kubelets (logs/exec/metrics).

---

## 5. Worker nodes

Per worker install `containerd`, `runc`, CNI plugins, `kubelet`, `kube-proxy`.

**CNI — decision point (see §9).** Any CNI works, but Unheaded leans hard on
**NetworkPolicy** (default-deny + explicit allows in `manifests/base/network-policies/`),
so pick a CNI that enforces them: **Calico** or **Cilium** (Cilium also gives you
eBPF-native policy, which rhymes with Unheaded's eBPF data plane). Flannel alone does
**not** enforce NetworkPolicy and will silently leave the default-deny posture
unenforced.

**kubelet** (`--config kubelet-config.yaml`): set `cgroupDriver: systemd`,
`containerRuntimeEndpoint: unix:///run/containerd/containerd.sock`,
`clusterDNS: [10.32.0.10]`, `clusterDomain: cluster.local`, plus its TLS cert and
the `ca.pem`. Register with `--kubeconfig` pointing at the node's kubeconfig.

**kube-proxy** (`--config`): `mode: ipvs` (or iptables), `clusterCIDR: 10.200.0.0/16`.

Verify:
```bash
kubectl get nodes -o wide        # workers Ready
```

---

## 6. Cluster add-ons

1. **CoreDNS** at `10.32.0.10` — the cluster DNS. This *replaces* the Docker
   `coredns` service from `docker-compose.yml`; do not deploy a second copy. The
   `allow-dns-egress` NetworkPolicy targets `kube-system` CoreDNS.
2. **CNI NetworkPolicy controller** (Calico/Cilium) — must be running before you
   apply our policies, or the default-deny will not take effect.
3. **A StorageClass** marked `is-default-class` (see §9) — `the-well`, `wotan`,
   `prometheus`, `loki`, `victoriametrics`, and `clickhouse` all request PVCs.
4. **The HAProxy Kubernetes Ingress Controller** — the on-prem edge. It serves
   `loadbalancers/ingress.yaml` (`ingressClassName: haproxy`) and owns the external
   Doom-Range NodePorts. Apply it as a STANDALONE kustomization (it lives in its own
   `haproxy-controller` namespace, so it is NOT part of the `unheaded` base):
   ```bash
   kubectl apply -k manifests/base/haproxy-ingress     # ns haproxy-controller, NodePort 21080/21443/21404
   kubectl -n haproxy-controller rollout status deploy/haproxy-kubernetes-ingress
   ```
   External entry becomes `https://<any-node-ip>:21443`. (This supersedes the
   standalone `loadbalancers/haproxy-edge`; binds only >1023 ports so it needs no
   `NET_BIND_SERVICE` and stays PSA `restricted`.)
5. *(gpu overlay)* the **AMD GPU device plugin** advertising `amd.com/gpu`.

---

## 7. Deploy Unheaded — the apply order

Everything below lives under `kubernetes/manifests/`. The base kustomization already
encodes the dependency order, but when bringing a fresh cluster up by hand, apply in
waves and wait for readiness between them. `initContainers` (`wait-for-wotan`,
`wait-for-the-well`) and probes enforce ordering at runtime, mirroring Docker
`depends_on: condition: service_healthy`.

```bash
# Wave 0 — namespace, config, default-deny + allow policies
kubectl apply -k manifests/base/namespace
kubectl apply -k manifests/base/config
kubectl apply -k manifests/base/network-policies

# Wave 1 — stateful spine (must be Ready before services start)
kubectl apply -k manifests/base/the-well
kubectl apply -k manifests/base/wotan
kubectl -n unheaded rollout status statefulset/wotan
kubectl -n unheaded rollout status statefulset/the-well

# Wave 2 — core services (depend on wotan; kanban also on the-well + timeguru)
for s in monad sophia timeguru architect captain micromanager cuirass \
         dashboard-backend kanban-app gateway; do
  kubectl apply -k manifests/base/$s
done

# Wave 3 — edge, observability, security
kubectl apply -k manifests/base/loadbalancers
kubectl apply -k manifests/base/telemetry
kubectl apply -k manifests/base/logging
kubectl apply -k manifests/base/suricata
```

Or, the whole thing through an overlay (recommended once you trust it):

```bash
kubectl apply -k manifests/overlays/dev      # NodePort exposure, LOG_LEVEL=debug
# or
kubectl apply -k manifests/overlays/prod     # LoadBalancer edge, replicas=2
```

Optional add-ons (layer after an environment overlay):

```bash
kubectl apply -k manifests/overlays/gpu-vllm     # needs amd.com/gpu + GPU node
kubectl apply -k manifests/overlays/wireguard    # host-net DaemonSet, 51820/udp
```

Pod Security Admission is set to **restricted** on the namespace
(`pod-security.kubernetes.io/enforce: restricted`). Every workload satisfies it:
`runAsNonRoot`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem` where
feasible, `capabilities.drop: [ALL]`, `seccompProfile: RuntimeDefault`. The three
documented exceptions (Suricata, node-exporter/promtail host mounts, and the optional
WireGuard/vLLM overlays) keep every restriction they can and add back only the single
capability they truly need.

---

## 8. Verify

```bash
kubectl -n unheaded get pods,svc,netpol
kubectl -n unheaded rollout status deploy/gateway

# health endpoints (dev NodePort)
curl -fsS http://<node-ip>:21000/health     # gateway
curl -fsS http://<node-ip>:20000/health     # dashboard-backend
curl -fsS http://<node-ip>:20001/health     # kanban (the Meta Moment)
curl -fsS http://<node-ip>:3000/api/health  # grafana

# the bus is reachable in-cluster
kubectl -n unheaded run probe --rm -it --image=busybox:1.36 --restart=Never -- \
  wget -qO- http://wotan:18000/health

# default-deny actually denies (should TIME OUT, not connect):
kubectl -n unheaded run deny-test --rm -it --image=busybox:1.36 --restart=Never -- \
  wget -qO- --timeout=3 http://the-well:5432   # blocked unless labelled well-client
```

---

## 9. Decisions you must make

| Decision | Default in these manifests | Action needed |
|----------|---------------------------|---------------|
| **Image registry** | `ghcr.io/stevenrbellis/unheaded/<svc>:dev` | Build & push the 11 service images; override via the overlay `images:` block. |
| **StorageClass** | cluster default (PVCs are `ReadWriteOnce`) | Ensure a default SC exists (local-path, EBS, Ceph, etc.). |
| **CNI** | none assumed | Install Calico or Cilium **before** the network policies, or default-deny is a no-op. |
| **GPU nodes** | none | For `gpu-vllm`: label a node `amd.com/gpu=true`, install the AMD device plugin, stage models at `/mnt/models`. |
| **Edge type** | ✅ RESOLVED — **HAProxy Kubernetes Ingress Controller** (`base/haproxy-ingress/`, ns `haproxy-controller`, NodePort 21080/21443/21404). | It IS the ingress controller for `loadbalancers/ingress.yaml` — converges edge + Ingress, no 21443 collision. The standalone `loadbalancers/haproxy-edge` is now legacy (keep only if you want a non-ingress L4/L7 edge). Optional: convert the controller to a DaemonSet+hostPort for an every-node edge. |
| **Secrets** | placeholders in `config/secret-shared.yaml` | Replace with SOPS/age, Sealed Secrets, External Secrets, or Vault. Never commit real values. |

---

## 10. The faster way — kubeadm

If you only want a working cluster (not the pedagogy), `kubeadm` collapses §2–§6:

```bash
kubeadm init --pod-network-cidr=10.200.0.0/16 \
  --service-cidr=10.32.0.0/24 \
  --apiserver-extra-args service-node-port-range=3000-32767
# install a CNI that enforces NetworkPolicy:
kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.27.0/manifests/calico.yaml
# join workers with the printed `kubeadm join ...` token
```

Then jump straight to §7. The hard way remains the primary path here because it makes
the apiserver flags (especially `--service-node-port-range`), the PKI, and the CNI
NetworkPolicy requirement explicit — all three are load-bearing for this stack.

---

## Appendix — Docker → Kubernetes mapping

| Docker artifact | Kubernetes form |
|-----------------|-----------------|
| `docker-compose.yml` services (wotan…cuirass) | `manifests/base/<svc>/{deployment,service}.yaml` |
| `wotan`, `postgres` (stateful) | `StatefulSet` + headless Service |
| `depends_on: service_healthy` | `initContainers` (wait-for-*) + readiness/liveness probes |
| `.env.shared` | `ConfigMap unheaded-shared` + `Secret` placeholders |
| Docker bridge networks (control/data/observe) | `NetworkPolicy` default-deny + explicit allows |
| `docker-compose.loadbalancers.yml` HAProxy edge/internal | `loadbalancers/haproxy-*.yaml` (Deployment+Service) |
| HAProxy/nginx per-app sidecars | per-service ClusterIP Services + optional `ingress.yaml` |
| `docker-compose.telemetry.yml` | `telemetry/*` (Deployments + node-exporter/promtail DaemonSets) |
| `docker/suricata` | `suricata/daemonset.yaml` (hostNetwork IDS) |
| `docker-compose.vllm.yml` | `overlays/gpu-vllm/` (`amd.com/gpu`) |
| `docker-compose.wireguard.yml` | `overlays/wireguard/` (host-net DaemonSet) |
| `docker/Makefile` build/push | image build loop in §1 + overlay `images:` |
| host port publish | `NodePort` (dev) / `LoadBalancer` (prod) only where Docker mapped a host port |
