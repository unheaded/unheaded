# Zhen Training Data

How answers of both brains become training data for the local model: captured, stored in [[The Well|The-Well]], labelled by what happened to them, filtered, split with a hold-out, and exported in the forge's format. Tool: `cmd/zhen-flywheel`. Plan: S-TWO-BRAINS Phases 10-11.

```
zhen-capture/v1 JSONL ──ingest──► zhen_captures ──label──► zhen_capture_labels
 (zhen-claude -capture,           (one row per answer,     (check, judge, escalation,
  local_llm, import-eval)          deduped by content)       review, Stevie's edits)
                                          │
                                       export ──► sft-*.jsonl, preference-*.jsonl, tool-traces-*.jsonl,
                                                  <split>.tok.jsonl + .tok.ids, MANIFEST.json
```

## Claude-authored text is off

`zhen-flywheel export` never puts text Claude wrote into a training target (an SFT answer, either side of a preference pair, a tool trace) unless it is run with `-include-claude-text` (or `ZHEN_FLYWHEEL_INCLUDE_CLAUDE_TEXT=1`). The default is off.

Why: Anthropic's help article "Can I use my Outputs to train an AI model?" allows Outputs for models that classify, summarise, extract or search, and lists as prohibited "models designed for open-ended text generation" and "using Outputs as training targets for models". A local coding model is the first. Stevie asked support for written permission on 2026-10-23 (S-TWO-BRAINS Step 340, open). Claude text is still captured and stored, tagged `author_kind='claude'`, so turning it on after a written yes is a one-line change, not a re-capture. A Claude label may train a non-generative router or classifier.

What trains instead:

- local answers that passed a check or a judge and have no failing outcome (rejection sampling);
- local answers in agent runs Stevie accepted;
- human-written or human-edited text (Stevie's edits at merge become their own captures).

Test results and verdicts are labels (they select), never text.

## Commands

The Well: `WELL_APP_ZHEN_DSN` (unheaded_app as `app_zhen`), required; `WELL_OPS_DSN` (unheaded_ops, read-only role) for `label` to see escalations. No default: credentials are never in the repo.

| Command | Does |
|---|---|
| `zhen-flywheel ingest [-dir D]` | Reads `D/zhen-capture-*.jsonl` (default `$ZHEN_CAPTURE_DIR`, else `~/.local/state/unheaded/zhen-capture`). Strict: a bad line is reported as `file:line: why` and that file stays put. Re-scans every text (gitleaks + the privacy tier patterns); a secret or personal identifier sends the record to `D/quarantine/`, not the Well. Clean files move to `D/ingested/`. Re-ingesting inserts nothing. |
| `zhen-flywheel label [-repo R]` | Labels from `zhen_actions` run_check rows, `llm_requests` escalations, `zhen_agent_runs` reviews, `eval/two-brains/results-*.jsonl` judge verdicts, and (with `-repo`) Stevie's edits at merge. Idempotent. |
| `zhen-flywheel export -out DIR` | Writes a new dataset directory (outside any repository, 0700, files 0600, never over an existing one) and marks every exported capture promoted. |
| `zhen-flywheel scan DIR` | Re-scans an export; exit 1 on any finding. Run with `gitleaks dir DIR` before anything trains on it. |
| `zhen-flywheel quarantine ID` | Takes one capture out of every later export; the next `SELECT zhen_captures_purge();` deletes it. |
| `zhen-flywheel import-eval -results F -answers-dir D` | Turns answers zhen-eval kept (`-answers-dir`) into capture records, only when the kept file is the one that was judged (SHA-256). |

## Where things live

| What | Where | Mode |
|---|---|---|
| Capture files | `~/.local/state/unheaded/zhen-capture/` (`ingested/`, `quarantine/` below it) | 0700 / 0600 |
| Captures and labels | the Well, `unheaded_app.zhen_captures`, `zhen_capture_labels` (migration 033) | `app_zhen` and owner only |
| Exports | `/var/zhen/datasets/flywheel-<date>/` (else `~/.local/state/unheaded/datasets/`) | 0700 / 0600 |
| Backups of the Well | nightly sets on west and east ([[Well Backups|Well-Backups]]) | root only |

Nothing captured or exported is ever in the repository: capture and export both refuse a directory inside a git work tree.

## The export

| File | Holds |
|---|---|
| `sft-train.jsonl`, `sft-holdout.jsonl` | `{id, family, split, author_kind, source, question, context, answer}` |
| `train.tok.jsonl`, `holdout.tok.jsonl` | forge records, exactly `{"tokens":[...],"answer_start":N}` (`scripts/tokenize-coding-for-gemma4.py --strict`) |
| `train.tok.ids`, `holdout.tok.ids` | the capture id of each forge line |
| `preference-*.jsonl` | same task: a good answer and a failed local one (for DPO later; not trained yet) |
| `tool-traces-*.jsonl` | traces of answers that passed a check (not trained yet) |
| `MANIFEST.json` | counts by dataset, split, source, author and family; why each excluded capture was left out (with ids); sha256 of every file; the Claude-text flag; git HEAD; hashed group keys per split; the capture ids in each file. No text. |

Filters: answers 1 byte to 8 KiB, prompts up to 16 KiB, no refusals or "I don't know", nothing truncated, no normalized duplicates.

Split, so a hold-out result means learning and not memorisation: every example of one task (agent run, eval task or conversation) is in the same split; a task is hold-out when `sha256(family + group)` mod 100 < 20; eval tasks never train (two-brains hold-out tasks stay hold-out, every other eval task is excluded); a train prompt that overlaps a coding-gate prompt, a two-brains hold-out task, or any flywheel hold-out prompt by 0.8 or more (8-word shingles) is excluded. Two exports of the same Well are byte-identical.

Train only with `zhenai-forge ... --strict-data`: any malformed line, extra key, missing or out-of-range `answer_start` stops the run and names the line (the WAVE14 H6 bug trained on corrupted data for 11 days). Check an export first with `scripts/two-brains/test_export_roundtrip.py <dir>`.

## Retention and removing one capture

A capture is kept 180 days from when it was captured (`purge_after`, set by the database), then deleted by `zhen_captures_purge()` with its labels, unless an export references it (`promoted`). To retire an export, set `promoted = false` for the ids in its `MANIFEST.json` (`.ids`) and delete the directory.

To remove one capture now:

1. Find it: its id is the 64-hex `id` in the export's text files and in `MANIFEST.json` `.ids`.
2. `zhen-flywheel quarantine <id>`, then `SELECT zhen_captures_purge();` (as `app_zhen`).
3. Export again into a new directory and delete the old one; never hand-edit an export.
4. It stays in Well backup sets taken before the purge until they rotate out (14 sets).
