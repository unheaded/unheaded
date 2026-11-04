<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-122: A Go embedding path on llama-server (parked)

**Status:** Proposed, parked (Stevie, 2026-11-04, decision pack round 4: DP-216 revised).
**Date:** 2026-11-04
**Related:** ADR-056, ADR-057, DP-216, RT-15 (the embedder pin).

## Why it is parked

DP-216 chose a Go service on llama-server's embeddings endpoint, with the Python embedder retiring at parity. Round 4 deferred the swap: the Python embedder stays in `raft/scripts/zhen_rag.py`, there is no third unit and no embeddings port, and S-ZHEN-POLISH's code index is lexical only. This ADR keeps what was measured so the swap starts from it.

## What a later attempt needs

- **A unit, an account and a port, with Stevie's word.** `llama-server --embedding` restricts a server to embeddings, so it cannot be the chat server. DP-712 approved two units; this would be a third.
- **A GGUF of the pinned weights.** None exists. `convert_hf_to_gguf.py --outtype f16` on snapshot `1110a243fdf4706b3f48f1d95db1a4f5529b4d41` works. DP-308 called llama.cpp's converter third party, so name that one-off run when asking.
- **The 256-token limit.** sentence-transformers truncates all-MiniLM-L6-v2 at `max_seq_length` 256; llama-server does not. `raft/zhen_app.py:346` embeds raw conversation text, so long input is the normal case.
- **The tokenize-and-clip fix.** `POST /tokenize` with `add_special:true`, keep `ids[:255]+[ids[-1]]`, send the id list as the input. A byte clip is not enough.
- **`HIP_VISIBLE_DEVICES=-1`.** Without it the ROCm build takes the GPU even at `-ngl 0`: a KFD process, 205 MB of VRAM, compute buffers on ROCm0, a cgroup peak near 950 MB. With it, or under `PrivateDevices=yes`, the server is CPU-only at 23-125 MB. A parity run must use the backend the unit uses.
- **The sha pin (RT-15).** Python refuses a wrong hash against the repo pin today. Build any unit sha file from a pin in `scripts/lib/model-pins.sh`, never from whatever file is on disk, and keep a refusal on the Python path.
- **`.github/workflows/raft-python.yml:63`** names `raft/tests/test_zhen_embedder_pin.py`; a swap that renames that test must change the workflow.

## Measured, 2026-11-04 (S-ZHEN-POLISH review)

| Input | Result against sentence-transformers |
|---|---|
| 300 short texts (under 200 tokens) | mean cosine 0.9999989, min 0.9999952, top-5 overlap 0.996 |
| 40 long ADR excerpts, over 256 tokens, no clip | mean 0.911, min 0.852 |
| The same set, over 512 tokens, no clip | HTTP 500 on 20 of 40 |
| The same 40 with the token clip | 40 of 40, mean 0.9999982 |
| Real source chunks, 1024-byte clip | 20 of 27,906 still over 512 tokens, up to 619; one such chunk fails its whole batch of 32 |

Embedding the full tree would take about 11 minutes, extrapolated from a 320-chunk rate; it was not run.

## Still to decide

- Whether a Go service or a client library is what "Go service" means here: Python would still need its own client while the Flask UI lives.
- Whether the code index gains vectors. It is lexical today; ADR-057 asked for a code-trained model, which this one is not.
