include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "${get_repo_root()}/deploy/tofu/modules/ebpf"
}

dependency "networking" {
  config_path = "../networking"
}

inputs = {
  environment               = "dev"
  ring_buffer_size_mb       = 16
  rate_limit_events_per_sec = 100
}
