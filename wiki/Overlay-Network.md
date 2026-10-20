# Overlay Network

West and east talk over WireGuard (`wg0`) on the direct P2P Ethernet link, with eBGP on top. Design and addressing plan: `docs/architecture/WIREGUARD_OVERLAY.md`; repo configs: `config/wireguard/west.conf`, `config/wireguard/east.conf`, `routing/frr/frr.conf`, `routing/bird/bird.conf`.

| | west | east |
|---|---|---|
| P2P link (IPv4, also SSH) | 192.168.13.2 (enp16s0) | 192.168.13.1 |
| wg0 | `fd00:dead:beef::1` | `fd00:dead:beef::2` |
| BGP | FRR, AS 65001 | BIRD, AS 65002 |
| originates | `fd00:dead:beef:1::/64` | `fd00:dead:beef:2::/64` |

Everything that names a host on the overlay uses these: Prometheus targets and remote_write in `monitoring/prometheus/prometheus.yml`, Grafana's VictoriaMetrics datasource, the validation scripts. SSH to east (`ssh east`) goes over the IPv4 P2P link, so overlay changes cannot cut it.

**2026-10-20:** the hosts ran the mirror image of this (west `::2`/`:2::/64`, east `::1`/`:1::/64`), so every `::1` in the repo reached east. Both hosts were re-addressed to the table above; backups of the previous wg0, FRR and BIRD configs are in `/var/lib/unheaded/remediate/20261020T145144Z-overlay-flip/` on each host. Verified: ping both ways, BGP Established with both prefixes exchanged.

Known gap: BIRD's BGP session has no `bfd on`, so the BFD peers FRR configures for east stay down. BGP does not depend on them (hold time 90 s).

Check:

```bash
ip -6 addr show wg0                                   # west: fd00:dead:beef::1
sudo vtysh -c 'show bgp ipv6 unicast summary'         # fd00:dead:beef::2 Established, 2 prefixes
ssh east 'sudo birdc show protocols'                  # west  BGP  up  Established
```
