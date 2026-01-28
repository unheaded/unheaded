{ buildGoModule }:

buildGoModule rec {
  pname = "micromanager";
  version = "0.1.0";

  src = ../../services/micromanager;

  vendorHash = null;

  subPackages = [ "." ];

  ldflags = [ "-s" "-w" "-X main.version=${version}" ];

  meta = {
    description = "Execution service for Unheaded";
  };
}
