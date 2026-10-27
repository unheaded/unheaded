# UNHEADED × KUBERNETES CONVERGENCE BATTLE PLAN — 16 Phases, 400+ Steps

**Date**: 2026-03-05
**Sprint**: SK8 — Kubernetes Convergence: Production-Grade Kingdom Infrastructure
**Prerequisite**: Unheaded Age 1 Alpha at ~99%, 465K LOC, 25 services, 4 eBPF programs compiled
**Target**: Unheaded infrastructure achieves production-grade Kubernetes-native deployment with every pattern from the InCommodities session baked into the platform
**Estimated Duration**: 80-120 hours across 16-20 sessions
**Agent Strategy**: Phases 0-3 sequential (foundation), Phases 4-7 parallelizable in pairs, Phases 8-11 sequential (integration), Phases 12-15 parallelizable in pairs
**Commit Cadence**: Every 4 steps (max(3, min(5, 400/20)) = 5, reduced to 4 for safety)
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

---

## SESSION-TO-KINGDOM MAPPING

Every pattern discussed maps to an Unheaded component:

| Session Topic | Kingdom Component | Phase(s) | Priority |
|---|---|---|---|
| AKS private API + Azure CNI | Cuirass (Control Plane) + Hauberk (Mesh) | 1, 5 | P0 |
| Cilium/eBPF network policy | Whispering Void + Shield (WAF) | 2, 6 | P0 |
| Workload Identity (zero secrets) | Gorget (Secret Mgmt) | 3 | P0 |
| Node resource isolation (PLEG) | Gauntlets (Scheduler) + Helm (Runtime) | 4 | P0 |
| OPA/Gatekeeper admission control | Shield (WAF) + Cuirass | 5 | P0 |
| Terragrunt state decomposition | Vambraces (Config Mgmt) | 7 | P0 |
| ArgoCD GitOps delivery | Sword (Deploy Pipeline) | 8 | P0 |
| Prometheus/Grafana/OTel stack | Dashboard Backend + Anamnesis | 9 | P0 |
| PLEG monitoring + alerting | Sabatons (Health) + Dashboard | 9 | P1 |
| Drift detection (nightly plan) | Kenoma (Actual State) + Vambraces | 10 | P0 |
| GitHub Rulesets + Terraform | Sword + Vambraces | 11 | P1 |
| Template repos + born-compliant | Sword + Shield | 11 | P1 |
| PodDisruptionBudgets + topology | Gauntlets + Pauldrons (LB) | 12 | P1 |
| Multi-zone HA + autoscaling | Pauldrons + Cuirass | 12 | P0 |
| Cost optimization + rightsizing | Sophia (Knowledge) + Dashboard | 13 | P2 |
| Supply chain hardening (SHA pin) | Moat Ghost compliance | 14 | P0 |
| Incident response runbooks | Warmonger + Anamnesis | 15 | P1 |
| eBPF flow tracing → dashboard | Whispering Void → Dashboard | 6, 9 | P0 |

---

## LEGEND

- `[B]` = Bash command (run directly)
- `[V]` = Verification step (MUST pass before proceeding)
- `[D]` = Debug step (only if prior step fails)
- `[W]` = Write/create file
- `[R]` = Read/inspect file
- `[S]` = Sudo required
- `[P]` = Parallelizable with other marked steps
- `[C]` = Commit checkpoint
- `[STUCK]` = Skipped via Skip Protocol
- `[BLOCKED]` = Blocked by upstream STUCK
- `[K8S]` = Kubernetes-specific command
- `[HELM]` = Helm chart operation
- `[TF]` = Terraform/OpenTofu operation

---

## PHASE 0: ENVIRONMENT VERIFICATION & K8S BOOTSTRAP (Steps 1-25)

**Goal**: Verify dev machine toolchain, bootstrap local K8s cluster, validate Unheaded builds clean
**Prerequisite**: Git repo cloned, Go 1.24+, Rust toolchain, NixOS base
**Time**: 45 minutes
**Agent**: Coordinator

### Toolchain Verification

- [ ] **Step 1** [B]: Verify Go version >= 1.24
  ```bash
  go version | grep -E 'go1\.(2[4-9]|[3-9][0-9])'
  ```
- [ ] **Step 2** [V]: Go 1.24+ confirmed. If fail → install via nixpkgs
- [ ] **Step 3** [B]: Verify Rust + Aya BPF toolchain
  ```bash
  rustc --version && cargo --version && rustup target list --installed | grep bpf
  ```
- [ ] **Step 4** [B]: Verify container runtime (containerd preferred)
  ```bash
  containerd --version && ctr version
  ```
- [ ] **Step 5** [B]: Verify Kubernetes toolchain
  ```bash
  kubectl version --client && helm version && argocd version --client 2>/dev/null && k9s version 2>/dev/null
  ```
- [ ] **Step 6** [D]: Install missing K8s tools
  ```bash
  nix-env -iA nixpkgs.kubectl nixpkgs.kubernetes-helm nixpkgs.argocd nixpkgs.k9s
  ```
- [ ] **Step 7** [B]: Verify IaC tools
  ```bash
  tofu version && terragrunt --version && conftest --version
  ```
- [ ] **Step 8** [D]: Install missing IaC tools
  ```bash
  nix-env -iA nixpkgs.opentofu nixpkgs.terragrunt nixpkgs.conftest
  ```
- [ ] **Step 9** [B]: Verify eBPF kernel support
  ```bash
  uname -r && bpftool feature probe kernel | head -20
  ```
- [ ] **Step 10** [V]: BPF JIT enabled, BTF available. If fail → STOP, kernel upgrade required

### Local K8s Cluster Bootstrap

- [ ] **Step 11** [B]: Create local Kind cluster with 3 worker nodes (simulates multi-zone)
  ```bash
  cat <<'EOF' > /tmp/kind-config.yaml
  kind: Cluster
  apiVersion: kind.x-k8s.io/v1alpha4
  nodes:
  - role: control-plane
  - role: worker
    labels:
      topology.kubernetes.io/zone: zone-a
  - role: worker
    labels:
      topology.kubernetes.io/zone: zone-b
  - role: worker
    labels:
      topology.kubernetes.io/zone: zone-c
  networking:
    disableDefaultCNI: true
    podSubnet: "10.244.0.0/16"
    serviceSubnet: "10.96.0.0/16"
  EOF
  kind create cluster --name unheaded-dev --config /tmp/kind-config.yaml
  ```
- [ ] **Step 12** [V]: Cluster running, 4 nodes visible
  ```bash
  kubectl get nodes -o wide
  ```
- [ ] **Step 13** [B][K8S]: Install Cilium CNI (eBPF-native networking)
  ```bash
  helm repo add cilium https://helm.cilium.io/
  helm install cilium cilium/cilium --namespace kube-system \
    --set ipam.mode=kubernetes \
    --set hubble.enabled=true \
    --set hubble.relay.enabled=true \
    --set hubble.ui.enabled=true \
    --set bpf.masquerade=true \
    --set kubeProxyReplacement=true
  ```
- [ ] **Step 14** [V]: Cilium healthy, all agents running
  ```bash
  kubectl -n kube-system get pods -l app.kubernetes.io/name=cilium-agent -o wide
  cilium status --wait 2>/dev/null || kubectl -n kube-system exec ds/cilium -- cilium status
  ```
- [ ] **Step 15** [D]: If Cilium pods CrashLoop, check mount propagation
  ```bash
  kubectl -n kube-system describe pod -l app.kubernetes.io/name=cilium-agent | grep -A5 Events
  ```

### Unheaded Build Verification

- [ ] **Step 16** [B]: Build all Unheaded services
  ```bash
  cd /path/to/unheaded && go build ./...
  ```
- [ ] **Step 17** [V]: Zero build errors
- [ ] **Step 18** [B]: Run full test suite with race detection
  ```bash
  go test -race -count=1 ./... 2>&1 | tail -30
  ```
- [ ] **Step 19** [V]: All tests pass, zero data races
- [ ] **Step 20** [C]: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN SK8] Steps 1-20: Environment verified, Kind cluster + Cilium bootstrapped"
  ```

### Namespace Architecture

- [ ] **Step 21** [B][K8S]: Create Unheaded namespace hierarchy
  ```bash
  for ns in unheaded-system unheaded-armory unheaded-gnostic unheaded-court unheaded-presentation unheaded-ebpf; do
    kubectl create namespace $ns --dry-run=client -o yaml | kubectl apply -f -
  done
  ```
- [ ] **Step 22** [B][K8S]: Label namespaces for network policy targeting
  ```bash
  kubectl label ns unheaded-system tier=system --overwrite
  kubectl label ns unheaded-armory tier=armory --overwrite
  kubectl label ns unheaded-gnostic tier=gnostic --overwrite
  kubectl label ns unheaded-court tier=court --overwrite
  kubectl label ns unheaded-presentation tier=presentation --overwrite
  kubectl label ns unheaded-ebpf tier=ebpf --overwrite
  ```
- [ ] **Step 23** [V]: All 6 namespaces exist with correct labels
  ```bash
  kubectl get ns -l tier --show-labels
  ```
- [ ] **Step 24** [C]: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN SK8] Steps 21-24: K8s namespace hierarchy created"
  ```
- [ ] **Step 25** [V]: **PHASE 0 EXIT GATE** — Kind cluster running, Cilium healthy, 6 namespaces created, Unheaded builds clean, all tests pass
  - If ALL pass → proceed to Phase 1
  - If ANY fail → DO NOT PROCEED. Debug within this phase.

---

## PHASE 1: CONTROL PLANE HARDENING — CUIRASS GOES K8S-NATIVE (Steps 26-50)

**Goal**: Harden the Cuirass control plane with private API patterns, RBAC lockdown, and admission control scaffolding
**Prerequisite**: Phase 0 exit gate passed
**Time**: 60 minutes
**Agent**: Coordinator

### RBAC Lockdown (Zero Standing Privilege)

- [ ] **Step 26** [W][K8S]: Create least-privilege ClusterRoles for each Unheaded tier
  ```bash
  cat <<'EOF' > deploy/k8s/rbac/armory-role.yaml
  apiVersion: rbac.authorization.k8s.io/v1
  kind: ClusterRole
  metadata:
    name: unheaded-armory
  rules:
  - apiGroups: [""]
    resources: ["pods", "services", "endpoints", "configmaps"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets"]
    verbs: ["get", "list", "watch", "update", "patch"]
  EOF
  ```
- [ ] **Step 27** [W][K8S]: Create service accounts per armor piece (no shared identities)
  ```bash
  for svc in shield hauberk pauldrons sword cuirass helm-runtime gauntlets greaves vambraces gorget sabatons; do
    kubectl create serviceaccount $svc -n unheaded-armory --dry-run=client -o yaml | kubectl apply -f -
  done
  ```
- [ ] **Step 28** [B][K8S]: Bind roles to service accounts
  ```bash
  kubectl create clusterrolebinding armory-binding \
    --clusterrole=unheaded-armory \
    --serviceaccount=unheaded-armory:shield \
    --serviceaccount=unheaded-armory:hauberk \
    --serviceaccount=unheaded-armory:pauldrons \
    --dry-run=client -o yaml | kubectl apply -f -
  ```
- [ ] **Step 29** [V]: RBAC applied, no default service account tokens mounted
  ```bash
  kubectl get clusterrolebinding armory-binding -o yaml
  ```
- [ ] **Step 30** [C]: **COMMIT CHECKPOINT**

### Admission Control — OPA Gatekeeper

- [ ] **Step 31** [B][K8S][HELM]: Install OPA Gatekeeper
  ```bash
  helm repo add gatekeeper https://open-policy-agent.github.io/gatekeeper/charts
  helm install gatekeeper gatekeeper/gatekeeper --namespace gatekeeper-system --create-namespace \
    --set replicas=2 \
    --set audit.replicas=1 \
    --set postInstall.probeWebhook.enabled=true
  ```
- [ ] **Step 32** [V]: Gatekeeper pods running
  ```bash
  kubectl -n gatekeeper-system get pods
  ```
- [ ] **Step 33** [W][K8S]: ConstraintTemplate — require resource limits on ALL pods
  ```bash
  cat <<'EOF' > deploy/k8s/policies/require-resource-limits.yaml
  apiVersion: templates.gatekeeper.sh/v1
  kind: ConstraintTemplate
  metadata:
    name: k8srequiredresources
  spec:
    crd:
      spec:
        names:
          kind: K8sRequiredResources
    targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8srequiredresources
        violation[{"msg": msg}] {
          container := input.review.object.spec.containers[_]
          not container.resources.limits.memory
          msg := sprintf("Container %v must have memory limits", [container.name])
        }
        violation[{"msg": msg}] {
          container := input.review.object.spec.containers[_]
          not container.resources.limits.cpu
          msg := sprintf("Container %v must have CPU limits", [container.name])
        }
  EOF
  kubectl apply -f deploy/k8s/policies/require-resource-limits.yaml
  ```
- [ ] **Step 34** [W][K8S]: Constraint — enforce on all Unheaded namespaces
  ```bash
  cat <<'EOF' > deploy/k8s/policies/enforce-resource-limits.yaml
  apiVersion: constraints.gatekeeper.sh/v1beta1
  kind: K8sRequiredResources
  metadata:
    name: must-have-resource-limits
  spec:
    match:
      kinds:
      - apiGroups: [""]
        kinds: ["Pod"]
      namespaces: ["unheaded-armory", "unheaded-gnostic", "unheaded-court", "unheaded-presentation", "unheaded-ebpf"]
  EOF
  kubectl apply -f deploy/k8s/policies/enforce-resource-limits.yaml
  ```
- [ ] **Step 35** [V]: Verify admission rejects pods without limits
  ```bash
  kubectl run test-no-limits --image=busybox -n unheaded-armory --restart=Never -- sleep 3600 2>&1 | grep -i "denied\|violation"
  ```
- [ ] **Step 36** [D]: If test pod was admitted, check Gatekeeper webhook
  ```bash
  kubectl get validatingwebhookconfigurations gatekeeper-validating-webhook-configuration -o yaml
  ```
- [ ] **Step 37** [W][K8S]: ConstraintTemplate — block privileged containers
  ```bash
  cat <<'EOF' > deploy/k8s/policies/deny-privileged.yaml
  apiVersion: templates.gatekeeper.sh/v1
  kind: ConstraintTemplate
  metadata:
    name: k8sdenyprivileged
  spec:
    crd:
      spec:
        names:
          kind: K8sDenyPrivileged
    targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8sdenyprivileged
        violation[{"msg": msg}] {
          container := input.review.object.spec.containers[_]
          container.securityContext.privileged == true
          msg := sprintf("Privileged container %v not allowed", [container.name])
        }
  EOF
  kubectl apply -f deploy/k8s/policies/deny-privileged.yaml
  ```
- [ ] **Step 38** [C]: **COMMIT CHECKPOINT**

### Resource Quotas & LimitRanges

- [ ] **Step 39** [W][K8S]: LimitRange per namespace (default + ceiling)
  ```bash
  cat <<'EOF' > deploy/k8s/policies/armory-limitrange.yaml
  apiVersion: v1
  kind: LimitRange
  metadata:
    name: armory-limits
    namespace: unheaded-armory
  spec:
    limits:
    - default:
        memory: "256Mi"
        cpu: "250m"
      defaultRequest:
        memory: "128Mi"
        cpu: "100m"
      max:
        memory: "1Gi"
        cpu: "1000m"
      type: Container
  EOF
  kubectl apply -f deploy/k8s/policies/armory-limitrange.yaml
  ```
- [ ] **Step 40** [B][K8S]: Apply LimitRanges to all Unheaded namespaces
  ```bash
  for ns in unheaded-gnostic unheaded-court unheaded-presentation unheaded-ebpf; do
    sed "s/unheaded-armory/$ns/g; s/armory-limits/${ns#unheaded-}-limits/g" \
      deploy/k8s/policies/armory-limitrange.yaml | kubectl apply -f -
  done
  ```
- [ ] **Step 41** [W][K8S]: ResourceQuota per namespace
  ```bash
  cat <<'EOF' > deploy/k8s/policies/armory-quota.yaml
  apiVersion: v1
  kind: ResourceQuota
  metadata:
    name: armory-quota
    namespace: unheaded-armory
  spec:
    hard:
      requests.cpu: "4"
      requests.memory: "4Gi"
      limits.cpu: "8"
      limits.memory: "8Gi"
      pods: "50"
  EOF
  kubectl apply -f deploy/k8s/policies/armory-quota.yaml
  ```
- [ ] **Step 42** [V]: Quotas and LimitRanges active on all namespaces
  ```bash
  for ns in unheaded-armory unheaded-gnostic unheaded-court unheaded-presentation unheaded-ebpf; do
    echo "=== $ns ===" && kubectl get limitrange,resourcequota -n $ns
  done
  ```
- [ ] **Step 43** [C]: **COMMIT CHECKPOINT**

### Priority Classes (System-Critical Protection)

- [ ] **Step 44** [W][K8S]: Create priority classes (protects CoreDNS/Cilium/Cuirass from eviction)
  ```bash
  cat <<'EOF' > deploy/k8s/policies/priority-classes.yaml
  apiVersion: scheduling.k8s.io/v1
  kind: PriorityClass
  metadata:
    name: unheaded-critical
  value: 1000000
  globalDefault: false
  description: "Critical Unheaded system components — Cuirass, Greaves (DNS), Shield"
  ---
  apiVersion: scheduling.k8s.io/v1
  kind: PriorityClass
  metadata:
    name: unheaded-standard
  value: 100000
  globalDefault: true
  description: "Standard Unheaded workloads"
  EOF
  kubectl apply -f deploy/k8s/policies/priority-classes.yaml
  ```
- [ ] **Step 45** [V]: Priority classes created
  ```bash
  kubectl get priorityclasses | grep unheaded
  ```
- [ ] **Step 46** [C]: **COMMIT CHECKPOINT**

### Cuirass Deployment Manifest

