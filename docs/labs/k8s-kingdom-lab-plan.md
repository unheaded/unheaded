# K8s Mock-Prod Lab — Executable Plan

**Companion to:** `docs/labs/k8s-kingdom-lab.md` (scope, rosetta stone, framing)
**Session ladder:** `docs/labs/k8s-kingdom-lab-sessions.md`
**Provisioning:** `scripts/k8s-lab-up.sh` / `scripts/k8s-lab-down.sh`
**Reference repo:** `~/tmp/kubernetes-the-hard-way/` (KTHW chapters 01-13)

This doc turns the 14-phase scope into numbered, checkable steps. Each phase has:
**Reading first** (do BEFORE typing) · **Stop before starting** (spike coexistence) · **Steps** (numbered, executable) · **Verification gate** (must pass to advance) · **Extraction notes** (write findings here for ADR-047).

Keep the scope doc open in another pane for the rosetta stone — you will look at it constantly.

---

## Pre-flight Checklist (do ONCE)

- [ ] PF.1  KTHW cloned: `ls ~/tmp/kubernetes-the-hard-way/docs/01-prerequisites.md`
- [ ] PF.2  `govan` is in `lxd` group on EAST: `ssh govan@east 'id | grep lxd'`
        — if not: `ssh govan@east 'sudo usermod -aG lxd govan && newgrp lxd'`
- [ ] PF.3  WireGuard up between WEST↔EAST: `ping6 -c 2 fd00:dead:beef::2`
- [ ] PF.4  Spike daemon inventory captured (so you can restart later):
        `ssh govan@east 'systemctl --user list-units --state=active | grep -E "heimdall|gjallarhorn"' > ~/spike-snapshot.txt`
- [ ] PF.5  `kubectl` installed locally: `kubectl version --client --short`
        — if not: `curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && sudo install kubectl /usr/local/bin/`
- [ ] PF.6  `helm` installed locally: `helm version --short`
        — if not: `curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash`
- [ ] PF.7  Disk free check: `df -h /var/lib/lxd` (~20 GB free needed)

**Gate:** every PF.N checked. Do not start Phase 0 with red items.

---

## Phase 0 — LXD Topology (~1h)

Stand up 8 containers across both hosts.

