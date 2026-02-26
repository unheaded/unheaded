{
  firewall-bridge = import ./firewall-bridge.nix;
  wireguard = import ./wireguard.nix;
  frr = import ./frr.nix;
  observability = import ./observability.nix;
}
