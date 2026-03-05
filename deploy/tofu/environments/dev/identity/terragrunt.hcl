include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "${get_repo_root()}/deploy/tofu/modules/identity"
}

inputs = {
  vault_enabled = true
}
