{ buildGoModule }:

buildGoModule rec {
  pname = "cuirass";
  version = "0.1.0";

  # cmd/cuirass has never existed. Cuirass is the armour-name for the control
  # plane, and the binary that implements it is unheaded-daemon — the same one
  # the Dockerfile's `cuirass` stage installs.
  src = ../../cmd/unheaded-daemon;

  vendorHash = null;

  subPackages = [ "." ];

  ldflags = [ "-s" "-w" "-X main.version=${version}" ];

  meta = {
    description = "Control plane for Unheaded container orchestration";
  };
}
