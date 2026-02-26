{
  description = "Unheaded Kingdom infrastructure flake (Forge & Outpost)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    home-manager = {
      url = "github:nix-community/home-manager";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, home-manager, ... }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};
    in
    {
      nixosConfigurations = {
        host-a = nixpkgs.lib.nixosSystem {
          inherit system;
          modules = [
            ./hosts/host-a/configuration.nix
          ];
        };

        host-b = nixpkgs.lib.nixosSystem {
          inherit system;
          modules = [
            ./hosts/host-b/configuration.nix
          ];
        };
      };

      overlays = { };

      packages.${system} = { };

      devShells.${system}.default = pkgs.mkShell {
        buildInputs = with pkgs; [
          frr
          bird2
          suricata
        ];
      };

      checks.${system} = {
        nixos-firewall-bridge = import ./tests/firewall-bridge.nix { inherit nixpkgs; };
        nixos-wireguard = import ./tests/wireguard.nix { inherit nixpkgs; };
        nixos-observability = import ./tests/observability.nix { inherit nixpkgs; };
      };
    };
}
