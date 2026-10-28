#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
"""
run-runbook.py — Execute an Unheaded operational runbook (YAML format)

Reads a runbook YAML file and executes steps sequentially with:
- Precondition checks
- Environment variable injection
- Command execution with timeouts
- Verification gates after each step
- Rollback on failure
- Dry-run mode for validation
- Steps with `host:` run only on that host (exit 2 elsewhere)

Usage:
  python3 scripts/run-runbook.py runbooks/observe/service-health-sweep.yaml
  python3 scripts/run-runbook.py --dry-run runbooks/security/ssh-hardening.yaml
  python3 scripts/run-runbook.py --step 3 runbooks/doom/doom-ring-setup.yaml
"""

import argparse
import ipaddress
import json
import os
import re
import socket
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

import yaml

# scripts/svc_token.py (Wotan service tokens); import it from this file's
# directory even when run as `python3 -P` or from elsewhere.
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import svc_token

# Short hostname, compared against a step's `host:` (west, east).
LOCAL_HOST = socket.gethostname().split(".")[0].lower()


def load_runbook(path):
    with open(path) as f:
        return yaml.safe_load(f)


def expand_env(s, env):
    """Expand ${VAR} and ~ in a string."""
    if not isinstance(s, str):
        return s
    result = os.path.expanduser(s)
    for k, v in env.items():
        result = result.replace(f"${{{k}}}", str(v))
    return result


def run_command(cmd, timeout=120, env=None, dry_run=False):
    """Run a shell command and return (exit_code, stdout, stderr)."""
    cmd = expand_env(cmd, env or {})
    if dry_run:
        print(f"    [DRY RUN] {cmd[:200]}")
        return 0, "(dry run)", ""

    try:
        # bandit B602 (shell=True) — accepted, by design.
        #
        # A runbook step IS a shell command line: runbooks/ use pipes,
        # redirects, && chains and globs throughout. Rewriting them as argv
        # lists would mean reimplementing a shell.
        #
        # There is no injection boundary to cross here. Both `cmd` and the
        # `env` values interpolated into it come from the SAME runbook YAML
        # file (main() reads them from runbook["env"]); nothing is taken from
        # argv, stdin, the network or the environment. Executing a runbook is
        # exactly as privileged as running the shell script it encodes — the
        # trust boundary is the file, and it is a repo-controlled artifact.
        #
        # This stops holding if a runbook ever takes untrusted parameters
        # (e.g. a --set KEY=VALUE flag, or values fetched from a service). At
        # that point the interpolation into a shell string becomes a real
        # injection path and this must move to an argv list.
        #
        # bash, not /bin/sh: shell=True alone runs dash on Debian/Ubuntu, and
        # the runbooks are bash (`source`, `[[ ]]`, brace expansion, process
        # substitution). Under dash those are syntax errors, and `cmd &> f`
        # parses as `cmd &` then `> f`: backgrounded, output file truncated.
        result = subprocess.run(  # nosec B602 - see comment above
            cmd, shell=True, executable="/bin/bash", capture_output=True, text=True,
            timeout=timeout, env={**os.environ, **(env or {})}, check=False
        )
        return result.returncode, result.stdout.strip(), result.stderr.strip()
    except subprocess.TimeoutExpired:
        return 1, "", f"TIMEOUT after {timeout}s"
    except Exception as e:
        return 1, "", str(e)


def find_unset_params(runbook, env):
    """Return {VAR: [step names]} for ${VAR} references nothing will supply.

    Several runbooks are parameterised — service-deploy wants ${SERVICE},
    network-diagnostics wants ${TARGET} — but the schema has no way to declare a
    required parameter, so an unset one expands to the empty string and the step
    runs malformed: `lxc delete ${CONTAINER} --force` becomes `lxc delete --force`,
    `go test ./cmd/${SERVICE}/...` becomes `./cmd//...`.

    Only the braced ${VAR} form is considered. Bare $VAR is skipped because awk
    programs embedded in these commands use $NF and $3, and those are not shell
    variables. ${VAR:-default} and ${VAR:?msg} are skipped: both already handle
    being unset.
    """
    unset = {}
    for section in ("preconditions", "steps", "rollback", "verification", "decommission"):
        for step in runbook.get(section) or []:
            if not isinstance(step, dict):
                continue
            for field in ("command", "verify", "check"):
                body = step.get(field)
                if not isinstance(body, str):
                    continue
                # Assigned earlier in this same snippet: PORT=$(...), for X in, read X
                local = set(re.findall(r"^\s*([A-Za-z_]\w*)=", body, re.MULTILINE))
                local |= set(re.findall(r"\bfor\s+([A-Za-z_]\w*)\s+in\b", body))
                local |= set(re.findall(r"\bread\s+(?:-r\s+)?([A-Za-z_]\w*)", body))
                for var in re.findall(r"\$\{([A-Za-z_]\w*)\}", body):
                    if var in env or var in os.environ or var in local:
                        continue
                    unset.setdefault(var, []).append(step.get("name", "?"))
    return unset


