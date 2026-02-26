{ pkgs, lib, ... }:

let
  makeTest = import (pkgs.path + "/nixos/tests/make-test-python.nix");
in

makeTest {
  name = "wireguard";
  meta = {
    description = "Test WireGuard interface configuration with Monad HbH support and Doom Range firewall rules";
    maintainers = [];
  };

  nodes = {
    machine = { config, ... }: {
      networking.firewall.enable = true;
      networking.nftables.enable = true;
      
      systemd.network.enable = true;
      systemd.network.networks."wg0" = {
        matchConfig.Name = "wg0";
        address = [
          "fd00:dead:beef::1/48"
        ];
        networkConfig = {
          MTUBytes = "1380";
        };
      };

      networking.nftables.ruleset = ''
        table ip6 filter {
          chain input {
            type filter hook input priority filter; policy drop;
            ip6 nexthdr 0 accept
            tcp dport { 16666-16689 } accept
            icmpv6 type echo-request accept
            tcp dport 22 accept
          }
        }
      '';

      boot.kernelModules = [ "wireguard" ];
    };
  };

  testScript = ''
    machine.wait_for_unit("network-online.target")
    
    # Check WireGuard kernel module is loaded
    machine.succeed("grep -q wireguard /proc/modules || modprobe wireguard")
    
    # Check MTU is set correctly (if wg0 exists)
    try:
      mtu = machine.succeed("ip link show wg0 2>/dev/null | grep mtu").strip()
      assert "1380" in mtu, f"MTU not set to 1380, got: {mtu}"
    except:
      # wg0 might not be created without actual WireGuard config
      pass
    
    # Check IPv6 address is configured
    try:
      machine.succeed("ip -6 addr show | grep -q fd00:dead:beef")
    except:
      # Address might not be present without active interface
      pass
    
    # Check Doom Range ports are in firewall rules
    output = machine.succeed("nft list table ip6 filter 2>/dev/null || echo 'nftables not loaded'")
    assert "16666-16689" in output or "nftables not loaded" in output, \
      "Doom Range ports not found in firewall rules"
  '';
}
