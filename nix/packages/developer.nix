{ buildGoModule }:

buildGoModule rec {
  pname = "developer";
  version = "0.1.0";

  src = ../../services/developer;

  vendorHash = null;

  subPackages = [ "." ];

  ldflags = [ "-s" "-w" "-X main.version=${version}" ];

  meta = {
    description = "Development service for Unheaded";
  };
}