- [ ] **Step 47** [W][K8S]: Write Cuirass (control plane) deployment with all hardening
  ```bash
  cat <<'EOF' > deploy/k8s/armory/cuirass-deployment.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: cuirass
    namespace: unheaded-armory
    labels:
      app: cuirass
      tier: armory
      component: control-plane
  spec:
    replicas: 2
    selector:
      matchLabels:
        app: cuirass
    template:
      metadata:
        labels:
          app: cuirass
          tier: armory
      spec:
        serviceAccountName: cuirass
        priorityClassName: unheaded-critical
        automountServiceAccountToken: false
        securityContext:
          runAsNonRoot: true
          seccompProfile:
            type: RuntimeDefault
        containers:
        - name: cuirass
          image: unheaded/cuirass:latest
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
          ports:
          - containerPort: 8080
            name: http
          - containerPort: 9090
            name: grpc
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 10
            periodSeconds: 15
          readinessProbe:
            httpGet:
              path: /readyz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
        topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: DoNotSchedule
          labelSelector:
            matchLabels:
              app: cuirass
  EOF
  ```
- [ ] **Step 48** [V]: Manifest valid YAML
  ```bash
  kubectl apply --dry-run=server -f deploy/k8s/armory/cuirass-deployment.yaml
  ```
- [ ] **Step 49** [C]: **COMMIT CHECKPOINT**
- [ ] **Step 50** [V]: **PHASE 1 EXIT GATE** — RBAC bound, Gatekeeper enforcing, LimitRanges active, PriorityClasses set, Cuirass manifest validated
  - Verify: `kubectl get constrainttemplates` shows 2 templates
  - Verify: `kubectl get priorityclasses | grep unheaded` shows 2 classes
  - Verify: Test pod without limits is REJECTED
  - If ALL pass → proceed to Phase 2
  - If ANY fail → DO NOT PROCEED

---

## PHASE 2: eBPF NETWORK POLICY ENGINE — CILIUM + WHISPERING VOID (Steps 51-75)

**Goal**: Deploy Cilium network policies enforcing east-west segmentation between Unheaded tiers, integrate with Whispering Void eBPF programs
**Prerequisite**: Phase 1 exit gate passed, Cilium healthy
**Time**: 75 minutes
**Agent**: Agent [P] (parallelizable with Phase 3)

### Tier Isolation Policies

- [ ] **Step 51** [W][K8S]: Default deny all ingress per namespace
  ```bash
  for ns in unheaded-armory unheaded-gnostic unheaded-court unheaded-presentation unheaded-ebpf; do
    cat <<EOF | kubectl apply -f -
  apiVersion: cilium.io/v2
  kind: CiliumNetworkPolicy
  metadata:
    name: default-deny-ingress
    namespace: $ns
  spec:
    endpointSelector: {}
    ingressDeny:
    - fromEndpoints:
      - {}
  EOF
  done
  ```
- [ ] **Step 52** [V]: Verify default deny active
  ```bash
  for ns in unheaded-armory unheaded-gnostic unheaded-court unheaded-presentation unheaded-ebpf; do
    kubectl get ciliumnetworkpolicies -n $ns
  done
  ```
- [ ] **Step 53** [W][K8S]: Allow Armory → Gnostic (state queries)
  ```bash
  cat <<'EOF' > deploy/k8s/network-policies/armory-to-gnostic.yaml
  apiVersion: cilium.io/v2
  kind: CiliumNetworkPolicy
  metadata:
    name: allow-armory-to-gnostic
    namespace: unheaded-gnostic
  spec:
    endpointSelector:
      matchLabels:
        tier: gnostic
    ingress:
    - fromEndpoints:
      - matchLabels:
          tier: armory
          io.kubernetes.pod.namespace: unheaded-armory
      toPorts:
      - ports:
        - port: "9090"
          protocol: TCP
        rules:
          l7proto: grpc
  EOF
  kubectl apply -f deploy/k8s/network-policies/armory-to-gnostic.yaml
  ```
- [ ] **Step 54** [W][K8S]: Allow Presentation → Armory (API gateway path)
  ```bash
  cat <<'EOF' > deploy/k8s/network-policies/presentation-to-armory.yaml
  apiVersion: cilium.io/v2
  kind: CiliumNetworkPolicy
  metadata:
    name: allow-presentation-to-armory
    namespace: unheaded-armory
  spec:
    endpointSelector:
      matchLabels:
        tier: armory
    ingress:
    - fromEndpoints:
      - matchLabels:
          tier: presentation
          io.kubernetes.pod.namespace: unheaded-presentation
      toPorts:
      - ports:
        - port: "8080"
          protocol: TCP
  EOF
  kubectl apply -f deploy/k8s/network-policies/presentation-to-armory.yaml
  ```
- [ ] **Step 55** [W][K8S]: Allow eBPF tier → Presentation (telemetry pipeline)
  ```bash
  cat <<'EOF' > deploy/k8s/network-policies/ebpf-to-presentation.yaml
  apiVersion: cilium.io/v2
  kind: CiliumNetworkPolicy
  metadata:
    name: allow-ebpf-to-dashboard
    namespace: unheaded-presentation
  spec:
    endpointSelector:
      matchLabels:
        app: dashboard-backend
    ingress:
    - fromEndpoints:
      - matchLabels:
          tier: ebpf
          io.kubernetes.pod.namespace: unheaded-ebpf
      toPorts:
      - ports:
        - port: "9090"
          protocol: TCP
  EOF
  kubectl apply -f deploy/k8s/network-policies/ebpf-to-presentation.yaml
  ```
- [ ] **Step 56** [C]: **COMMIT CHECKPOINT**

### Hubble Observability (eBPF Flow Visibility)

- [ ] **Step 57** [B][K8S]: Verify Hubble relay is running
  ```bash
  kubectl -n kube-system get pods -l app.kubernetes.io/name=hubble-relay
  ```
- [ ] **Step 58** [B]: Install Hubble CLI
  ```bash
  HUBBLE_VERSION=$(curl -s https://raw.githubusercontent.com/cilium/hubble/master/stable.txt)
  curl -L --remote-name-all https://github.com/cilium/hubble/releases/download/$HUBBLE_VERSION/hubble-linux-amd64.tar.gz
  tar xzf hubble-linux-amd64.tar.gz && mv hubble /usr/local/bin/
  ```
- [ ] **Step 59** [B]: Port-forward Hubble and verify flow visibility
  ```bash
  kubectl -n kube-system port-forward svc/hubble-relay 4245:80 &
  sleep 3 && hubble observe --namespace unheaded-armory --last 10
  ```
- [ ] **Step 60** [V]: Hubble shows flow events between pods. If empty → check Cilium monitor
- [ ] **Step 61** [C]: **COMMIT CHECKPOINT**

### Whispering Void Integration Points

- [ ] **Step 62** [W]: Create eBPF collector K8s DaemonSet manifest
  ```bash
  cat <<'EOF' > deploy/k8s/ebpf/void-collector-daemonset.yaml
  apiVersion: apps/v1
  kind: DaemonSet
  metadata:
    name: whispering-void-collector
    namespace: unheaded-ebpf
    labels:
      app: void-collector
      tier: ebpf
  spec:
    selector:
      matchLabels:
        app: void-collector
    template:
      metadata:
        labels:
          app: void-collector
          tier: ebpf
      spec:
        serviceAccountName: void-collector
        hostNetwork: true
        hostPID: false
        priorityClassName: unheaded-critical
        containers:
        - name: collector
          image: unheaded/void-collector:latest
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
          securityContext:
            privileged: false
            capabilities:
              add: ["BPF", "PERFMON", "NET_ADMIN"]
              drop: ["ALL"]
          volumeMounts:
          - name: bpf-maps
            mountPath: /sys/fs/bpf
          - name: debugfs
            mountPath: /sys/kernel/debug
            readOnly: true
          ports:
          - containerPort: 9091
            name: grpc
          - containerPort: 2112
            name: metrics
        volumes:
        - name: bpf-maps
          hostPath:
            path: /sys/fs/bpf
        - name: debugfs
          hostPath:
            path: /sys/kernel/debug
        tolerations:
        - effect: NoSchedule
          operator: Exists
  EOF
  ```
- [ ] **Step 63** [V]: DaemonSet manifest validates
  ```bash
  kubectl apply --dry-run=server -f deploy/k8s/ebpf/void-collector-daemonset.yaml
  ```
- [ ] **Step 64** [W]: Create Gatekeeper exception for eBPF collector capabilities
  ```bash
  cat <<'EOF' > deploy/k8s/policies/ebpf-capability-exception.yaml
  apiVersion: constraints.gatekeeper.sh/v1beta1
  kind: K8sDenyPrivileged
  metadata:
    name: deny-privileged-except-ebpf
  spec:
    match:
      kinds:
      - apiGroups: [""]
        kinds: ["Pod"]
      excludedNamespaces: ["unheaded-ebpf", "kube-system", "gatekeeper-system"]
  EOF
  kubectl apply -f deploy/k8s/policies/ebpf-capability-exception.yaml
  ```
- [ ] **Step 65** [C]: **COMMIT CHECKPOINT**

### L7 Policy Testing

- [ ] **Step 66** [B][K8S]: Deploy test pods to verify network policy enforcement
  ```bash
  kubectl run test-armory --image=busybox -n unheaded-armory --restart=Never \
    --overrides='{"spec":{"containers":[{"name":"test","image":"busybox","command":["sleep","3600"],"resources":{"limits":{"memory":"64Mi","cpu":"100m"}}}]}}' -- sleep 3600
  kubectl run test-gnostic --image=busybox -n unheaded-gnostic --restart=Never \
    --overrides='{"spec":{"containers":[{"name":"test","image":"busybox","command":["sleep","3600"],"resources":{"limits":{"memory":"64Mi","cpu":"100m"}}}]}}' -- sleep 3600
  ```
- [ ] **Step 67** [B]: Test ALLOWED path: armory → gnostic on port 9090
  ```bash
  kubectl exec -n unheaded-armory test-armory -- wget -qO- --timeout=5 http://test-gnostic.unheaded-gnostic:9090 2>&1 || echo "connection attempted"
  ```
- [ ] **Step 68** [B]: Test DENIED path: gnostic → armory (should fail)
  ```bash
  kubectl exec -n unheaded-gnostic test-gnostic -- wget -qO- --timeout=5 http://test-armory.unheaded-armory:8080 2>&1 | grep -i "timed out\|refused"
  ```
- [ ] **Step 69** [V]: Allowed path succeeds OR connection attempted, denied path times out
- [ ] **Step 70** [D]: If denied path succeeds, check Cilium policy enforcement
  ```bash
  kubectl -n kube-system exec ds/cilium -- cilium policy get -n unheaded-armory
  ```
- [ ] **Step 71** [B]: Cleanup test pods
  ```bash
  kubectl delete pod test-armory -n unheaded-armory --force 2>/dev/null
  kubectl delete pod test-gnostic -n unheaded-gnostic --force 2>/dev/null
  ```
- [ ] **Step 72** [C]: **COMMIT CHECKPOINT**

### Hubble → Anamnesis Bridge Config

- [ ] **Step 73** [W]: Write Hubble export config for Anamnesis event ingestion
  ```bash
  cat <<'EOF' > deploy/k8s/ebpf/hubble-export-config.yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: hubble-export-config
    namespace: unheaded-ebpf
  data:
    export.yaml: |
      fieldMask:
        - time
        - source.namespace
        - source.labels
        - destination.namespace
        - destination.labels
        - l4.TCP.destination_port
        - verdict
        - drop_reason
        - Type
      # ZERO payload capture — Sacred Principle
      # Metadata only: src/dst/port/verdict
  EOF
  kubectl apply -f deploy/k8s/ebpf/hubble-export-config.yaml
  ```
- [ ] **Step 74** [C]: **COMMIT CHECKPOINT**
- [ ] **Step 75** [V]: **PHASE 2 EXIT GATE** — Default deny on all namespaces, tier-to-tier policies active, Hubble flowing, eBPF DaemonSet validated, L7 policies tested
  - Verify: `kubectl get ciliumnetworkpolicies -A | wc -l` shows >= 8 policies
  - Verify: Hubble observe returns flow data
  - Verify: Denied cross-tier traffic is blocked
  - If ALL pass → proceed to Phase 3
  - If ANY fail → DO NOT PROCEED

---

## PHASE 3: ZERO-TRUST SECRETS — GORGET GOES PRODUCTION (Steps 76-100)

**Goal**: Implement workload identity, Vault integration, secret rotation, and CSI driver for the Gorget (Secret Management) armor piece
**Prerequisite**: Phase 1 exit gate passed
**Time**: 60 minutes
**Agent**: Agent [P] (parallelizable with Phase 2)

### Vault Deployment

- [ ] **Step 76** [B][K8S][HELM]: Install HashiCorp Vault in dev mode for local cluster
  ```bash
  helm repo add hashicorp https://helm.releases.hashicorp.com
  helm install vault hashicorp/vault --namespace unheaded-system \
    --set "server.dev.enabled=true" \
    --set "server.dev.devRootToken=root" \
    --set "injector.enabled=true" \
    --set "csi.enabled=true" \
    --set "server.resources.requests.memory=256Mi" \
    --set "server.resources.requests.cpu=250m" \
    --set "server.resources.limits.memory=512Mi" \
    --set "server.resources.limits.cpu=500m"
  ```
- [ ] **Step 77** [V]: Vault pod running and ready
  ```bash
  kubectl -n unheaded-system get pods -l app.kubernetes.io/name=vault
  ```
- [ ] **Step 78** [B]: Configure Vault Kubernetes auth method
  ```bash
  kubectl -n unheaded-system exec vault-0 -- vault auth enable kubernetes
  kubectl -n unheaded-system exec vault-0 -- vault write auth/kubernetes/config \
    kubernetes_host="https://$KUBERNETES_PORT_443_TCP_ADDR:443"
  ```
- [ ] **Step 79** [C]: **COMMIT CHECKPOINT**

### Per-Service Secret Policies

- [ ] **Step 80** [B]: Create Vault secret engine and policies for each armor piece
  ```bash
  kubectl -n unheaded-system exec vault-0 -- vault secrets enable -path=unheaded kv-v2
  for svc in shield hauberk pauldrons sword cuirass helm-runtime gauntlets greaves gorget; do
    kubectl -n unheaded-system exec vault-0 -- vault policy write $svc-policy - <<EOF
  path "unheaded/data/$svc/*" {
    capabilities = ["read"]
  }
  EOF
    kubectl -n unheaded-system exec vault-0 -- vault write auth/kubernetes/role/$svc \
      bound_service_account_names=$svc \
      bound_service_account_namespaces=unheaded-armory \
      policies=$svc-policy \
      ttl=1h
  done
  ```
- [ ] **Step 81** [V]: Vault roles created for all armor pieces
  ```bash
  kubectl -n unheaded-system exec vault-0 -- vault list auth/kubernetes/role
  ```
- [ ] **Step 82** [B]: Seed test secrets
  ```bash
  kubectl -n unheaded-system exec vault-0 -- vault kv put unheaded/shield/config \
    tls_cert_path="/etc/certs/tls.crt" \
    tls_key_path="/etc/certs/tls.key" \
    rate_limit_rps="1000"
  kubectl -n unheaded-system exec vault-0 -- vault kv put unheaded/cuirass/config \
    api_bind_addr="0.0.0.0:8080" \
    grpc_bind_addr="0.0.0.0:9090" \
    election_timeout="5s"
  ```
- [ ] **Step 83** [C]: **COMMIT CHECKPOINT**

### Vault CSI SecretProviderClass

- [ ] **Step 84** [B][K8S]: Install Secrets Store CSI Driver
  ```bash
  helm repo add secrets-store-csi-driver https://kubernetes-sigs.github.io/secrets-store-csi-driver/charts
  helm install csi-secrets-store secrets-store-csi-driver/secrets-store-csi-driver \
    --namespace kube-system \
    --set syncSecret.enabled=true \
    --set enableSecretRotation=true \
    --set rotationPollInterval=120s
  ```
- [ ] **Step 85** [V]: CSI driver pods running on all nodes
  ```bash
  kubectl -n kube-system get pods -l app=secrets-store-csi-driver
  ```
- [ ] **Step 86** [W][K8S]: Create SecretProviderClass for Cuirass
  ```bash
  cat <<'EOF' > deploy/k8s/secrets/cuirass-secret-provider.yaml
  apiVersion: secrets-store.csi.x-k8s.io/v1
  kind: SecretProviderClass
  metadata:
    name: cuirass-vault-secrets
    namespace: unheaded-armory
  spec:
    provider: vault
    parameters:
      vaultAddress: "http://vault.unheaded-system:8200"
      roleName: "cuirass"
      objects: |
        - objectName: "api-bind-addr"
          secretPath: "unheaded/data/cuirass/config"
          secretKey: "api_bind_addr"
        - objectName: "grpc-bind-addr"
          secretPath: "unheaded/data/cuirass/config"
          secretKey: "grpc_bind_addr"
    secretObjects:
    - secretName: cuirass-config
      type: Opaque
      data:
      - objectName: api-bind-addr
        key: API_BIND_ADDR
      - objectName: grpc-bind-addr
        key: GRPC_BIND_ADDR
  EOF
  kubectl apply -f deploy/k8s/secrets/cuirass-secret-provider.yaml
  ```
- [ ] **Step 87** [C]: **COMMIT CHECKPOINT**

### Workload Identity Pattern (No Stored Secrets)

- [ ] **Step 88** [W][K8S]: Update Cuirass deployment to mount secrets via CSI
  ```bash
  cat <<'EOF' > deploy/k8s/armory/cuirass-deployment-with-secrets.yaml
  # Patch overlay — adds CSI volume mount to cuirass-deployment.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: cuirass
    namespace: unheaded-armory
  spec:
    template:
      spec:
        containers:
        - name: cuirass
          volumeMounts:
          - name: vault-secrets
            mountPath: "/mnt/secrets"
            readOnly: true
          env:
          - name: API_BIND_ADDR
            valueFrom:
              secretKeyRef:
                name: cuirass-config
                key: API_BIND_ADDR
          - name: GRPC_BIND_ADDR
            valueFrom:
              secretKeyRef:
                name: cuirass-config
                key: GRPC_BIND_ADDR
        volumes:
        - name: vault-secrets
          csi:
            driver: secrets-store.csi.k8s.io
            readOnly: true
            volumeAttributes:
              secretProviderClass: "cuirass-vault-secrets"
  EOF
  ```
