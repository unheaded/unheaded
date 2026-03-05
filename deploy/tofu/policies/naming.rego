package main

# All resources must follow Unheaded naming convention
deny[msg] {
  resource := input.resource_changes[_]
  not startswith(resource.name, "unheaded-")
  msg := sprintf("Resource '%s' must be prefixed with 'unheaded-'", [resource.name])
}
