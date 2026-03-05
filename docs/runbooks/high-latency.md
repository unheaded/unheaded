# Runbook: High Latency (P99 > 2s)

## Symptoms
- Grafana alert: HighLatency fires
- Dashboard shows slow responses
- Users report timeouts

## Severity: P2

## Triage
1. Check which service: Grafana > Kingdom Overview > P99 Latency
2. Check pod resources: `kubectl top pods -n <namespace> --sort-by=cpu`
3. Check network: `hubble observe -n <namespace> --last 50`

## Common Causes
- Pod resource exhaustion (near limits)
- Network policy dropping legitimate traffic
- Upstream dependency slowdown
- GC pressure (Go services)

## Mitigation
1. Scale up: `kubectl scale deploy/<service> -n <ns> --replicas=<n+1>`
2. Check resource limits: increase if near ceiling
3. Check HPA is active and scaling