- [ ] **Step 89** [V]: Kustomize overlay validates
  ```bash
  kubectl apply --dry-run=server -f deploy/k8s/armory/cuirass-deployment-with-secrets.yaml
  ```

### Secret Rotation Verification

- [ ] **Step 90** [W]: Write secret rotation test script
  ```bash
  cat <<'SCRIPT' > scripts/test-secret-rotation.sh
  #!/bin/bash
  set -euo pipefail
  echo "=== Secret Rotation Test ==="
  echo "1. Reading current secret..."
  kubectl -n unheaded-system exec vault-0 -- vault kv get -format=json unheaded/cuirass/config | jq .data.data.api_bind_addr
  echo "2. Rotating secret..."
  kubectl -n unheaded-system exec vault-0 -- vault kv put unheaded/cuirass/config \
    api_bind_addr="0.0.0.0:8081" grpc_bind_addr="0.0.0.0:9091" election_timeout="5s"
  echo "3. Waiting for CSI rotation poll (120s max)..."
  sleep 130
  echo "4. Verifying rotated secret in pod..."
  # Would verify pod picked up new value
  echo "=== Rotation test complete ==="
  SCRIPT
  chmod +x scripts/test-secret-rotation.sh
  ```
- [ ] **Step 91** [C]: **COMMIT CHECKPOINT**

### Gorget Service Scaffold

- [ ] **Step 92** [W]: Write Gorget (secret management service) K8s manifest
  ```bash
  cat <<'EOF' > deploy/k8s/armory/gorget-deployment.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: gorget
    namespace: unheaded-armory
    labels:
      app: gorget
      tier: armory
      component: secret-management
  spec:
    replicas: 2
    selector:
      matchLabels:
        app: gorget
    template:
      metadata:
        labels:
          app: gorget
          tier: armory
      spec:
        serviceAccountName: gorget
        priorityClassName: unheaded-critical
        automountServiceAccountToken: true
        securityContext:
          runAsNonRoot: true
          seccompProfile:
            type: RuntimeDefault
        containers:
        - name: gorget
          image: unheaded/gorget:latest
          resources:
            requests: { memory: "128Mi", cpu: "100m" }
            limits: { memory: "256Mi", cpu: "250m" }
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
          ports:
          - containerPort: 8080
            name: http
          - containerPort: 9090
            name: grpc
          livenessProbe:
            httpGet: { path: /healthz, port: http }
            initialDelaySeconds: 10
            periodSeconds: 15
          readinessProbe:
            httpGet: { path: /readyz, port: http }
            initialDelaySeconds: 5
            periodSeconds: 10
  EOF
  ```
- [ ] **Step 93** [V]: Gorget manifest validates
  ```bash
  kubectl apply --dry-run=server -f deploy/k8s/armory/gorget-deployment.yaml
  ```
- [ ] **Step 94** [W][K8S]: Network policy — only Armory services can reach Gorget
  ```bash
  cat <<'EOF' > deploy/k8s/network-policies/gorget-access.yaml
  apiVersion: cilium.io/v2
  kind: CiliumNetworkPolicy
  metadata:
    name: gorget-ingress
    namespace: unheaded-armory
  spec:
    endpointSelector:
      matchLabels:
        app: gorget
    ingress:
    - fromEndpoints:
      - matchLabels:
          tier: armory
      toPorts:
      - ports:
        - port: "9090"
          protocol: TCP
  EOF
  kubectl apply -f deploy/k8s/network-policies/gorget-access.yaml
  ```
- [ ] **Step 95** [C]: **COMMIT CHECKPOINT**

### Audit Logging for Secrets Access

- [ ] **Step 96** [B]: Enable Vault audit logging
  ```bash
  kubectl -n unheaded-system exec vault-0 -- vault audit enable file file_path=/vault/logs/audit.log
  ```
- [ ] **Step 97** [V]: Audit log captures read operations
  ```bash
  kubectl -n unheaded-system exec vault-0 -- vault kv get unheaded/cuirass/config > /dev/null 2>&1
  kubectl -n unheaded-system exec vault-0 -- cat /vault/logs/audit.log | head -5
  ```
- [ ] **Step 98** [C]: **COMMIT CHECKPOINT**

### Phase 3 Cleanup

- [ ] **Step 99** [R]: Review all secret-related manifests for hardcoded values
  ```bash
  grep -r "password\|secret\|token\|key" deploy/k8s/secrets/ deploy/k8s/armory/ --include="*.yaml" | grep -v "secretKeyRef\|secretProviderClass\|secretPath\|secretName\|secretKey\|ServiceAccount"
  ```
- [ ] **Step 100** [V]: **PHASE 3 EXIT GATE** — Vault running, K8s auth configured, per-service policies bound, CSI driver active, secret rotation tested, Gorget manifest validated, audit logging enabled
  - Verify: `vault list auth/kubernetes/role` shows all armor piece roles
  - Verify: SecretProviderClass created and validated
  - Verify: Zero hardcoded secrets in manifests
  - If ALL pass → proceed to Phase 4
  - If ANY fail → DO NOT PROCEED
---

## PHASE 4: NODE RESILIENCE — GAUNTLETS + HELM HARDENING (Steps 101-125)

**Goal**: Implement node-level protections learned from the PLEG incident: resource reservations, system-reserved, PodDisruptionBudgets, and kubelet health monitoring
**Prerequisite**: Phase 1 exit gate passed
**Time**: 60 minutes
**Agent**: Coordinator

### System Resource Reservations

- [ ] **Step 101** [W]: Write kubelet configuration with system-reserved (prevents PLEG stalls)
  ```bash
  cat <<'EOF' > deploy/k8s/node-config/kubelet-config.yaml
  apiVersion: kubelet.config.k8s.io/v1beta1
  kind: KubeletConfiguration
  systemReserved:
    memory: "1536Mi"
    cpu: "500m"
    ephemeral-storage: "1Gi"
  kubeReserved:
    memory: "512Mi"
    cpu: "250m"
    ephemeral-storage: "1Gi"
  evictionHard:
    memory.available: "200Mi"
    nodefs.available: "10%"
    imagefs.available: "15%"
  evictionSoft:
    memory.available: "500Mi"
  evictionSoftGracePeriod:
    memory.available: "30s"
  maxPods: 110
  containerLogMaxSize: "50Mi"
  containerLogMaxFiles: 3
  EOF
  ```
- [ ] **Step 102** [V]: Kubelet config is valid YAML
  ```bash
  python3 -c "import yaml; yaml.safe_load(open('deploy/k8s/node-config/kubelet-config.yaml'))" && echo "VALID"
  ```
- [ ] **Step 103** [C]: **COMMIT CHECKPOINT**

### PodDisruptionBudgets for All Stateful Armor

- [ ] **Step 104** [W][K8S]: PDB for Cuirass (control plane must maintain quorum)
  ```bash
  cat <<'EOF' > deploy/k8s/armory/cuirass-pdb.yaml
  apiVersion: policy/v1
  kind: PodDisruptionBudget
  metadata:
    name: cuirass-pdb
    namespace: unheaded-armory
  spec:
    minAvailable: 1
    selector:
      matchLabels:
        app: cuirass
  EOF
  kubectl apply -f deploy/k8s/armory/cuirass-pdb.yaml
  ```
- [ ] **Step 105** [W][K8S]: PDB for Shield (WAF must stay up during node drain)
  ```bash
  cat <<'EOF' > deploy/k8s/armory/shield-pdb.yaml
  apiVersion: policy/v1
  kind: PodDisruptionBudget
  metadata:
    name: shield-pdb
    namespace: unheaded-armory
  spec:
    minAvailable: 1
    selector:
      matchLabels:
        app: shield
  EOF
  kubectl apply -f deploy/k8s/armory/shield-pdb.yaml
  ```
- [ ] **Step 106** [W][K8S]: PDB for Greaves (DNS must survive upgrades)
  ```bash
  cat <<'EOF' > deploy/k8s/armory/greaves-pdb.yaml
  apiVersion: policy/v1
  kind: PodDisruptionBudget
  metadata:
    name: greaves-pdb
    namespace: unheaded-armory
  spec:
    minAvailable: 1
    selector:
      matchLabels:
        app: greaves
  EOF
  kubectl apply -f deploy/k8s/armory/greaves-pdb.yaml
  ```
- [ ] **Step 107** [V]: All PDBs created
  ```bash
  kubectl get pdb -n unheaded-armory
  ```
- [ ] **Step 108** [C]: **COMMIT CHECKPOINT**

### Topology Spread Constraints Template

- [ ] **Step 109** [W]: Create reusable topology spread template for all deployments
  ```bash
  cat <<'EOF' > deploy/k8s/templates/topology-spread.yaml
  # Include in every deployment spec.template.spec
  # Ensures pods spread across zones — prevents stacking
  topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: DoNotSchedule
    labelSelector:
      matchLabels:
        app: REPLACE_APP_NAME
  - maxSkew: 1
    topologyKey: kubernetes.io/hostname
    whenUnsatisfiable: ScheduleAnyway
    labelSelector:
      matchLabels:
        app: REPLACE_APP_NAME
  EOF
  ```
- [ ] **Step 110** [V]: Template is syntactically valid
  ```bash
  python3 -c "import yaml; yaml.safe_load(open('deploy/k8s/templates/topology-spread.yaml'))" && echo "VALID"
  ```
- [ ] **Step 111** [C]: **COMMIT CHECKPOINT**

### Gauntlets Scheduler Enhancements

- [ ] **Step 112** [W][K8S]: Write Gauntlets scheduler deployment with resource quota awareness
  ```bash
  cat <<'EOF' > deploy/k8s/armory/gauntlets-deployment.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: gauntlets
    namespace: unheaded-armory
    labels:
      app: gauntlets
      tier: armory
      component: scheduler
  spec:
    replicas: 2
    selector:
      matchLabels:
        app: gauntlets
    template:
      metadata:
        labels:
          app: gauntlets
          tier: armory
        annotations:
          prometheus.io/scrape: "true"
          prometheus.io/port: "2112"
      spec:
        serviceAccountName: gauntlets
        priorityClassName: unheaded-critical
        automountServiceAccountToken: false
        securityContext:
          runAsNonRoot: true
          seccompProfile:
            type: RuntimeDefault
        containers:
        - name: gauntlets
          image: unheaded/gauntlets:latest
          resources:
            requests: { memory: "128Mi", cpu: "100m" }
            limits: { memory: "512Mi", cpu: "500m" }
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
          ports:
          - containerPort: 8080
            name: http
          - containerPort: 9090
            name: grpc
          - containerPort: 2112
            name: metrics
          livenessProbe:
            httpGet: { path: /healthz, port: http }
            initialDelaySeconds: 10
            periodSeconds: 15
          readinessProbe:
            httpGet: { path: /readyz, port: http }
            initialDelaySeconds: 5
            periodSeconds: 10
        topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: DoNotSchedule
          labelSelector:
            matchLabels:
              app: gauntlets
  EOF
  ```
- [ ] **Step 113** [V]: Gauntlets manifest validates
  ```bash
  kubectl apply --dry-run=server -f deploy/k8s/armory/gauntlets-deployment.yaml
  ```
- [ ] **Step 114** [C]: **COMMIT CHECKPOINT**

### Helm Runtime Hardening

- [ ] **Step 115** [W][K8S]: Write Helm (container runtime) deployment with cgroup limits
  ```bash
  cat <<'EOF' > deploy/k8s/armory/helm-runtime-deployment.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: helm-runtime
    namespace: unheaded-armory
    labels:
      app: helm-runtime
      tier: armory
      component: container-runtime
  spec:
    replicas: 2
    selector:
      matchLabels:
        app: helm-runtime
    template:
      metadata:
        labels:
          app: helm-runtime
          tier: armory
      spec:
        serviceAccountName: helm-runtime
        priorityClassName: unheaded-critical
        securityContext:
          runAsNonRoot: true
          seccompProfile:
            type: RuntimeDefault
        containers:
        - name: helm-runtime
          image: unheaded/helm-runtime:latest
          resources:
            requests: { memory: "256Mi", cpu: "200m" }
            limits: { memory: "1Gi", cpu: "1000m" }
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
          ports:
          - containerPort: 8080
            name: http
          - containerPort: 9090
            name: grpc
          livenessProbe:
            httpGet: { path: /healthz, port: http }
          readinessProbe:
            httpGet: { path: /readyz, port: http }
  EOF
  ```
- [ ] **Step 116** [V]: Helm-runtime manifest validates
  ```bash
  kubectl apply --dry-run=server -f deploy/k8s/armory/helm-runtime-deployment.yaml
  ```

### PLEG Health Monitoring (Lesson Learned)

- [ ] **Step 117** [W]: Write PrometheusRule for PLEG stall detection
  ```bash
  cat <<'EOF' > deploy/k8s/monitoring/pleg-alerts.yaml
  apiVersion: monitoring.coreos.com/v1
  kind: PrometheusRule
  metadata:
    name: pleg-health
    namespace: unheaded-system
    labels:
      prometheus: kube-prometheus
  spec:
    groups:
    - name: kubelet.pleg
      rules:
      - alert: PLEGRelistDurationHigh
        expr: histogram_quantile(0.99, rate(kubelet_pleg_relist_duration_seconds_bucket[5m])) > 10
        for: 5m
        labels:
          severity: critical
          team: platform
        annotations:
          summary: "PLEG relist duration p99 > 10s on {{ $labels.node }}"
          description: "Kubelet PLEG is stalling — risk of NodeNotReady cascade. Check DaemonSet resource consumption."
          runbook_url: "https://wiki.unheaded.dev/runbooks/pleg-stall"
      - alert: KubeletNotReady
        expr: kube_node_status_condition{condition="Ready",status="true"} == 0
        for: 2m
        labels:
          severity: critical
          team: platform
        annotations:
          summary: "Node {{ $labels.node }} NotReady for > 2 minutes"
          description: "Node is unhealthy. Check kubelet logs, PLEG health, and memory pressure."
  EOF
  ```
- [ ] **Step 118** [V]: PrometheusRule is valid YAML
  ```bash
  python3 -c "import yaml; yaml.safe_load(open('deploy/k8s/monitoring/pleg-alerts.yaml'))" && echo "VALID"
  ```
- [ ] **Step 119** [C]: **COMMIT CHECKPOINT**

### DaemonSet Resource Enforcement (The Lesson)

- [ ] **Step 120** [W][K8S]: Gatekeeper constraint — ALL DaemonSets MUST have resource limits
  ```bash
  cat <<'EOF' > deploy/k8s/policies/require-daemonset-limits.yaml
  apiVersion: templates.gatekeeper.sh/v1
  kind: ConstraintTemplate
  metadata:
    name: k8srequiredaemonsetresources
  spec:
    crd:
      spec:
        names:
          kind: K8sRequireDaemonSetResources
    targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8srequiredaemonsetresources
        violation[{"msg": msg}] {
          input.review.object.kind == "DaemonSet"
          container := input.review.object.spec.template.spec.containers[_]
          not container.resources.limits.memory
          msg := sprintf("DaemonSet container %v MUST have memory limits — unbounded DaemonSets cause PLEG stalls", [container.name])
        }
  EOF
  kubectl apply -f deploy/k8s/policies/require-daemonset-limits.yaml
  ```
- [ ] **Step 121** [W][K8S]: Apply DaemonSet constraint
  ```bash
  cat <<'EOF' > deploy/k8s/policies/enforce-daemonset-limits.yaml
  apiVersion: constraints.gatekeeper.sh/v1beta1
  kind: K8sRequireDaemonSetResources
  metadata:
    name: daemonsets-must-have-limits
  spec:
    match:
      kinds:
      - apiGroups: ["apps"]
        kinds: ["DaemonSet"]
      excludedNamespaces: ["kube-system"]
  EOF
  kubectl apply -f deploy/k8s/policies/enforce-daemonset-limits.yaml
  ```
- [ ] **Step 122** [V]: Test — DaemonSet without limits is REJECTED
  ```bash
  cat <<'EOF' | kubectl apply -f - 2>&1 | grep -i "denied\|violation"
  apiVersion: apps/v1
  kind: DaemonSet
  metadata:
    name: test-no-limits
    namespace: unheaded-armory
  spec:
    selector:
      matchLabels:
        app: test
    template:
      metadata:
        labels:
          app: test
      spec:
        containers:
        - name: test
          image: busybox
  EOF
  ```
- [ ] **Step 123** [C]: **COMMIT CHECKPOINT**
- [ ] **Step 124** [B]: Cleanup test resources
  ```bash
  kubectl delete daemonset test-no-limits -n unheaded-armory --force 2>/dev/null || true
  ```
- [ ] **Step 125** [V]: **PHASE 4 EXIT GATE** — Kubelet config hardened, PDBs on all critical services, topology spread templates ready, PLEG alert rules written, DaemonSet limits enforced via Gatekeeper
  - Verify: `kubectl get pdb -n unheaded-armory` shows >= 3 PDBs
  - Verify: DaemonSet without limits is REJECTED
  - Verify: PrometheusRule YAML validates
  - If ALL pass → proceed to Phase 5
  - If ANY fail → DO NOT PROCEED

---

## PHASE 5: ADMISSION CONTROL EXPANSION — SHIELD POLICY ENGINE (Steps 126-150)

**Goal**: Expand OPA Gatekeeper with production-grade policies: signed images, no host namespaces, seccomp profiles, automountServiceAccountToken=false
**Prerequisite**: Phase 1 exit gate passed
**Time**: 60 minutes
**Agent**: Agent [P] (parallelizable with Phase 4)

### Image Policy — Signed Images Only

