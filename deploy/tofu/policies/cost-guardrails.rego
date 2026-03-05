package main

# Prevent accidentally creating oversized deployments
deny[msg] {
  resource := input.resource_changes[_]
  resource.type == "kubernetes_deployment"
  replicas := resource.change.after.spec.replicas
  replicas > 10
  msg := sprintf("Deployment '%s' has %d replicas — max 10 without cost approval", [resource.name, replicas])
}
