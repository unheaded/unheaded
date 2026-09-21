#!/usr/bin/env python3
"""
run-runbook.py — Execute an Unheaded operational runbook (YAML format)

Reads a runbook YAML file and executes steps sequentially with:
- Precondition checks
- Environment variable injection
- Command execution with timeouts
- Verification gates after each step
- Rollback on failure
- Dry-run mode for validation

Usage:
  python3 scripts/run-runbook.py runbooks/observe/service-health-sweep.yaml
  python3 scripts/run-runbook.py --dry-run runbooks/security/ssh-hardening.yaml
  python3 scripts/run-runbook.py --step 3 runbooks/doom/doom-ring-setup.yaml
"""

import argparse
import os
import re
import subprocess
import sys

import yaml


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
        result = subprocess.run(  # nosec B602 - see comment above
            cmd, shell=True, capture_output=True, text=True,
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


def execute_steps(runbook, env, dry_run=False, start_step=0):
    """Execute runbook steps sequentially."""
    steps = runbook.get("steps", [])
    if not steps:
        print("No steps defined.")
        return True

    print(f"\n=== Executing {len(steps)} Steps ===\n")
    failed_step = None

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
                break

    return failed_step


def execute_rollback(runbook, env, failed_step, dry_run=False):
    """Execute rollback steps if available."""
    rollbacks = runbook.get("rollback", [])
    if not rollbacks:
        return

    print(f"\n=== Rollback (triggered by: {failed_step}) ===")
    for rb in rollbacks:
        name = rb.get("name", "rollback")
        command = rb.get("command", "")

        print(f"  [ROLLBACK] {name}")
        if command:
            run_command(command, env=env, dry_run=dry_run)


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

    print("=" * 60)
    print(f"RUNBOOK: {meta.get('name', args.runbook)}")
    print(f"  {meta.get('description', '')}")
    print(f"  Risk: {meta.get('risk', 'unknown')} | ETA: {meta.get('estimated_duration', '?')}")
    if args.dry_run:
        print("  MODE: DRY RUN (no commands will be executed)")
    if meta.get("status") in ("stub", "draft"):
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
    failed = execute_steps(runbook, env, dry_run=args.dry_run, start_step=args.step)

    if failed:
        print(f"\n[FAILED] Runbook stopped at: {failed}")
        execute_rollback(runbook, env, failed, dry_run=args.dry_run)
        sys.exit(1)
    else:
        print("\n[SUCCESS] Runbook completed.")
        sys.exit(0)


if __name__ == "__main__":
    main()