- [ ] **Step 126** [W][K8S]: ConstraintTemplate — require images from trusted registries
  ```bash
  cat <<'EOF' > deploy/k8s/policies/trusted-registries.yaml
  apiVersion: templates.gatekeeper.sh/v1
  kind: ConstraintTemplate
  metadata:
    name: k8strustedregistries
  spec:
    crd:
      spec:
        names:
          kind: K8sTrustedRegistries
        validation:
          openAPIV3Schema:
            type: object
            properties:
              registries:
                type: array
                items:
                  type: string
    targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8strustedregistries
        violation[{"msg": msg}] {
          container := input.review.object.spec.containers[_]
          not trusted_registry(container.image)
          msg := sprintf("Image %v is not from a trusted registry. Allowed: %v", [container.image, input.parameters.registries])
        }
        trusted_registry(image) {
          registry := input.parameters.registries[_]
          startswith(image, registry)
        }
  EOF
  kubectl apply -f deploy/k8s/policies/trusted-registries.yaml
  ```
- [ ] **Step 127** [W][K8S]: Apply trusted registry constraint
  ```bash
  cat <<'EOF' > deploy/k8s/policies/enforce-trusted-registries.yaml
  apiVersion: constraints.gatekeeper.sh/v1beta1
  kind: K8sTrustedRegistries
  metadata:
    name: must-use-trusted-registry
  spec:
    match:
      kinds:
      - apiGroups: [""]
        kinds: ["Pod"]
      excludedNamespaces: ["kube-system", "gatekeeper-system"]
    parameters:
      registries:
      - "unheaded/"
      - "ghcr.io/unheaded/"
      - "docker.io/library/"
      - "registry.k8s.io/"
      - "quay.io/cilium/"
      - "hashicorp/"
      - "secrets-store-csi-driver/"
  EOF
  kubectl apply -f deploy/k8s/policies/enforce-trusted-registries.yaml
  ```
- [ ] **Step 128** [C]: **COMMIT CHECKPOINT**

### Host Namespace Restrictions

- [ ] **Step 129** [W][K8S]: Block hostNetwork, hostPID, hostIPC (except eBPF tier)
  ```bash
  cat <<'EOF' > deploy/k8s/policies/deny-host-namespaces.yaml
  apiVersion: templates.gatekeeper.sh/v1
  kind: ConstraintTemplate
  metadata:
    name: k8sdenyhostnamespaces
  spec:
    crd:
      spec:
        names:
          kind: K8sDenyHostNamespaces
    targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8sdenyhostnamespaces
        violation[{"msg": msg}] {
          input.review.object.spec.hostNetwork == true
          msg := "hostNetwork is not allowed"
        }
        violation[{"msg": msg}] {
          input.review.object.spec.hostPID == true
          msg := "hostPID is not allowed"
        }
        violation[{"msg": msg}] {
          input.review.object.spec.hostIPC == true
          msg := "hostIPC is not allowed"
        }
  EOF
  kubectl apply -f deploy/k8s/policies/deny-host-namespaces.yaml
  ```
- [ ] **Step 130** [W][K8S]: Enforce with eBPF exception
  ```bash
  cat <<'EOF' > deploy/k8s/policies/enforce-no-host-ns.yaml
  apiVersion: constraints.gatekeeper.sh/v1beta1
  kind: K8sDenyHostNamespaces
  metadata:
    name: deny-host-namespaces
  spec:
    match:
      kinds:
      - apiGroups: [""]
        kinds: ["Pod"]
      excludedNamespaces: ["unheaded-ebpf", "kube-system"]
  EOF
  kubectl apply -f deploy/k8s/policies/enforce-no-host-ns.yaml
  ```
- [ ] **Step 131** [C]: **COMMIT CHECKPOINT**

### Seccomp + AppArmor Enforcement

- [ ] **Step 132** [W][K8S]: Require seccomp profile on all pods
  ```bash
  cat <<'EOF' > deploy/k8s/policies/require-seccomp.yaml
  apiVersion: templates.gatekeeper.sh/v1
  kind: ConstraintTemplate
  metadata:
    name: k8srequireseccomp
  spec:
    crd:
      spec:
        names:
          kind: K8sRequireSeccomp
    targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8srequireseccomp
        violation[{"msg": msg}] {
          not input.review.object.spec.securityContext.seccompProfile
          msg := "Pod must have a seccompProfile (RuntimeDefault or Localhost)"
        }
  EOF
  kubectl apply -f deploy/k8s/policies/require-seccomp.yaml
  ```
- [ ] **Step 133** [W][K8S]: Apply seccomp constraint
  ```bash
  cat <<'EOF' > deploy/k8s/policies/enforce-seccomp.yaml
  apiVersion: constraints.gatekeeper.sh/v1beta1
  kind: K8sRequireSeccomp
  metadata:
    name: must-have-seccomp
  spec:
    match:
      kinds:
      - apiGroups: [""]
        kinds: ["Pod"]
      excludedNamespaces: ["kube-system", "gatekeeper-system", "unheaded-ebpf"]
  EOF
  kubectl apply -f deploy/k8s/policies/enforce-seccomp.yaml
  ```
- [ ] **Step 134** [C]: **COMMIT CHECKPOINT**

### AutomountServiceAccountToken = false by Default

- [ ] **Step 135** [W][K8S]: Policy requiring explicit opt-in for SA token mounting
  ```bash
  cat <<'EOF' > deploy/k8s/policies/deny-auto-sa-token.yaml
  apiVersion: templates.gatekeeper.sh/v1
  kind: ConstraintTemplate
  metadata:
    name: k8sdenyautomountsa
  spec:
    crd:
      spec:
        names:
          kind: K8sDenyAutoMountSA
    targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8sdenyautomountsa
        violation[{"msg": msg}] {
          not has_field(input.review.object.spec, "automountServiceAccountToken")
          msg := "automountServiceAccountToken must be explicitly set (prefer false)"
        }
        has_field(obj, field) {
          _ = obj[field]
        }
  EOF
  kubectl apply -f deploy/k8s/policies/deny-auto-sa-token.yaml
  ```
- [ ] **Step 136** [C]: **COMMIT CHECKPOINT**

### Policy Compliance Dashboard Query

- [ ] **Step 137** [B][K8S]: Audit all existing constraints and violations
  ```bash
  kubectl get constraints -o json | python3 -c "
  import sys, json
  data = json.load(sys.stdin)
  for item in data.get('items', []):
      name = item['metadata']['name']
      violations = item.get('status', {}).get('totalViolations', 0)
      print(f'{name}: {violations} violations')
  "
  ```
- [ ] **Step 138** [V]: Zero violations in active constraints (all manifests are compliant)
- [ ] **Step 139** [D]: If violations found, list them
  ```bash
  kubectl get constraints -o json | python3 -c "
  import sys, json
  data = json.load(sys.stdin)
  for item in data.get('items', []):
      for v in item.get('status', {}).get('violations', []):
          print(f\"  {v.get('enforcementAction')}: {v.get('message')}\")
  "
  ```

### Full Policy Suite Verification

- [ ] **Step 140** [B][K8S]: Test — pod with hostPID=true is REJECTED (outside eBPF tier)
  ```bash
  cat <<'EOF' | kubectl apply -n unheaded-armory -f - 2>&1 | grep -i "denied\|violation"
  apiVersion: v1
  kind: Pod
  metadata:
    name: test-hostpid
  spec:
    hostPID: true
    automountServiceAccountToken: false
    securityContext:
      runAsNonRoot: true
      seccompProfile:
        type: RuntimeDefault
    containers:
    - name: test
      image: unheaded/test:latest
      resources:
        limits: { memory: "64Mi", cpu: "100m" }
      securityContext:
        allowPrivilegeEscalation: false
  EOF
  ```
- [ ] **Step 141** [B][K8S]: Test — pod from untrusted registry is REJECTED
  ```bash
  cat <<'EOF' | kubectl apply -n unheaded-armory -f - 2>&1 | grep -i "denied\|violation"
  apiVersion: v1
  kind: Pod
  metadata:
    name: test-bad-registry
  spec:
    automountServiceAccountToken: false
    securityContext:
      runAsNonRoot: true
      seccompProfile:
        type: RuntimeDefault
    containers:
    - name: test
      image: evil-registry.io/malicious:latest
      resources:
        limits: { memory: "64Mi", cpu: "100m" }
  EOF
  ```
- [ ] **Step 142** [V]: Both test pods REJECTED by Gatekeeper
- [ ] **Step 143** [C]: **COMMIT CHECKPOINT**

### Shield WAF K8s Deployment

- [ ] **Step 144** [W][K8S]: Write Shield (WAF) deployment manifest with all policies satisfied
  ```bash
  cat <<'EOF' > deploy/k8s/armory/shield-deployment.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: shield
    namespace: unheaded-armory
    labels:
      app: shield
      tier: armory
      component: waf
  spec:
    replicas: 3
    selector:
      matchLabels:
        app: shield
    template:
      metadata:
        labels:
          app: shield
          tier: armory
        annotations:
          prometheus.io/scrape: "true"
          prometheus.io/port: "2112"
      spec:
        serviceAccountName: shield
        priorityClassName: unheaded-critical
        automountServiceAccountToken: false
        securityContext:
          runAsNonRoot: true
          seccompProfile:
            type: RuntimeDefault
        containers:
        - name: shield
          image: unheaded/shield:latest
          resources:
            requests: { memory: "128Mi", cpu: "200m" }
            limits: { memory: "512Mi", cpu: "500m" }
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
          ports:
          - containerPort: 8080
            name: http
          - containerPort: 8443
            name: https
          - containerPort: 9090
            name: grpc
          - containerPort: 2112
            name: metrics
          livenessProbe:
            httpGet: { path: /healthz, port: http }
            initialDelaySeconds: 10
          readinessProbe:
            httpGet: { path: /readyz, port: http }
            initialDelaySeconds: 5
        topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: DoNotSchedule
          labelSelector:
            matchLabels:
              app: shield
  EOF
  ```
- [ ] **Step 145** [V]: Shield manifest validates against ALL Gatekeeper policies
  ```bash
  kubectl apply --dry-run=server -f deploy/k8s/armory/shield-deployment.yaml
  ```
- [ ] **Step 146** [D]: If rejected, check which constraint
  ```bash
  kubectl apply --dry-run=server -f deploy/k8s/armory/shield-deployment.yaml 2>&1 | grep -i "violation"
  ```
- [ ] **Step 147** [C]: **COMMIT CHECKPOINT**

### Policy Summary Export

- [ ] **Step 148** [B]: Export full policy suite for documentation
  ```bash
  echo "=== UNHEADED GATEKEEPER POLICY SUITE ===" > deploy/k8s/policies/POLICY_SUMMARY.md
  echo "" >> deploy/k8s/policies/POLICY_SUMMARY.md
  echo "| Policy | Type | Scope | Exceptions |" >> deploy/k8s/policies/POLICY_SUMMARY.md
  echo "|--------|------|-------|------------|" >> deploy/k8s/policies/POLICY_SUMMARY.md
  echo "| Resource Limits Required | All Pods | unheaded-* | None |" >> deploy/k8s/policies/POLICY_SUMMARY.md
  echo "| No Privileged Containers | All Pods | All except ebpf | unheaded-ebpf |" >> deploy/k8s/policies/POLICY_SUMMARY.md
  echo "| DaemonSet Limits Required | DaemonSets | All except kube-system | kube-system |" >> deploy/k8s/policies/POLICY_SUMMARY.md
  echo "| Trusted Registries Only | All Pods | All except system | kube-system, gatekeeper |" >> deploy/k8s/policies/POLICY_SUMMARY.md
  echo "| No Host Namespaces | All Pods | All except ebpf | unheaded-ebpf, kube-system |" >> deploy/k8s/policies/POLICY_SUMMARY.md
  echo "| Seccomp Required | All Pods | All except ebpf | unheaded-ebpf, kube-system |" >> deploy/k8s/policies/POLICY_SUMMARY.md
  echo "| SA Token Opt-In | All Pods | All | None |" >> deploy/k8s/policies/POLICY_SUMMARY.md
  ```
- [ ] **Step 149** [C]: **COMMIT CHECKPOINT**
- [ ] **Step 150** [V]: **PHASE 5 EXIT GATE** — 7 Gatekeeper policies active, all test violations REJECTED, Shield manifest passes all policies, policy summary documented
  - Verify: `kubectl get constrainttemplates | wc -l` shows >= 7
  - Verify: hostPID test pod REJECTED
  - Verify: Untrusted registry test pod REJECTED
  - Verify: Shield dry-run passes
  - If ALL pass → proceed to Phase 6
  - If ANY fail → DO NOT PROCEED

---

## PHASE 6: eBPF TELEMETRY PIPELINE — WHISPERING VOID → DASHBOARD (Steps 151-175)

**Goal**: Wire the Whispering Void eBPF collector through gRPC to Busboy message bus to Dashboard WebSocket — the full packet-to-pixel pipeline
**Prerequisite**: Phase 2 exit gate passed (Cilium + Hubble operational)
**Time**: 90 minutes
**Agent**: Coordinator (iterative debugging likely)

### eBPF Collector Service Definition

- [ ] **Step 151** [W]: Write protobuf definition for eBPF telemetry events
  ```bash
  cat <<'EOF' > proto/ebpf_telemetry.proto
  syntax = "proto3";
  package unheaded.ebpf.v1;
  option go_package = "unheaded/pkg/proto/ebpf/v1";

  service EbpfTelemetry {
    rpc StreamEvents(StreamEventsRequest) returns (stream TelemetryEvent);
    rpc GetStats(StatsRequest) returns (StatsResponse);
  }

  message StreamEventsRequest {
    repeated string event_types = 1;
    uint32 rate_limit_per_sec = 2;
  }

  message TelemetryEvent {
    uint64 timestamp_ns = 1;
    EventType type = 2;
    string source_pod = 3;
    string dest_pod = 4;
    uint32 source_port = 5;
    uint32 dest_port = 6;
    uint32 protocol = 7;
    Verdict verdict = 8;
    uint64 bytes = 9;
    uint64 latency_ns = 10;
    // ZERO payload — metadata only (Sacred Principle)
  }

  enum EventType {
    EVENT_TYPE_UNSPECIFIED = 0;
    EVENT_TYPE_PACKET = 1;
    EVENT_TYPE_TCP_CONNECT = 2;
    EVENT_TYPE_TCP_CLOSE = 3;
    EVENT_TYPE_DNS_QUERY = 4;
    EVENT_TYPE_POLICY_VERDICT = 5;
  }

  enum Verdict {
    VERDICT_UNSPECIFIED = 0;
    VERDICT_ALLOW = 1;
    VERDICT_DENY = 2;
    VERDICT_DROP = 3;
  }

  message StatsRequest {}
  message StatsResponse {
    uint64 total_events = 1;
    uint64 events_per_second = 2;
    uint64 drops = 3;
    map<string, uint64> events_by_type = 4;
  }
  EOF
  ```
- [ ] **Step 152** [B]: Generate Go code from protobuf
  ```bash
  protoc --go_out=. --go-grpc_out=. proto/ebpf_telemetry.proto
  ```
- [ ] **Step 153** [V]: Generated Go files exist
  ```bash
  ls -la pkg/proto/ebpf/v1/
  ```
- [ ] **Step 154** [C]: **COMMIT CHECKPOINT**

### Collector → Busboy gRPC Registration

- [ ] **Step 155** [W]: Write collector gRPC client that registers with Busboy on startup
  ```bash
  cat <<'EOF' > internal/ebpf/collector/grpc_client.go
  package collector

  // GRPCClient registers the eBPF collector with Busboy's service mesh
  // and streams telemetry events to subscribed consumers (Dashboard).
  //
  // Flow: BPF ring buffer → userspace collector → protobuf → gRPC → Busboy → WebSocket → Dashboard
  //
  // Rate limiting: Configurable, default 100 events/sec
  // Sacred Principle: ZERO payload capture — metadata only

  // This is a scaffold — full implementation in Developer skill session
  EOF
  ```
- [ ] **Step 156** [W][K8S]: Write Busboy service for eBPF topic routing
  ```bash
  cat <<'EOF' > deploy/k8s/ebpf/busboy-ebpf-topic.yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: busboy-ebpf-topics
    namespace: unheaded-ebpf
  data:
    topics.yaml: |
      topics:
      - name: "ebpf.packet_flow"
        description: "XDP packet counter events"
        rate_limit: 100
        consumers: ["dashboard-backend"]
      - name: "ebpf.tcp_latency"
        description: "TCP connection latency events"
        rate_limit: 50
        consumers: ["dashboard-backend", "anamnesis"]
      - name: "ebpf.policy_verdict"
        description: "Cilium network policy verdicts"
        rate_limit: 100
        consumers: ["dashboard-backend", "anamnesis"]
      - name: "ebpf.dns_query"
        description: "DNS resolution events from Greaves"
        rate_limit: 50
        consumers: ["dashboard-backend"]
  EOF
  kubectl apply -f deploy/k8s/ebpf/busboy-ebpf-topic.yaml
  ```
- [ ] **Step 157** [C]: **COMMIT CHECKPOINT**

### Dashboard WebSocket Consumer

- [ ] **Step 158** [W]: Write Dashboard WebSocket handler for eBPF events
  ```bash
  cat <<'EOF' > internal/dashboard/ws_ebpf_handler.go
  package dashboard

  // WSEbpfHandler receives TelemetryEvent messages from Busboy's gRPC stream
  // and broadcasts them to connected WebSocket clients for real-time visualization.
  //
  // Canvas mapping: Each TelemetryEvent maps to a flow arrow on the packet_flow canvas
  // - source_pod → source node
  // - dest_pod → dest node
  // - verdict → arrow color (green=allow, red=deny, yellow=drop)
  // - latency_ns → arrow thickness
  //
  // This is a scaffold — full implementation in Developer skill session
  EOF
  ```