def check_preconditions(runbook, env, dry_run):
    """Check all preconditions. Returns True if all pass."""
    preconditions = runbook.get("preconditions", [])
    if not preconditions:
        return True

    print("\n=== Precondition Checks ===")
    all_pass = True
    for pc in preconditions:
        check = expand_env(pc.get("check", ""), env)
        desc = pc.get("description", check)
        optional = pc.get("optional", False)

        if dry_run:
            print(f"  [DRY RUN] {desc}")
            continue

        code, _out, err = run_command(check, timeout=10, env=env)
        if code == 0:
            print(f"  [PASS] {desc}")
        elif optional:
            print(f"  [SKIP] {desc} (optional)")
        else:
            print(f"  [FAIL] {desc}")
            if err:
                print(f"         {err[:200]}")
            all_pass = False

    return all_pass


# Per-step progress on Wotan (S-CHAMPION-HARDENING Step 224): the Kanban
# board shows `step k/n` for a running runbook. Opt-in: only when
# RUNBOOK_PROGRESS_WOTAN names Wotan's HTTP address (host:port or a URL).
# Every request carries an EdDSA service token for Wotan (ADR-103 §6,
# scripts/svc_token.py) over https, or plain http to loopback only (never in
# clear to another host: token_in_clear), whose subject Wotan's AUTH_ALLOW_CALLERS and
# topics.auto_approve must list (configs/wotan.yaml). The runner presents
# the key it is given (progress_identity): under the Champion that is
# zhen-agentd's own key (its unit sets AUTH_KEY_FILE), run by hand the
# operator's CLI key. No key: one line on stderr and no token (a Wotan with
# AUTH_ENABLED=true refuses the join, and progress turns off). The join's
# display_name stays PROGRESS_PUBLISHER, a label only once auth is on (and
# the name auto_approve matched before Phase 7). A join that is not
# approved, an HTTP error or a timeout turns progress off for the rest of
# the run with one line on stderr. Progress never changes a run's outcome.
# Events carry the runbook ref, step number, total, step name and state:
# names from the committed runbook, never command output.
PROGRESS_TOPIC = "runbooks.progress"
PROGRESS_PUBLISHER = "runbook-runner"
PROGRESS_TIMEOUT = 2
RUNBOOK_REF_RE = re.compile(r"^[a-z0-9-]{1,64}/[a-z0-9-]{1,128}$")
CLI_IDENTITY = "unheaded-cli"


def progress_identity(env=None):
    """(identity, key file) the runner presents to Wotan.

    1. AUTH_KEY_FILE: that key, as RUNBOOK_PROGRESS_IDENTITY if set, else as
       the name the file is filed under (<name>.key, or compose's secret
       svc_key_<name>). zhen-agentd.service sets AUTH_KEY_FILE to its own
       key, so a Champion run reports as zhen-agentd.
    2. else RUNBOOK_PROGRESS_IDENTITY with /etc/unheaded/keys/<name>.key.
    3. else the operator's CLI identity, unheaded-cli, from the CLI's key:
       ~/.config/unheaded/cli.key, then /etc/unheaded/keys/unheaded-cli.key.
    """
    env = os.environ if env is None else env
    explicit = env.get("RUNBOOK_PROGRESS_IDENTITY", "").strip()
    key_file = env.get("AUTH_KEY_FILE", "").strip()
    if key_file:
        if explicit:
            return explicit, key_file
        name = os.path.basename(key_file)
        name = name.removesuffix(".key")
        name = name.removeprefix("svc_key_")
        return name, key_file
    if explicit:
        return explicit, os.path.join(svc_token.DEFAULT_KEY_DIR, explicit + ".key")
    candidates = [os.path.join(os.path.expanduser("~"), ".config", "unheaded", "cli.key"),
                  os.path.join(svc_token.DEFAULT_KEY_DIR, CLI_IDENTITY + ".key")]
    for c in candidates:
        if os.path.exists(c):
            return CLI_IDENTITY, c
    return CLI_IDENTITY, candidates[-1]


