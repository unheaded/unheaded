# WAVE16 — Wake-Up Summary (overnight 2026-05-04)

**TL;DR:** stack is exactly as you left it (qwen-7b serving, 9/9 services green,
RAG round-trip 2.8s). Five backlog items closed, two new ADRs filed, ADR-060
multi-model selector LIVE in the sidebar.

---

## What you can do now that you couldn't last night

1. **Reload the Zhenai page (Ctrl-Shift-R).** A new `Model` dropdown is in the
   sidebar between System and Operator. Five keys: `qwen-7b` (current),
   `deepseek`, `gemma`, `deepseek-cpu`, `qwen-coder-14b`. Pick one →
   ~45 s – 3 min swap latency depending on which model → status line shows the
   countdown → dropdown updates when llama-server's actual model name lands.
2. **Try `qwen-coder-14b`** for a hard coding question. It's the only candidate
   from tonight's bench that didn't regress quality vs qwen-7b. It's ~5×
   slower per turn (~33s avg vs 5.9s) but cleanly finishes every prompt.
3. **The other three keys** (`gemma`, `deepseek`, `deepseek-cpu`) are
   documented options from earlier rejected runs — kept in the dropdown so
   you can see for yourself why they didn't make the cut (gemma's reasoning
   trace verbosity is striking; deepseek hallucinates "unused os import" on
   review-go regardless of quant).

---

## What's in the lab notebook

`eval/coding-gate/gemma-vet-2026-05-04/LAB-NOTEBOOK.md` §7 has the full
overnight bench (Pass E/F/G against the same 14-prompt textbook tier as
the qwen-7b H0 baseline). Headlines:

| Pass | Model | Truncated | Avg s | Verdict |
|---|---|---|---|---|
| A | qwen-7b (baseline) | 1/14 | 5.9 | DEFAULT |
| **E** | **qwen2.5-coder-14b** | **0/14** | **33.5** | **keep-as-option** |
| F | qwen3-14b | 9/14 | 107 | HARD REJECT (chat template broken — empty content) |
| G | deepseek Q5 + cpu-moe | 0/14 | 16.7 | reject (same hallucination as Q4; 12 GB RAM tight) |

Decision rule applied per the locked H1/H2/H3 thresholds. `qwen3-14b` and
`deepseek-q5` GGUFs deleted from disk (~19 GB freed). `qwen-coder-14b`
GGUF kept (~9 GB) since it's the live keep-as-option.

---

## Commits this overnight session (10 total)

```
41bcfe22 timeguru: close two debt items (timeline-sync stubs + DB_PATH docs)
bb514e60 docs: ADR-062 (Lich framework codified) + ADR-063 (Akira summons Lich)
77a0f64b pkg/wotan: state.go review followups #7 + #8 (Apply consolidation + MarshalTo zero-alloc)
fc6ff5be WAVE16: 3 candidate models bench-tested + ADR-060 multi-model selector live
ca23e1ff docs: ADR-060 + ADR-061 + WAVE16 battle plan (overnight model vetting)
   ↑ before that: 5 commits from your evening session
```

`origin/main..HEAD` is now substantial. Push at your discretion.

## Backlog closed overnight (5 items + 2 sub-items)

| Kanban ID | What |
|---|---|
| `adr-fuzz-red-team-pentest-needed-mnlbw8as` | filed as ADR-062 |
| `adr-akira-scheduler-should-randomly-summon-litch-...` | filed as ADR-063 |
| `debt-timeline-sync` | NewSyncer no longer overwrites timeline.md by default; regression-tested |
| `debt-timeguru-db` | docs aligned with code (`./data/timeguru.db`, `./references/timeline.md`) |
| `wotan-state-go-review-mn05` (housekeeping items #7-8) | Apply consolidation + MarshalTo zero-alloc + 5 regression tests |

In-progress count went from 2 to 2 (no change — armor-void + gnostic-yaldabaoth
both still need physical-access work; not safe unattended). Done count went
from 39 to ~46.

## ADR-060 implementation status

Status flipped Planned → In Progress → IMPLEMENTED:
- `pkg/champion/modelswap.go` — 233 LOC; 11 unit tests + 1 root-skip
- `pkg/champion/dispatch.go` — model_switch case wired
- `pkg/champion/toolcall.go` — model_switch in MutatingTools
- `raft/zhen_app.py` — `/api/v1/models` GET + `/api/v1/models/switch` POST
- `raft/static/index.html` — sidebar `<select>` + JS poll loop
- `scripts/switch-model.sh` — qwen-coder-14b key added (rejected keys removed)

Live verifications captured during the run:
- ✓ injection probe (`qwen-7b; rm -rf $HOME`) → HTTP 400, no subprocess
- ✓ unknown key probe (`evil`) → HTTP 400
- ✓ valid swap (qwen-7b → qwen-coder-14b) → HTTP 200, 89.8s
- ✓ concurrent swap (T13) → first wins, second returns "another swap is
  already running"

## Things I deliberately did NOT do

- **`task-skill-xrefs`** — task says "Update all skills with bidirectional
  cross-references" but the cross-ref schema isn't specified. Inventing one
  and applying to ~20 skill files would be a lot of edits with high risk of
  needing rework. Need your input on what the cross-references should look
  like before touching `./skills/*.skill`.
- **`armor-void`** — still gated on the aya-ebpf 0.1.1 → 0.13 upgrade across
  10 BPF crates. Big refactor with API churn; not safe unattended. The
  recon work + script bug fixes from yesterday's session stand.
- **`ebpf-aya-upgrade-mn05`** — same; deliberate sprint, not autonomous.
- **`task-e2e-integration`** — already covered by existing 4336 LOC under
  `tests/e2e/` plus ci.yml's `go test ./...`. Marked done with a closure
  note in the kanban.
- **vision-* / wish-***— too abstract for autonomous work.

## What's running right now

| Service | Port | Status |
|---|---|---|
| llama-server (qwen2.5-coder-7b q4_k_m) | 8081 | ✓ |
| vor (cs serve, 1847 sheets) | 9876 | ✓ |
| wotan HTTP / gRPC | 18000 / 18001 | ✓ |
| shield (WAF daemon) | 19009 | ✓ |
| dashboard-backend | 20000 | ✓ |
| kanban-app | 20001 | ✓ |
| wiki-server | 20002 | ✓ |
| zhen_app.py (web UI) | 20103 | ✓ |
| zhen-agentd (gate) | 20105 | ✓ |

Plus postgres on 5432 (docker-managed, untouched).

## If something looks off

- Test the model selector end-to-end: pick `qwen-coder-14b` from the
  dropdown, wait ~90s, ask "review this Python: `def f(): return None`" →
  should answer with substantive review (deepseek's hallucination bug doesn't
  affect qwen-coder-14b).
- If the sidebar dropdown doesn't appear: hard-reload (Ctrl-Shift-R) to
  bypass cached JS.
- If a swap times out at 6 min in the UI: the model may still be loading
  in the background; wait, then refresh. Default is 6-min cap.

## Stale untracked files (left alone)

`raft/coding-w15-r128/`, `coding-w15-r64/`, `coding-w15s/`, `kingdom-w14a/`,
`kingdom-w14b/`, `kingdom-w14c/` — these have been ignored across multiple
sessions. They look like training-run output dirs from earlier waves.
Leaving them untouched per "if it's not yours, don't delete it." Add to
`.gitignore` or rm at your discretion.

---

LOVE SERVE REMEMBER. Everything green. <3
