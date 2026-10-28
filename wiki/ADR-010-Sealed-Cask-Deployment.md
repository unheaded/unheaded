# ADR-010: Soul Vessel Deployment Model — Immutable Encrypted VMs

**Status:** Accepted — Alpha scope implemented (2026-04-03)  
**Date:** 2026-02-18

> Wiki stub generated 2026-05-06 by Marshal overnight sweep (NORTH-STAR Appendix A B5). See the canonical ADR for full text and rationale.

## Signed Binding Rune (2026-10-27)

A cask carries `BINDING_RUNE.sha256` (every file's sha256) and `BINDING_RUNE.sig`, the evidence build
key's ML-DSA-65 seal over that manifest (purpose `sealed-cask-v1`).

```bash
UNHEADED_BUILD_KEY=/etc/unheaded/keys/<key>.key scripts/build-sealed-cask.sh --output cask.tar.gz
cask-verify verify  --cask cask.tar.gz --pubkey compliance/keys/build.pubkey
cask-verify extract --cask cask.tar.gz --pubkey compliance/keys/build.pubkey --dest /srv/cask-new
scripts/verify-binding-rune.sh cask.tar.gz          # wraps cask-verify
```

| Refused | Why |
|---|---|
| no `BINDING_RUNE.sig` | `UNSIGNED CASK` |
| seal by any key but a trusted `build.pubkey` | the key id names the signer |
| a changed, missing or extra file | the rune lists exactly the cask's regular files |
| `..`, absolute names, symlinks, hard links, devices | checked from the tar headers before anything is written |
| a member over 2 GiB, more than 8 GiB in all | caps trip from the headers |
| bytes appended after the archive | the gzip stream must end the file |

`--pubkey` repeats during a quarterly rotation overlap. The key's custody (S-IRON-GATE E1) and its
ceremony: `runbooks/security/build-key-ceremony.yaml`. Code: `pkg/sealedcask`, `cmd/cask-verify`,
`yggdrasil-evidence cask sign|verify`; round trip `tests/sealed-cask/round_trip_test.sh`.

## Canonical

[docs/adr/ADR-010-sealed-cask-deployment.md](../docs/adr/ADR-010-sealed-cask-deployment.md)

## Cross-references

- [ADR Index](ADR-Index.md)
- [Architecture overview](Architecture.md)