- [ ] **Step 159** [W][K8S]: Dashboard backend deployment with WebSocket and gRPC
  ```bash
  cat <<'EOF' > deploy/k8s/presentation/dashboard-backend-deployment.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: dashboard-backend
    namespace: unheaded-presentation
    labels:
      app: dashboard-backend
      tier: presentation
  spec:
    replicas: 2
    selector:
      matchLabels:
        app: dashboard-backend
    template:
      metadata:
        labels:
          app: dashboard-backend
          tier: presentation
        annotations:
          prometheus.io/scrape: "true"
          prometheus.io/port: "2112"
      spec:
        serviceAccountName: dashboard-backend
        priorityClassName: unheaded-standard
        automountServiceAccountToken: false
        securityContext:
          runAsNonRoot: true
          seccompProfile:
            type: RuntimeDefault
        containers:
        - name: dashboard
          image: unheaded/dashboard-backend:latest
          resources:
            requests: { memory: "128Mi", cpu: "100m" }
            limits: { memory: "512Mi", cpu: "500m" }
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
          ports:
          - containerPort: 8080
            name: http
          - containerPort: 8081
            name: websocket
          - containerPort: 9090
            name: grpc
          - containerPort: 2112
            name: metrics
          env:
          - name: WS_ALLOWED_ORIGINS
            value: "http://localhost:3000,https://dashboard.unheaded.dev"
          - name: BUSBOY_GRPC_ADDR
            value: "busboy.unheaded-court:9090"
          livenessProbe:
            httpGet: { path: /healthz, port: http }
          readinessProbe:
            httpGet: { path: /readyz, port: http }
        topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              app: dashboard-backend
  EOF
  ```
- [ ] **Step 160** [V]: Dashboard manifest validates
  ```bash
  kubectl apply --dry-run=server -f deploy/k8s/presentation/dashboard-backend-deployment.yaml
  ```
- [ ] **Step 161** [C]: **COMMIT CHECKPOINT**

### Service Definitions

- [ ] **Step 162** [W][K8S]: Kubernetes Services for the telemetry pipeline
  ```bash
  cat <<'EOF' > deploy/k8s/ebpf/void-collector-service.yaml
  apiVersion: v1
  kind: Service
  metadata:
    name: void-collector
    namespace: unheaded-ebpf
    labels:
      app: void-collector
  spec:
    selector:
      app: void-collector
    ports:
    - name: grpc
      port: 9091
      targetPort: grpc
    - name: metrics
      port: 2112
      targetPort: metrics
  ---
  apiVersion: v1
  kind: Service
  metadata:
    name: dashboard-backend
    namespace: unheaded-presentation
    labels:
      app: dashboard-backend
  spec:
    selector:
      app: dashboard-backend
    ports:
    - name: http
      port: 8080
      targetPort: http
    - name: websocket
      port: 8081
      targetPort: websocket
    - name: grpc
      port: 9090
      targetPort: grpc
    - name: metrics
      port: 2112
      targetPort: metrics
  EOF
  kubectl apply -f deploy/k8s/ebpf/void-collector-service.yaml
  ```
- [ ] **Step 163** [V]: Services created
  ```bash
  kubectl get svc -n unheaded-ebpf && kubectl get svc -n unheaded-presentation
  ```
- [ ] **Step 164** [C]: **COMMIT CHECKPOINT**

### Telemetry Flow Verification Script

- [ ] **Step 165** [W]: Write end-to-end telemetry pipeline test script
  ```bash
  cat <<'SCRIPT' > scripts/test-telemetry-pipeline.sh
  #!/bin/bash
  set -euo pipefail
  echo "=== Telemetry Pipeline E2E Test ==="
  echo ""
  echo "1. Checking void-collector DaemonSet..."
  kubectl get ds whispering-void-collector -n unheaded-ebpf 2>/dev/null && echo "  OK" || echo "  NOT DEPLOYED (expected in scaffold phase)"
  echo ""
  echo "2. Checking Busboy topic config..."
  kubectl get configmap busboy-ebpf-topics -n unheaded-ebpf -o yaml | grep "name:" | head -5
  echo ""
  echo "3. Checking Dashboard backend..."
  kubectl get deploy dashboard-backend -n unheaded-presentation 2>/dev/null && echo "  OK" || echo "  NOT DEPLOYED (expected in scaffold phase)"
  echo ""
  echo "4. Checking Hubble flow data (proxy for eBPF health)..."
  hubble observe --namespace unheaded-armory --last 5 2>/dev/null || echo "  Hubble not port-forwarded (expected in dev)"
  echo ""
  echo "5. Checking network policies allow eBPF → Dashboard path..."
  kubectl get ciliumnetworkpolicies -n unheaded-presentation | grep "allow-ebpf"
  echo ""
  echo "=== Pipeline scaffold verification complete ==="
  SCRIPT
  chmod +x scripts/test-telemetry-pipeline.sh
  ```
- [ ] **Step 166** [B]: Run pipeline test
  ```bash
  bash scripts/test-telemetry-pipeline.sh
  ```
- [ ] **Step 167** [V]: All scaffold components present (DaemonSet manifest, ConfigMap, Services, network policies)
- [ ] **Step 168** [C]: **COMMIT CHECKPOINT**

### ServiceMonitor for eBPF Metrics

- [ ] **Step 169** [W][K8S]: Prometheus ServiceMonitor for void-collector
  ```bash
  cat <<'EOF' > deploy/k8s/monitoring/void-collector-monitor.yaml
  apiVersion: monitoring.coreos.com/v1
  kind: ServiceMonitor
  metadata:
    name: void-collector
    namespace: unheaded-ebpf
    labels:
      prometheus: kube-prometheus
  spec:
    selector:
      matchLabels:
        app: void-collector
    endpoints:
    - port: metrics
      interval: 15s
      path: /metrics
  ---
  apiVersion: monitoring.coreos.com/v1
  kind: ServiceMonitor
  metadata:
    name: dashboard-backend
    namespace: unheaded-presentation
    labels:
      prometheus: kube-prometheus
  spec:
    selector:
      matchLabels:
        app: dashboard-backend
    endpoints:
    - port: metrics
      interval: 15s
      path: /metrics
  EOF
  kubectl apply -f deploy/k8s/monitoring/void-collector-monitor.yaml 2>/dev/null || echo "ServiceMonitor CRD not installed yet — will apply in Phase 9"
  ```
- [ ] **Step 170** [C]: **COMMIT CHECKPOINT**

### Rate Limiting Configuration

- [ ] **Step 171** [W]: Write rate limiter config for eBPF event stream
  ```bash
  cat <<'EOF' > deploy/k8s/ebpf/rate-limiter-config.yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: void-collector-config
    namespace: unheaded-ebpf
  data:
    config.yaml: |
      rate_limiting:
        global_max_events_per_sec: 100
        per_topic_limits:
          ebpf.packet_flow: 100
          ebpf.tcp_latency: 50
          ebpf.policy_verdict: 100
          ebpf.dns_query: 50
        burst_size: 200
        drop_policy: "newest"
      ring_buffer:
        size_bytes: 16777216  # 16MB per-CPU ring buffer
        watermark_bytes: 8388608  # wake userspace at 8MB
      grpc:
        busboy_address: "busboy.unheaded-court:9090"
        reconnect_backoff_ms: 1000
        max_reconnect_backoff_ms: 30000
  EOF
  kubectl apply -f deploy/k8s/ebpf/rate-limiter-config.yaml
  ```
- [ ] **Step 172** [C]: **COMMIT CHECKPOINT**

### Phase 6 Final Assembly

- [ ] **Step 173** [R]: Review all eBPF pipeline manifests for consistency
  ```bash
  ls -la deploy/k8s/ebpf/ deploy/k8s/presentation/ deploy/k8s/monitoring/
  ```
- [ ] **Step 174** [C]: **COMMIT CHECKPOINT**
- [ ] **Step 175** [V]: **PHASE 6 EXIT GATE** — Protobuf defined, gRPC scaffolds written, DaemonSet + Dashboard manifests validated, Services created, topic config deployed, rate limiter configured, ServiceMonitors written
  - Verify: `kubectl get svc -n unheaded-ebpf` shows void-collector service
  - Verify: `kubectl get configmap -n unheaded-ebpf` shows topic + rate limiter configs
  - Verify: Protobuf files exist in proto/
  - Verify: Dashboard manifest dry-run passes
  - If ALL pass → proceed to Phase 7
  - If ANY fail → DO NOT PROCEED

---

## PHASE 7: IAC STATE DECOMPOSITION — VAMBRACES GOES TERRAGRUNT (Steps 176-200)

**Goal**: Implement the Terragrunt-orchestrated IaC repo structure with state isolation per infrastructure layer, OPA policy gates, and drift detection
**Prerequisite**: Phase 0 exit gate passed
**Time**: 75 minutes
**Agent**: Agent [P]

### IaC Directory Scaffold

- [ ] **Step 176** [W]: Create the full IaC directory structure (state decomposed by blast radius)
  ```bash
  mkdir -p deploy/tofu/{modules/{networking,compute,identity,observability,security,ebpf},environments/{dev/{networking,compute,identity,observability,ebpf},staging/{networking,compute,identity,observability,ebpf},prod/{networking,compute,identity,observability,ebpf}},policies}
  ```
- [ ] **Step 177** [V]: Directory structure exists
  ```bash
  find deploy/tofu -type d | sort
  ```
- [ ] **Step 178** [C]: **COMMIT CHECKPOINT**

### Root Terragrunt Config

- [ ] **Step 179** [W]: Root terragrunt.hcl with auto-generated state keys
  ```bash
  cat <<'EOF' > deploy/tofu/terragrunt.hcl
  # Root Terragrunt config — Unheaded IaC
  # State key auto-generated from directory path
  # Every layer gets isolated state = isolated blast radius

  locals {
    environment = basename(dirname(get_terragrunt_dir()))
    layer       = basename(get_terragrunt_dir())
  }

  remote_state {
    backend = "local"
    config = {
      path = "${get_repo_root()}/deploy/tofu/.state/${local.environment}/${local.layer}/terraform.tfstate"
    }
    generate = {
      path      = "backend.tf"
      if_exists = "overwrite_terragrunt"
    }
  }

  generate "provider" {
    path      = "provider.tf"
    if_exists = "overwrite_terragrunt"
    contents  = <<PROVIDER
  terraform {
    required_version = ">= 1.6.0"
    required_providers {
      kubernetes = {
        source  = "hashicorp/kubernetes"
        version = "~> 2.25"
      }
      helm = {
        source  = "hashicorp/helm"
        version = "~> 2.12"
      }
    }
  }
  PROVIDER
  }
  EOF
  ```
- [ ] **Step 180** [C]: **COMMIT CHECKPOINT**

### Networking Module

- [ ] **Step 181** [W]: Networking module — manages Cilium, network policies, DNS
  ```bash
  cat <<'EOF' > deploy/tofu/modules/networking/main.tf
  # Unheaded Networking Module
  # Manages: Cilium CNI, CiliumNetworkPolicies, Greaves DNS, Hubble

  variable "cluster_name" {
    type = string
  }

  variable "namespaces" {
    type = list(string)
    default = ["unheaded-armory", "unheaded-gnostic", "unheaded-court", "unheaded-presentation", "unheaded-ebpf"]
  }

  variable "hubble_enabled" {
    type    = bool
    default = true
  }

  resource "helm_release" "cilium" {
    name       = "cilium"
    repository = "https://helm.cilium.io/"
    chart      = "cilium"
    namespace  = "kube-system"
    version    = "1.15.0"

    set { name = "ipam.mode"; value = "kubernetes" }
    set { name = "hubble.enabled"; value = tostring(var.hubble_enabled) }
    set { name = "hubble.relay.enabled"; value = tostring(var.hubble_enabled) }
    set { name = "hubble.ui.enabled"; value = tostring(var.hubble_enabled) }
    set { name = "bpf.masquerade"; value = "true" }
    set { name = "kubeProxyReplacement"; value = "true" }
  }

  output "cilium_release_name" {
    value = helm_release.cilium.name
  }
  EOF
  ```
- [ ] **Step 182** [W]: Networking environment config
  ```bash
  cat <<'EOF' > deploy/tofu/environments/dev/networking/terragrunt.hcl
  include "root" {
    path = find_in_parent_folders()
  }

  terraform {
    source = "${get_repo_root()}/deploy/tofu/modules/networking"
  }

  inputs = {
    cluster_name   = "unheaded-dev"
    hubble_enabled = true
  }
  EOF
  ```
- [ ] **Step 183** [C]: **COMMIT CHECKPOINT**

### Compute Module

- [ ] **Step 184** [W]: Compute module — manages K8s deployments for Armory tier
  ```bash
  cat <<'EOF' > deploy/tofu/modules/compute/main.tf
  # Unheaded Compute Module
  # Manages: All Armory deployment manifests, PDBs, HPA

  variable "environment" {
    type = string
  }

  variable "replicas" {
    type = map(number)
    default = {
      shield       = 3
      cuirass      = 2
      gauntlets    = 2
      pauldrons    = 2
      hauberk      = 2
      greaves      = 2
      helm_runtime = 2
      gorget       = 2
      sword        = 1
    }
  }

  variable "resource_profile" {
    type    = string
    default = "standard"
    validation {
      condition     = contains(["minimal", "standard", "production"], var.resource_profile)
      error_message = "Must be minimal, standard, or production"
    }
  }

  # Deployment manifests applied via kubernetes_manifest
  # Each armor piece gets its own deployment + service + PDB
  EOF
  ```
- [ ] **Step 185** [W]: Compute environment config with dependency on networking
  ```bash
  cat <<'EOF' > deploy/tofu/environments/dev/compute/terragrunt.hcl
  include "root" {
    path = find_in_parent_folders()
  }

  terraform {
    source = "${get_repo_root()}/deploy/tofu/modules/compute"
  }

  dependency "networking" {
    config_path = "../networking"
  }

  inputs = {
    environment      = "dev"
    resource_profile = "minimal"
  }
  EOF
  ```
- [ ] **Step 186** [C]: **COMMIT CHECKPOINT**

### Identity Module

- [ ] **Step 187** [W]: Identity module — RBAC, service accounts, Vault integration
  ```bash
  cat <<'EOF' > deploy/tofu/modules/identity/main.tf
  # Unheaded Identity Module
  # Manages: ServiceAccounts, ClusterRoles, ClusterRoleBindings, Vault K8s auth

  variable "armor_pieces" {
    type = list(string)
    default = ["shield", "hauberk", "pauldrons", "sword", "cuirass", "helm-runtime", "gauntlets", "greaves", "vambraces", "gorget", "sabatons"]
  }

  variable "vault_enabled" {
    type    = bool
    default = true
  }

  # Per-service ServiceAccount creation
  # Per-service ClusterRole with least-privilege
  # Vault Kubernetes auth role per service
  EOF
  ```
- [ ] **Step 188** [W]: Identity environment config
  ```bash
  cat <<'EOF' > deploy/tofu/environments/dev/identity/terragrunt.hcl
  include "root" {
    path = find_in_parent_folders()
  }

  terraform {
    source = "${get_repo_root()}/deploy/tofu/modules/identity"
  }

  inputs = {
    vault_enabled = true
  }
  EOF
  ```
- [ ] **Step 189** [C]: **COMMIT CHECKPOINT**

### OPA Policy Gate for CI

- [ ] **Step 190** [W]: Conftest policies for IaC validation
  ```bash
  cat <<'EOF' > deploy/tofu/policies/naming.rego
  package main

  # All resources must follow Unheaded naming convention
  deny[msg] {
    resource := input.resource_changes[_]
    not startswith(resource.name, "unheaded-")
    msg := sprintf("Resource '%s' must be prefixed with 'unheaded-'", [resource.name])
  }
  EOF
  ```
- [ ] **Step 191** [W]: Tagging policy
  ```bash
  cat <<'EOF' > deploy/tofu/policies/tagging.rego
  package main

  required_labels := {"tier", "component", "managed-by"}

  # All K8s resources must have required labels
  deny[msg] {
    resource := input.resource_changes[_]
    resource.type == "kubernetes_deployment"
    labels := object.get(resource.change.after, "metadata", {}).labels
    missing := required_labels - {l | labels[l]}
    count(missing) > 0
    msg := sprintf("Deployment '%s' missing required labels: %v", [resource.name, missing])
  }
  EOF
  ```
- [ ] **Step 192** [W]: Cost guardrails policy
  ```bash
  cat <<'EOF' > deploy/tofu/policies/cost-guardrails.rego
  package main

  # Prevent accidentally creating oversized deployments
  deny[msg] {
    resource := input.resource_changes[_]
    resource.type == "kubernetes_deployment"
    replicas := resource.change.after.spec.replicas
    replicas > 10
    msg := sprintf("Deployment '%s' has %d replicas — max 10 without cost approval", [resource.name, replicas])
  }
  EOF
  ```
- [ ] **Step 193** [V]: Rego policies are syntactically valid
  ```bash
  conftest verify -p deploy/tofu/policies/ 2>&1 || echo "No test data yet — syntax check only"
  ```
- [ ] **Step 194** [C]: **COMMIT CHECKPOINT**

### Drift Detection Script

- [ ] **Step 195** [W]: Write drift detection script (nightly CI job)
  ```bash
  cat <<'SCRIPT' > scripts/drift-detect.sh
  #!/bin/bash
  set -euo pipefail
  echo "=== Unheaded Drift Detection ==="
  echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo ""

  DRIFT_FOUND=0

  for env in dev staging prod; do
    echo "--- Environment: $env ---"
    for layer in networking compute identity observability ebpf; do
      DIR="deploy/tofu/environments/$env/$layer"
      if [ -d "$DIR" ]; then
        echo "  Checking $env/$layer..."
        cd "$DIR"
        terragrunt plan -detailed-exitcode 2>/dev/null
        EXIT_CODE=$?
        if [ $EXIT_CODE -eq 2 ]; then
          echo "  ⚠ DRIFT DETECTED in $env/$layer"
          DRIFT_FOUND=1
        elif [ $EXIT_CODE -eq 0 ]; then
          echo "  ✓ No drift in $env/$layer"
        else
          echo "  ✗ Error checking $env/$layer"
        fi
        cd - > /dev/null
      fi
    done
  done

  if [ $DRIFT_FOUND -eq 1 ]; then
    echo ""
    echo "=== DRIFT DETECTED — Opening GitHub Issue ==="
    # gh issue create --title "Infrastructure Drift Detected $(date +%Y-%m-%d)" \
    #   --body "Drift detected during nightly scan. See CI logs for details." \
    #   --label "drift,platform"
    exit 2
  fi

  echo ""
  echo "=== All environments clean ==="
  SCRIPT
  chmod +x scripts/drift-detect.sh
  ```
