{ buildGoModule, fetchFromGitHub }:

buildGoModule rec {
  pname = "wotan";
  version = "1.0.0";

  # TODO: Update with actual GitHub source when ready
  src = /opt/unheaded/wotan;

  # Vendor hash will be computed automatically
  vendorHash = null;

  subPackages = [ "cmd/wotan" ];

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  meta = {
    description = "Wotan message bus for Unheaded infrastructure platform";
    homepage = "https://github.com/unheaded/wotan";
    license = "MIT";
    maintainers = [ "Unheaded Team" ];
  };
}
