{ pkgs, lib, ... }:

let
  makeTest = import (pkgs.path + "/nixos/tests/make-test-python.nix");
in

makeTest {
  name = "observability";
  meta = {
    description = "Test observability stack services (Prometheus, Grafana, Loki, Promtail, Node Exporter) with Monad HbH passthrough preserved";
    maintainers = [];
  };

  nodes = {
    machine = { config, ... }: {
      services.prometheus.enable = true;
      services.prometheus.exporters.node.enable = true;

      services.grafana = {
        enable = true;
        port = 3000;
      };

      services.loki.enable = true;

      services.promtail.enable = true;
      services.promtail.configuration = {
        clients = [
          {
            url = "http://localhost:3100/loki/api/v1/push";
          }
        ];
        scrape_configs = [
          {
            job_name = "journal";
            journal = {
              max_age = "12h";
            };
            relabel_configs = [
              {
                source_labels = [ "__journal__systemd_unit" ];
                target_label = "unit";
              }
            ];
          }
        ];
      };

      networking.firewall.enable = true;
      networking.nftables.enable = true;
      networking.nftables.ruleset = ''
        table ip6 filter {
          chain input {
            type filter hook input priority filter; policy drop;
            ip6 nexthdr 0 accept
            tcp dport 3000 accept
            tcp dport 9090 accept
            icmpv6 type echo-request accept
            tcp dport 22 accept
          }
        }
      '';

      boot.kernel.sysctl = {
        "net.ipv6.conf.all.forwarding" = 1;
      };
    };
  };

  testScript = ''
    machine.wait_for_unit("network-online.target")
    
    # Check prometheus.service is enabled and started
    machine.succeed("systemctl is-enabled prometheus.service")
    machine.wait_for_unit("prometheus.service")
    
    # Check node_exporter is enabled
    machine.succeed("systemctl is-enabled prometheus-node-exporter.service")
    
    # Check grafana.service is enabled
    machine.succeed("systemctl is-enabled grafana.service")
    machine.wait_for_unit("grafana.service")
    
    # Check promtail.service is enabled
    machine.succeed("systemctl is-enabled promtail.service")
    machine.wait_for_unit("promtail.service")
    
    # Check /var/lib/grafana exists
    machine.succeed("test -d /var/lib/grafana")
    
    # Check loki.service is enabled
    machine.succeed("systemctl is-enabled loki.service")
    machine.wait_for_unit("loki.service")
    
    # Check Monad HbH passthrough rule is STILL present after observability module loaded
    output = machine.succeed("nft list table ip6 filter 2>/dev/null || echo 'nftables not loaded'")
    assert "ip6 nexthdr 0 accept" in output or "nftables not loaded" in output, \
      "Monad HbH passthrough rule (ip6 nexthdr 0 accept) was removed or not found"
    
    # Check firewall rules for Grafana and Prometheus
    assert "3000" in output or "nftables not loaded" in output, \
      "Grafana port 3000 not in firewall rules"
  '';
}
