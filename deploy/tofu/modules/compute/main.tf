# Unheaded Compute Module
# Manages: All Armory deployment manifests, PDBs, HPA

variable "environment" {
  type = string
}

variable "replicas" {
  type = map(number)
  default = {
    shield       = 3
    cuirass      = 2
    gauntlets    = 2
    pauldrons    = 2
    hauberk      = 2
    greaves      = 2
    helm_runtime = 2
    gorget       = 2
    sword        = 1
  }
}

variable "resource_profile" {
  type    = string
  default = "standard"
  validation {
    condition     = contains(["minimal", "standard", "production"], var.resource_profile)
    error_message = "Must be minimal, standard, or production"
  }
}

# Deployment manifests applied via kubernetes_manifest
# Each armor piece gets its own deployment + service + PDB
