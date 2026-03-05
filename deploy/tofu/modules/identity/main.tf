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
