<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# LICH-016: syslog relay

**Status:** fuzz targets run 2026-11-04 (no crashers); no red-team campaign has run against a live relay.
**Design:** ADR-124.

## Target

`cmd/syslog-relay` is the first Kingdom listener whose whole job is reading bytes from senders that do not authenticate. It takes RFC 5424 and RFC 3164 over UDP and TCP on 514 and writes into the journal, where audit records live.

| Surface | Code | Why it matters |
|---|---|---|
| Message parser | `pkg/syslog.ParseMax` | every byte comes from the network |
| TCP framing | `pkg/syslog.Frame` | an announced length decides an allocation |
| Journal encoding | `pkg/journald.Writer.Send` | a field break here forges journal fields |
| Allowlist, rate limit, connection cap | `cmd/syslog-relay` | the only thing between a flood and the 1 GB journal |

## Corpus

Seeds are in `pkg/syslog/parse_fuzz_test.go`: the RFC 5424 §6.5 examples, BSD lines with and without hostname and PRI, cut headers, unterminated structured data, a NUL, invalid UTF-8, an escape sequence, a line separator and a bidi override; for framing, valid octet counts and LF lines, lengths over the cap and smaller than the frame, leading zeros, a 20-digit length, and a stream ending inside a frame. A crasher becomes a regression input under `pkg/syslog/testdata/fuzz/`.

## Invariants

For any input, at any cap:

1. No panic.
2. `len(Text)` and `len(StructuredData)` stay within the cap, after escaping; every header field stays within its RFC limit.
3. No field holds a raw C0, DEL or C1 control, a line or paragraph separator, or invalid UTF-8.
4. Facility is 0-23 and severity 0-7, whatever the PRI said.
5. `Frame` never returns more than its cap and always consumes input; only its documented errors come back.
6. A payload survives octet counting byte for byte.

## How to run

```bash
go test ./pkg/syslog/ -run '^$' -fuzz '^FuzzParse$' -fuzztime 120s
go test ./pkg/syslog/ -run '^$' -fuzz '^FuzzFrame$' -fuzztime 120s
go test ./pkg/syslog/ -run '^$' -fuzz '^FuzzFrameRoundTrip$' -fuzztime 30s
```

## Not yet tried

Against a running relay: spoofed UDP sources on lxdbr0, slow TCP senders at `--max-conns`, a SIGHUP during a flood, and journald stalled under load. Forged `Failed password` lines through Vector's auth counter are a known gap (ADR-124, Consequences).
