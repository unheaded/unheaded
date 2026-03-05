include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "${get_repo_root()}/deploy/tofu/modules/compute"
}

dependency "networking" {
  config_path = "../networking"
}

inputs = {
  environment      = "dev"
  resource_profile = "minimal"
}