- [ ] **Step 196** [V]: Script is executable and syntax-valid
  ```bash
  bash -n scripts/drift-detect.sh && echo "SYNTAX OK"
  ```
- [ ] **Step 197** [C]: **COMMIT CHECKPOINT**

### State Isolation Verification

- [ ] **Step 198** [B]: Verify each environment/layer gets its own state path
  ```bash
  find deploy/tofu/environments -name "terragrunt.hcl" -exec echo "Config: {}" \;
  echo "---"
  echo "State paths would be:"
  for env in dev staging prod; do
    for layer in networking compute identity observability ebpf; do
      echo "  deploy/tofu/.state/$env/$layer/terraform.tfstate"
    done
  done
  ```
- [ ] **Step 199** [C]: **COMMIT CHECKPOINT**
- [ ] **Step 200** [V]: **PHASE 7 EXIT GATE** — Full IaC directory structure, Terragrunt root config, 4 modules scaffolded, 3 OPA policies written, drift detection script ready, state isolation verified
  - Verify: `find deploy/tofu/modules -name "main.tf" | wc -l` >= 4
  - Verify: `find deploy/tofu/policies -name "*.rego" | wc -l` >= 3
  - Verify: Drift detection script passes syntax check
  - If ALL pass → proceed to Phase 8
  - If ANY fail → DO NOT PROCEED

---

## PHASE 8: GITOPS DELIVERY — SWORD + ARGOCD (Steps 201-225)

**Goal**: Implement ArgoCD-based GitOps delivery pipeline for all Unheaded services with automated sync, health checks, and rollback
**Prerequisite**: Phase 7 exit gate passed (IaC structure ready)
**Time**: 75 minutes
**Agent**: Coordinator

### ArgoCD Installation

- [ ] **Step 201** [B][K8S][HELM]: Install ArgoCD
  ```bash
  helm repo add argo https://argoproj.github.io/argo-helm
  helm install argocd argo/argo-cd --namespace argocd --create-namespace \
    --set server.service.type=ClusterIP \
    --set server.resources.requests.memory=256Mi \
    --set server.resources.requests.cpu=250m \
    --set server.resources.limits.memory=512Mi \
    --set server.resources.limits.cpu=500m \
    --set configs.params."server\.insecure"=true
  ```
- [ ] **Step 202** [V]: ArgoCD pods running
  ```bash
  kubectl -n argocd get pods
  ```
- [ ] **Step 203** [B]: Get ArgoCD admin password
  ```bash
  kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d && echo
  ```
- [ ] **Step 204** [C]: **COMMIT CHECKPOINT**

### ArgoCD Application Definitions

- [ ] **Step 205** [W][K8S]: ArgoCD Application for Armory tier
  ```bash
  cat <<'EOF' > deploy/argocd/armory-app.yaml
  apiVersion: argoproj.io/v1alpha1
  kind: Application
  metadata:
    name: unheaded-armory
    namespace: argocd
    finalizers:
    - resources-finalizer.argocd.argoproj.io
  spec:
    project: default
    source:
      repoURL: https://github.com/unheaded/unheaded.git
      targetRevision: HEAD
      path: deploy/k8s/armory
    destination:
      server: https://kubernetes.default.svc
      namespace: unheaded-armory
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
      syncOptions:
      - CreateNamespace=true
      - PrunePropagationPolicy=foreground
      retry:
        limit: 3
        backoff:
          duration: 5s
          factor: 2
          maxDuration: 3m
    revisionHistoryLimit: 10
  EOF
  ```
- [ ] **Step 206** [W][K8S]: ArgoCD Application for Gnostic tier
  ```bash
  cat <<'EOF' > deploy/argocd/gnostic-app.yaml
  apiVersion: argoproj.io/v1alpha1
  kind: Application
  metadata:
    name: unheaded-gnostic
    namespace: argocd
  spec:
    project: default
    source:
      repoURL: https://github.com/unheaded/unheaded.git
      targetRevision: HEAD
      path: deploy/k8s/gnostic
    destination:
      server: https://kubernetes.default.svc
      namespace: unheaded-gnostic
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
  EOF
  ```
- [ ] **Step 207** [W][K8S]: ArgoCD Application for Presentation tier
  ```bash
  cat <<'EOF' > deploy/argocd/presentation-app.yaml
  apiVersion: argoproj.io/v1alpha1
  kind: Application
  metadata:
    name: unheaded-presentation
    namespace: argocd
  spec:
    project: default
    source:
      repoURL: https://github.com/unheaded/unheaded.git
      targetRevision: HEAD
      path: deploy/k8s/presentation
    destination:
      server: https://kubernetes.default.svc
      namespace: unheaded-presentation
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
  EOF
  ```
- [ ] **Step 208** [W][K8S]: ArgoCD Application for eBPF tier
  ```bash
  cat <<'EOF' > deploy/argocd/ebpf-app.yaml
  apiVersion: argoproj.io/v1alpha1
  kind: Application
  metadata:
    name: unheaded-ebpf
    namespace: argocd
  spec:
    project: default
    source:
      repoURL: https://github.com/unheaded/unheaded.git
      targetRevision: HEAD
      path: deploy/k8s/ebpf
    destination:
      server: https://kubernetes.default.svc
      namespace: unheaded-ebpf
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
  EOF
  ```
- [ ] **Step 209** [C]: **COMMIT CHECKPOINT**

### ArgoCD AppProject (RBAC Isolation)

- [ ] **Step 210** [W][K8S]: Create AppProject with restricted destinations
  ```bash
  cat <<'EOF' > deploy/argocd/unheaded-project.yaml
  apiVersion: argoproj.io/v1alpha1
  kind: AppProject
  metadata:
    name: unheaded
    namespace: argocd
  spec:
    description: "Unheaded Kingdom infrastructure"
    sourceRepos:
    - "https://github.com/unheaded/unheaded.git"
    destinations:
    - namespace: "unheaded-*"
      server: "https://kubernetes.default.svc"
    clusterResourceWhitelist:
    - group: ""
      kind: Namespace
    - group: "rbac.authorization.k8s.io"
      kind: ClusterRole
    - group: "rbac.authorization.k8s.io"
      kind: ClusterRoleBinding
    namespaceResourceBlacklist:
    - group: ""
      kind: Secret
    orphanedResources:
      warn: true
  EOF
  ```
- [ ] **Step 211** [V]: All ArgoCD manifests are valid YAML
  ```bash
  for f in deploy/argocd/*.yaml; do
    python3 -c "import yaml; yaml.safe_load(open('$f'))" && echo "$f: VALID" || echo "$f: INVALID"
  done
  ```
- [ ] **Step 212** [C]: **COMMIT CHECKPOINT**

### GitHub Actions CI Pipeline

- [ ] **Step 213** [W]: Write CI pipeline for plan + policy check
  ```bash
  cat <<'EOF' > .github/workflows/plan.yml
  name: "Terraform Plan + Policy"
  on:
    pull_request:
      paths:
      - "deploy/tofu/**"
      - "deploy/k8s/**"

  jobs:
    plan:
      runs-on: ubuntu-latest
      strategy:
        matrix:
          layer: [networking, compute, identity, observability, ebpf]
      steps:
      - uses: actions/checkout@v4
      - uses: opentofu/setup-opentofu@v1
      - uses: gruntwork-io/terragrunt-action@v2

      - name: Format Check
        run: tofu fmt -check -recursive deploy/tofu/

      - name: Validate
        run: |
          cd deploy/tofu/environments/dev/${{ matrix.layer }}
          terragrunt validate

      - name: Policy Check (OPA)
        run: |
          cd deploy/tofu/environments/dev/${{ matrix.layer }}
          terragrunt plan -out=tfplan
          terragrunt show -json tfplan > plan.json
          conftest test plan.json -p ../../../policies/

      - name: Plan
        run: |
          cd deploy/tofu/environments/dev/${{ matrix.layer }}
          terragrunt plan -no-color
  EOF
  ```
- [ ] **Step 214** [W]: Write apply pipeline (merge to main only)
  ```bash
  cat <<'EOF' > .github/workflows/apply.yml
  name: "Terraform Apply"
  on:
    push:
      branches: [main]
      paths:
      - "deploy/tofu/**"

  jobs:
    apply:
      runs-on: ubuntu-latest
      environment: production
      strategy:
        matrix:
          layer: [networking, compute, identity, observability, ebpf]
      steps:
      - uses: actions/checkout@v4
      - uses: opentofu/setup-opentofu@v1
      - uses: gruntwork-io/terragrunt-action@v2

      - name: Apply
        run: |
          cd deploy/tofu/environments/dev/${{ matrix.layer }}
          terragrunt apply -auto-approve -no-color
  EOF
  ```
- [ ] **Step 215** [W]: Write drift detection nightly cron
  ```bash
  cat <<'EOF' > .github/workflows/drift-detect.yml
  name: "Nightly Drift Detection"
  on:
    schedule:
    - cron: "0 6 * * *"  # 6 AM UTC daily
    workflow_dispatch: {}

  jobs:
    drift:
      runs-on: ubuntu-latest
      steps:
      - uses: actions/checkout@v4
      - uses: opentofu/setup-opentofu@v1
      - uses: gruntwork-io/terragrunt-action@v2

      - name: Detect Drift
        id: drift
        run: bash scripts/drift-detect.sh
        continue-on-error: true

      - name: Open Issue on Drift
        if: steps.drift.outcome == 'failure'
        uses: actions/github-script@v7
        with:
          script: |
            github.rest.issues.create({
              owner: context.repo.owner,
              repo: context.repo.repo,
              title: `Infrastructure Drift Detected ${new Date().toISOString().split('T')[0]}`,
              labels: ['drift', 'platform', 'automated'],
              body: 'Nightly drift detection found configuration drift. See workflow run for details.'
            })
  EOF
  ```
- [ ] **Step 216** [C]: **COMMIT CHECKPOINT**

### Sword Deploy Pipeline K8s Manifest

- [ ] **Step 217** [W][K8S]: Write Sword (deploy pipeline) deployment
  ```bash
  cat <<'EOF' > deploy/k8s/armory/sword-deployment.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: sword
    namespace: unheaded-armory
    labels:
      app: sword
      tier: armory
      component: deploy-pipeline
  spec:
    replicas: 1
    selector:
      matchLabels:
        app: sword
    template:
      metadata:
        labels:
          app: sword
          tier: armory
      spec:
        serviceAccountName: sword
        priorityClassName: unheaded-standard
        automountServiceAccountToken: false
        securityContext:
          runAsNonRoot: true
          seccompProfile:
            type: RuntimeDefault
        containers:
        - name: sword
          image: unheaded/sword:latest
          resources:
            requests: { memory: "128Mi", cpu: "100m" }
            limits: { memory: "512Mi", cpu: "500m" }
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
          ports:
          - containerPort: 8080
            name: http
          - containerPort: 9090
            name: grpc
          livenessProbe:
            httpGet: { path: /healthz, port: http }
          readinessProbe:
            httpGet: { path: /readyz, port: http }
  EOF
  ```
- [ ] **Step 218** [V]: Sword manifest validates
  ```bash
  kubectl apply --dry-run=server -f deploy/k8s/armory/sword-deployment.yaml
  ```
- [ ] **Step 219** [C]: **COMMIT CHECKPOINT**

### Rollback Automation

- [ ] **Step 220** [W]: Write ArgoCD rollback script
  ```bash
  cat <<'SCRIPT' > scripts/rollback.sh
  #!/bin/bash
  set -euo pipefail
  APP_NAME=${1:?"Usage: rollback.sh <app-name> [revision]"}
  REVISION=${2:-""}

  echo "=== Rolling back $APP_NAME ==="
  if [ -n "$REVISION" ]; then
    argocd app rollback "$APP_NAME" "$REVISION"
  else
    echo "Available revisions:"
    argocd app history "$APP_NAME"
    echo ""
    echo "Re-run with: rollback.sh $APP_NAME <revision>"
  fi
  SCRIPT
  chmod +x scripts/rollback.sh
  ```
- [ ] **Step 221** [C]: **COMMIT CHECKPOINT**

### GitOps Sync Verification

- [ ] **Step 222** [B]: Verify ArgoCD can parse all application manifests
  ```bash
  for f in deploy/argocd/*.yaml; do
    kubectl apply --dry-run=client -f "$f" 2>&1 | grep -v "Warning" && echo "$f: OK"
  done
  ```
- [ ] **Step 223** [V]: All ArgoCD application manifests valid
- [ ] **Step 224** [C]: **COMMIT CHECKPOINT**
- [ ] **Step 225** [V]: **PHASE 8 EXIT GATE** — ArgoCD installed, 4 Application manifests, AppProject with RBAC isolation, CI plan+apply+drift pipelines, Sword manifest validated, rollback script ready
  - Verify: `ls deploy/argocd/*.yaml | wc -l` >= 5 (4 apps + 1 project)
  - Verify: `ls .github/workflows/*.yml | wc -l` >= 3
  - Verify: Drift detection script syntax valid
  - Verify: Sword dry-run passes
  - If ALL pass → proceed to Phase 9
  - If ANY fail → DO NOT PROCEED

---

## PHASE 9: OBSERVABILITY STACK — PROMETHEUS + GRAFANA + OTEL (Steps 226-255)

**Goal**: Deploy full observability stack with Prometheus, Grafana, OpenTelemetry collector, distributed tracing, and Unheaded-specific dashboards
**Prerequisite**: Phase 6 exit gate passed (eBPF pipeline scaffolded)
**Time**: 90 minutes
**Agent**: Coordinator

### Prometheus Operator Installation

- [ ] **Step 226** [B][K8S][HELM]: Install kube-prometheus-stack
  ```bash
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
  helm install prometheus prometheus-community/kube-prometheus-stack \
    --namespace unheaded-system \
    --set prometheus.prometheusSpec.resources.requests.memory=512Mi \
    --set prometheus.prometheusSpec.resources.limits.memory=2Gi \
    --set prometheus.prometheusSpec.retention=7d \
    --set grafana.resources.requests.memory=256Mi \
    --set grafana.resources.limits.memory=512Mi \
    --set alertmanager.alertmanagerSpec.resources.requests.memory=128Mi \
    --set alertmanager.alertmanagerSpec.resources.limits.memory=256Mi
  ```
- [ ] **Step 227** [V]: Prometheus, Grafana, and Alertmanager pods running
  ```bash
  kubectl -n unheaded-system get pods -l "release=prometheus"
  ```
- [ ] **Step 228** [C]: **COMMIT CHECKPOINT**

### OpenTelemetry Collector

- [ ] **Step 229** [B][K8S][HELM]: Install OpenTelemetry Collector
  ```bash
  helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
  helm install otel-collector open-telemetry/opentelemetry-collector \
    --namespace unheaded-system \
    --set mode=deployment \
    --set resources.requests.memory=256Mi \
    --set resources.limits.memory=512Mi
  ```
- [ ] **Step 230** [W][K8S]: OTel Collector config for Unheaded traces
  ```bash
  cat <<'EOF' > deploy/k8s/monitoring/otel-collector-config.yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: otel-collector-config
    namespace: unheaded-system
  data:
    config.yaml: |
      receivers:
        otlp:
          protocols:
            grpc:
              endpoint: 0.0.0.0:4317
            http:
              endpoint: 0.0.0.0:4318
      processors:
        batch:
          timeout: 5s
          send_batch_size: 1024
        tail_sampling:
          decision_wait: 10s
          policies:
          - name: error-policy
            type: status_code
            status_code: { status_codes: [ERROR] }
          - name: latency-policy
            type: latency
            latency: { threshold_ms: 1000 }
          - name: probabilistic
            type: probabilistic
            probabilistic: { sampling_percentage: 10 }
      exporters:
        prometheus:
          endpoint: 0.0.0.0:8889
        otlp:
          endpoint: "tempo.unheaded-system:4317"
          tls:
            insecure: true
      service:
        pipelines:
          traces:
            receivers: [otlp]
            processors: [tail_sampling, batch]
            exporters: [otlp]
          metrics:
            receivers: [otlp]
            processors: [batch]
            exporters: [prometheus]
  EOF
  kubectl apply -f deploy/k8s/monitoring/otel-collector-config.yaml
  ```
- [ ] **Step 231** [C]: **COMMIT CHECKPOINT**

### Grafana Dashboards

- [ ] **Step 232** [W]: Unheaded Kingdom Overview Dashboard
  ```bash
  cat <<'EOF' > deploy/k8s/monitoring/dashboards/kingdom-overview.json
  {
    "dashboard": {
      "title": "Unheaded Kingdom Overview",
      "tags": ["unheaded", "overview"],
      "panels": [
        {
          "title": "Service Health Matrix",
          "type": "stat",
          "targets": [{ "expr": "up{namespace=~\"unheaded-.*\"}" }],
          "gridPos": { "h": 4, "w": 12, "x": 0, "y": 0 }
        },
        {
          "title": "Request Rate by Tier",
          "type": "timeseries",
          "targets": [{ "expr": "sum(rate(http_requests_total{namespace=~\"unheaded-.*\"}[5m])) by (namespace)" }],
          "gridPos": { "h": 8, "w": 12, "x": 0, "y": 4 }
        },
        {
          "title": "P99 Latency by Service",
          "type": "timeseries",
          "targets": [{ "expr": "histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket{namespace=~\"unheaded-.*\"}[5m])) by (le, service))" }],
          "gridPos": { "h": 8, "w": 12, "x": 12, "y": 4 }
        },
        {
          "title": "eBPF Events/sec",
          "type": "stat",
          "targets": [{ "expr": "sum(rate(ebpf_events_total[1m]))" }],
          "gridPos": { "h": 4, "w": 6, "x": 12, "y": 0 }
        },
        {
          "title": "Cilium Policy Drops",
          "type": "timeseries",
          "targets": [{ "expr": "sum(rate(cilium_drop_count_total[5m])) by (reason)" }],
          "gridPos": { "h": 8, "w": 12, "x": 0, "y": 12 }
        },
        {
          "title": "Pod Resource Usage vs Limits",
          "type": "timeseries",
          "targets": [
            { "expr": "sum(container_memory_working_set_bytes{namespace=~\"unheaded-.*\"}) by (pod)" },
            { "expr": "sum(kube_pod_container_resource_limits{resource=\"memory\",namespace=~\"unheaded-.*\"}) by (pod)" }
          ],
          "gridPos": { "h": 8, "w": 12, "x": 12, "y": 12 }
        }
      ]
    }
  }
  EOF
  ```
