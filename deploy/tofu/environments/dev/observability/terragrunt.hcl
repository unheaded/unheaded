include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "${get_repo_root()}/deploy/tofu/modules/observability"
}

dependency "networking" {
  config_path = "../networking"
}

dependency "compute" {
  config_path = "../compute"
}

inputs = {
  environment            = "dev"
  retention_days         = 7
  grafana_admin_password = "changeme-dev"
}
