{ buildGoModule }:

buildGoModule rec {
  pname = "kanban-app";
  version = "0.1.0";

  src = ../../cmd/kanban-app;

  vendorHash = null;

  subPackages = [ "." ];

  ldflags = [ "-s" "-w" "-X main.version=${version}" ];

  meta = {
    description = "Kanban board app for Unheaded meta moment";
  };
}
