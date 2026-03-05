include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "${get_repo_root()}/deploy/tofu/modules/networking"
}

inputs = {
  cluster_name   = "unheaded-dev"
  hubble_enabled = true
}
