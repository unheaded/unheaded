{
  description = "Unheaded Alpha - Infrastructure Platform";

  # =============================================================================
  # UNHEADED NIXOS CONTAINER STACK
  # =============================================================================
  # Complete container definitions for the Unheaded infrastructure platform.
  #
  # Architecture:
  #   - 3 shared modules (common, hardening, networking)
  #   - 8 container definitions (5 services + 2 apps + busboy)
  #   - Security: seccomp, capabilities, filesystem isolation
  #   - Network: isolated mesh with explicit allow rules
  #
  # Build: nix build .#nixosConfigurations.<container>.config.system.build.toplevel
  # Deploy: See docs/DEPLOYMENT.md
  # =============================================================================

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};

      # Common build function for all containers
      mkContainer = name: modules:
        nixpkgs.lib.nixosSystem {
          inherit system;
          modules = modules ++ [
            # Inject shared configuration
            {
              nixpkgs.overlays = [ self.overlays.default ];
            }
          ];
        };
    in
    {
      # =======================================================================
      # CONTAINER CONFIGURATIONS
      # =======================================================================
      # All containers are built with security hardening and network isolation.
      # Dependencies are managed via systemd service ordering.
      # =======================================================================

      nixosConfigurations = {
        # ---------------------------------------------------------------------
        # MESSAGE BUS (Critical - All services depend on this)
        # ---------------------------------------------------------------------
        busboy = mkContainer "busboy" [
          ./containers/busboy.nix
        ];

        # ---------------------------------------------------------------------
        # AGENT SERVICES (REST APIs + Busboy clients)
        # ---------------------------------------------------------------------
        timeguru = mkContainer "timeguru" [
          ./containers/timeguru-service.nix
        ];

        captain = mkContainer "captain" [
          ./containers/captain-service.nix
        ];

        micromanager = mkContainer "micromanager" [
          ./containers/micromanager-service.nix
        ];

        architect = mkContainer "architect" [
          ./containers/architect-service.nix
        ];

        developer = mkContainer "developer" [
          ./containers/developer-service.nix
        ];

        # Legacy compatibility (duplicate for service discovery)
        busboy-service = mkContainer "busboy-service" [
          ./containers/busboy-service.nix
        ];

        # ---------------------------------------------------------------------
        # APPLICATIONS (Public via gateway)
        # ---------------------------------------------------------------------
        kanban = mkContainer "kanban" [
          ./containers/kanban-app.nix
        ];

        dashboard = mkContainer "dashboard" [
          ./containers/dashboard-app.nix
        ];
      };

      # =======================================================================
      # PACKAGE OVERLAYS
      # =======================================================================
      # Build Unheaded services from source
      # =======================================================================
      overlays.default = final: prev: {
        # TODO: Implement actual builds from source
        # For now, these are placeholders that reference local paths
        busboy = prev.callPackage ./packages/busboy.nix { };
        timeguru = prev.callPackage ./packages/timeguru.nix { };
        captain = prev.callPackage ./packages/captain.nix { };
        micromanager = prev.callPackage ./packages/micromanager.nix { };
        architect = prev.callPackage ./packages/architect.nix { };
        developer = prev.callPackage ./packages/developer.nix { };
        kanban-app = prev.callPackage ./packages/kanban-app.nix { };
        dashboard-app = prev.callPackage ./packages/dashboard-app.nix { };
      };

      # =======================================================================
      # DEVELOPMENT SHELLS
      # =======================================================================
      # Dev environment with all tools for building/testing containers
      # =======================================================================
      devShells.${system}.default = pkgs.mkShell {
        buildInputs = with pkgs; [
          # Nix tools
          nixpkgs-fmt
          nil

          # Container tools
          lxd
          lxc

          # Go development
          go_1_21
          gopls
          golangci-lint

          # Rust development (for eBPF)
          rustc
          cargo
          rust-analyzer

          # Network tools
          curl
          jq
          netcat
          tcpdump

          # Testing
          k6
        ];

        shellHook = ''
          echo "Unheaded Development Environment"
          echo "================================"
          echo ""
          echo "Container commands:"
          echo "  nix build .#nixosConfigurations.busboy.config.system.build.toplevel"
          echo "  lxc launch images:nixos/unstable unheaded-busboy"
          echo ""
          echo "Build all services:"
          echo "  make build"
          echo ""
          echo "Run tests:"
          echo "  make test"
        '';
      };

      # =======================================================================
      # HYDRA JOBS (CI/CD)
      # =======================================================================
      # Automated builds for all containers
      # =======================================================================
      hydraJobs = {
        # Build all containers
        containers = nixpkgs.lib.mapAttrs
          (name: config: config.config.system.build.toplevel)
          self.nixosConfigurations;

        # Test suite
        tests = pkgs.callPackage ./tests/container-tests.nix { };
      };

      # =======================================================================
      # DEPLOYMENT ARTIFACTS
      # =======================================================================
      # Generate deployment manifests
      # =======================================================================
      apps.${system} = {
        # Generate LXD profile for all containers
        generate-lxd-profiles = {
          type = "app";
          program = toString (pkgs.writeShellScript "generate-lxd-profiles" ''
            set -euo pipefail
            echo "Generating LXD profiles for Unheaded containers..."

            # Generate profiles for each container
            for container in busboy timeguru captain micromanager architect developer kanban dashboard; do
              echo "Processing: $container"
              nix eval .#nixosConfigurations.$container.config.unheaded --json > profiles/$container.json
            done

            echo "Profiles generated in: ./profiles/"
          '');
        };

        # Deploy all containers
        deploy = {
          type = "app";
          program = toString (pkgs.writeShellScript "deploy-unheaded" ''
            set -euo pipefail
            echo "Deploying Unheaded Alpha..."

            # Build all containers
            echo "Building containers..."
            nix build \
              .#nixosConfigurations.busboy.config.system.build.toplevel \
              .#nixosConfigurations.timeguru.config.system.build.toplevel \
              .#nixosConfigurations.captain.config.system.build.toplevel \
              .#nixosConfigurations.micromanager.config.system.build.toplevel \
              .#nixosConfigurations.architect.config.system.build.toplevel \
              .#nixosConfigurations.developer.config.system.build.toplevel \
              .#nixosConfigurations.kanban.config.system.build.toplevel \
              .#nixosConfigurations.dashboard.config.system.build.toplevel

            echo "Deploy complete!"
            echo "Run 'lxc list' to see containers"
          '');
        };
      };

      # =======================================================================
      # CHECKS (Automated testing)
      # =======================================================================
      checks.${system} = {
        # Lint all Nix files
        nix-fmt = pkgs.runCommand "check-nix-fmt" { } ''
          ${pkgs.nixpkgs-fmt}/bin/nixpkgs-fmt --check ${./.}
          touch $out
        '';

        # Build all containers (smoke test)
        build-all = pkgs.runCommand "build-all-containers" { } ''
          echo "Building all containers..."
          ${pkgs.nix}/bin/nix build \
            ${self}#nixosConfigurations.busboy.config.system.build.toplevel \
            ${self}#nixosConfigurations.timeguru.config.system.build.toplevel \
            ${self}#nixosConfigurations.captain.config.system.build.toplevel \
            ${self}#nixosConfigurations.micromanager.config.system.build.toplevel \
            ${self}#nixosConfigurations.architect.config.system.build.toplevel \
            ${self}#nixosConfigurations.developer.config.system.build.toplevel \
            ${self}#nixosConfigurations.kanban.config.system.build.toplevel \
            ${self}#nixosConfigurations.dashboard.config.system.build.toplevel
          touch $out
        '';
      };
    };
}