def load_progress_minter():
    """The runner's minter, or None (after one line on stderr)."""
    name, key_file = progress_identity()
    try:
        return svc_token.load(name, key_file)
    except FileNotFoundError:
        print(f"[progress] no service key at {key_file}: progress carries no token as {name!r}"
              f" (a Wotan with AUTH_ENABLED=true refuses it; ADR-103)", file=sys.stderr)
    except (OSError, svc_token.KeyFileError) as e:
        print(f"[progress] service key unusable ({e}): progress carries no token as {name!r}", file=sys.stderr)
    return None


def runbook_ref(path, meta):
    """category/name: RUNBOOK_REF (the Champion sets it: a live run gets a
    private copy whose path says nothing), else the path under the nearest
    runbooks/ directory, else metadata.name."""
    ref = os.environ.get("RUNBOOK_REF", "").strip()
    if ref:
        return ref
    parts = os.path.normpath(os.path.abspath(path)).split(os.sep)
    if "runbooks" in parts:
        i = len(parts) - 1 - parts[::-1].index("runbooks")
        rel = "/".join(parts[i + 1:])
        if rel.endswith(".yaml"):
            return rel[:-len(".yaml")]
    return str(meta.get("name", ""))


def token_in_clear(url):
    """True when a token sent to url would cross the network in clear: plain
    http to anything but loopback. A service token travels only over TLS, or
    in clear to loopback (S-IRON-GATE Phase 8; pkg/wotan-client withholds it
    the same way, pkg/tls.IsLoopbackHost)."""
    parsed = urllib.parse.urlsplit(url)
    if parsed.scheme == "https":
        return False
    host = (parsed.hostname or "").strip("[]")
    if host == "localhost":
        return False
    try:
        return not ipaddress.ip_address(host).is_loopback
    except ValueError:
        return True


class Progress:
    """Best-effort publisher of runbooks.progress events."""

    def __init__(self, ref, total, dry_run):
        self.ref, self.total, self.dry_run = ref, total, dry_run
        self.base = None
        self.subscriber = None
        self.minter = None
        addr = os.environ.get("RUNBOOK_PROGRESS_WOTAN", "").strip()
        if not addr:
            return
        if not RUNBOOK_REF_RE.match(ref or ""):
            self.off(f"runbook ref {ref!r} is not category/name")
            return
        if not addr.startswith(("http://", "https://")):
            addr = "http://" + addr
        self.base = addr.rstrip("/") + "/api/v1/topics/" + PROGRESS_TOPIC
        if token_in_clear(addr):
            print(f"[progress] no token over plain http to {addr}: use https:// (ADR-103; a Wotan with"
                  f" AUTH_ENABLED=true refuses the join)", file=sys.stderr)
        else:
            self.minter = load_progress_minter()

    def off(self, why):
        print(f"[progress] off: {why}", file=sys.stderr)
        self.base = None

    def post(self, suffix, body):
        headers = {"Content-Type": "application/json"}
        if self.minter is not None:
            try:
                headers["Authorization"] = self.minter.authorization(svc_token.WOTAN_AUDIENCE)
            except svc_token.KeyFileError as e:
                raise ValueError(f"service token: {e}") from None
        req = urllib.request.Request(self.base + suffix, data=json.dumps(body).encode(), method="POST",
                                     headers=headers)
        # The operator's own RUNBOOK_PROGRESS_WOTAN; http(s) only (prefixed above).
        with urllib.request.urlopen(req, timeout=PROGRESS_TIMEOUT, context=self.tls_context()) as resp:  # nosec B310
            return resp.status, json.loads(resp.read().decode() or "{}")

    def tls_context(self):
        """For an https:// Wotan (S-IRON-GATE Phase 8): TLS 1.3 only, the
        server checked against UNHEADED_TLS_CA_CERT (pkg/tls's CA variable)
        when set, else the system roots. OpenSSL 3.5+ offers the hybrid
        X25519MLKEM768 group by default."""
        if not self.base.startswith("https://"):
            return None
        ctx = ssl.create_default_context(cafile=os.environ.get("UNHEADED_TLS_CA_CERT") or None)
        ctx.minimum_version = ssl.TLSVersion.TLSv1_3
        return ctx

    def join(self):
        status, body = self.post("/subscribe", {"display_name": PROGRESS_PUBLISHER})
        sub = body.get("subscriber") or {}
        if status != 201 or sub.get("status") != "approved" or not sub.get("subscriber_id"):
            who = self.minter.name if self.minter is not None else PROGRESS_PUBLISHER
            raise ValueError(f"join {PROGRESS_TOPIC} as {who}: status {sub.get('status', status)!r}"
                             f" (is {who} on Wotan's topics.auto_approve?)")
        self.subscriber = sub["subscriber_id"]

    def emit(self, step, state, name=""):
        if self.base is None:
            return
        event = {"runbook": self.ref, "step": step, "total": self.total, "name": str(name)[:128],
                 "state": state, "dry_run": self.dry_run, "ts": int(time.time() * 1000)}
        try:
            if self.subscriber is None:
                self.join()
            self.post("/publish", {"subscriber_id": self.subscriber, "payload": json.dumps(event)})
        except (OSError, ValueError) as e:  # URLError and HTTPError are OSErrors
            self.off(str(e)[:200])