**Reading first:** scope doc §"Topology — 8 Nodes" + §"Spike Coexistence Rules" (~10 min)
**Stop before starting:** nothing (containers don't conflict with spike)
**State entering:** Pre-flight green

### Steps

- [ ] 0.1  `cd ~/tmp/unheaded && ./scripts/k8s-lab-up.sh`
- [ ] 0.2  Confirm 5 WEST containers RUNNING: `sudo lxc list k8s-`
- [ ] 0.3  Confirm 3 EAST containers RUNNING: `ssh govan@east 'sudo lxc list k8s-'`
- [ ] 0.4  Cross-host reachability test: from WEST `k8s-jumpbox`, ping `k8s-cp-3` on EAST
        ```
        sudo lxc exec k8s-jumpbox -- bash -c 'apt-get update -qq && apt-get install -y -qq iputils-ping'
        sudo lxc exec k8s-jumpbox -- ping6 -c 2 <cp-3 IPv6 address from east lxc list>
        ```
- [ ] 0.5  Bake a hosts file mapping name→IP for all 8 containers; copy to jumpbox at `/etc/hosts`
- [ ] 0.6  SSH key from jumpbox to all 7 other containers (KTHW jumpbox model):
        `sudo lxc exec k8s-jumpbox -- ssh-keygen -N "" -f /root/.ssh/id_ed25519`
        then push pub key into each container via `lxc exec ... bash -c 'echo "..." >> /root/.ssh/authorized_keys'`

### Verification gate

- [ ] `kubectl` not yet installed on any container (correct — KTHW does it later)
- [ ] Jumpbox can SSH to all 7 other containers without password
- [ ] All 8 containers in RUNNING state across both hosts
- [ ] `free -h` on WEST shows ≥4 GB free (containers booted + headroom)

### Extraction notes

> What was painful about LXD provisioning that K8s would have solved? (Hint: the answer is "nothing — LXD was fine for 8 containers." The K8s pitch only kicks in at scale.)

---

## Phase 1 — KTHW Walkthrough, Single Control Plane (~4h, weekend)

Walk KTHW chapters 02-12 with the rebadged hostnames. Produces a working but NON-HA cluster.

**Reading first:** Skim KTHW `01-prerequisites.md` and `02-jumpbox.md` (~20 min). Bookmark `03-compute-resources.md` through `12-smoke-test.md`.
**Stop before starting:** nothing
**State entering:** Phase 0 verification gate green

**Hostname rebadging (paste into your notes):**

| KTHW name | Lab name      | Host |
|-----------|---------------|------|
| jumpbox   | k8s-jumpbox   | WEST |
| server    | k8s-cp-1      | WEST |
| node-0    | k8s-worker-1  | WEST |
| node-1    | k8s-worker-3  | EAST (cross-host worker — exercises WG) |

(node-2/node-3/cp-2/cp-3/worker-2/worker-4 are added in Phase 2.)

### Steps

- [ ] 1.1  KTHW 02 — jumpbox tooling: install `kubectl`, `cfssl`, `cfssljson` inside `k8s-jumpbox`
- [ ] 1.2  KTHW 03 — record IPs of cp-1, worker-1, worker-3 on jumpbox
- [ ] 1.3  KTHW 04 — generate CA + per-component certs on jumpbox; distribute to nodes
- [ ] 1.4  KTHW 05 — generate kubeconfigs (admin, kube-controller-manager, kube-scheduler, kube-proxy, kubelets)
- [ ] 1.5  KTHW 06 — generate data encryption config; copy to cp-1
- [ ] 1.6  KTHW 07 — bootstrap **single-node etcd** on cp-1 (we go HA in Phase 2)
- [ ] 1.7  KTHW 08 — bootstrap kube-apiserver, kube-controller-manager, kube-scheduler on cp-1
- [ ] 1.8  KTHW 09 — bootstrap kubelet + kube-proxy + containerd on worker-1 AND worker-3
- [ ] 1.9  KTHW 10 — local kubectl context pointing at cp-1 from your dev machine
- [ ] 1.10 KTHW 11 — pod network routes (manual — Cilium replaces this in Phase 3)
- [ ] 1.11 KTHW 12 — smoke test: `kubectl run nginx --image=nginx`, `kubectl expose pod nginx --port=80`, hit it via `kubectl port-forward`
- [ ] 1.12 Schedule a deployment with `replicas=4`; verify pods land on BOTH worker-1 (WEST) and worker-3 (EAST)

### Verification gate

- [ ] `kubectl get nodes` shows worker-1 + worker-3 as `Ready`
- [ ] `kubectl get pods -A` shows core system pods Running
- [ ] Cross-host pod-to-pod traffic confirmed (a pod on worker-1 can reach a pod on worker-3)
- [ ] You can `kubectl describe pod` and read it without referring to docs

### Extraction notes

> KTHW chapter 04 (CA + cert distribution): how many minutes? cert-manager (Phase 5) automates this. Worth it? Think about the **5-engineer team** case from the scope doc.

> KTHW chapter 11 (manual pod routes): this is what CNI plugins exist to remove. Compare to what `crates/heimdall-bpf/` does in 200 lines.

---

## Phase 2 — HA Control Plane (~2h)

Add cp-2 (WEST) and cp-3 (EAST) as control plane members. etcd quorum = 3.

**Reading first:** etcd clustering docs `https://etcd.io/docs/v3.5/op-guide/clustering/` (~15 min). Don't read the whole site — just the "Static" section.
**Stop before starting:** nothing
**State entering:** Phase 1 gate green; cluster is single-cp

### Steps

- [ ] 2.1  Generate etcd member certs for cp-2 and cp-3 on jumpbox; distribute
- [ ] 2.2  On cp-1: edit `/etc/systemd/system/etcd.service` to add cp-2 + cp-3 as initial-cluster peers; reload but don't restart yet
- [ ] 2.3  On cp-2 + cp-3: install + configure etcd with the same initial-cluster string; start
- [ ] 2.4  Restart etcd on cp-1; verify `etcdctl member list` shows 3 members
- [ ] 2.5  Replicate apiserver, controller-manager, scheduler systemd units onto cp-2 + cp-3
- [ ] 2.6  On jumpbox: install `haproxy`; configure backend pool of all 3 apiservers on :6443; expose VIP on jumpbox :6443
- [ ] 2.7  Update local kubeconfig to point at jumpbox VIP instead of cp-1
- [ ] 2.8  Verify `kubectl get nodes` still works through VIP
- [ ] 2.9  **Failure drill:** `sudo lxc stop k8s-cp-1`. Wait 10s. Run `kubectl get nodes`. It should still work via cp-2/cp-3.
- [ ] 2.10 `sudo lxc start k8s-cp-1`. Verify rejoins.

### Verification gate

- [ ] `etcdctl endpoint status --cluster` shows 3 healthy endpoints
- [ ] `kubectl get --raw /readyz` returns `ok` from VIP
- [ ] Killing any ONE control plane member does not stop kubectl
- [ ] You can name the leader election semantics for controller-manager + scheduler (only one active, others standby)

### Extraction notes

> The HA story is the moment K8s "feels" like prod. Compare to Unheaded's current single-daemon control plane. What would Unheaded need to be HA? (Hint: Wotan ring-buffer replication + Heimdall leader election.)

---

## Phase 3 — Cilium CNI (~1h)

Replace kube-proxy with Cilium. Closest analogue to what Unheaded does natively.

**Reading first:** Cilium "Getting Started → Concepts" page (~15 min). Skim Hubble docs.
**Stop before starting:** nothing
**State entering:** Phase 2 HA gate green

### Steps

- [ ] 3.1  On jumpbox: `helm repo add cilium https://helm.cilium.io/ && helm repo update`
- [ ] 3.2  Find VIP value used in haproxy backend; export as `VIP`
- [ ] 3.3  Install Cilium with kube-proxy replacement:
        ```
        helm install cilium cilium/cilium --namespace kube-system \
          --set kubeProxyReplacement=true \
          --set k8sServiceHost=$VIP \
          --set k8sServicePort=6443
        ```
- [ ] 3.4  Wait for cilium pods Ready: `kubectl -n kube-system rollout status ds/cilium --timeout=180s`
- [ ] 3.5  Delete kube-proxy daemonset (if KTHW left one): `kubectl -n kube-system delete ds kube-proxy --ignore-not-found`
- [ ] 3.6  Install Cilium CLI on jumpbox; run `cilium status` and `cilium connectivity test --test "no-policies/pod-to-pod"`
- [ ] 3.7  Install Hubble UI: `cilium hubble enable --ui`
- [ ] 3.8  Port-forward Hubble UI: `cilium hubble ui` and open in browser

### Verification gate

- [ ] `cilium status` shows OK / no errors
- [ ] `cilium connectivity test` passes the "pod-to-pod" subtest
- [ ] Hubble UI shows live flow graph between pods

### Extraction notes

> Open Hubble side-by-side with Unheaded's `dashboard-backend` packet-flow view. Which one tells a richer story for an oncall engineer? Be honest.

> Cilium replaced ~200 lines of KTHW manual routing. What does Unheaded's `crates/heimdall-bpf/` replace?

---

## Phase 4 — Storage (Longhorn) (~1h)

**Reading first:** Longhorn docs "Quick Installation" (~10 min)
**Stop before starting:** nothing
**State entering:** Cilium green

### Steps

- [ ] 4.1  `helm repo add longhorn https://charts.longhorn.io && helm repo update`
- [ ] 4.2  Pre-req on every worker: `iscsiadm` + `nfs-common` installed (Longhorn needs them)
        — for each worker: `sudo lxc exec k8s-worker-N -- apt-get install -y open-iscsi nfs-common`
- [ ] 4.3  `helm install longhorn longhorn/longhorn -n longhorn-system --create-namespace`
- [ ] 4.4  Wait for ready: `kubectl -n longhorn-system rollout status ds/longhorn-manager --timeout=300s`
- [ ] 4.5  Create a PVC (5Gi) + a Pod that mounts it; write a file
- [ ] 4.6  Delete the pod, recreate, verify file persists
- [ ] 4.7  Verify cross-node replica: `kubectl -n longhorn-system get volumes -o wide` shows replicas on multiple nodes
- [ ] 4.8  Forward Longhorn UI port; click around

### Verification gate

- [ ] StorageClass `longhorn` is default
- [ ] PVC bound, Pod running, file persists across pod recreate
- [ ] Replica count ≥ 2, distributed across nodes

### Extraction notes

> Longhorn uses iSCSI under the hood. Unheaded has `Phylactery` for storage. Are they solving the same problem at the same layer?

---

## Phase 5 — Ingress + cert-manager (~1.5h)

**Reading first:** ingress-nginx + cert-manager quickstarts (~15 min). Skim Let's Encrypt ACME flow.
**Stop before starting:** nothing
**State entering:** Storage green

### Steps

- [ ] 5.1  `helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx && helm repo add jetstack https://charts.jetstack.io && helm repo update`
- [ ] 5.2  Install ingress-nginx as NodePort:
        `helm install ingress-nginx ingress-nginx/ingress-nginx -n ingress-nginx --create-namespace --set controller.service.type=NodePort`
- [ ] 5.3  Install cert-manager with CRDs:
        `helm install cert-manager jetstack/cert-manager -n cert-manager --create-namespace --set installCRDs=true`
- [ ] 5.4  Create a self-signed `ClusterIssuer` (Let's Encrypt won't issue against private IPs)
- [ ] 5.5  Deploy a sample nginx, expose via Ingress with TLS using the self-signed issuer
- [ ] 5.6  Curl the ingress NodePort with `--insecure`; verify cert chain is your self-signed issuer
- [ ] 5.7  Trigger renewal: shorten cert duration, watch cert-manager re-issue

### Verification gate

- [ ] Ingress reachable from jumpbox
- [ ] TLS cert auto-issued and visible in `kubectl get certificate`
- [ ] Cert renewal completes without manual intervention

### Extraction notes

> cert-manager + Let's Encrypt vs `pkg/gungnir` + ML-DSA-65. Same problem (cert lifecycle), different threat models (x509-WebPKI vs PQ-internal). Which fits which use case? Note for ADR-047.

---

## Phase 6 — Observability (kube-prometheus-stack + Loki) (~2h)

**Reading first:** kube-prometheus-stack README (~15 min). Skim ServiceMonitor CRD.
**Stop before starting:** nothing
**State entering:** Ingress green
**RAM check:** kube-prometheus-stack is heavy. `free -h` should show ≥3 GB free across the cluster before installing.

### Steps

- [ ] 6.1  `helm repo add prometheus-community https://prometheus-community.github.io/helm-charts && helm repo add grafana https://grafana.github.io/helm-charts && helm repo update`
- [ ] 6.2  Install Prometheus stack:
        `helm install monitoring prometheus-community/kube-prometheus-stack -n monitoring --create-namespace`
- [ ] 6.3  Wait Ready: `kubectl -n monitoring rollout status deploy/monitoring-grafana --timeout=300s`
- [ ] 6.4  Port-forward Grafana; default creds `admin / prom-operator`; explore the auto-installed dashboards (cluster, nodes, pods)
- [ ] 6.5  Install Loki:
        `helm install loki grafana/loki-stack -n monitoring --set grafana.enabled=false`
- [ ] 6.6  Add Loki as a Grafana datasource; query container logs in Explore
- [ ] 6.7  Create a custom alert rule (e.g., "any pod restarted >3 times in 5min"); trigger it by `kubectl delete pod` on a Deployment

### Verification gate

- [ ] Grafana shows live metrics for nodes/pods
- [ ] Loki returns logs for at least 2 namespaces
- [ ] Alert fires on intentional pod kill loop

### Extraction notes

> This is the layer where "YOLO write your own" loses to ecosystem. What would it take to make `dashboard-backend` + `pkg/logagg/` reach kube-prometheus-stack maturity? Be honest about person-years.

---

## Phase 7 — GitOps (Argo CD) (~1h)

**Reading first:** Argo CD getting started + "App of Apps" pattern (~15 min)
**Stop before starting:** nothing
**State entering:** Observability green

### Steps

- [ ] 7.1  `helm repo add argo https://argoproj.github.io/argo-helm && helm repo update`
- [ ] 7.2  `helm install argocd argo/argo-cd -n argocd --create-namespace`
- [ ] 7.3  Get initial admin password: `kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d`
- [ ] 7.4  Port-forward argocd-server, log in
- [ ] 7.5  Create a tiny git repo with one Deployment YAML (use a private gitlab/gitea container OR public GitHub repo for the lab)
- [ ] 7.6  Create an Argo `Application` pointing at that repo + path
- [ ] 7.7  Verify reconcile: change replicas in the repo, push, watch Argo reconcile within ~3 min
- [ ] 7.8  Force a drift: `kubectl scale deploy/x --replicas=10`. Watch Argo revert it.

### Verification gate

- [ ] Application status: Synced + Healthy
- [ ] Manual `kubectl scale` is reverted by Argo within 3 min
- [ ] You can describe what `app-of-apps` means and why teams use it

### Extraction notes

> Argo CD = Heimdall daemon + UI + multi-tenant + 5 years of polish. What would it take to put a UI on Heimdall and call it "GitOps for bare metal"? Note as ADR-047 extraction candidate.

---

## Phase 8 — Sample 3-Tier App (~1h)

**Reading first:** Online Boutique repo README (~10 min)
**Stop before starting:** nothing
**State entering:** GitOps green

### Steps

- [ ] 8.1  Clone Online Boutique: `git clone https://github.com/GoogleCloudPlatform/microservices-demo.git ~/tmp/microservices-demo`
- [ ] 8.2  Apply manifests: `kubectl apply -f ~/tmp/microservices-demo/release/kubernetes-manifests.yaml`
- [ ] 8.3  Wait for all 11 services Ready: `kubectl rollout status` per Deployment (or watch `kubectl get pods -w`)
- [ ] 8.4  Expose `frontend-external` via Ingress (replace its LoadBalancer with NodePort/Ingress)
- [ ] 8.5  Open the storefront in a browser
- [ ] 8.6  Run the built-in `loadgenerator`; watch traffic in Hubble UI + Grafana
- [ ] 8.7  Tail logs of the `cart` service via Loki; correlate with a checkout event

### Verification gate

- [ ] Storefront loads, you can add to cart, check out
- [ ] Loadgen traffic visible in Hubble flow graph
- [ ] Grafana shows per-service request rate

### Extraction notes

> 11 services + ingress + storage + observability — this is the workload K8s actually shines on. Could Unheaded handle it today? Where would it break?

---

## Phase 9 — NetworkPolicy + RBAC (~1h)

**Reading first:** K8s NetworkPolicy concept page + RBAC concept page (~20 min)
**Stop before starting:** nothing
**State entering:** Sample app running

### Steps

- [ ] 9.1  Apply default-deny NetworkPolicy in the `default` namespace
- [ ] 9.2  Refresh the storefront — it should be broken (services can't talk)
- [ ] 9.3  Add explicit allow policies between front→cart→productcatalog→...
- [ ] 9.4  Verify Hubble shows allowed flows green, denied flows red
- [ ] 9.5  Create a `ServiceAccount` `ci-bot` + `Role` (read-only on Deployments) + `RoleBinding`
- [ ] 9.6  Generate a kubeconfig for ci-bot: `kubectl create token ci-bot --duration=24h`
- [ ] 9.7  Verify ci-bot CAN list deployments, CANNOT create them

### Verification gate

- [ ] Default-deny breaks the app, explicit allow heals it
- [ ] ci-bot RBAC enforced (read works, write denied)
- [ ] You can describe the difference between Role / ClusterRole / RoleBinding / ClusterRoleBinding without docs

### Extraction notes

> NetworkPolicy + Cilium = same idea as Unheaded's NixOS firewall + iptables default-deny. Different syntax, same model. Which is more debuggable when it breaks at 2am?

---

## Phase 10 — Day-2 Ops Drills (~2h)

The interview-gold list. Each drill is a one-liner; the LEARNING is in interpreting output.

**Reading first:** none — this is muscle memory
**Stop before starting:** nothing
**State entering:** Sample app + RBAC running

### Drill steps (do each, observe what happens)

- [ ] 10.1 Drain: `kubectl drain k8s-worker-1 --ignore-daemonsets --delete-emptydir-data` — watch pods reschedule
- [ ] 10.2 Uncordon: `kubectl uncordon k8s-worker-1` — verify schedulable again
- [ ] 10.3 Rolling update: `kubectl set image deploy/frontend server=gcr.io/google-samples/microservices-demo/frontend:v0.10.0` (replace tag) — watch the rollout
- [ ] 10.4 Rollback: `kubectl rollout undo deploy/frontend` — confirm previous version returns
- [ ] 10.5 Scale: `kubectl scale deploy/frontend --replicas=5` — watch HPA disagree if HPA exists
- [ ] 10.6 Logs across pods: `kubectl logs -l app=frontend --tail=50 -f`
- [ ] 10.7 Exec: `kubectl exec -it $(kubectl get pod -l app=cart -o name | head -1) -- sh`
- [ ] 10.8 Port-forward: `kubectl port-forward svc/frontend 8080:80`
- [ ] 10.9 etcd snapshot from cp-1:
        `sudo lxc exec k8s-cp-1 -- etcdctl --endpoints=https://127.0.0.1:2379 --cacert=... snapshot save /tmp/etcd-backup.db`
- [ ] 10.10 etcd restore drill (DO NOT execute on live cluster — read the procedure, write it down)
- [ ] 10.11 Describe a deliberately-broken pod: `kubectl run badimage --image=nope/nope:404` then `kubectl describe pod badimage` — read the Events section out loud

### Verification gate

- [ ] You can do each drill from memory by end of phase
- [ ] You can read a `kubectl describe` Events section and predict the failure cause
- [ ] etcd snapshot file exists and is non-zero size

### Extraction notes

> Which of these drills do NOT have an Unheaded equivalent? (Hint: rolling update + rollback. Unheaded restarts services with new configs but doesn't have the "10% rollout" primitive.)

---

## Phase 11 — Disaster Drills (~1h)

Time to break things deliberately.

**Reading first:** none — this is the chaos phase
**Stop before starting:** snapshot etcd first (Phase 10.9)
**State entering:** Sample app + everything green

### Steps

- [ ] 11.1 Kill a control plane member: `sudo lxc stop k8s-cp-2`. Verify cluster survives. Restart, verify rejoins.
- [ ] 11.2 Kill a worker: `sudo lxc stop k8s-worker-1`. Watch pods reschedule onto other workers within ~5 min.
- [ ] 11.3 Network-partition WEST↔EAST: `sudo iptables -I OUTPUT -d <east-WG-ip> -j DROP` for 60s. Observe etcd quorum behavior. Remove the rule.
- [ ] 11.4 Corrupt etcd on cp-3: shell into it, mv the data dir; restart etcd; verify it recovers from peers.
- [ ] 11.5 (DRY RUN) Document an etcd snapshot restore procedure end-to-end: stop all etcd → restore on each member from snapshot → start in order → verify quorum
- [ ] 11.6 (OPTIONAL) Actually run the snapshot restore on a fresh test cluster

### Verification gate

- [ ] Cluster recovered fully from each drill (kubectl works after each step)
- [ ] You can describe the difference between losing 1, 2, or 3 etcd members and what each scenario means

### Extraction notes

> Which failures did K8s handle gracefully that Unheaded would NOT today? Which did Unheaded's spike (Mímir's Law) actually catch that K8s might miss? Both directions matter for ADR-047.

---

## Phase 12 — Honest Comparison Notes (~1h)

Write `docs/labs/k8s-vs-unheaded-notes.md`. This is the OUTPUT of the lab. Don't skip.

**Reading first:** scope doc §"After the Lab — Update" + ADR-047
**Stop before starting:** nothing
**State entering:** Phases 0-11 complete OR explicitly skipped with notes

### Steps

- [ ] 12.1 Open scope doc §"What K8s Is Actually For" — re-read with fresh eyes after the lab
- [ ] 12.2 Open ADR-047 — note any claim that the lab confirmed or contradicted
- [ ] 12.3 Write comparison doc with these sections:
       - **K8s wins:** with concrete evidence from this lab
       - **Unheaded wins:** with concrete evidence (e.g. "14GB footprint vs N GB for kube-prometheus-stack alone")
       - **Mímir's Law concepts that map to K8s operators:** list with rough effort estimate per CRD
       - **Mímir's Law concepts that don't map cleanly:** list with WHY (probably: PQ crypto, eBPF substrate, wire-format programmability)
       - **Extraction candidates for ADR-047:** Unheaded components that could ship as K8s plugins (rank by ease)
- [ ] 12.4 Update ADR-047 with a "Lab Complete" addendum referencing the comparison doc
- [ ] 12.5 Commit (git commit -m "docs(labs): k8s mock-prod lab complete + comparison notes")

### Verification gate

- [ ] Comparison doc exists, has all 5 sections
- [ ] At least 3 concrete extraction candidates identified
- [ ] ADR-047 references the new doc

### Extraction notes

> This is the doc you will actually re-read in 6 months when an interviewer asks "so why DO you not use K8s?". Make it good.

---

## Phase 13 — Custom Operator (~3h, OPTIONAL but high-value)

Build a real K8s operator from a Mímir's Law concept. The moment Unheaded ↔ K8s converges in your head.

**Reading first:** kubebuilder book chapters 1-4 (~30 min) OR operator-sdk Go tutorial
**Stop before starting:** nothing
**State entering:** Phase 12 done so you have the comparison fresh
**Concept to implement:** `BaselineManifest` CRD with file-hash drift detection (mirrors Mímir's Law `pkg/enkrateia/`)

### Steps

- [ ] 13.1 `kubebuilder init --domain unheaded.dev --repo github.com/unheaded/baseline-operator`
- [ ] 13.2 `kubebuilder create api --group config --version v1alpha1 --kind BaselineManifest`
- [ ] 13.3 Define spec: list of `{path, expectedSha256, severity}` entries
- [ ] 13.4 Define status: `lastScan timestamp`, `driftCount`, conditions
- [ ] 13.5 Implement reconciler: for each pod matching a label, exec into it, hash the listed files, compare, emit Events on drift
- [ ] 13.6 `make manifests && make install` — install CRD into cluster
- [ ] 13.7 `make run` locally OR `make docker-build deploy` to run in cluster
- [ ] 13.8 Apply a test BaselineManifest; tamper with a file in a target pod; verify Event fires
- [ ] 13.9 Wire alert: ServiceMonitor for the operator, Prometheus rule on driftCount metric

### Verification gate

- [ ] CRD installed, controller running
- [ ] Drift triggers a K8s Event AND a Prometheus alert
- [ ] You can explain how this operator differs from Unheaded's `cmd/heimdall-daemon/` despite implementing the same concept

### Extraction notes

> THE big one: now that you've built the same thing twice (once as a daemon in Unheaded, once as an operator in K8s), which audience is each version actually FOR? Capture this before it fades.

---

## What to Skip (do NOT spend time on)

| Skip | Why |
|------|-----|
| CCM (cloud controller manager) | bare metal — irrelevant |
| Service mesh (Istio/Linkerd) | optional Phase 14, not core |
| Multi-cluster (ClusterAPI, KCP) | beyond mock-prod |
| Custom scheduler plugins | too deep for scope |
| Pod Security Standards detail | note + return |
| Audit log streaming setup | note + return |

If you find yourself going down one of these, stop and write a one-line note in the comparison doc instead.

---

## Stress-Test Toolkit (use during Phase 8/10/11)

| Tool | When to use |
|------|-------------|
| `k9s` | Always — terminal cluster watcher |
| `cilium hubble ui` | Phase 3 onward — eBPF flow graph |
| `vegeta` | Phase 8/10 — quick HTTP load |
| `k6` | Phase 8 — scriptable load |
| Chaos Mesh | Phase 11 — pod kills + network partition |
| `kube-burner` | Phase 11 — control-plane stress |

Install only when you need them. Don't build a Christmas tree.

---

## Tear-down

When done (or at end of each weekend session if you need the RAM back):

```
~/tmp/unheaded/scripts/k8s-lab-down.sh --yes
```

This stops + deletes ONLY containers prefixed `k8s-`. Spike daemons untouched.

To restart from where you left off: re-run `k8s-lab-up.sh`. You'll need to redo whatever cluster state was inside the containers (etcd snapshot from Phase 10.9 is your friend).

---

*K8s Mock-Prod Lab — Plan companion. Forged 2026-04-17 alongside scope doc.*
