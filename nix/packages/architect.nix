{ buildGoModule }:

buildGoModule rec {
  pname = "architect";
  version = "0.1.0";

  src = ../../services/architect;

  vendorHash = null;

  subPackages = [ "." ];

  ldflags = [ "-s" "-w" "-X main.version=${version}" ];

  meta = {
    description = "Architecture service for Unheaded";
  };
}
