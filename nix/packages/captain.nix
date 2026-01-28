{ buildGoModule }:

buildGoModule rec {
  pname = "captain";
  version = "0.1.0";

  src = ../../services/captain;

  vendorHash = null;

  subPackages = [ "." ];

  ldflags = [ "-s" "-w" "-X main.version=${version}" ];

  meta = {
    description = "Strategy service for Unheaded";
  };
}
