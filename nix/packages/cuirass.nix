{ buildGoModule }:

buildGoModule rec {
  pname = "cuirass";
  version = "0.1.0";

  src = ../../cmd/cuirass;

  vendorHash = null;

  subPackages = [ "." ];

  ldflags = [ "-s" "-w" "-X main.version=${version}" ];

  meta = {
    description = "Control plane for Unheaded container orchestration";
  };
}
