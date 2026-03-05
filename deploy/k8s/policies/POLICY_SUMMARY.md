=== UNHEADED GATEKEEPER POLICY SUITE ===

| Policy | Type | Scope | Exceptions |
|--------|------|-------|------------|
| Resource Limits Required | All Pods | unheaded-* | None |
| No Privileged Containers | All Pods | All except ebpf | unheaded-ebpf |
| DaemonSet Limits Required | DaemonSets | All except kube-system | kube-system |
| Trusted Registries Only | All Pods | All except system | kube-system, gatekeeper |
| No Host Namespaces | All Pods | All except ebpf | unheaded-ebpf, kube-system |
| Seccomp Required | All Pods | All except ebpf | unheaded-ebpf, kube-system |
| SA Token Opt-In | All Pods | All | None |
