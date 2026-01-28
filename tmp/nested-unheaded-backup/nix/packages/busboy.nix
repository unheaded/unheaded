{ buildGoModule, fetchFromGitHub }:

buildGoModule rec {
  pname = "busboy";
  version = "1.0.0";

  # TODO: Update with actual GitHub source when ready
  src = /opt/unheaded/busboy;

  # Vendor hash will be computed automatically
  vendorHash = null;

  subPackages = [ "cmd/busboy" ];

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  meta = {
    description = "Busboy message bus for Unheaded infrastructure platform";
    homepage = "https://github.com/unheaded/busboy";
    license = "MIT";
    maintainers = [ "Unheaded Team" ];
  };
}
