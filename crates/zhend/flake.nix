{
  description = "zhend — Zhen Layer 0: Anti-fragile knowledge substrate for the Unheaded Protocol";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    rust-overlay = {
      url = "github:oxalica/rust-overlay";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, flake-utils, rust-overlay }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        overlays = [ (import rust-overlay) ];
        pkgs = import nixpkgs { inherit system overlays; };

        rustToolchain = pkgs.rust-bin.stable.latest.default.override {
          extensions = [ "rust-src" "rust-analyzer" ];
        };

      in {
        packages.default = pkgs.callPackage ./nix/default.nix {};

        devShells.default = pkgs.mkShell {
          buildInputs = [
            rustToolchain
            pkgs.pkg-config
            pkgs.protobuf
            pkgs.openssl
            pkgs.cargo-audit
            pkgs.cargo-deny
            pkgs.cargo-watch
            pkgs.cargo-nextest
          ];

          shellHook = ''
            echo "真 zhend dev shell — the Library that cannot burn"
            echo "  cargo build         — build zhend"
            echo "  cargo test          — run tests"
            echo "  cargo bench         — benchmarks"
            echo "  cargo audit         — security audit"
            echo "  cargo deny check    — license + advisory check"
          '';
        };
      }
    ) // {
      # NixOS module.
      nixosModules.default = import ./nix/module.nix;
      nixosModules.zhend = import ./nix/module.nix;
    };
}
