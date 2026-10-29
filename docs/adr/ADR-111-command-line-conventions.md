<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-111: Command-line conventions — every binary answers `--help`, `--version` and a mistake the same way

**Status:** Proposed (Stevie to accept).
**Date:** 2026-10-28
**Related:** ADR-083 (terminology register), ADR-090 (no fake success), ADR-093 (what shape is this thing), the Gauntlets Law (every CLI command has an API).

## Context

Sixty-seven programs live under `cmd/`. Before this ADR they answered the
three questions an operator asks first in sixty-seven slightly different
ways:

- `--help`: thirty printed Go's bare `flag.PrintDefaults` dump ("Usage of
  akira:" with no sentence saying what akira is); ten had a hand-written
  usage; ten took no flags at all and ran on any argument, including
  `--help`.
- `--version`: twenty answered; the rest did not.
- A typo (`akira --prot 1`): Go's flag package prints `flag provided but
  not defined: -prot` **and then the whole usage dump** to stderr, exit 2.
  Several subcommand tools printed nothing at all and exited 2.

GNU coreutils settled this in the 1990s and every operator's hands know
the shape: `--help` on stdout with a usage line, one sentence, the options
and examples; `--version` on one line; a mistake is **one line** on
stderr followed by `Try 'NAME --help' for more information.`, exit 2, and
never the usage dump (`chown -?` is the reference transcript).

## Decision

1. **`pkg/cmdline`** is the one place these conventions live. It wraps the
   standard `flag` package; it does not replace it.
   - `cmdline.Parse(spec)` replaces `flag.Parse()` for ordinary binaries.
   - `cmdline.ParseSet`, `HandleParseError`, `ReportUsageError`,
     `ReportUnknownCommand`, `PrintVersion`, `IsHelpArg`, `IsVersionArg`
     and `Translate` serve the subcommand tools that drive their own
     `flag.FlagSet`s.
2. **Shape.** `--help`/`-h` → stdout, exit 0: `Usage: NAME [OPTION]...
   OPERANDS`, the one-line purpose, `Commands:` when there are any,
   `Options:` (long form, type placeholder, non-zero defaults), `Details`,
   `Examples:`. `--version`/`-V` → `NAME (unheaded) VERSION`, exit 0. Any
   usage mistake → `NAME: unrecognized option '--x'` / `option '--n'
   requires an argument` / `invalid argument 'abc' for '--port'` / `missing
   operand` / `extra operand 'x'` / `unknown command 'x'`, then the `Try`
   line, exit 2, nothing else.
3. **Operands are declared.** A `Spec` with no `Operands` accepts none;
   a stray word is an error, not silently ignored.
4. **Version** comes from `-X unheaded/pkg/cmdline.Version=...` at link
   time, else the build's VCS revision, else `dev`. Binaries with their own
   `version` variable pass it as `Spec.Version`.
5. **The ratchet.** `pkg/cmdline/policy_test.go` fails when a program under
   `cmd/` does not import `pkg/cmdline`. New binaries add the call; they
   do not add an exception.
6. **Existing wording is kept where tests assert it** (`unknown
   subcommand` in yggdrasil-evidence and cask-verify); only the usage dump
   goes and the `Try` line arrives.

## Consequences

- `-h` now writes to **stdout** (it wrote to stderr before). One test
  (`cmd/unheaded-keygen`) was updated for this; scripts that captured
  `--help` from stderr need the same change.
- `flag.CommandLine` runs in `ContinueOnError` after `cmdline.Parse`; no
  binary re-parses, so nothing observes this.
- Subcommand-level `flag.FlagSet`s inside the nineteen multi-command tools
  still print the flag package's own text on a bad option (they set their
  output to stderr). Converting them is mechanical with `HandleParseError`
  and is left per tool.
- The six Rust binaries (`mbc-asm`, `mbc-emulate`, `rv32i-to-mbc`,
  `cart`, `trainquant`, `zhenai-forge`) parse `std::env::args` by hand:
  `-h` prints a usage line to **stderr**, none answers `--version`, and an
  unknown word falls through to the usage text. They are not changed by
  this ADR; the follow-up is a small shared `usage` module in
  `crates/monad-mbc` (or `clap`, already a dependency of `doom-runner` and
  `zhend`) giving them the same shape as `pkg/cmdline`.

## Verification

```
go build ./... && go vet ./... && go test -race ./pkg/cmdline/ ./cmd/...
for b in bin/*; do "$b" --help >/dev/null; "$b" --version; "$b" --no-such-flag; done
```
