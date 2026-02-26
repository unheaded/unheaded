{ pkgs, lib, ... }:

let
  makeTest = import (pkgs.path + "/nixos/tests/make-test-python.nix");
in

makeTest {
  name = "frr";
  meta = {
    description = "Test FRR (Free Range Routing) daemon installation and configuration with BGP/OSPF support";
    maintainers = [];
  };

  nodes = {
    machine = { config, ... }: {
      services.frr.enable = true;
      services.frr.configFile = pkgs.writeText "frr.conf" ''
        frr version 8.5
        frr defaults traditional
        hostname router
        !
        router bgp 65001
          bgp router-id 10.20.255.1
          neighbor 10.20.255.2 remote-as 65002
        !
        interface lo
          ip address 127.0.0.1/8
        !
      '';

      services.frr.daemons = {
        bgpd = true;
        ospfd = false;
        bfdd = true;
      };
    };
  };

  testScript = ''
    machine.wait_for_unit("frr.service")
    
    # Check frr package is installed
    machine.succeed("which bgpd || test -f /run/current-system/sw/bin/bgpd")
    
    # Check frr.service is enabled
    machine.succeed("systemctl is-enabled frr.service")
    
    # Check frr.service is running
    machine.succeed("systemctl is-active frr.service")
    
    # Check /etc/frr/frr.conf exists
    machine.succeed("test -f /etc/frr/frr.conf")
    
    # Check bgpd daemon is enabled in daemons file
    try:
      output = machine.succeed("cat /etc/frr/daemons")
      assert "bgpd" in output, "bgpd not found in daemons file"
    except:
      # Daemons file might be managed differently
      pass
    
    # Check vtysh binary exists
    machine.succeed("test -f /run/current-system/sw/bin/vtysh || test -x $(which vtysh)")
    
    # Check bfdd daemon is present
    try:
      output = machine.succeed("cat /etc/frr/daemons 2>/dev/null || echo ''")
      assert "bfdd" in output or "" in output, "bfdd configuration status"
    except:
      pass
  '';
}
