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
3. Check PLEG dashboard: Grafana > Node Health > PLEG Relist Duration
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
