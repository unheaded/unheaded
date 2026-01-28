{ pkgs, lib, ... }:

# ==============================================================================
# CONTAINER TEST SUITE - NIX BUILD
# ==============================================================================
# Automated testing for Unheaded NixOS container stack.
#
# Tests run in isolated NixOS VMs via nixosTest framework.
# Verifies: startup, health, security, networking, observability.
#
# Run: nix build .#hydraJobs.tests
# ==============================================================================

pkgs.nixosTest {
  name = "unheaded-container-tests";

  nodes = {
    # Test machine with LXD
    testHost = { config, pkgs, ... }: {
      # LXD setup
      virtualisation.lxd.enable = true;
      virtualisation.lxd.recommendedSysctlSettings = true;

      # Test dependencies
      environment.systemPackages = with pkgs; [
        lxd
        lxc
        curl
        jq
        go_1_21
      ];

      # Network bridge for containers
      networking.bridges.lxdbr0 = {
        interfaces = [ ];
      };

      networking.interfaces.lxdbr0.ipv4.addresses = [{
        address = "10.10.10.1";
        prefixLength = 24;
      }];

      # Copy test suite
      environment.etc."unheaded-tests/container_test.go".source = ./container_test.go;
      environment.etc."unheaded-tests/go.mod".text = ''
        module github.com/unheaded/unheaded/nix/tests

        go 1.21
      '';
    };
  };

  testScript = ''
    start_all()

    # Wait for LXD to be ready
    testHost.wait_for_unit("lxd.service")
    testHost.succeed("lxd waitready")

    # Initialize LXD
    testHost.succeed("lxd init --auto")

    # Import container images (built from flake)
    # TODO: Import actual built images
    # testHost.succeed("lxc image import /nix/store/.../busboy.tar.gz --alias unheaded-busboy")

    # Run Go test suite
    print("Running container orchestration tests...")
    testHost.succeed("""
      cd /etc/unheaded-tests && \
      go test -v -race -timeout=10m . 2>&1 | tee test-output.log
    """)

    # Verify test results
    testHost.succeed("grep -q 'PASS' /etc/unheaded-tests/test-output.log")

    # Cleanup
    testHost.succeed("lxc delete --force $(lxc list -c n --format csv) || true")
  '';

  meta = {
    description = "Integration tests for Unheaded NixOS container stack";
    maintainers = [ "Unheaded Team" ];
  };
}
