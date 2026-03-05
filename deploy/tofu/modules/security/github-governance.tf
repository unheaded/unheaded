variable "org_name" { type = string; default = "unheaded" }

resource "github_repository" "template_go_service" {
  name                   = "template-go-service"
  visibility             = "private"
  is_template            = true
  delete_branch_on_merge = true
  allow_squash_merge     = true
  allow_merge_commit     = false
  vulnerability_alerts   = true
  security_and_analysis {
    secret_scanning { status = "enabled" }
    secret_scanning_push_protection { status = "enabled" }
  }
}

resource "github_actions_organization_permissions" "org" {
  allowed_actions      = "selected"
  enabled_repositories = "all"
  allowed_actions_config {
    github_owned_allowed = true
    verified_allowed     = true
    patterns_allowed     = ["unheaded/*"]
  }
}
