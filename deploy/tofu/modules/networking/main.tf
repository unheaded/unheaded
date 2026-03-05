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
