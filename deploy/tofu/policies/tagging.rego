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