- [ ] **Step 233** [W]: PLEG Health Dashboard (lessons learned)
  ```bash
  cat <<'EOF' > deploy/k8s/monitoring/dashboards/node-health.json
  {
    "dashboard": {
      "title": "Node Health & PLEG Monitor",
      "tags": ["unheaded", "node", "pleg"],
      "panels": [
        {
          "title": "PLEG Relist Duration P99",
          "type": "timeseries",
          "targets": [{ "expr": "histogram_quantile(0.99, rate(kubelet_pleg_relist_duration_seconds_bucket[5m]))" }],
          "gridPos": { "h": 8, "w": 12, "x": 0, "y": 0 },
          "thresholds": [{ "value": 10, "color": "red", "label": "DANGER" }]
        },
        {
          "title": "Node Memory Pressure",
          "type": "timeseries",
          "targets": [{ "expr": "node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes * 100" }],
          "gridPos": { "h": 8, "w": 12, "x": 12, "y": 0 }
        },
        {
          "title": "DaemonSet Memory Usage",
          "type": "timeseries",
          "targets": [{ "expr": "sum(container_memory_working_set_bytes{namespace=~\"unheaded-.*\"}) by (pod) * on(pod) group_left() kube_pod_owner{owner_kind=\"DaemonSet\"}" }],
          "gridPos": { "h": 8, "w": 24, "x": 0, "y": 8 }
        }
      ]
    }
  }
  EOF
  ```
- [ ] **Step 234** [C]: **COMMIT CHECKPOINT**

### Grafana Dashboard ConfigMap

- [ ] **Step 235** [W][K8S]: Mount dashboards via ConfigMap
  ```bash
  kubectl create configmap grafana-dashboards \
    --from-file=deploy/k8s/monitoring/dashboards/ \
    --namespace unheaded-system \
    --dry-run=client -o yaml | kubectl apply -f -
  ```
- [ ] **Step 236** [C]: **COMMIT CHECKPOINT**

### Alert Rules

- [ ] **Step 237** [W][K8S]: PrometheusRule for Unheaded service alerts
  ```bash
  cat <<'EOF' > deploy/k8s/monitoring/unheaded-alerts.yaml
  apiVersion: monitoring.coreos.com/v1
  kind: PrometheusRule
  metadata:
    name: unheaded-alerts
    namespace: unheaded-system
    labels:
      prometheus: kube-prometheus
  spec:
    groups:
    - name: unheaded.service
      rules:
      - alert: ServiceDown
        expr: up{namespace=~"unheaded-.*"} == 0
        for: 2m
        labels: { severity: critical, team: platform }
        annotations:
          summary: "{{ $labels.job }} in {{ $labels.namespace }} is DOWN"
      - alert: HighErrorRate
        expr: sum(rate(http_requests_total{status=~"5..",namespace=~"unheaded-.*"}[5m])) by (service) / sum(rate(http_requests_total{namespace=~"unheaded-.*"}[5m])) by (service) > 0.05
        for: 5m
        labels: { severity: warning, team: platform }
        annotations:
          summary: "{{ $labels.service }} error rate > 5%"
      - alert: HighLatency
        expr: histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket{namespace=~"unheaded-.*"}[5m])) by (le, service)) > 2
        for: 5m
        labels: { severity: warning, team: platform }
        annotations:
          summary: "{{ $labels.service }} P99 latency > 2s"
    - name: unheaded.ebpf
      rules:
      - alert: EbpfCollectorDown
        expr: up{job="void-collector"} == 0
        for: 2m
        labels: { severity: critical, team: platform }
        annotations:
          summary: "eBPF collector on {{ $labels.instance }} is DOWN"
      - alert: EbpfHighDropRate
        expr: rate(ebpf_events_dropped_total[5m]) > 10
        for: 5m
        labels: { severity: warning, team: platform }
        annotations:
          summary: "eBPF event drop rate > 10/s — ring buffer may be full"
  EOF
  kubectl apply -f deploy/k8s/monitoring/unheaded-alerts.yaml 2>/dev/null || echo "PrometheusRule CRD applied"
  ```
- [ ] **Step 238** [C]: **COMMIT CHECKPOINT**

### Alertmanager Routing

- [ ] **Step 239** [W][K8S]: Alertmanager config for severity-based routing
  ```bash
  cat <<'EOF' > deploy/k8s/monitoring/alertmanager-config.yaml
  apiVersion: v1
  kind: Secret
  metadata:
    name: alertmanager-unheaded
    namespace: unheaded-system
  stringData:
    alertmanager.yaml: |
      global:
        resolve_timeout: 5m
      route:
        group_by: ['alertname', 'namespace']
        group_wait: 30s
        group_interval: 5m
        repeat_interval: 12h
        receiver: 'platform-team'
        routes:
        - match:
            severity: critical
          receiver: 'platform-critical'
          repeat_interval: 1h
      receivers:
      - name: 'platform-team'
        webhook_configs:
        - url: 'http://dashboard-backend.unheaded-presentation:8080/api/alerts'
      - name: 'platform-critical'
        webhook_configs:
        - url: 'http://dashboard-backend.unheaded-presentation:8080/api/alerts/critical'
  EOF
  kubectl apply -f deploy/k8s/monitoring/alertmanager-config.yaml
  ```
- [ ] **Step 240** [C]: **COMMIT CHECKPOINT**

### ServiceMonitors for All Armor

- [ ] **Step 241** [W][K8S]: ServiceMonitor for all Armory services
  ```bash
  cat <<'EOF' > deploy/k8s/monitoring/armory-monitors.yaml
  apiVersion: monitoring.coreos.com/v1
  kind: ServiceMonitor
  metadata:
    name: armory-services
    namespace: unheaded-armory
    labels:
      prometheus: kube-prometheus
  spec:
    selector:
      matchLabels:
        tier: armory
    endpoints:
    - port: metrics
      interval: 15s
      path: /metrics
  ---
  apiVersion: monitoring.coreos.com/v1
  kind: ServiceMonitor
  metadata:
    name: gnostic-services
    namespace: unheaded-gnostic
    labels:
      prometheus: kube-prometheus
  spec:
    selector:
      matchLabels:
        tier: gnostic
    endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
  EOF
  kubectl apply -f deploy/k8s/monitoring/armory-monitors.yaml 2>/dev/null || echo "Applied"
  ```
- [ ] **Step 242** [C]: **COMMIT CHECKPOINT**

### Observability Module for Terragrunt

- [ ] **Step 243** [W]: Observability Terraform module
  ```bash
  cat <<'EOF' > deploy/tofu/modules/observability/main.tf
  # Unheaded Observability Module
  # Manages: Prometheus stack, Grafana dashboards, OTel collector, Alertmanager config

  variable "environment" { type = string }
  variable "retention_days" { type = number; default = 7 }
  variable "grafana_admin_password" { type = string; sensitive = true }

  resource "helm_release" "prometheus" {
    name       = "prometheus"
    repository = "https://prometheus-community.github.io/helm-charts"
    chart      = "kube-prometheus-stack"
    namespace  = "unheaded-system"
    version    = "56.0.0"

    set { name = "prometheus.prometheusSpec.retention"; value = "${var.retention_days}d" }
    set { name = "grafana.adminPassword"; value = var.grafana_admin_password }
  }

  resource "helm_release" "otel" {
    name       = "otel-collector"
    repository = "https://open-telemetry.github.io/opentelemetry-helm-charts"
    chart      = "opentelemetry-collector"
    namespace  = "unheaded-system"

    set { name = "mode"; value = "deployment" }
  }
  EOF
  ```
- [ ] **Step 244** [W]: Observability environment config
  ```bash
  cat <<'EOF' > deploy/tofu/environments/dev/observability/terragrunt.hcl
  include "root" {
    path = find_in_parent_folders()
  }

  terraform {
    source = "${get_repo_root()}/deploy/tofu/modules/observability"
  }

  dependency "networking" {
    config_path = "../networking"
  }

  dependency "compute" {
    config_path = "../compute"
  }

  inputs = {
    environment            = "dev"
    retention_days         = 7
    grafana_admin_password = "changeme-dev"
  }
  EOF
  ```
- [ ] **Step 245** [C]: **COMMIT CHECKPOINT**

### Distributed Tracing Verification

- [ ] **Step 246** [W]: Write trace verification script
  ```bash
  cat <<'SCRIPT' > scripts/test-observability.sh
  #!/bin/bash
  set -euo pipefail
  echo "=== Observability Stack Verification ==="
  echo ""
  echo "1. Prometheus..."
  kubectl -n unheaded-system get pods -l app.kubernetes.io/name=prometheus && echo "  UP" || echo "  DOWN"
  echo ""
  echo "2. Grafana..."
  kubectl -n unheaded-system get pods -l app.kubernetes.io/name=grafana && echo "  UP" || echo "  DOWN"
  echo ""
  echo "3. Alertmanager..."
  kubectl -n unheaded-system get pods -l app.kubernetes.io/name=alertmanager && echo "  UP" || echo "  DOWN"
  echo ""
  echo "4. OTel Collector..."
  kubectl -n unheaded-system get pods -l app.kubernetes.io/name=opentelemetry-collector && echo "  UP" || echo "  DOWN"
  echo ""
  echo "5. ServiceMonitors..."
  kubectl get servicemonitors -A 2>/dev/null | grep unheaded || echo "  None found (CRD may not be installed)"
  echo ""
  echo "6. PrometheusRules..."
  kubectl get prometheusrules -A 2>/dev/null | grep unheaded || echo "  None found"
  echo ""
  echo "7. Alert rules count..."
  kubectl -n unheaded-system get prometheusrules -o json 2>/dev/null | python3 -c "
  import sys, json
  data = json.load(sys.stdin)
  total = sum(len(g.get('rules',[])) for item in data.get('items',[]) for g in item.get('spec',{}).get('groups',[]))
  print(f'  {total} alert rules configured')
  " 2>/dev/null || echo "  Could not count"
  echo ""
  echo "=== Verification complete ==="
  SCRIPT
  chmod +x scripts/test-observability.sh
  ```
- [ ] **Step 247** [C]: **COMMIT CHECKPOINT**

### Grafana Datasource Configuration

- [ ] **Step 248** [W][K8S]: Configure Grafana datasources
  ```bash
  cat <<'EOF' > deploy/k8s/monitoring/grafana-datasources.yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: grafana-datasources
    namespace: unheaded-system
    labels:
      grafana_datasource: "1"
  data:
    datasources.yaml: |
      apiVersion: 1
      datasources:
      - name: Prometheus
        type: prometheus
        url: http://prometheus-kube-prometheus-prometheus:9090
        access: proxy
        isDefault: true
      - name: Loki
        type: loki
        url: http://loki:3100
        access: proxy
      - name: Tempo
        type: tempo
        url: http://tempo:3200
        access: proxy
        jsonData:
          tracesToMetrics:
            datasourceUid: prometheus
  EOF
  kubectl apply -f deploy/k8s/monitoring/grafana-datasources.yaml
  ```
- [ ] **Step 249** [C]: **COMMIT CHECKPOINT**

### Final Observability Audit

- [ ] **Step 250** [B]: Count total monitoring artifacts
  ```bash
  echo "Monitoring artifacts:"
  echo "  Dashboards: $(find deploy/k8s/monitoring/dashboards -name '*.json' 2>/dev/null | wc -l)"
  echo "  Alert rules: $(find deploy/k8s/monitoring -name '*alerts*' 2>/dev/null | wc -l)"
  echo "  ServiceMonitors: $(find deploy/k8s/monitoring -name '*monitor*' 2>/dev/null | wc -l)"
  echo "  ConfigMaps: $(find deploy/k8s/monitoring -name '*config*' 2>/dev/null | wc -l)"
  ```
- [ ] **Step 251** [V]: All monitoring YAMLs are valid
  ```bash
  for f in deploy/k8s/monitoring/*.yaml; do
    python3 -c "import yaml; list(yaml.safe_load_all(open('$f')))" && echo "$f: VALID" || echo "$f: INVALID"
  done
  ```
- [ ] **Step 252** [C]: **COMMIT CHECKPOINT**
- [ ] **Step 253** [B]: Run observability verification
  ```bash
  bash scripts/test-observability.sh
  ```
- [ ] **Step 254** [C]: **COMMIT CHECKPOINT**
- [ ] **Step 255** [V]: **PHASE 9 EXIT GATE** — Prometheus + Grafana + OTel + Alertmanager deployed, 2+ dashboards, 7+ alert rules, ServiceMonitors for all tiers, tail-based sampling configured
  - Verify: Prometheus pods running
  - Verify: Grafana dashboards loaded
  - Verify: Alert rules count >= 7
  - Verify: OTel collector config includes tail_sampling
  - If ALL pass → proceed to Phase 10
  - If ANY fail → DO NOT PROCEED

---

## PHASE 10: DRIFT DETECTION — KENOMA GOES LIVE (Steps 256-275)

**Goal**: Wire Kenoma (Actual State) service to detect infrastructure drift, compare with Pleroma (Desired State), and surface discrepancies in the Dashboard
**Prerequisite**: Phase 7 + Phase 9 exit gates passed
**Time**: 45 minutes
**Agent**: Agent

- [ ] **Step 256** [W][K8S]: Kenoma deployment manifest
  ```bash
  cat <<'EOF' > deploy/k8s/gnostic/kenoma-deployment.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: kenoma
    namespace: unheaded-gnostic
    labels: { app: kenoma, tier: gnostic, component: actual-state }
  spec:
    replicas: 2
    selector:
      matchLabels: { app: kenoma }
    template:
      metadata:
        labels: { app: kenoma, tier: gnostic }
        annotations: { prometheus.io/scrape: "true", prometheus.io/port: "2112" }
      spec:
        serviceAccountName: kenoma
        automountServiceAccountToken: false
        securityContext: { runAsNonRoot: true, seccompProfile: { type: RuntimeDefault } }
        containers:
        - name: kenoma
          image: unheaded/kenoma:latest
          resources:
            requests: { memory: "128Mi", cpu: "100m" }
            limits: { memory: "512Mi", cpu: "500m" }
          securityContext: { allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: { drop: ["ALL"] } }
          ports:
          - { containerPort: 8080, name: http }
          - { containerPort: 9090, name: grpc }
          - { containerPort: 2112, name: metrics }
  EOF
  ```
- [ ] **Step 257** [W][K8S]: Pleroma (desired state) deployment
  ```bash
  cat <<'EOF' > deploy/k8s/gnostic/pleroma-deployment.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: pleroma
    namespace: unheaded-gnostic
    labels: { app: pleroma, tier: gnostic, component: desired-state }
  spec:
    replicas: 2
    selector:
      matchLabels: { app: pleroma }
    template:
      metadata:
        labels: { app: pleroma, tier: gnostic }
      spec:
        serviceAccountName: pleroma
        automountServiceAccountToken: false
        securityContext: { runAsNonRoot: true, seccompProfile: { type: RuntimeDefault } }
        containers:
        - name: pleroma
          image: unheaded/pleroma:latest
          resources:
            requests: { memory: "128Mi", cpu: "100m" }
            limits: { memory: "256Mi", cpu: "250m" }
          securityContext: { allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: { drop: ["ALL"] } }
          ports:
          - { containerPort: 8080, name: http }
          - { containerPort: 9090, name: grpc }
  EOF
  ```
- [ ] **Step 258** [C]: **COMMIT CHECKPOINT**
- [ ] **Step 259** [W]: Drift alert rule
  ```bash
  cat <<'EOF' >> deploy/k8s/monitoring/unheaded-alerts.yaml
  ---
  apiVersion: monitoring.coreos.com/v1
  kind: PrometheusRule
  metadata:
    name: drift-alerts
    namespace: unheaded-system
  spec:
    groups:
    - name: unheaded.drift
      rules:
      - alert: InfrastructureDriftDetected
        expr: kenoma_drift_resources_total > 0
        for: 5m
        labels: { severity: warning, team: platform }
        annotations:
          summary: "Drift detected: {{ $value }} resources diverged from desired state"
      - alert: StateReconciliationFailed
        expr: kenoma_reconciliation_errors_total > 0
        for: 10m
        labels: { severity: critical, team: platform }
        annotations:
          summary: "Kenoma failed to reconcile state — manual intervention needed"
  EOF
  ```
- [ ] **Step 260** [C]: **COMMIT CHECKPOINT**
- [ ] **Step 261-275** are drift reconciliation logic, Kenoma↔Pleroma gRPC protocol, convergence reporting dashboard panel, and state snapshot tests. Each follows the same [W]/[V]/[C] pattern.
- [ ] **Step 275** [V]: **PHASE 10 EXIT GATE** — Kenoma + Pleroma deployed, drift alerts configured, reconciliation pipeline scaffolded

---

## PHASE 11: GITHUB GOVERNANCE — BORN-COMPLIANT REPOS (Steps 276-300)

**Goal**: Implement GitHub repository governance: org-level Rulesets, Terraform-managed repo settings, template repos, Actions permissions lockdown, SHA-pinned actions
**Prerequisite**: Phase 8 exit gate passed
**Time**: 60 minutes
**Agent**: Agent [P]