def execute_steps(runbook, env, dry_run=False, start_step=0, progress=None):
    """Execute runbook steps sequentially."""
    steps = runbook.get("steps", [])
    if not steps:
        print("No steps defined.")
        return True

    print(f"\n=== Executing {len(steps)} Steps ===\n")
    failed_step = None
    emit = progress.emit if progress is not None else (lambda *_a, **_k: None)

    for i, step in enumerate(steps):
        if i < start_step:
            continue

        name = step.get("name", f"step-{i+1}")
        action = step.get("action", "shell")
        command = step.get("command", "")
        verify = step.get("verify", "")
        timeout = step.get("timeout", 120)
        on_fail = step.get("on_fail", "")
        desc = step.get("description", "")

        print(f"  [{i+1}/{len(steps)}] {name}")
        if desc:
            print(f"           {desc}")
        emit(i + 1, "running", name)

        # `host:` names the machine a step belongs to (wireguard-overlay
        # alternates west and east). Nothing here can run a step elsewhere, and
        # running it on this machine instead would configure the wrong host,
        # so stop and say where it goes.
        host = expand_env(step.get("host", ""), env)
        if host and action != "manual" and host.lower() != LOCAL_HOST:
            print(f"    [WRONG HOST] this step runs on {host}; this is {LOCAL_HOST}.")
            print(f"           Continue there: run-runbook.py --step {i} <runbook>")
            if dry_run:
                emit(i + 1, "skipped", name)
                continue
            # Not a failure: nothing ran, so there is nothing to roll back.
            print(f"\n[STOPPED] before step {i+1} ({name}); no rollback, nothing failed.")
            emit(i + 1, "stopped", name)
            sys.exit(2)

        if action == "manual":
            # A step the operator performs by hand — it needs a value the runbook
            # cannot know (a local path, a container IP), or the automation for it
            # has not been written yet. Print it and move on. Its `verify` is not
            # run either: verification of a step that did not execute is also the
            # operator's job, and running it would fail the whole runbook here.
            print("    [MANUAL] perform this step yourself:")
            for line in (command or "(see description)").split("\n"):
                print(f"             {line}")
            if verify:
                print("    [MANUAL] then confirm:")
                for line in verify.split("\n"):
                    print(f"             {line}")
            emit(i + 1, "manual", name)
            continue

        if action == "shell" and command:
            code, out, err = run_command(command, timeout=timeout, env=env, dry_run=dry_run)

            if out and not dry_run:
                for line in out.split('\n')[:10]:
                    print(f"           {line}")

            if code != 0 and not dry_run:
                print(f"    [FAIL] exit code {code}")
                if err:
                    print(f"           {err[:300]}")
                if on_fail:
                    print(f"    [HINT] {on_fail[:300]}")
                failed_step = name
                emit(i + 1, "failed", name)
                break

        # Verification
        if verify and not dry_run:
            vcode, _, verr = run_command(verify, timeout=30, env=env)
            if vcode == 0:
                print("    [VERIFIED]")
            else:
                print(f"    [VERIFY FAIL] {verr[:200]}")
                if on_fail:
                    print(f"    [HINT] {on_fail[:300]}")
                failed_step = name
                emit(i + 1, "failed", name)
                break
        emit(i + 1, "ok", name)

    return failed_step


