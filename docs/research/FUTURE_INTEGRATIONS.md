# Future Integrations — Unheaded Kingdom

Research and planning for future integration sprints.

---

## Suricata IDS/IPS — FUTURE SPRINT

**Source**: https://github.com/OISF/suricata (cloned to ~/tmp/suricata/)
**License**: GPL-2.0
**Status**: FLAGGED FOR LATER — do not integrate in current sprint

### Why Suricata Matters for Unheaded

Suricata is the IDS/IPS layer that sits inline with OPNsense (host-a) and IPFire (host-b):
- OPNsense already has Suricata plugin in `plugins-master/security/` (suricata-based IDS feeds)
- IPFire lfs includes `suricata` and `suricata-reporter` packages
- Fills the gap between eBPF packet marking (Monad) and application-layer threat detection

### Integration Points (future sprint)

1. **OPNsense + Suricata** (host-a):
   - Enable via OPNsense GUI: Services → Intrusion Detection
   - ET-Open rules + custom Monad-aware rules
   - Inline IPS mode on WAN interface
   - Alert logs → Loki → Grafana (unified with existing telemetry)

2. **IPFire + Suricata** (host-b):
   - Built-in via `pakfire install suricata`
   - Web UI: IPS tab in IPFire
   - Suricata-reporter for log visualization

3. **Custom Monad rules** (Suricata signatures for protocol validation):
   ```
   # Detect malformed Monad HbH headers (invalid CRC-16)
   alert ip6 any any -> any any (msg:"UNHEADED Monad HbH CRC mismatch"; \
     ip6_exthdr:hbh; content:"|1E|"; offset:0; depth:1; \
     sid:9000001; rev:1; classtype:protocol-command-decode;)
   
   # Detect Monad version mismatch
   alert ip6 any any -> any any (msg:"UNHEADED Monad version unknown"; \
     ip6_exthdr:hbh; byte_test:1,>,1,2; \
     sid:9000002; rev:1;)
   ```

4. **eBPF + Suricata AF_PACKET** integration:
   - Suricata can use AF_PACKET with eBPF load balancing
   - Share BPF maps between Shield (Whispering Void) and Suricata
   - Zero-copy packet path: XDP → AF_PACKET → Suricata

5. **Anamnesis events** from Suricata:
   - Suricata EVE JSON output → Loki → Anamnesis event stream
   - Alert events feed BlackMage threat intelligence
   - Moat Ghost compliance: Suricata logs satisfy NIST AU-3/AU-12 controls

### Build Notes (from source)
- Requires: libnss, libpcre2, libhtp, libyaml, libjansson
- eBPF support: `--enable-ebpf --enable-ebpf-build`
- AF_PACKET bypass: `--enable-af-packet`
- Build: `./configure --enable-ebpf --enable-ebpf-build --enable-af-packet && make -j$(nproc)`

### Action Items (future sprint)
- [ ] Sync ~/tmp/suricata/ (OISF/suricata git clone)
- [ ] Write Dockerfile for Suricata from source
- [ ] Write NixOS module: nixos/modules/suricata.nix
- [ ] Write LXD container: lxd/containers/suricata.yaml
- [ ] Write Docker compose service: docker/security/suricata/
- [ ] Create custom Monad protocol signatures (sid 9000001-9000099)
- [ ] Integrate EVE JSON → Loki pipeline
- [ ] Configure OPNsense Suricata plugin via API
- [ ] Configure IPFire Suricata via pakfire

### License Note
Suricata is GPL-2.0. Unheaded is MIT. Suricata runs as a separate process/container
— no linking. GPL-2.0 isolation maintained. Barrister-approved pattern.

---

