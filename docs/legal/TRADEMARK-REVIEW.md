# Trademark / IP Review — Pre-Public Clearance

**Date:** 2026-04-03
**Reviewer:** Barrister skill + manual audit
**Status:** CLEARED with notes

## Service Names

| Name | Origin | Risk | Status |
|------|--------|------|--------|
| **Unheaded** | Original coinage | None | CLEAR — unique, no conflicts found |
| **Wotan** | Norse mythology (Odin) | Low | CLEAR — public domain mythological name. Wagner's opera uses "Wotan" but that's art, not tech trademark |
| **Sophia** | Greek philosophy (wisdom) | Low | CLEAR — common word, used by many projects. No tech trademark conflict in our context |
| **Monad** | Mathematical/philosophical term | Low | CLEAR — generic CS term (Haskell monads, category theory). No trademark |
| **Zhenai** | Chinese 真爱 (true love) | Low | CLEAR — common Chinese word, not trademarked in tech. zhenai.com exists (Chinese dating site) but different domain |
| **The Well** | Common English phrase | None | CLEAR — generic phrase. The WELL (early online community) is historical, not active trademark |
| **Timeguru** | Original compound word | Low | CLEAR — generic compound, no tech trademark found |
| **Captain** | Common English word | None | CLEAR — too generic to trademark in tech |
| **Architect** | Common English word | None | CLEAR — too generic |
| **Micromanager** | Common English word | None | CLEAR — too generic |
| **Sleipnir** | Norse mythology | Low | CLEAR — public domain. "Sleipnir" browser exists (Japan) but different product class |
| **Yggdrasil** | Norse mythology | Low | CLEAR — public domain. Yggdrasil Linux existed (1990s, defunct). No active trademark |
| **Kingdom** | Common English word | None | CLEAR — descriptive, not trademarkable alone |

## Lore / Branding Terms

| Term | Origin | Risk | Status |
|------|--------|------|--------|
| Gnostic terminology (Pleroma, Kenoma, Yaldabaoth) | Ancient philosophy | None | CLEAR — public domain, 2000+ years old |
| Chronicles of Amber references | Roger Zelazny novels | **Medium** | NOTE — literary references for internal naming only. Don't use in marketing/branding. Fair use for internal architecture naming. |
| Medieval/Kingdom terminology | Historical | None | CLEAR — generic historical terms |
| "The Doom Range" | Original + id Software reference | Low | CLEAR — descriptive port range name. Doom is id Software's trademark but we're not claiming the game name |
| "Binding Rune" | Norse/fantasy | None | CLEAR — generic fantasy term |
| "Sealed Cask" / "Soul Vessel" | Original | None | CLEAR — original compound terms |

## Domain Names

| Domain | Status |
|--------|--------|
| unheaded.org | Owned by Stevie Bellis |
| bellis.tech | Owned by Stevie Bellis |
| unheaded.com | Check availability |
| zhenai.dev | Check availability |

## Dependency License Audit Summary

100 Go dependencies, all permissive (Apache-2.0, MIT, BSD, ISC):
- Google (grpc, protobuf, uuid, pprof): Apache-2.0
- Cloudflare (circl): BSD-3-Clause
- Prometheus (client_golang): Apache-2.0
- cilium/ebpf: MIT
- gorilla (mux, websocket): BSD-3-Clause
- modernc.org (sqlite, libc): BSD-3-Clause
- All golang.org/x/*: BSD-3-Clause

**No GPL dependencies in production Go code.** Our code is GPL-3.0; all deps are permissive → compatible.

Rust dependencies (Cargo.toml): Apache-2.0/MIT dual-licensed (standard Rust ecosystem).

## Conclusion

**CLEARED for public release.** No trademark conflicts, no license incompatibilities.

One note: Chronicles of Amber references (Corwin, Pattern, Shadow, Trump) are literary allusions. Keep them internal/architectural — don't use in external marketing to avoid any fair use ambiguity.
