# Runbook: Vault Sealed

## Symptoms
- Services cannot read secrets
- Secret rotation fails
- vault-0 pod shows sealed status

## Severity: P1

## Triage
1. Check seal status: `kubectl -n unheaded-system exec vault-0 -- vault status`
2. Check pod health: `kubectl -n unheaded-system get pods -l app.kubernetes.io/name=vault`

## Mitigation
1. In dev mode: `kubectl -n unheaded-system exec vault-0 -- vault operator unseal <key>`
2. Check audit log: `kubectl -n unheaded-system exec vault-0 -- cat /vault/logs/audit.log | tail -20`
3. For production: follow unsealing ceremony procedure