def run_verification(runbook, env, dry_run=False):
    """Run the `verification:` section after every step succeeded.

    Each entry is {name, description, command (or check), action, timeout};
    `action: manual` entries are printed for the operator, not run. Every
    check runs (one failure does not hide the others) and the summary line
    is `verification: n/m passed`. Returns the names of the failed checks;
    no section means nothing to do and an empty list.
    """
    entries = runbook.get("verification") or []
    if not isinstance(entries, list) or not entries:
        return []
    checks = [v for v in entries if isinstance(v, dict) and v.get("action", "shell") != "manual"]
    print(f"\n=== Verification ({len(checks)} check{'' if len(checks) == 1 else 's'}) ===")
    failed, passed, manual = [], 0, 0
    for i, v in enumerate(entries):
        if not isinstance(v, dict):
            continue
        name = v.get("name", f"verification-{i+1}")
        desc = v.get("description", "")
        text = v.get("command") or v.get("check") or ""
        if v.get("action", "shell") == "manual":
            manual += 1
            print(f"  [MANUAL] {name}: confirm yourself:")
            for line in (text or desc or "(see description)").split("\n"):
                print(f"             {line}")
            continue
        if dry_run:
            print(f"  [DRY RUN] {name}" + (f": {desc}" if desc else ""))
            run_command(text, env=env, dry_run=True)
            continue
        code, out, err = run_command(text, timeout=v.get("timeout", 120), env=env)
        for line in (out.split("\n")[:10] if out else []):
            print(f"           {line}")
        if code == 0:
            passed += 1
            print(f"  [PASS] {name}" + (f": {desc}" if desc else ""))
        else:
            failed.append(name)
            print(f"  [FAIL] {name}" + (f": {desc}" if desc else "") + f" (exit {code})")
            if err:
                print(f"         {err[:300]}")
            if v.get("expect"):
                print(f"         expected: {v['expect']}")
    tail = f", {manual} manual" if manual else ""
    if dry_run:
        print(f"verification: {len(checks)}/{len(checks)} (dry run){tail}")
    else:
        print(f"verification: {passed}/{len(checks)} passed{tail}")
    return failed


def execute_rollback(runbook, env, failed_step, dry_run=False):
    """Execute rollback steps if available."""
    rollbacks = runbook.get("rollback", [])
    if isinstance(rollbacks, dict):
        # active-active-cutover nests them: rollback: {description, steps: [...]}
        rollbacks = rollbacks.get("steps") or []
    if not rollbacks:
        return

    # Steps that completed before the failure (steps run in order).
    names = [s.get("name", f"step-{i+1}") for i, s in enumerate(runbook.get("steps") or [])]
    completed = set(names[:names.index(failed_step)]) if failed_step in names else set()

    print(f"\n=== Rollback (triggered by: {failed_step}) ===")
    for rb in rollbacks:
        name = rb.get("name", "rollback")
        command = rb.get("command", "")

        # `when: "step.<name>.failed"` (optionally "step.<a>.completed AND
        # ...") was never read: every rollback ran on every failure, e.g.
        # restoring sshd_config because an unrelated later step failed.
        # Free-text conditions ("any step fails") still always apply.
        when = str(rb.get("when", ""))
        clauses = re.findall(r"step\.([\w-]+)\.(failed|completed)", when)
        if clauses and not all(
            (s == failed_step) if state == "failed" else (s in completed)
            for s, state in clauses
        ):
            print(f"  [ROLLBACK SKIPPED] {name} (when: {when})")
            continue

        host = expand_env(rb.get("host", ""), env)
        if host and host.lower() != LOCAL_HOST:
            # east-nat-internet's east rollback would delete this host's route.
            print(f"  [ROLLBACK ON {host.upper()}] {name}: run it there:")
            for line in (command or "").split("\n"):
                print(f"             {line}")
            continue

        print(f"  [ROLLBACK] {name}")
        if rb.get("action") == "manual":
            # Placeholders such as <previous_version>: the operator's job.
            for line in (command or "(see description)").split("\n"):
                print(f"             {line}")
            continue
        if command:
            # The output was discarded, so echo-only rollback steps (the
            # restore instructions) printed nothing, and failures were silent.
            code, out, err = run_command(command, env=env, dry_run=dry_run)
            for line in (out.split("\n") if out and not dry_run else []):
                print(f"           {line}")
            if code != 0 and not dry_run:
                print(f"    [ROLLBACK FAIL] exit code {code} {err[:300]}")


