# Root Terragrunt config — Unheaded IaC
# State key auto-generated from directory path
# Every layer gets isolated state = isolated blast radius

locals {
  environment = basename(dirname(get_terragrunt_dir()))
  layer       = basename(get_terragrunt_dir())
}

remote_state {
  backend = "local"
  config = {
    path = "${get_repo_root()}/deploy/tofu/.state/${local.environment}/${local.layer}/terraform.tfstate"
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<PROVIDER
terraform {
  required_version = ">= 1.6.0"
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.25"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.12"
    }
  }
}
PROVIDER
}
