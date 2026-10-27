#!/usr/bin/env python3
"""
Zhen RAG Pipeline — Retrieval-Augmented Generation for Unheaded

WAVE15 REWIRE (2026-05-02): retrieval substrate is vor (cs serve at
:9876); inference is qwen-coder-7b via llama-server at :8081 using
the OpenAI /v1/chat/completions shape. The legacy FAISS index over
ring_all.jsonl is retired — see docs/battle-plans/WAVE15-ZHENAI-REWIRE.md.

The sentence-transformers all-MiniLM-L6-v2 embedder is kept solely
for memory-cache cosine recall in zhen_app.py:_search_memories. It is
NOT used for primary retrieval anymore.

Memory recall semantics (T1 closure):
    Python's pre-rewire code returned a cached answer when cosine
    similarity >= 0.9, BYPASSING the LLM. That was T1 (stored prompt
    injection via memory replay). This module now NEVER short-circuits
    the live LLM. zhen_app.py:/api/v1/query surfaces the matched memory
    as a side-channel ("matched_memory" field), but query() always
    runs the full RAG path.
"""
import logging
import os
import re
import time
from pathlib import Path
from urllib.parse import quote, urlencode

import requests
from sentence_transformers import SentenceTransformer

# The system prompt, the per-turn code-comment reminder and the worked
# example are generated from pkg/zhenrag, the one copy every Zhen surface
# (cmd/zhen-rag, cmd/zhen-cli, pkg/agent, this file) renders from:
#   go generate ./pkg/zhenrag/...
# TestGenerated_PythonInSync (Go) fails when the generated module is stale,
# and raft/tests/test_zhen_prompt.py pins the bytes this path sends.
# _generate_via_llama appends CODE_COMMENT_REMINDER after the question and
# places CODE_COMMENT_EXAMPLE before the real conversation: a small model
# copies the shape of an example far more reliably than it follows a rule.
from zhen_prompt_generated import (
    CODE_COMMENT_EXAMPLE,
    CODE_COMMENT_REMINDER,
    DEFAULT_SYSTEM_PROMPT,
)

# Default URLs — match the live Go-stack backends.
DEFAULT_VOR_URL = "http://localhost:9876"
DEFAULT_INFERENCE_URL = "http://localhost:8081"
DEFAULT_AGENTD_URL = "http://localhost:20105"   # zhen-agentd: the chat path's gate
DEFAULT_MODEL_NAME = "qwen2.5-coder-7b-instruct"

log = logging.getLogger("zhen_rag")

# The one message the UI shows when the gate is not there (S-CLEAN-SLATE
# M-036): the chat path never falls back to llama-server around it.
GATE_REQUIRED_MSG = "zhen-agentd not reachable; the gate is required"


class AgentdGateUnavailable(RuntimeError):
    """zhen-agentd cannot gate this request (down, timing out, refusing the
    UI's credentials, or no credentials configured). zhen_app.py answers 503.
    Never caught to fall back to the direct llama-server path."""


def proxy_via_agentd_from_env(environ=None):
    """ZHEN_PROXY_VIA_AGENTD: only an explicit false (0/false/no/off) turns
    the gate off for the chat path (a debug escape, never shipped); unset or
    any other value keeps it on."""
    env = os.environ if environ is None else environ
    val = env.get("ZHEN_PROXY_VIA_AGENTD", "1").strip().lower()
    return val not in ("0", "false", "no", "off")


def agentd_auth_from_env(environ=None):
    """(api_key, error) for the UI's calls into zhen-agentd.

    zhen-agentd refuses to serve unauthenticated unless it runs -no-auth on a
    loopback listener; the UI must match it. ZHEN_AGENTD_API_KEY (one of the
    daemon's AUTH_API_KEYS, sent as X-API-Key), or ZHEN_AGENTD_NO_AUTH=1 for
    that dev daemon. Neither set is a configuration error, reported on every
    gated request rather than guessed around.
    """
    env = os.environ if environ is None else environ
    key = env.get("ZHEN_AGENTD_API_KEY", "").strip()
    if key:
        return key, None
    if env.get("ZHEN_AGENTD_NO_AUTH", "").strip() == "1":
        return "", None
    return "", ("ZHEN_AGENTD_API_KEY is not set: the web UI cannot authenticate to "
                "zhen-agentd (set it to one of the daemon's AUTH_API_KEYS, or "
                "ZHEN_AGENTD_NO_AUTH=1 for a -no-auth loopback dev daemon)")


