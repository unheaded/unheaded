{ buildGoModule }:

buildGoModule rec {
  pname = "timeguru";
  version = "0.1.0";

  src = ../../services/timeguru;

  vendorHash = null;

  subPackages = [ "." ];

  ldflags = [ "-s" "-w" "-X main.version=${version}" ];

  meta = {
    description = "Timeline tracking service for Unheaded";
  };
}
