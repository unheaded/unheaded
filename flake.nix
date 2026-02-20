{
  description = "Unheaded - Configuration Management Automation IaC";

  # ========================================================================
  # NixOS Flake Configuration
  # ========================================================================
  # Development shell with all required tools for Unheaded development.
  # Both Go and Rust environments, plus additional utilities for testing
  # and deployment.
  #
  # Usage:
  #   nix flake update              # Update inputs
  #   nix develop                   # Enter dev shell
  #   nix build .#<target>          # Build specific target
  # ========================================================================

  inputs = {
    # NixOS Packages
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

    # Flake utilities for system detection
    flake-utils.url = "github:numtide/flake-utils";

    # For Rust toolchain management
    rust-overlay.url = "github:oxalica/rust-overlay";
  };

  outputs = { self, nixpkgs, flake-utils, rust-overlay }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        overlays = [ (import rust-overlay) ];
        pkgs = import nixpkgs {
          inherit system overlays;
        };

        # Rust toolchain with eBPF targets
        rustToolchain = pkgs.rust-bin.stable.latest.default.override {
          targets = [ "bpfel-unknown-none" ];
        };

        # Go with version pinned to 1.24
        go = pkgs.go_1_24;

      in
      {
        # ===================================================================
        # Development Shell
        # ===================================================================
        # Complete development environment with all dependencies needed
        # for Unheaded development, testing, and deployment.
        devShells.default = pkgs.mkShell {
          name = "unheaded-dev";

          buildInputs = with pkgs; [
            # ===============================================================
            # Core Development Tools
            # ===============================================================
            go
            gopls              # Go language server
            golangci-lint      # Go linting
            gosec              # Go security checker
            go-tools           # Additional Go tools (goimports, etc.)

            # ===============================================================
            # Rust / eBPF Development
            # ===============================================================
            rustToolchain
            rust-analyzer      # Rust language server
            cargo-edit         # Cargo dependency management
            cargo-watch        # Watch for changes and recompile
            llvm_18            # For eBPF compilation
            clang_18           # C compiler for eBPF
            libllvm            # LLVM libraries
            bpf                # BPF tools

            # ===============================================================
            # Protobuf / Code Generation
            # ===============================================================
            protobuf           # Protocol buffers
            protoc-gen-go      # Go protoc plugin
            protoc-gen-go-grpc # gRPC code generation for Go

            # ===============================================================
            # NixOS / Deployment
            # ===============================================================
            nix                # Nix package manager
            nixpkgs-fmt        # Nix formatter
            nil                # Nix LSP
            lxd                # Linux Containers (for NixOS containers)
            lxc                # LXC client

            # ===============================================================
            # Testing & Benchmarking
            # ===============================================================
            k6                 # Load testing
            sqlite             # SQLite for testing

            # ===============================================================
            # Debugging & Profiling
            # ===============================================================
            gdb                # GNU Debugger
            valgrind           # Memory profiling
            strace             # System call tracer
            perf               # Linux performance tools

            # ===============================================================
            # Network & Observability
            # ===============================================================
            curl               # HTTP client
            jq                 # JSON processing
            netcat-gnu         # Network testing
            tcpdump            # Packet capture
            mtr                # Network diagnostic tool

            # ===============================================================
            # Documentation
            # ===============================================================
            mdbook             # Markdown documentation generator
            graphviz           # Diagram generation

            # ===============================================================
            # Utilities
            # ===============================================================
            git                # Version control
            git-crypt          # Encrypted git files
            gnupg              # GPG for signing
            openssh            # SSH client/server
          ];

          # Shell initialization and helpful information
          shellHook = ''
            echo "=========================================="
            echo "Unheaded Development Environment"
            echo "=========================================="
            echo ""
            echo "Go version: $(${go}/bin/go version)"
            echo "Rust version: $(rustc --version)"
            echo ""
            echo "Quick Start:"
            echo "  make help          - Show available make targets"
            echo "  make build         - Build all binaries"
            echo "  make test          - Run all tests"
            echo "  make lint          - Run linters"
            echo ""
            echo "Development:"
            echo "  ./scripts/build.sh - Build with version metadata"
            echo "  nix flake show     - Show flake outputs"
            echo ""
            echo "Container Commands:"
            echo "  nix build .#nixosConfigurations.wotan.config.system.build.toplevel"
            echo "  lxc launch images:nixos/unstable unheaded-wotan"
            echo ""
            echo "Deploy:"
            echo "  make deploy        - Deploy to host"
            echo "  make setup-host    - Setup host environment"
            echo ""
          '';
        };

        # ===================================================================
        # Packages
        # ===================================================================
        # Individual package definitions can be added here for releases
        packages = {
          default = null; # No default package
        };

        # ===================================================================
        # Checks (CI/CD targets)
        # ===================================================================
        # Automated checks that run in CI
        checks = {
          # Lint all Nix files
          nix-fmt = pkgs.runCommand "check-nix-fmt" { } ''
            ${pkgs.nixpkgs-fmt}/bin/nixpkgs-fmt --check ${./.}
            touch $out
          '';

          # Check flake evaluation
          flake = pkgs.runCommand "flake-check" { } ''
            ${pkgs.nix}/bin/nix flake check ${./.}
            touch $out
          '';
        };

        # ===================================================================
        # Apps
        # ===================================================================
        # Runnable flake apps
        apps = {
          # Example: nix run .#build
          build = {
            type = "app";
            program = "${self.outputs.devShells.${system}.default}/bin/bash";
          };
        };
      }
    ) // {
      # ===================================================================
      # Container Configurations (NixOS)
      # ===================================================================
      # Note: These come from nix/flake.nix but can be referenced here
      # for combined deployment workflows
      # ===================================================================
    };
}
