# Yggdrasil Apt Repository

**Status**: SCAFFOLD — directory + publish script in place; real reprepro init lights up at task #65 implementation.

This directory holds the artifacts needed to publish a Debian apt repository for the UPC tooling that ships with every Yggdrasil image (task #71). The Jenkins pipeline (`../Jenkinsfile`) calls `publish.sh` on tag builds.

## Repository layout (reprepro-managed)

```
apt.unheaded.dev/yggdrasil/
├── conf/
│   ├── distributions          # codename, components, signing key
│   └── options                # reprepro defaults
├── dists/
│   └── bookworm/
│       └── main/
│           └── binary-amd64/
│               ├── Packages
│               ├── Packages.gz
│               └── Release
└── pool/
    └── main/
        ├── u/
        │   ├── upc-bootctl/
        │   ├── upc-tty-bridge/
        │   ├── unheaded-shared/
        │   └── unheaded-runner/
        ├── m/
        │   └── monad-cpu-ebpf/
        └── y/
            └── yggdrasil-evidence/
```

## Required packages (per task #71 §"Apt repo layout")

| Package | Source crate | Built by |
|---------|--------------|----------|
| `upc-bootctl` | `cmd/upc-bootctl/` | `cargo deb` |
| `upc-tty-bridge` | `cmd/upc-tty-bridge/` | `dpkg-buildpackage` (Go binary) |
| `monad-cpu-ebpf` | `ebpf/monad-cpu-ebpf/` | Custom Makefile target (compiles BPF object → arch=all .deb) |
| `unheaded-shared` | Assets bundle (browser xterm.js etc) | Custom Makefile target |
| `unheaded-runner` | `crates/doom-runner/` | `cargo deb` |
| `yggdrasil-evidence` | `cmd/yggdrasil-evidence/` | `dpkg-buildpackage` (Go binary) |

## Signing

The repo is signed with the Yggdrasil ML-DSA-65 build key (per `pkg/gungnir/`). The public key ships at `/etc/apt/trusted.gpg.d/unheaded-upc.gpg` in every Yggdrasil image (per `overlay/upc/0001-add-upc-apt-source.patch`).

## Publish

```bash
# Manual (operator)
nix/yggdrasil/repo/publish.sh "$VERSION"

# CI (Jenkins, on tag)
# Runs via the "Publish to apt repo" stage in Jenkinsfile.
```

## Verify (operator-side)

```bash
# Each Yggdrasil image:
apt-key list                   # Shows trusted unheaded-upc key
apt-cache policy upc-bootctl   # Shows pinned source
apt-cache madison upc-bootctl  # Shows versions available
```

## See also

- `../Jenkinsfile` — the CI pipeline that calls `publish.sh`
- `../overlay/upc/README.md` — what the apt source patches look like in the image
- `../evidence-pack/README.md` — the signed-manifest evidence pack for each build
- `docs/OS-FORK-DISCIPLINE.md` §7.5 — Pillar 5 UPC integration requirement