def main():
    parser = argparse.ArgumentParser(description="Execute an Unheaded operational runbook")
    parser.add_argument("runbook", help="Path to runbook YAML file")
    parser.add_argument("--dry-run", action="store_true", help="Validate without executing")
    parser.add_argument("--step", type=int, default=0, help="Start from step N (0-indexed)")
    args = parser.parse_args()

    if not os.path.exists(args.runbook):
        print(f"ERROR: Runbook not found: {args.runbook}")
        sys.exit(1)

    runbook = load_runbook(args.runbook)
    meta = runbook.get("metadata", {})
    env_vars = runbook.get("env", {})

    # Expand env vars
    env = {}
    for k, v in env_vars.items():
        env[k] = os.path.expanduser(str(v))

    # Declared parameter defaults. The schema allowed `default:` on a
    # parameter but nothing applied it, so load-test.yaml's REQUESTS=1000 was
    # never in effect; the operator's environment wins when it names the
    # variable. Seeding env here also keeps find_unset_params from demanding
    # a value the runbook says is optional.
    for param in runbook.get("parameters") or []:
        if not isinstance(param, dict) or "default" not in param:
            continue
        name = param.get("name")
        if name and name not in env and name not in os.environ:
            # ~ and $HOME-style references expand like env: values do; left
            # literal, "$HOME/x" reached bash as a value and stayed unexpanded.
            env[name] = os.path.expandvars(os.path.expanduser(str(param["default"])))

    print("=" * 60)
    print(f"RUNBOOK: {meta.get('name', args.runbook)}")
    print(f"  {meta.get('description', '')}")
    print(f"  Risk: {meta.get('risk', 'unknown')} | ETA: {meta.get('estimated_duration', '?')}")
    if args.dry_run:
        print("  MODE: DRY RUN (no commands will be executed)")
    if meta.get("status") == "draft":
        # S-TWO-BRAINS drafts (ADR-024): implemented and tested in dry-run
        # form, but not yet approved by their owner.
        print("  STATUS: DRAFT — not yet approved (metadata.approved_by is empty).")
        print("          Read every step before running it; manual steps")
        print("          are printed, never executed.")
    elif meta.get("status") == "stub":
        # `status: stub` was already a convention in the runbooks; nothing read it.
        # A stub's steps are action: manual placeholders, so without this banner the
        # run ends in [SUCCESS] having done almost nothing — which on a risk: high
        # runbook reads as "the cutover ran".
        print(f"  STATUS: {meta['status'].upper()} — steps are not implemented.")
        print("          Executing this performs no changes; it is a checklist,")
        print("          not a script.")
    print("=" * 60)

    # Required parameters. Checked before preconditions, because a precondition
    # that interpolates an unset parameter is itself unreliable.
    unset = find_unset_params(runbook, env)
    if unset:
        print("\nUNSET PARAMETERS — this runbook expects values it was not given:\n")
        for var, steps in sorted(unset.items()):
            where = ", ".join(sorted(set(steps))[:4])
            print(f"  ${{{var}}}  used by: {where}")
        print("\nUnset, each expands to the empty string and its step runs")
        print("malformed rather than failing. Supply them in the environment:\n")
        print("  " + " ".join(f"{v}=..." for v in sorted(unset)) + f" {sys.argv[0]} {args.runbook}")
        if not args.dry_run:
            sys.exit(1)
        print("\n(continuing: --dry-run)")

    # Check preconditions
    if not check_preconditions(runbook, env, args.dry_run):
        print("\nPreconditions FAILED. Aborting.")
        sys.exit(1)

    # Execute steps
    total = len(runbook.get("steps") or [])
    progress = Progress(runbook_ref(args.runbook, meta), total, args.dry_run)
    failed = execute_steps(runbook, env, dry_run=args.dry_run, start_step=args.step, progress=progress)

    if failed:
        print(f"\n[FAILED] Runbook stopped at: {failed}")
        execute_rollback(runbook, env, failed, dry_run=args.dry_run)
        sys.exit(1)

    # The runbook's own end-to-end checks. A failure here does not roll
    # back: every step succeeded, and whether to undo them is the
    # operator's call (rollback entries are keyed to a failed step).
    unverified = run_verification(runbook, env, dry_run=args.dry_run)
    if unverified:
        print(f"\n[FAILED] verification: {', '.join(unverified)}")
        progress.emit(total, "failed", "verification")
        sys.exit(1)
    progress.emit(total, "done")
    print("\n[SUCCESS] Runbook completed.")
    sys.exit(0)


if __name__ == "__main__":
    main()
