# Runbook: eBPF Collector Failure

## Symptoms
- EbpfCollectorDown alert fires
- Dashboard packet-flow stops updating
- void-collector pods in CrashLoopBackOff

## Severity: P2

## Triage
1. Check collector pods: `kubectl get pods -n unheaded-ebpf -l app=void-collector`
2. Check logs: `kubectl logs -n unheaded-ebpf -l app=void-collector --tail=50`
3. Check BPF maps: `kubectl exec -n unheaded-ebpf <pod> -- bpftool map show`

## Common Causes
- BPF map exhaustion
- Ring buffer overflow (check drop rate)
- Kernel version mismatch
- Missing BPF/PERFMON capabilities

## Mitigation
1. Restart collector: `kubectl rollout restart ds/whispering-void-collector -n unheaded-ebpf`
2. If ring buffer full: increase size in void-collector-config ConfigMap
3. Check capabilities: ensure BPF, PERFMON, NET_ADMIN caps granted
