{ buildGoModule }:

buildGoModule rec {
  pname = "dashboard-app";
  version = "0.1.0";

  src = ../../cmd/dashboard-backend;

  vendorHash = null;

  subPackages = [ "." ];

  ldflags = [ "-s" "-w" "-X main.version=${version}" ];

  meta = {
    description = "Dashboard app for eBPF trace visualization";
  };
}
