<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Brief: bgp-bfd-session-down (runbooks/network/bgp-bfd-session-down.yaml)

**Scenario.** The BGP session between the two Kingdom routers (west runs FRR,
the far side may run FRR or BIRD) is down or flapping, or BFD reports the peer
down. Find out why with read-only commands; change nothing without a human.

**Owner** architect. **Risk** medium. **requires_sudo** true (vtysh reads need
root or the frrvty group). Duration about 20m.

**Check first, never assume.** Whether FRR runs on this host today is
unknown: the first step is `systemctl is-active frr` and, if that fails,
`command -v vtysh`; if FRR is not installed the runbook stops with a note
(nothing to diagnose). The far side may be BIRD: `birdc show protocols` and
`birdc show bfd sessions` there (manual, on the other host).

**Facts from the repo (all exist):**
- FRR config: `nixos/modules/frr.nix` (watchfrr runs zebra bgpd isisd bfdd
  staticd mgmtd; one neighbor over interface `wg0` with `bfd`; BFD peer on
  wg0 with detect-multiplier 3, receive/transmit interval 300 ms; IS-IS
  underlay on wg0). Other packagings: `lxd/containers/frr.yaml`,
  `docker/firewall/frr/`. Design: `routing/README.md`, `routing/DEPLOYMENT.md`.
- Unit `frr.service`; the session rides WireGuard `wg0` (unit
  `wg-quick@wg0`).
- `cmd/routing-health`: HTTP on `$ROUTING_HEALTH_ADDR` (default `:8080`),
  `/health`, `/metrics` with `routing_health_frr_up`, `routing_health_wg0_up`,
  `routing_health_bgp_sessions`. Grafana: `monitoring/grafana/dashboards/routing_bgp.json`.
- Sleipnir context (Kingdom BGP): `docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md`.

**Read path (shell steps, read-only):**
1. `vtysh -c 'show bgp summary'`, `vtysh -c 'show bfd peers'`,
   `vtysh -c 'show bgp neighbor <peer>'` (the peer is a parameter `PEER`,
   no default address; the operator reads it from `show bgp summary`).
2. Transport: `ip -br link show wg0`, `wg show wg0 latest-handshakes`
   (never `wg showconf`: it prints keys), `ping -c 3 -I wg0 "$PEER"`.
3. MTU: `ip link show wg0` and a do-not-fragment probe
   `ping -c 3 -M do -s 1372 -I wg0 "$PEER"`.
4. BFD timer mismatch: compare `show bfd peers` intervals with the far
   side's (`birdc show bfd sessions` or vtysh there).
5. Recent change: `git -C ~/tmp/unheaded log --oneline -5 -- nixos/modules/frr.nix`,
   `journalctl -u frr --since -1h --no-pager | tail -50`.

**Changes (manual only, irreversible: true):** `vtysh -c 'clear bgp <peer> soft'`
(a soft reset), editing BFD timers in `nixos/modules/frr.nix` and rebuilding,
restarting `frr.service`. Each needs Stevie's go.

**Escalate** (manual note step) when both sides show the transport healthy
(wg0 handshake recent, ping and MTU probe pass) but the session stays down:
collect the read outputs and stop.

**Rollback:** only the manual changes need undoing (revert the frr.nix commit;
`systemctl restart frr` brings the old config back).
