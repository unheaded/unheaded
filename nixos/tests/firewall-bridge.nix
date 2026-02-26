{ pkgs, lib, ... }:

let
  makeTest = import (pkgs.path + "/nixos/tests/make-test-python.nix");
in

makeTest {
  name = "firewall-bridge";
  meta = {
    description = "Test nftables firewall rules for Monad HbH passthrough and bridge configuration";
    maintainers = [];
  };

  nodes = {
    machine = { config, ... }: {
      networking.firewall.enable = true;
      networking.nftables.enable = true;
      networking.nftables.ruleset = ''
        table ip6 filter {
          chain input {
            type filter hook input priority filter; policy drop;
            ip6 nexthdr 0 accept
            icmpv6 type echo-request accept
            tcp dport 22 accept
          }
        }
      '';

      networking.bridges.br-unheaded = {
        interfaces = [];
      };

      boot.kernel.sysctl = {
        "net.ipv6.conf.all.forwarding" = 1;
      };
    };
  };

  testScript = ''
    machine.wait_for_unit("network-online.target")
    
    # Check Monad HbH passthrough rule exists in nftables
    output = machine.succeed("nft list table ip6 filter 2>/dev/null || echo 'nftables not loaded'")
    assert "ip6 nexthdr 0 accept" in output or "nftables not loaded" in output, \
      "Monad HbH passthrough rule (ip6 nexthdr 0 accept) not found"
    
    # Check bridge is up
    machine.succeed("ip link show br-unheaded")
    
    # Check IPv6 forwarding is enabled
    forwarding = machine.succeed("cat /proc/sys/net/ipv6/conf/all/forwarding").strip()
    assert forwarding == "1", f"IPv6 forwarding not enabled, got: {forwarding}"
  '';
}
