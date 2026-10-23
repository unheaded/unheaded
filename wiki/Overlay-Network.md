# Overlay Network

West and east talk over WireGuard (`wg0`) on the direct P2P Ethernet link, with eBGP on top. Design and addressing plan: `docs/architecture/WIREGUARD_OVERLAY.md`; repo configs: `config/wireguard/west.conf`, `config/wireguard/east.conf`, `routing/frr/west.conf`, `routing/bird/east.conf` (copies of what is deployed; `routing/frr/frr.conf` and `routing/bird/bird.conf` are the host-a/host-b design, not what runs).

| | west | east |
|---|---|---|
| P2P link (IPv4, also SSH) | 192.168.13.2 (enp16s0) | 192.168.13.1 |
| wg0 | `fd00:dead:beef::1` | `fd00:dead:beef::2` |
| BGP | FRR, AS 65001 | BIRD, AS 65002 |
| originates | `fd00:dead:beef:1::/64` | `fd00:dead:beef:2::/64` |

Everything that names a host on the overlay uses these: Prometheus targets and remote_write in `monitoring/prometheus/prometheus.yml`, Grafana's VictoriaMetrics datasource, the validation scripts. SSH to east (`ssh east`) goes over the IPv4 P2P link, so overlay changes cannot cut it.

LLDP on the P2P link: east runs lldpd (installed 2026-10-23, user `_lldpd`), announcing only on `enp2s0` as `east` (`/etc/lldpd.d/unheaded.conf`: `interface pattern enp2s0`, `system hostname east`; widen the pattern to announce on more links). West does not announce yet; orb's links page on west hears east (and the home router, which speaks LLDP on Wi-Fi) with its own listener.

**2026-10-20:** the hosts ran the mirror image of this (west `::2`/`:2::/64`, east `::1`/`:1::/64`), so every `::1` in the repo reached east. Both hosts were re-addressed to the table above; backups of the previous wg0, FRR and BIRD configs are in `/var/lib/unheaded/remediate/20261020T145144Z-overlay-flip/` on each host. Verified: ping both ways, BGP Established with both prefixes exchanged.

Alerting: west's monitoring Prometheus scrapes east's node_exporter at `[fd00:dead:beef::2]:9100` over wg0, so `NodeExporterDown` for that target fires when the overlay (or east) is down.

BFD: one multihop session (UDP 4784, 300 ms, multiplier 3, so about 0.9 s to detect a dead path) that both BGP sessions are bound to: BIRD `bfd on`, FRR `neighbor ... bfd`. BIRD sends multihop BFD with hop limit 64, so FRR's peer sets `minimum-ttl 64`. Until 2026-10-20 BIRD never asked for a session and FRR only accepted hop limit 254+, so BFD had never come up and a dead tunnel took the 90 s BGP hold time to notice. Previous configs: `/var/lib/unheaded/remediate/*-bfd/` on each host.

Check:

```bash
ip -6 addr show wg0                                   # west: fd00:dead:beef::1
sudo vtysh -c 'show bgp ipv6 unicast summary'         # fd00:dead:beef::2 Established, 2 prefixes
ssh east 'sudo birdc show protocols'                  # west  BGP  up  Established
sudo vtysh -c 'show bfd peers brief'                  # fd00:dead:beef::2  up
ssh east 'sudo birdc show bfd sessions'               # fd00:dead:beef::1  Up  0.300
```
