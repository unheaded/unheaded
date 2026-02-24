# Binary Lore Names — The Kingdom Armory

**Date**: 2026-02-24
**Sprint**: S41 — Kingdom Hardening

Every compiled binary in the Unheaded Kingdom carries a lore name drawn from the three pillars:
Medieval Armory, Gnostic Cosmology, and Norse/Chronicles of Amber mythology.

Functional names are retained as aliases for discoverability. Lore names are the canonical identity.

## Binary Naming Map

| # | Functional Name | Lore Name | Pillar | Description | Port |
|---|----------------|-----------|--------|-------------|------|
| 1 | dashboard-backend | **visor** | Medieval Armory | The visor that reveals the battlefield — the knight's window to all data | 20000 |
| 2 | trace-collector-go | **vambraces** | Medieval Armory | Arm guards that sense and record every blow — eBPF event collector | 16670/16671 |
| 3 | wotan (via services/) | **wotan** | Norse/Gnostic | The All-Father of memory — message bus and state persistence | 18000/18001 |
| 4 | wotan-ctl | **gungnir** | Norse | Wotan's spear — his command-line instrument | CLI |
| 5 | unheaded-daemon | **aegis** | Greek/Medieval | The divine shield — control plane daemon, reconciliation engine | 17000/17001 |
| 6 | kanban-app | **herald** | Medieval | The herald who announces and tracks tasks — the Meta Moment | 20001 |
| 7 | doom-bridge | **bifrost** | Norse | The rainbow bridge between realms — Doom to Kingdom data bridge | 16666 |
| 8 | doom | **ragnarok** | Norse | The twilight of the gods — Doom game engine runner | CLI |
| 9 | doom-go-injector | **mjolnir** | Norse | Thor's hammer — packet injection tool for the Doom ring | CLI |
| 10 | monad | **monad** | Gnostic | The divine spark — unified state management | 19004 |
| 11 | sophia | **sophia** | Gnostic | Divine wisdom — knowledge graph and BPF map state | 19005 |
| 12 | unheaded | **unheaded** | Core | The headless king — main entry point | CLI |
| 13 | unheaded-cli | **crown** | Medieval | The crown without a head — CLI management tool | CLI |
| 14 | cert-gen | **sigil** | Medieval | The royal seal — TLS certificate generator | CLI |
| 15 | lich-security | **lich** | Fantasy/Gnostic | The undying sorcerer — security audit and fuzzing tool | CLI |
| 16 | wiki-server | **codex** | Medieval | The great codex — wiki and knowledge server | 20002 |

## Service Binaries (from services/)

| Service | Lore Name | Pillar | Port |
|---------|-----------|--------|------|
| timeguru | **hourglass** | Medieval | 19000 |
| architect | **mason** | Medieval | 19001 |
| captain | **pennant** | Naval/Medieval | 19002 |
| micromanager | **quartermaster** | Military | 19003 |
| gateway | **portcullis** | Medieval | 21000/21443 |

## Armor Services (from services/)

| Service | Already Lore-Named | Pillar |
|---------|-------------------|--------|
| shield | shield | Medieval Armory |
| sword | sword | Medieval Armory |
| cape | cape | Medieval Armory |
| cloak | cloak | Medieval Armory |
| hauberk | hauberk | Medieval Armory |
| cuirass | cuirass | Medieval Armory |
| gauntlets | gauntlets | Medieval Armory |
| vambraces (svc) | vambraces | Medieval Armory |
| pauldrons | pauldrons | Medieval Armory |
| tassets | tassets | Medieval Armory |
| sabatons | sabatons | Medieval Armory |

## Gnostic Services

| Service | Lore Name | Pillar |
|---------|-----------|--------|
| pleroma | pleroma | Gnostic — The fullness |
| kenoma | kenoma | Gnostic — The void |
| yaldabaoth | yaldabaoth | Gnostic — The demiurge |

## Usage

```bash
# Build with lore names (symlinks)
make binaries

# Both names work:
bin/visor          # = bin/dashboard-backend
bin/vambraces      # = bin/trace-collector-go
bin/gungnir        # = bin/wotan-ctl
bin/aegis          # = bin/unheaded-daemon
bin/herald         # = bin/kanban-app
bin/bifrost        # = bin/doom-bridge
bin/ragnarok       # = bin/doom
bin/mjolnir        # = bin/doom-go-injector
bin/crown          # = bin/unheaded-cli
bin/sigil          # = bin/cert-gen
bin/lich           # = bin/lich-security
bin/codex          # = bin/wiki-server
```

## Naming Principles

1. **Medieval Armory** for infrastructure components (they protect the kingdom)
2. **Gnostic Cosmology** for data/knowledge services (they hold divine truth)
3. **Norse Mythology** for messaging and bridge components (Wotan's domain)
4. **Functional names as aliases** — never break discoverability
5. **Armor services already follow convention** — no rename needed
