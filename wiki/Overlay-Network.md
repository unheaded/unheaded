# Overlay Network

West and east talk over WireGuard (`wg0`) on the direct P2P Ethernet link, with eBGP on top. Design and addressing plan: `docs/architecture/WIREGUARD_OVERLAY.md`; repo configs: `config/wireguard/west.conf`, `config/wireguard/east.conf`, `routing/frr/west.conf`, `routing/bird/east.conf` (copies of what is deployed; `routing/frr/frr.conf` and `routing/bird/bird.conf` are the host-a/host-b design, not what runs).

| | west | east |
|---|---|---|
| P2P link (IPv4, also SSH) | 192.168.13.2 (enp16s0) | 192.168.13.1 |
| wg0 | `fd00:dead:beef::1` | `fd00:dead:beef::2` |
| BGP | FRR, AS 65001 | BIRD, AS 65002 |
| originates | `fd00:dead:beef:1::/64` | `fd00:dead:beef:2::/64` |

Everything that names a host on the overlay uses these: Prometheus targets and remote_write in `monitoring/prometheus/prometheus.yml`, Grafana's VictoriaMetrics datasource, the validation scripts. SSH to east (`ssh east`) goes over the IPv4 P2P link, so overlay changes cannot cut it.

LLDP on the P2P link: both hosts run lldpd (installed 2026-10-23, user `_lldpd`), announcing only on the P2P interface: east on `enp2s0`, west on `enp16s0` (`/etc/lldpd.d/unheaded.conf`: `interface pattern <if>`, `system hostname <host>`; widen the pattern to announce on more links). `sudo lldpcli show neighbors` on either shows the other; orb's links page hears it too (and the home router, which speaks LLDP on Wi-Fi) with its own listener.

**2026-10-20:** the hosts ran the mirror image of this (west `::2`/`:2::/64`, east `::1`/`:1::/64`), so every `::1` in the repo reached east. Both hosts were re-addressed to the table above; backups of the previous wg0, FRR and BIRD configs are in `/var/lib/unheaded/remediate/20261020T145144Z-overlay-flip/` on each host. Verified: ping both ways, BGP Established with both prefixes exchanged.

Alerting: west's monitoring Prometheus scrapes east's node_exporter at `[fd00:dead:beef::2]:9100` over wg0, so `NodeExporterDown` for that target fires when the overlay (or east) is down.

BFD: one multihop session (UDP 4784, 300 ms, multiplier 3, so about 0.9 s to detect a dead path) that both BGP sessions are bound to: BIRD `bfd on`, FRR `neighbor ... bfd`. BIRD sends multihop BFD with hop limit 64, so FRR's peer sets `minimum-ttl 64`. Until 2026-10-20 BIRD never asked for a session and FRR only accepted hop limit 254+, so BFD had never come up and a dead tunnel took the 90 s BGP hold time to notice. Previous configs: `/var/lib/unheaded/remediate/*-bfd/` on each host.

## Pre-shared key (the post-quantum part)

wg0 mixes a pre-shared key into every handshake, so recorded tunnel traffic stays confidential even if Curve25519 falls to a quantum computer, as long as the PSK stays secret (ADR-105). One PSK since setup (2026-03-25), never rotated (FND-016).

| | west | east |
|---|---|---|
| PSK lives in | inline `PresharedKey =` in `/etc/wireguard/wg0.conf` | `/etc/wireguard/preshared.key` and `wg0.conf` |
| after the first rotation | also `/etc/wireguard/wg0.psk` (mtime = last rotation) | same |

- **Rotation tool:** `scripts/wireguard/rotate-psk.sh` on west (dry run by default; `--apply` rotates). It refuses unless SSH to east negotiated a post-quantum KEX, moves the PSK through pipes and 0600 files only, waits up to 200 s for a handshake (WireGuard uses a new PSK only at its next handshake, after 120 s) and rolls both hosts back to `<if>.psk.old` on failure. Journal lines: `psk-rotated`, `psk-rotation-rolled-back`. Test harness: `scripts/wireguard/test-rotate-psk.sh` (meta-gate row `wireguard/test-rotate-psk`).
- **Timer:** `deploy/systemd/unheaded-wg-psk-rotate.timer` (weekly, 1 h random delay) runs `unheaded-wg-psk-rotate.service` → `/usr/local/lib/unheaded/wireguard/rotate-psk.sh --apply`. Shipped, **not installed or enabled**: the first live rotation and enabling it are E3 (`docs/security/pq-host-escalations.md`).
- **Check:** `sudo wg show wg0 latest-handshakes` (current) and `sudo stat -c %y /etc/wireguard/wg0.psk` (under 8 days once the timer runs).

Check:

```bash
ip -6 addr show wg0                                   # west: fd00:dead:beef::1
sudo vtysh -c 'show bgp ipv6 unicast summary'         # fd00:dead:beef::2 Established, 2 prefixes
ssh east 'sudo birdc show protocols'                  # west  BGP  up  Established
sudo vtysh -c 'show bfd peers brief'                  # fd00:dead:beef::2  up
ssh east 'sudo birdc show bfd sessions'               # fd00:dead:beef::1  Up  0.300
```