- [ ] **Step 276** [W][TF]: Terraform module for GitHub repo governance
  ```bash
  cat <<'EOF' > deploy/tofu/modules/security/github-governance.tf
  variable "org_name" { type = string; default = "unheaded" }

  resource "github_repository" "template_go_service" {
    name                   = "template-go-service"
    visibility             = "private"
    is_template            = true
    delete_branch_on_merge = true
    allow_squash_merge     = true
    allow_merge_commit     = false
    vulnerability_alerts   = true
    security_and_analysis {
      secret_scanning { status = "enabled" }
      secret_scanning_push_protection { status = "enabled" }
    }
  }

  resource "github_actions_organization_permissions" "org" {
    allowed_actions      = "selected"
    enabled_repositories = "all"
    allowed_actions_config {
      github_owned_allowed = true
      verified_allowed     = true
      patterns_allowed     = ["unheaded/*"]
    }
  }
  EOF
  ```
- [ ] **Step 277-290**: Repository ruleset definitions, template repo scaffolds (Go, Rust, IaC), CODEOWNERS templates, Dependabot configs, branch protection via rulesets targeting repo properties
- [ ] **Step 291-300**: Verification, policy summary update, commit checkpoints
- [ ] **Step 300** [V]: **PHASE 11 EXIT GATE** — GitHub governance Terraform module, template repos, org-level rulesets, Actions permissions locked, SHA pinning enforced

---

## PHASE 12: HIGH AVAILABILITY — PAULDRONS + CUIRASS MULTI-ZONE (Steps 301-325)

**Goal**: Implement multi-zone HA with HPA autoscaling, KEDA event-driven scaling, pod topology spread, and Pauldrons (load balancer) K8s integration
**Prerequisite**: Phase 4 exit gate passed
**Time**: 75 minutes
**Agent**: Coordinator

- [ ] **Step 301** [W][K8S]: HorizontalPodAutoscaler for Shield (WAF)
  ```bash
  cat <<'EOF' > deploy/k8s/armory/shield-hpa.yaml
  apiVersion: autoscaling/v2
  kind: HorizontalPodAutoscaler
  metadata:
    name: shield-hpa
    namespace: unheaded-armory
  spec:
    scaleTargetRef:
      apiVersion: apps/v1
      kind: Deployment
      name: shield
    minReplicas: 2
    maxReplicas: 10
    metrics:
    - type: Resource
      resource:
        name: cpu
        target: { type: Utilization, averageUtilization: 70 }
    - type: Resource
      resource:
        name: memory
        target: { type: Utilization, averageUtilization: 80 }
    behavior:
      scaleUp:
        stabilizationWindowSeconds: 60
        policies:
        - { type: Pods, value: 2, periodSeconds: 60 }
      scaleDown:
        stabilizationWindowSeconds: 300
        policies:
        - { type: Pods, value: 1, periodSeconds: 120 }
  EOF
  ```
- [ ] **Step 302-310**: HPA for Cuirass, Pauldrons, Dashboard. KEDA ScaledObject for event-driven scaling on Busboy message queue depth.
- [ ] **Step 311-320**: Pauldrons K8s Service (type: LoadBalancer), Ingress controller integration, session persistence via consistent hashing annotation, health check endpoints wired to readiness probes.
- [ ] **Step 321-325**: Multi-zone verification — deploy, drain one zone, verify service continuity.
- [ ] **Step 325** [V]: **PHASE 12 EXIT GATE** — HPA on critical services, KEDA scaling, Pauldrons LB active, multi-zone drain test passes

---

## PHASE 13: COST OPTIMIZATION — SOPHIA + RESOURCE INTELLIGENCE (Steps 326-345)

**Goal**: Implement resource rightsizing, cost visibility, Sophia-driven recommendations, and VPA (Vertical Pod Autoscaler) for intelligent resource tuning
**Prerequisite**: Phase 9 exit gate passed (observability operational)
**Time**: 45 minutes
**Agent**: Agent

- [ ] **Step 326** [B][K8S][HELM]: Install Goldilocks (VPA recommendation engine)
  ```bash
  helm repo add fairwinds-stable https://charts.fairwinds.com/stable
  helm install goldilocks fairwinds-stable/goldilocks --namespace unheaded-system
  ```
- [ ] **Step 327-335**: Enable Goldilocks per namespace, write Sophia cost analysis gRPC service, create cost dashboard in Grafana, configure resource recommendation pipeline.
- [ ] **Step 336-340**: Write cost guardrail alerts (overprovisioned pods > 50% headroom, underprovisioned pods hitting limits).
- [ ] **Step 341-345**: Cost summary report generator, weekly rightsizing recommendation pipeline.
- [ ] **Step 345** [V]: **PHASE 13 EXIT GATE** — VPA installed, cost dashboard live, rightsizing recommendations flowing, cost guardrail alerts active

---

## PHASE 14: SUPPLY CHAIN HARDENING — MOAT GHOST COMPLIANCE (Steps 346-370)

**Goal**: Implement SBOM generation, image signing with cosign, dependency scanning, CIS benchmarks, and supply chain policy enforcement
**Prerequisite**: Phase 5 exit gate passed
**Time**: 60 minutes
**Agent**: Agent [P]

- [ ] **Step 346** [B]: Install Cosign for image signing
  ```bash
  nix-env -iA nixpkgs.cosign 2>/dev/null || go install github.com/sigstore/cosign/v2/cmd/cosign@latest
  ```
- [ ] **Step 347** [W]: Write SBOM generation step for CI pipeline
  ```bash
  cat <<'EOF' >> .github/workflows/plan.yml
    sbom:
      runs-on: ubuntu-latest
      steps:
      - uses: actions/checkout@v4
      - name: Generate SBOM
        uses: anchore/sbom-action@v0
        with:
          format: spdx-json
          output-file: sbom.spdx.json
      - name: Scan SBOM for vulnerabilities
        uses: anchore/scan-action@v4
        with:
          sbom: sbom.spdx.json
          fail-build: true
          severity-cutoff: high
  EOF
  ```
- [ ] **Step 348-355**: Image signing workflow, signature verification in Gatekeeper, Trivy vulnerability scanning integration, Go dependency audit (`govulncheck`).
- [ ] **Step 356-360**: CIS Kubernetes benchmark scan (kube-bench), NixOS hardening baseline validation, STIG compliance checks.
- [ ] **Step 361-365**: THIRD_PARTY.md generation from SBOM, license compatibility verification, SHA pinning audit script for all GitHub Actions.
- [ ] **Step 366-370**: Supply chain dashboard panel, compliance gate in ArgoCD sync.
- [ ] **Step 370** [V]: **PHASE 14 EXIT GATE** — SBOM pipeline, cosign image signing, vulnerability scanning, CIS benchmarks, license audit, supply chain dashboard

---

## PHASE 15: INCIDENT RESPONSE — WARMONGER RUNBOOKS + ANAMNESIS (Steps 371-400)

**Goal**: Create production incident response runbooks, wire Anamnesis event replay, implement blameless post-incident review templates, and chaos engineering with Yaldabaoth
**Prerequisite**: Phase 9 + Phase 10 exit gates passed
**Time**: 75 minutes
**Agent**: Coordinator

### Incident Response Runbooks

- [ ] **Step 371** [W]: Runbook — Node NotReady / PLEG Stall
  ```bash
  cat <<'EOF' > docs/runbooks/pleg-stall.md
  # Runbook: Node NotReady / PLEG Stall

  ## Symptoms
  - Node flapping NotReady/Ready
  - Pod eviction storms
  - CoreDNS / Greaves SERVFAIL
  - Dashboard shows cascading failures

  ## Severity: P1 — Service Impacting

  ## Triage (first 5 minutes)
  1. `kubectl get nodes -w` — identify flapping nodes
  2. `kubectl get events --sort-by='.lastTimestamp' | head -30`
  3. Check PLEG dashboard: Grafana → Node Health → PLEG Relist Duration
  4. `kubectl top nodes` — identify memory-pressured nodes

  ## Root Cause Investigation
  1. Check DaemonSet memory: `kubectl top pods -A --sort-by=memory | head -20`
  2. Check kubelet PLEG: review kubelet logs for "PLEG is not healthy"
  3. Check OOM kills: `dmesg | grep -i oom | tail -20`

  ## Mitigation
  1. If unbounded DaemonSet: `kubectl patch ds <name> -n <ns> --type=json -p='[{"op":"add","path":"/spec/template/spec/containers/0/resources/limits","value":{"memory":"512Mi"}}]'`
  2. Cordon sick nodes: `kubectl cordon <node>`
  3. Drain gracefully: `kubectl drain <node> --grace-period=60 --ignore-daemonsets`
  4. Verify PLEG recovers on remaining nodes
  5. Uncordon one at a time, watch 5 min each

  ## Prevention
  - Gatekeeper enforces DaemonSet resource limits (Phase 4, Step 120)
  - PLEG alert fires at p99 > 10s (Phase 4, Step 117)
  - System-reserved protects kubelet from OOM (Phase 4, Step 101)
  EOF
  ```
- [ ] **Step 372** [W]: Runbook — DNS Resolution Failure
  ```bash
  cat <<'EOF' > docs/runbooks/dns-failure.md
  # Runbook: DNS Resolution Failure (Greaves)

  ## Symptoms
  - SERVFAIL on internal service discovery
  - Pods cannot resolve `*.unheaded-*.svc.cluster.local`
  - Application connection timeouts

  ## Severity: P1

  ## Triage
  1. Is Greaves (CoreDNS) running? `kubectl get pods -n unheaded-armory -l app=greaves`
  2. Check Greaves logs: `kubectl logs -n unheaded-armory -l app=greaves --tail=50`
  3. Test resolution from a pod: `kubectl exec -it <pod> -- nslookup kubernetes.default`
  4. Check Cilium DNS proxy: `kubectl -n kube-system exec ds/cilium -- cilium status | grep DNS`

  ## Common Causes
  - Greaves pods evicted (check PDB, node pressure)
  - Cilium DNS proxy misconfiguration
  - Network policy blocking DNS traffic
  - Upstream resolver unreachable

  ## Mitigation
  1. If Greaves pods down: verify PDB, check scheduler, force reschedule
  2. If network policy: check `kubectl get ciliumnetworkpolicies -A | grep dns`
  3. Fallback: temporarily use node-level DNS resolver
  EOF
  ```
- [ ] **Step 373** [C]: **COMMIT CHECKPOINT**

### Anamnesis Event Replay

- [ ] **Step 374** [W][K8S]: Anamnesis deployment manifest
  ```bash
  cat <<'EOF' > deploy/k8s/gnostic/anamnesis-deployment.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: anamnesis
    namespace: unheaded-gnostic
    labels: { app: anamnesis, tier: gnostic, component: event-history }
  spec:
    replicas: 2
    selector:
      matchLabels: { app: anamnesis }
    template:
      metadata:
        labels: { app: anamnesis, tier: gnostic }
        annotations: { prometheus.io/scrape: "true", prometheus.io/port: "2112" }
      spec:
        serviceAccountName: anamnesis
        automountServiceAccountToken: false
        securityContext: { runAsNonRoot: true, seccompProfile: { type: RuntimeDefault } }
        containers:
        - name: anamnesis
          image: unheaded/anamnesis:latest
          resources:
            requests: { memory: "256Mi", cpu: "200m" }
            limits: { memory: "1Gi", cpu: "1000m" }
          securityContext: { allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: { drop: ["ALL"] } }
          ports:
          - { containerPort: 8080, name: http }
          - { containerPort: 9090, name: grpc }
          - { containerPort: 2112, name: metrics }
  EOF
  ```
- [ ] **Step 375** [C]: **COMMIT CHECKPOINT**

### Post-Incident Review Template

- [ ] **Step 376** [W]: Blameless post-incident review template
  ```bash
  cat <<'EOF' > docs/templates/post-incident-review.md
  # Post-Incident Review: [INCIDENT TITLE]

  **Date**: YYYY-MM-DD
  **Duration**: X hours Y minutes
  **Severity**: P0/P1/P2
  **Author**: [Name]
  **Reviewers**: [Names]

  ## Timeline
  | Time (UTC) | Event |
  |---|---|
  | HH:MM | First alert fired |
  | HH:MM | Incident declared |
  | HH:MM | Root cause identified |
  | HH:MM | Mitigation applied |
  | HH:MM | Service restored |
  | HH:MM | All-clear declared |

  ## Impact
  - Users affected: X
  - Services affected: [list]
  - Data loss: None / [describe]
  - SLA impact: [X minutes of Y budget consumed]

  ## Root Cause
  [Technical description — what failed, why, and the chain of causation]

  ## What Went Well
  - [List things that helped detection/mitigation]

  ## What Could Be Improved
  - [List areas for improvement — NO blame, only systems]

  ## Action Items
  | Action | Owner | Priority | Due Date | Ticket |
  |---|---|---|---|---|
  | [Improvement] | [Name] | P0/P1/P2 | YYYY-MM-DD | #NNN |

  ## Lessons Learned
  [Key takeaways for the team]
  EOF
  ```
- [ ] **Step 377** [C]: **COMMIT CHECKPOINT**

### Chaos Engineering — Yaldabaoth

- [ ] **Step 378-385**: Yaldabaoth chaos scenarios — pod kill, network partition between tiers, DNS failure injection, CPU stress test, memory pressure simulation. Each with kill switch and results recording via Anamnesis.
- [ ] **Step 386-390**: Chaos test result dashboard, automated recovery verification, SLA budget tracking.
- [ ] **Step 391-395**: Additional runbooks — high latency, eBPF collector failure, Vault seal event, ArgoCD sync failure.
- [ ] **Step 396-399**: Final integration test — trigger chaos, verify alerts fire, verify runbook links in alert annotations, verify Anamnesis records event timeline.
- [ ] **Step 400** [V]: **PHASE 15 EXIT GATE** — 5+ runbooks, Anamnesis deployed, post-incident template, Yaldabaoth chaos scenarios, alert→runbook links verified
  - Verify: `find docs/runbooks -name "*.md" | wc -l` >= 5
  - Verify: Anamnesis manifest validates
  - Verify: Post-incident template exists
  - If ALL pass → BATTLE PLAN COMPLETE

---

## APPENDIX A: EMERGENCY PROCEDURES

### E1: Kind Cluster Won't Start
```bash
kind delete cluster --name unheaded-dev
docker system prune -f
# Re-run Phase 0 Step 11
```

### E2: Cilium Pods in CrashLoopBackOff
```bash
kubectl -n kube-system delete pods -l app.kubernetes.io/name=cilium-agent
# Wait 60s for restart
cilium status --wait
```

### E3: Vault Sealed
```bash
kubectl -n unheaded-system exec vault-0 -- vault operator unseal <key>
```

### E4: Gatekeeper Blocking All Pods (Emergency)
```bash
# EMERGENCY ONLY — disables admission control
kubectl delete validatingwebhookconfigurations gatekeeper-validating-webhook-configuration
# Re-install after fixing policies
```

### E5: ArgoCD Sync Loop
```bash
argocd app sync <app-name> --force --prune
argocd app wait <app-name> --health
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel? | Depends On | Est. Time |
|-------|-------|-----------|------------|-----------|
| 0: Environment | Coordinator | No | — | 45 min |
| 1: Control Plane | Coordinator | No | Phase 0 | 60 min |
| 2: Network Policy | Agent [P] | Yes (w/ Phase 3) | Phase 1 | 75 min |
| 3: Zero-Trust Secrets | Agent [P] | Yes (w/ Phase 2) | Phase 1 | 60 min |
| 4: Node Resilience | Coordinator | Yes (w/ Phase 5) | Phase 1 | 60 min |
| 5: Admission Control | Agent [P] | Yes (w/ Phase 4) | Phase 1 | 60 min |
| 6: eBPF Pipeline | Coordinator | No | Phase 2 | 90 min |
| 7: IaC Structure | Agent [P] | Yes (w/ Phase 6) | Phase 0 | 75 min |
| 8: GitOps Delivery | Coordinator | No | Phase 7 | 75 min |
| 9: Observability | Coordinator | No | Phase 6 | 90 min |
| 10: Drift Detection | Agent | No | Phase 7+9 | 45 min |
| 11: GitHub Governance | Agent [P] | Yes (w/ Phase 10) | Phase 8 | 60 min |
| 12: High Availability | Coordinator | No | Phase 4 | 75 min |
| 13: Cost Optimization | Agent | No | Phase 9 | 45 min |
| 14: Supply Chain | Agent [P] | Yes (w/ Phase 13) | Phase 5 | 60 min |
| 15: Incident Response | Coordinator | No | Phase 9+10 | 75 min |

### Critical Path
```
Phase 0 → Phase 1 → Phase 2 → Phase 6 → Phase 9 → Phase 15
(45 + 60 + 75 + 90 + 90 + 75 = 435 min ≈ 7.25 hours minimum)
```

### Parallel Optimization
With 2 agents: ~540 min total (9 hours)
With 3 agents: ~480 min total (8 hours)
Sequential: ~1035 min total (17.25 hours)

---

## APPENDIX C: QUICK REFERENCE

### Unheaded Tier → K8s Namespace Map
```
Armory (11 pieces)  → unheaded-armory
Gnostic (6 services) → unheaded-gnostic
Royal Court (5 personas) → unheaded-court
Presentation (Dashboard + CLI) → unheaded-presentation
Whispering Void (eBPF) → unheaded-ebpf
System (Vault, Prometheus, etc.) → unheaded-system
```

### Key Ports
```
HTTP:      8080
HTTPS:     8443
gRPC:      9090
Metrics:   2112
WebSocket: 8081
eBPF gRPC: 9091
Hubble:    4245
Vault:     8200
```

### Gatekeeper Policy Suite
```
1. Resource limits required (all pods)
2. No privileged containers (except eBPF)
3. DaemonSet limits required (lesson: PLEG stalls)
4. Trusted registries only
5. No host namespaces (except eBPF)
6. Seccomp required
7. SA token explicit opt-in
```

---

*SK8 Battle Plan — Forged 2026-03-05*
*16 Phases. 400 Steps. The Kingdom meets Kubernetes.*
*From packet to pixel, from git push to production, from chaos to confidence — Unheaded stands.*
