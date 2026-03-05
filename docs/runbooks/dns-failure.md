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
