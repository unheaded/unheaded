{
  description = "Unheaded Alpha - Infrastructure Platform";

  # =============================================================================
  # UNHEADED NIXOS CONTAINER STACK
  # =============================================================================
  # Complete container definitions for the Unheaded infrastructure platform.
  #
  # Architecture:
  #   - 4 shared modules (base, common, hardening, networking)
  #   - 10 container definitions:
  #     - Control plane: cuirass
  #     - Message bus: busboy
  #     - Agent services: timeguru, captain, micromanager, architect, developer
  #     - Applications: kanban, dashboard
  #   - Security: seccomp, capabilities, read-only FS, minimal capabilities
  #   - Network: isolated mesh with default-deny, explicit allow rules
  #
  # Build: nix build .#nixosConfigurations.<container>.config.system.build.toplevel
  # Deploy: See docs/DEPLOYMENT.md
  # =============================================================================

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      # Supported systems for development shells and packages
      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];

      # Container system (NixOS containers run on Linux only)
      containerSystem = "x86_64-linux";

      # Common build function for all containers
      mkContainer = name: modules:
        nixpkgs.lib.nixosSystem {
          system = containerSystem;
          modules = modules ++ [
            # Inject base container module
            ./containers/base.nix
            # Inject shared configuration
            {
              nixpkgs.overlays = [ self.overlays.default ];
            }
          ];
        };
    in
    # Use flake-utils for per-system outputs (devShells, packages, checks)
    flake-utils.lib.eachSystem supportedSystems (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        # =======================================================================
        # DEVELOPMENT SHELLS
        # =======================================================================
        devShells.default = pkgs.mkShell {
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
        # CHECKS (Automated testing)
        # =======================================================================
        checks = {
          # Lint all Nix files
          nix-fmt = pkgs.runCommand "check-nix-fmt" { } ''
            ${pkgs.nixpkgs-fmt}/bin/nixpkgs-fmt --check ${./.}
            touch $out
          '';
        };
      }
    ) // {
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

        # developer: Placeholder - no service code yet, will be added post-alpha

        # ---------------------------------------------------------------------
        # STATE & KNOWLEDGE SERVICES
        # ---------------------------------------------------------------------
        monad = mkContainer "monad" [
          ./containers/monad-service.nix
        ];

        sophia = mkContainer "sophia" [
          ./containers/sophia-service.nix
        ];

        # Legacy compatibility (duplicate for service discovery)
        busboy-service = mkContainer "busboy-service" [
          ./containers/busboy-service.nix
        ];

        # ---------------------------------------------------------------------
        # GATEWAY (Public access point)
        # ---------------------------------------------------------------------
        gateway = mkContainer "gateway" [
          ./containers/gateway.nix
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

        # ---------------------------------------------------------------------
        # CONTROL PLANE
        # ---------------------------------------------------------------------
        cuirass = mkContainer "cuirass" [
          ./containers/cuirass.nix
        ];
      };

      # =======================================================================
      # PACKAGE OVERLAYS
      # =======================================================================
      # Build Unheaded services from source
      # =======================================================================
      overlays.default = final: prev: {
        # ALPHA: Uses pre-built binaries. Build from Go source (services/*/) planned for Age 2. See docs/SERVICE_BREAKOUT_STRATEGY.md
        # For now, these are placeholders that reference local paths
        busboy = prev.callPackage ./packages/busboy.nix { };
        timeguru = prev.callPackage ./packages/timeguru.nix { };
        captain = prev.callPackage ./packages/captain.nix { };
        micromanager = prev.callPackage ./packages/micromanager.nix { };
        architect = prev.callPackage ./packages/architect.nix { };
        monad = prev.callPackage ./packages/monad.nix { };
        sophia = prev.callPackage ./packages/sophia.nix { };
        kanban-app = prev.callPackage ./packages/kanban-app.nix { };
        dashboard-app = prev.callPackage ./packages/dashboard-app.nix { };
        cuirass = prev.callPackage ./packages/cuirass.nix { };
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
        tests = nixpkgs.legacyPackages.x86_64-linux.callPackage ./tests/container-tests.nix { };
      };

      # =======================================================================
      # DEPLOYMENT ARTIFACTS
      # =======================================================================
      # Generate deployment manifests (using flake-utils for per-system apps)
      # =======================================================================
      apps = flake-utils.lib.eachDefaultSystemMap (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          # Generate LXD profile for all containers
          generate-lxd-profiles = {
            type = "app";
            program = toString (pkgs.writeShellScript "generate-lxd-profiles" ''
              set -euo pipefail
              echo "Generating LXD profiles for Unheaded containers..."

              # Generate profiles for each container
              for container in busboy timeguru captain micromanager architect monad sophia gateway kanban dashboard cuirass; do
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
                .#nixosConfigurations.monad.config.system.build.toplevel \
                .#nixosConfigurations.sophia.config.system.build.toplevel \
                .#nixosConfigurations.gateway.config.system.build.toplevel \
                .#nixosConfigurations.kanban.config.system.build.toplevel \
                .#nixosConfigurations.dashboard.config.system.build.toplevel \
                .#nixosConfigurations.cuirass.config.system.build.toplevel

              echo "Deploy complete!"
              echo "Run 'lxc list' to see containers"
            '');
          };
        }
      );
    };
}