def agentd_headers(api_key):
    h = {"Content-Type": "application/json"}
    if api_key:
        h["X-API-Key"] = api_key
    return h


# Log "gate is required" once per outage, not once per question.
_gate_down_logged = False


def _log_gate_down(detail):
    global _gate_down_logged
    if not _gate_down_logged:
        log.warning("%s (%s)", GATE_REQUIRED_MSG, detail)
        _gate_down_logged = True


def _gate_up():
    global _gate_down_logged
    _gate_down_logged = False


def _reset_gate_log_for_test():
    _gate_up()


# Strip JSON tool-call payloads from prior assistant turns before
# replaying them as LLM context. T7 mitigation: a prior turn that
# contained a {"tool_call": ...} JSON object cannot be reused by a
# later turn as a re-justification template. Conservative regex —
# matches the canonical {"tool_call":{"name":...,"args":{...}}} shape
# emitted by the Go agent, plus a few neighboring whitespace variants.
_TOOL_CALL_JSON_RE = re.compile(
    r'\{\s*"tool_call"\s*:\s*\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}\s*\}',
    re.DOTALL,
)


def _strip_tool_call_json(text: str) -> str:
    """Remove {"tool_call": ...} JSON objects from a string.

    Used on every prior turn before re-emitting it to the LLM. Defense
    against a poisoned prior turn smuggling a tool-call template into
    the current turn's context.
    """
    return _TOOL_CALL_JSON_RE.sub("[tool_call payload stripped]", text)


# Retrieval query shaping (2026-10-25, "what is wotan" regression).
#
# vor's /api/search is an AND of case-insensitive SUBSTRING matches, ranked
# first by how many query terms equal a token of the topic's name. Question
# words therefore cost ranking and filter nothing: "is" is a substring of
# almost every doc, and "what" + "is" ranked the `what-is-unheaded` ELI5 page
# above every Wotan doc. They are dropped before the query goes to vor.
# Words that carry meaning in a code question (if, for, while, not, with,
# else) are deliberately not on the list.
_QUERY_STOPWORDS = frozenset([
    "a", "an", "the", "and", "or", "of", "to", "in", "on", "at", "by", "from", "as", "into", "about",
    "is", "are", "was", "were", "be", "been", "being", "am",
    "do", "does", "did", "done", "can", "could", "should", "would", "will", "shall", "may", "might", "must",
    "what", "whats", "what's", "who", "whom", "whose", "which", "where", "when", "why", "how",
    "i", "me", "my", "you", "your", "we", "us", "our", "it", "its", "it's", "this", "that", "these", "those", "there",
    "please", "tell", "explain", "describe", "define", "mean", "means", "meaning",
])

_TERM_PUNCT = "?!.,;:\"'`()[]{}"

# Topics vor indexes under an agent worktree (`.claude/worktrees/<id>/...`)
# are stale copies of the main tree; never promote them by name.
_WORKTREE_CATEGORY_PREFIX = ".claude/"

_NAME_TOKEN_RE = re.compile(r"[-_.\s]+")

# vor answers a search in ~0.1 s, but a search scans every sheet, and when
# west swaps (14 GiB RAM, llama-server + a build) the paged-out index took
# 20 s. With a 10 s timeout retrieve() returned nothing and the model
# answered without references ("Wotan is a fictional character", 2026-10-25).
# A slow answer beats a made-up one; start-zhen.sh also says to run vor
# with swap off.
VOR_TIMEOUT = 60


def _search_terms(query):
    """Lower-cased query words with question/filler words removed.

    Falls back to every word when nothing else is left ("what is this"),
    so a query is never emptied.
    """
    words = [w.strip(_TERM_PUNCT) for w in query.lower().split()]
    words = [w for w in words if w]
    kept = [w for w in words if w not in _QUERY_STOPWORDS]
    return kept or words


def _name_tokens(name):
    """Split a vor topic name the way its words read: `Service-Wotan`,
    `WOTAN_TOPICS` and `draft-...-wotan-memory-03` all yield `wotan`."""
    return [t for t in _NAME_TOKEN_RE.split(name.lower()) if t]


