{
  description = "Unheaded Alpha - Infrastructure Platform";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }: {
    # Container definitions
    nixosConfigurations = {
      busboy = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [ ./containers/busboy.nix ];
      };

      timeguru = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [ ./containers/timeguru.nix ];
      };

      # TODO: Add other containers
    };
  };
}