class RAGPipeline:
    def __init__(
        self,
        index_dir=None,
        corpus_file=None,
        inference_url=DEFAULT_INFERENCE_URL,
        vor_url=DEFAULT_VOR_URL,
        agentd_url=DEFAULT_AGENTD_URL,
        model_name=DEFAULT_MODEL_NAME,
        max_topic_chars=10000,
        proxy_via_agentd=None,
        agentd_api_key=None,
    ):
        """
        Backward-compatible constructor: zhen_app.py still calls
        RAGPipeline(index_dir, corpus_file). Those args are accepted
        and IGNORED — vor is the retrieval substrate now.

        New keyword args:
            vor_url:        cs serve REST endpoint (default :9876).
            inference_url:  llama-server endpoint (default :8081).
            agentd_url:     zhen-agentd endpoint (Phase 2 proxy target).
            model_name:     model identifier sent to /v1/chat/completions.
            max_topic_chars:per-topic content cap.
            proxy_via_agentd:
                            If True, generate() routes through
                            cmd/zhen-agentd /api/v1/agent/ask so every
                            LLM-emitted tool call traverses Champion's
                            three-rule gate (T6 closure); an unreachable
                            or refusing daemon raises AgentdGateUnavailable.
                            If False, generate() POSTs directly to
                            llama-server (no gate; debug only).
                            If None (default), proxy_via_agentd_from_env():
                            on unless ZHEN_PROXY_VIA_AGENTD is 0/false/no/off
                            (S-CLEAN-SLATE M-036; it was off by default).
            agentd_api_key: X-API-Key for zhen-agentd; None reads
                            agentd_auth_from_env().
        """
        # Legacy args — accepted for API compat, intentionally unused.
        # (Kept here so zhen_app.py boots without modification today.)
        self._legacy_index_dir = Path(index_dir) if index_dir else None
        self._legacy_corpus_file = Path(corpus_file) if corpus_file else None

        self.vor_url = vor_url.rstrip("/")
        self.inference_url = inference_url.rstrip("/")
        self.agentd_url = agentd_url.rstrip("/")
        self.model_name = model_name
        self.max_topic_chars = max_topic_chars

        # vor's topic listing (/api/topics, ~2.5 MB for ~3.5k topics), cached
        # for name matching in retrieve(). New docs show up within the TTL.
        self.topic_list_ttl = 300.0
        self._topic_list = None
        self._topic_list_at = 0.0

        # Phase 2 proxy toggle. Reading env late lets zhen_app.py override
        # at construction time without setting environment globally.
        if proxy_via_agentd is None:
            self.proxy_via_agentd = proxy_via_agentd_from_env()
        else:
            self.proxy_via_agentd = bool(proxy_via_agentd)
        if agentd_api_key is None:
            self.agentd_api_key, self.agentd_auth_error = agentd_auth_from_env()
        else:
            self.agentd_api_key, self.agentd_auth_error = agentd_api_key, None

        # Context window config — used by chunk truncation in generate().
        self.local_max_tokens = int(os.environ.get("ZHEN_LOCAL_MAX_TOKENS", "2048"))

        # Embedding model — kept SOLELY for memory-cache cosine recall
        # in zhen_app.py:_search_memories. NOT used for primary retrieval
        # anymore. Loading is fast (~1 s for the ~80 MB model).
        print("Loading memory embedder (all-MiniLM-L6-v2)...")
        self.embedding_model = SentenceTransformer("all-MiniLM-L6-v2")

        # Memory store (loaded from PG by zhen_app.py).
        self._memories = []

        # Startup banner — pinned values for audit. Mirrors the format
        # of the Go agent's startup line.
        proxy_status = "via-agentd (Champion gate, T6 closed)" if self.proxy_via_agentd else "direct-to-llama (NO GATE: ZHEN_PROXY_VIA_AGENTD=0, debug only)"
        if self.proxy_via_agentd and self.agentd_auth_error:
            print(f"WARNING: chat will answer 503 until fixed: {self.agentd_auth_error}")
        print(
            f"RAG: vor={self.vor_url} inference={self.inference_url} "
            f"agentd={self.agentd_url} model={self.model_name} "
            f"memory_embedder=all-MiniLM-L6-v2 "
            f"chat_path={proxy_status} memory_recall=display-only-T1-closed"
        )

    # ------------------------------------------------------------------
    # Token estimation (kept for caller compat; used in chunk truncation).
    # ------------------------------------------------------------------

    def estimate_tokens(self, text):
        """Rough token estimate: ~4 chars per token for English."""
        return len(text) // 4 + 50  # +50 for system prompt overhead

    # ------------------------------------------------------------------
    # RETRIEVAL — vor-backed.
    #
    # Ports cmd/zhen-rag/main.go:vorSearch + vorTopicFetch verbatim.
    # Returns dicts in the same shape zhen_app.py already consumes:
    #     { id, content, source, type, distance, trust, label }
    # ------------------------------------------------------------------

    def retrieve(self, query, k=5):
        """Retrieve top-k chunks via vor's /api/search + /api/topics/<name>.

        Replaces the legacy FAISS query path. Returns the SAME dict shape
        as the legacy retrieve() so zhen_app.py needs no changes.

        Two departures from cmd/zhen-rag/main.go (2026-10-25, Zhen called
        Wotan a scheduler because no Wotan definition reached the model):

        1. Question words are dropped before the query goes to vor
           (_search_terms): "what is wotan" searches "wotan".
        2. Topics whose NAME carries every query term (Service-Wotan,
           Wotan-Memory-Model, ...) are interleaved with vor's hits, most
           specific name first. vor caps a search at 50 lines and ranks
           section-title hits above name specificity, so one long doc with
           dozens of "Wotan ..." sections (wotan-cloud-battle-plan) filled
           all 50 lines and was the only topic returned for "wotan".
           Without a name match, vor's order is used unchanged.
        """
        if not query or not query.strip():
            return []

        terms = _search_terms(query)

        # 1. /api/search — list of (topic, category, section, line) hits
        #    plus B1 source-trust labels (source_kind, source_trust,
        #    source_path, source_label).
        try:
            search_url = f"{self.vor_url}/api/search?{urlencode({'q': ' '.join(terms)})}"
            resp = requests.get(search_url, timeout=VOR_TIMEOUT)
            resp.raise_for_status()
            hits = resp.json()
        except (requests.RequestException, ValueError) as exc:
            print(f"[zhen-rag] vor search failed: {exc}")
            return []

        if not isinstance(hits, list):
            return []

        # Worktree copies are stale, and /api/topics/<name> can return one
        # in place of the main tree's doc of the same name: skip them here
        # as well as in the name matches.
        searched = [
            h.get("topic", "") for h in hits
            if isinstance(h, dict)
            and not str(h.get("category", "")).startswith(_WORKTREE_CATEGORY_PREFIX)
        ]
        named = self._topics_named_by(terms)

        # Interleave: named[0], searched[0], named[1], searched[1], ...
        order = []
        for i in range(max(len(named), len(searched))):
            if i < len(named):
                order.append(named[i])
            if i < len(searched):
                order.append(searched[i])

        # 2. For top-K UNIQUE topics, fetch full content via /api/topics/<name>.
        #    Same dedup pattern as cmd/zhen-rag/main.go:201-208.
        seen = set()
        out = []
        for topic in order:
            if not topic or topic in seen:
                continue
            seen.add(topic)

            try:
                topic_url = f"{self.vor_url}/api/topics/{quote(topic, safe='')}"
                t_resp = requests.get(topic_url, timeout=VOR_TIMEOUT)
                t_resp.raise_for_status()
                t = t_resp.json()
            except (requests.RequestException, ValueError):
                continue

            content = t.get("content", "")
            if self.max_topic_chars > 0 and len(content) > self.max_topic_chars:
                content = content[: self.max_topic_chars] + "\n\n…[truncated]"

            # B1 source-trust default: pre-B1 vor servers omit source_trust;
            # default to "canonical" matches the Go side's back-compat.
            trust = t.get("source_trust") or "canonical"

            out.append({
                "id":          topic,
                "category":    t.get("category", ""),
                "source":      t.get("source_path", topic),
                "type":        t.get("source_kind", "embedded"),
                "trust":       trust,
                "label":       t.get("source_label", ""),
                "content":     content,
                # vor doesn't expose a numeric distance; use a placeholder
                # so existing callers that read this field don't crash.
                # Lower is "better" by convention; rank in retrieval order.
                "distance":    float(len(out)),
            })
            if len(out) >= k:
                break

        return out

    def _topics_named_by(self, terms):
        """Topic names whose tokens include every query term.

        Ordered by fewest name tokens first (the name is mostly the query:
        `Service-Wotan` before `ADR-023-wotan-virtual-memory`), then by name.
        Agent-worktree copies are skipped. Returns [] when vor's listing is
        unavailable, which leaves retrieve() on vor's search order.
        """
        wanted = set()
        for term in terms:
            wanted.update(_name_tokens(term))
        if not wanted:
            return []

        now = time.monotonic()
        if self._topic_list is None or now - self._topic_list_at > self.topic_list_ttl:
            try:
                resp = requests.get(f"{self.vor_url}/api/topics", timeout=VOR_TIMEOUT)
                resp.raise_for_status()
                listing = resp.json()
            except (requests.RequestException, ValueError) as exc:
                print(f"[zhen-rag] vor topic listing failed: {exc}")
                return []
            if not isinstance(listing, list):
                return []
            self._topic_list = [
                (t.get("name", ""), t.get("category", ""))
                for t in listing if isinstance(t, dict)
            ]
            self._topic_list_at = now

        best = {}
        for name, category in self._topic_list:
            if not name or category.startswith(_WORKTREE_CATEGORY_PREFIX):
                continue
            tokens = _name_tokens(name)
            if wanted.issubset(tokens):
                best[name] = len(tokens)
        return sorted(best, key=lambda n: (best[n], n.lower()))

    # ------------------------------------------------------------------
    # GENERATION — qwen-coder via OpenAI /v1/chat/completions.
    #
    # Ports cmd/zhen-rag/main.go:llamaChat verbatim. Strips JSON tool-call
    # payloads from prior turns (T7). Memory recall is NOT done here —
    # zhen_app.py owns that decision.
    # ------------------------------------------------------------------

    def generate(self, query, context_chunks, file_content=None, history=None,
                 system_prompt=None, temperature=0.0, seed=42, max_tokens=600,
                 live_context=None):
        """Generate response. Dispatches to direct or proxied path.

        When self.proxy_via_agentd is True (WAVE15 Phase 2):
            POST cmd/zhen-agentd /api/v1/agent/ask with the question
            as the goal. The daemon does ITS OWN retrieval + agent loop
            + Champion gate-check. This is the T6-closed path: any tool
            call the model emits goes through pkg/champion.Dispatch.
            context_chunks/file_content/history/system_prompt are
            ignored — the daemon owns that build-step.

        When self.proxy_via_agentd is False (ZHEN_PROXY_VIA_AGENTD=0, debug):
            POST llama-server /v1/chat/completions directly with refs
            built from context_chunks. T6 open — no gate in the chat path.

        Raises AgentdGateUnavailable when proxying and the daemon cannot
        gate the request; there is no fall-back to the direct path.
        """
        if self.proxy_via_agentd:
            return self._generate_via_agentd(
                query, context_chunks,
                file_content=file_content, history=history,
                temperature=temperature, seed=seed, max_tokens=max_tokens,
                live_context=live_context,
            )
        return self._generate_via_llama(
            query, context_chunks,
            file_content=file_content, history=history,
            system_prompt=system_prompt,
            temperature=temperature, seed=seed, max_tokens=max_tokens,
            live_context=live_context,
        )

    def _generate_via_llama(self, query, context_chunks, file_content=None,
                            history=None, system_prompt=None,
                            temperature=0.0, seed=42, max_tokens=600,
                            live_context=None):
        """Direct path — POST llama-server /v1/chat/completions.

        Phase 1 mode. T6 OPEN — tool-call-emitting model output (if it
        ever happens for chat prompts) bypasses Champion. Used when
        proxy_via_agentd is False.

        live_context, when supplied, is a string containing live system
        state (kanban tasks, audit feed, etc) that the caller pre-fetched
        based on intent detection. It's prepended to the user message so
        the model has REAL data instead of hallucinating CLI commands.
        """
        if system_prompt is None:
            system_prompt = DEFAULT_SYSTEM_PROMPT

        # 1. Build the references block — byte-identical format to
        #    cmd/zhen-rag/main.go:formatReferences (commit b6c656c4-era).
        #    Format: "\n\n--- [<trust>] <category>/<topic> [(source: <label>)] ---\n<content>"
        refs = []
        for c in context_chunks:
            trust = c.get("trust", "canonical") or "canonical"
            label = c.get("label", "")
            label_suffix = f" (source: {label})" if label else ""
            category = c.get("category", "")
            name = c.get("id", "")
            refs.append(
                f"\n\n--- [{trust}] {category}/{name}{label_suffix} ---\n"
                f"{c.get('content', '')}"
            )
        refs_str = "".join(refs)

        # 2. Build the user prompt — same shape as cmd/zhen-rag.
        # The code-comment reminder goes after the question, where a small
        # model weighs it most: the same rule sits in the system prompt, but
        # the references in between are often uncommented code it copies.
        user_msg = f"References:{refs_str}\n\nQuestion: {query}\n\n{CODE_COMMENT_REMINDER}"
        if file_content:
            user_msg = f"FILE CONTENT:\n{file_content}\n\n{user_msg}"
        if live_context:
            # Live system state — kanban, audit, daemon health, etc.
            # Prepended above References so the model grounds answers
            # about live data in REAL rows instead of hallucinating
            # imaginary CLIs.
            user_msg = f"LIVE SYSTEM STATE (authoritative for this query):\n{live_context}\n\n{user_msg}"

        # 3. Compose messages: system + (summarized prior turns) + current.
        #    History is bounded by total content length (rough budget), and
        #    each prior turn's content has tool-call JSON stripped (T7).
        messages = []
        if system_prompt:
            messages.append({"role": "system", "content": system_prompt})
        # The worked example goes after the system prompt and before any real
        # history, so it reads as the conversation's first exchange.
        messages.extend(CODE_COMMENT_EXAMPLE)

        if history:
            # Last 6 turns max (3 user/assistant pairs), strip tool-call
            # payloads. Same pattern as the Go agent's history handling.
            recent = list(history)[-6:]
            for prior in recent:
                role = prior.get("role")
                if role not in ("user", "assistant"):
                    continue
                content = _strip_tool_call_json(prior.get("content", ""))
                if content:
                    messages.append({"role": role, "content": content})

        messages.append({"role": "user", "content": user_msg})

        # 4. POST /v1/chat/completions.
        try:
            response = requests.post(
                f"{self.inference_url}/v1/chat/completions",
                json={
                    "model": self.model_name,
                    "messages": messages,
                    "max_tokens": max_tokens,
                    "temperature": temperature,
                    "seed": seed,
                    "stream": False,
                },
                timeout=180,  # qwen-coder review prompts can take 30-60 s.
            )
            response.raise_for_status()
            body = response.json()
        except requests.exceptions.ConnectionError:
            return {
                "answer": f"Error: Inference server not reachable at {self.inference_url}",
                "tokens_used": 0,
                "model": self.model_name,
            }
        except requests.exceptions.Timeout:
            return {
                "answer": "Error: Inference timed out (180s)",
                "tokens_used": 0,
                "model": self.model_name,
            }
        except (requests.RequestException, ValueError) as exc:
            return {
                "answer": f"Inference error: {exc}",
                "tokens_used": 0,
                "model": self.model_name,
            }

        choices = body.get("choices") or []
        if not choices:
            return {"answer": "(empty response from llama-server)",
                    "tokens_used": 0, "model": self.model_name}

        msg = choices[0].get("message") or {}
        answer = (msg.get("content") or "").strip()

        usage = body.get("usage") or {}
        tokens = int(usage.get("completion_tokens") or 0)

        return {
            "answer": answer,
            "tokens_used": tokens,
            "model": self.model_name,
        }

    # ------------------------------------------------------------------
    # GENERATION via cmd/zhen-agentd (WAVE15 Phase 2 — T6 closure).
    #
    # POST the user question as a "goal" to /api/v1/agent/ask. The
    # daemon runs the full agent loop:
    #   * its own vor retrieval (refs stay grounded)
    #   * pkg/champion.Dispatch on every tool call
    #   * the agent's tool-shaped system prompt (Shape A vs Shape B)
    #
    # When the model emits Shape A (terminal answer — typical for chat
    # prompts), the daemon returns askResponse{Answer, ...} and we
    # extract the answer cleanly. When Shape B (tool call), Champion's
    # gate runs; refused calls produce a pending-confirm token in the
    # trace; the daemon's answer ends up being a refusal explanation
    # (per the agent's PENDING-CONFIRMATION clause).
    #
    # The daemon does its OWN retrieval, so context_chunks/system_prompt
    # passed in here are not used. context_chunks IS still useful at the
    # caller (zhen_app.py uses it to populate the UI's sources panel),
    # which is why query() runs retrieve() locally even when proxying.
    # ------------------------------------------------------------------

    def _generate_via_agentd(self, query, context_chunks, file_content=None,
                             history=None, temperature=0.0, seed=42,
                             max_tokens=600, live_context=None):
        """Proxy path — POST cmd/zhen-agentd /api/v1/agent/ask.

        WAVE15 Phase 2 path. T6 CLOSED — every LLM-emitted tool call
        traverses pkg/champion's three rules + audit log.

        live_context, when supplied, is prepended to the goal sent to
        the daemon so the agent loop sees the same authoritative live
        state the direct path injects.
        """
        # If the user uploaded a file, prepend it to the goal so the
        # daemon's agent loop sees it. History is sent as `session_id`
        # — the daemon's pool keys per session, so a stable id keeps
        # the conversation context coherent. For the H0 gate run each
        # prompt has a unique session id.
        goal = query
        if file_content:
            goal = f"FILE CONTENT:\n{file_content}\n\nQUESTION: {query}"
        if live_context:
            goal = f"LIVE SYSTEM STATE (authoritative):\n{live_context}\n\nQUESTION: {query}"

        # The daemon doesn't consume `history` directly (its agent loop
        # is per-goal, not multi-turn). Strip JSON tool-call payloads
        # from prior turns and concatenate as a brief context preamble.
        # This mirrors the T7 mitigation in _generate_via_llama.
        if history:
            recent = list(history)[-4:]   # last 2 pairs
            preamble = []
            for prior in recent:
                role = prior.get("role")
                content = _strip_tool_call_json(prior.get("content", ""))
                if role and content:
                    preamble.append(f"[{role}] {content}")
            if preamble:
                goal = "Prior conversation:\n" + "\n".join(preamble) + f"\n\nCurrent question: {query}"

        if self.agentd_auth_error:
            raise AgentdGateUnavailable(self.agentd_auth_error)

        try:
            response = requests.post(
                f"{self.agentd_url}/api/v1/agent/ask",
                json={
                    "goal":         goal,
                    "k":            5,
                    "max_tokens":   max_tokens,
                    "max_turns":    4,
                    "temperature":  temperature,
                    "seed":         seed,
                },
                headers=agentd_headers(self.agentd_api_key),
                timeout=300,  # agent loops can do multi-turn (read → answer)
            )
        except (requests.exceptions.ConnectionError, requests.exceptions.Timeout) as exc:
            # Fail closed: no silent bypass of the gate via llama-server.
            _log_gate_down(f"{self.agentd_url}: {type(exc).__name__}")
            raise AgentdGateUnavailable(
                f"{GATE_REQUIRED_MSG} ({self.agentd_url})") from exc
        except requests.RequestException as exc:
            _log_gate_down(f"{self.agentd_url}: {exc}")
            raise AgentdGateUnavailable(
                f"{GATE_REQUIRED_MSG} ({self.agentd_url}: {exc})") from exc

        if response.status_code in (401, 403):
            _log_gate_down(f"{self.agentd_url} refused the UI: HTTP {response.status_code}")
            raise AgentdGateUnavailable(
                f"zhen-agentd refused the web UI (HTTP {response.status_code}): check "
                "ZHEN_AGENTD_API_KEY against the daemon's AUTH_API_KEYS")
        if response.status_code >= 500 or response.status_code == 429:
            _log_gate_down(f"{self.agentd_url}: HTTP {response.status_code}")
            raise AgentdGateUnavailable(
                f"{GATE_REQUIRED_MSG} ({self.agentd_url}: HTTP {response.status_code})")
        try:
            response.raise_for_status()
            body = response.json()
        except (requests.RequestException, ValueError) as exc:
            return {
                "answer": f"zhen-agentd error: {exc}",
                "tokens_used": 0,
                "model": f"{self.model_name}@agentd",
            }
        _gate_up()

        answer = (body.get("answer") or "").strip()
        if not answer:
            answer = "(empty answer from zhen-agentd)"

        # The daemon doesn't surface a token count today; use 0.
        # The model id is annotated with @agentd so the audit trail in
        # zhen_conversations records the gate-protected path.
        return {
            "answer":      answer,
            "tokens_used": 0,
            "model":       f"{self.model_name}@agentd",
            # Pass through useful daemon-side metadata for the caller.
            "agent_trace":  body.get("trace", []),
            "session_id":   body.get("session_id", ""),
            "turns_used":   body.get("turns_used", 0),
            "budget_hit":   bool(body.get("budget_hit", False)),
        }

    # ------------------------------------------------------------------
    # Full RAG path — retrieve + generate.
    # ------------------------------------------------------------------

    def query(self, question, file_content=None, history=None,
              k=5, temperature=0.0, seed=42, max_tokens=600,
              live_context=None):
        """Full RAG query: retrieve top-k via vor, then generate via qwen-coder.

        Returns the SAME dict shape zhen_app.py expects:
            { question, retrieved, answer, tokens_used, model }

        IMPORTANT: this function NEVER consults the memory cache. The
        memory short-circuit (Python's pre-rewire path that returned a
        cached answer for similarity >= 0.9, BYPASSING the LLM) was T1
        (stored prompt injection via memory replay). zhen_app.py is now
        responsible for surfacing matched memories as a SIDE-CHANNEL
        ("matched_memory" field on /api/v1/query response) without
        gating the LLM call.
        """
        retrieved = self.retrieve(question, k=k)
        result = self.generate(
            question, retrieved,
            file_content=file_content, history=history,
            temperature=temperature, seed=seed, max_tokens=max_tokens,
            live_context=live_context,
        )
        return {
            "question":    question,
            "retrieved":   retrieved,
            "answer":      result["answer"],
            "tokens_used": result.get("tokens_used", 0),
            "model":       result.get("model", self.model_name),
        }

    # ------------------------------------------------------------------
    # Teach endpoint — STUB.
    #
    # The legacy implementation appended chunks to the live FAISS index +
    # ring_all.jsonl. With vor as substrate, "teaching" means writing a
    # markdown file under ~/.config/cs/sources/<source>/ which cs picks
    # up on its next index sweep.
    #
    # Phase 1 stubs this with a clear message. A future change can wire
    # it to write under ~/.config/cs/sources/zhen-taught/.
    # ------------------------------------------------------------------

    def add_to_corpus(self, text, source="user"):
        """STUB after WAVE15 rewire. See module docstring."""
        return {
            "added":  0,
            "status": "deprecated",
            "reason": (
                "WAVE15 rewire: 'teach' is no longer wired to a live index. "
                "vor is the retrieval substrate; to add content, drop a markdown "
                "file under ~/.config/cs/sources/<source>/. cs picks it up on "
                "the next index sweep."
            ),
            "received_chars": len(text or ""),
        }


def main():
    """Smoke test: run a few sample queries against the rewired pipeline.

    Boots in ~2 s (no FAISS, no ring_all, no Wikipedia offset index).
    Requires vor :9876 and llama-server :8081 to be reachable.
    """
    rag = RAGPipeline()  # defaults: vor :9876, llama :8081, qwen-coder.

    test_queries = [
        "What is Unheaded?",
        "How does the eBPF layer work in Unheaded?",
        "What are the core services in Unheaded?",
        "What is the Wotan message bus?",
        "How does the Monad wire format work?",
    ]

    for q in test_queries:
        print("\n" + "=" * 60)
        result = rag.query(q, temperature=0.0, seed=42, max_tokens=200)
        print(f"Q: {result['question']}")
        sources = [r.get("source", r.get("id", "?")) for r in result["retrieved"][:3]]
        print(f"Sources: {sources}")
        print(f"A: {result['answer'][:300]}...")
        print(f"Tokens: {result['tokens_used']} | Model: {result['model']}")


if __name__ == "__main__":
    main()
