# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/security/build-key-ceremony.yaml (S-IRON-GATE
Step 339; custody per DP-06, S-E1-LIVE Step 13). No key is generated and
no host is touched: the runbook is dry-run and its text is checked. The
ceremony is Stevie's, with both tokens, and the runbook stays a draft until
he approves it."""
import re

import harness

RB = "runbooks/security/build-key-ceremony.yaml"
ORDER = ["generate-build-key", "print-public-key", "publish-public-key", "sign-a-test-cask",
         "close-and-prove-second-token", "audit-watch", "copy-encrypted-file-to-backup-host",
         "rotate-quarterly", "revoke"]
VOLUME = "/var/lib/unheaded/custody/buildkey.luks"
MOUNT = "/run/unheaded-buildkey"


def test_dry_run_lists_every_step_in_order():
    names = harness.dry_run(RB)
    assert names == [s["name"] for s in harness.steps(harness.load(RB))]
    assert names == ORDER


def test_draft_until_stevie_approves():
    meta = harness.load(RB)["metadata"]
    assert meta["status"] == "draft" and meta["approved_by"] == ""
    assert meta["risk"] == "critical"
    notes = harness.load(RB)["notes"]
    assert "DP-06" in notes and "WAITS ON" not in notes


def test_every_host_changing_step_is_manual():
    rb = harness.load(RB)
    for s in harness.steps(rb):
        if re.search(r"\b(sudo|scp|git -C|shred|augenrules|tee)\b", s["command"]):
            assert s["action"] == "manual", s["name"]
    for name in ("generate-build-key", "revoke"):
        assert harness.step(rb, name).get("irreversible") is True, name


def test_shell_steps_only_read_and_never_read_the_seed():
    rb = harness.load(RB)
    for s in harness.steps(rb) + list(rb.get("verification") or []):
        if s.get("action") != "shell":
            continue
        for c in harness.commands(s["command"]):
            assert not re.search(r"\b(sudo|rm|kill|chmod|chown|tee|shred|scp|cp|mv)\b", c), f"{s['name']}: {c}"
            # stat on the seed is fine; anything that opens it is not.
            if ".key" in c:
                assert c.startswith(("stat ", "test ")), f"{s['name']} reads the seed: {c}"


def test_key_is_mldsa65_owned_by_the_custody_account():
    gen = harness.step(harness.load(RB), "generate-build-key")["command"]
    assert f"unheaded-keygen -algo mldsa65 -dir {MOUNT} -owner unheaded-buildkey ${{KEY_NAME}}" in gen


def test_only_the_public_key_travels():
    rb = harness.load(RB)
    for s in harness.steps(rb):
        for line in s["command"].splitlines():
            if re.search(r"\b(scp|cp|rsync|git -C \S+ add)\b", line):
                assert ".key" not in line.replace(".pubkey", ""), f"{s['name']} moves a seed: {line}"
    assert "secrets" not in harness.step(rb, "publish-public-key")["command"].lower()
    assert "never in GitHub secrets" in " ".join(rb["notes"].split())


def test_publish_is_a_signed_commit_recorded_in_security_md():
    pub = harness.step(harness.load(RB), "publish-public-key")["command"]
    assert "compliance/keys/build.pubkey" in pub
    assert "commit -S" in pub and "SECURITY.md" in pub


def test_test_cask_signs_through_the_custody_account_and_refuses_tamper():
    s = harness.step(harness.load(RB), "sign-a-test-cask")["command"]
    assert "UNHEADED_BUILD_SIGN_AS=unheaded-buildkey" in s
    assert f"UNHEADED_BUILD_KEY={MOUNT}/${{KEY_NAME}}.key" in s
    assert "cask-verify verify" in s and "printf x >>" in s and "tamper_rc" in s
    assert "mktemp -d" in s and 'rm -rf "$D"' in s


def test_rotation_keeps_one_quarter_of_overlap():
    rot = harness.step(harness.load(RB), "rotate-quarterly")["command"]
    assert "build-previous.pubkey" in rot and "--pubkey twice" in rot


def _code(text):
    """A step's command lines without its comments."""
    return [ln for ln in text.splitlines() if ln.strip() and not ln.lstrip().startswith("#")]


def test_seed_lives_only_inside_the_custody_volume():
    # DP-06: no seed path on a plain disk anywhere in the runbook.
    text = (harness.REPO / RB).read_text()
    assert "/etc/unheaded/keys" not in text
    paths = set(re.findall(r"[^\s=]+\.key\b", text))
    assert f"{MOUNT}/${{KEY_NAME}}.key" in paths
    for path in paths:
        if path.endswith("name>.key"):  # "<old name>.key", in the mounted volume too
            continue
        assert path.startswith(MOUNT + "/"), path
    assert VOLUME in harness.load(RB)["notes"]


def test_format_refuses_an_existing_volume():
    # luksFormat and mkfs destroy a seed: only in the first ceremony, and
    # the format line itself refuses when the file is there.
    rb = harness.load(RB)
    for s in harness.steps(rb):
        for ln in _code(s["command"]):
            if "luksFormat" in ln or "mkfs" in ln:
                assert s["name"] == "generate-build-key", s["name"]
            if "luksFormat" in ln:
                assert ln.lstrip().startswith(f"sudo test ! -e {VOLUME} && "), ln
    rot = "\n".join(_code(harness.step(rb, "rotate-quarterly")["command"]))
    assert "systemd-cryptsetup attach unheaded-buildkey" in rot and "unheaded-keygen" in rot


def test_password_slot_goes_only_after_each_token_opened_the_volume():
    gen = harness.step(harness.load(RB), "generate-build-key")["command"]
    assert gen.count("--fido2-device=auto --fido2-with-client-pin=yes --fido2-with-user-presence=yes") == 2
    assert gen.index("echo TOKEN_OPENS") < gen.index("--wipe-slot=password") < gen.index("unheaded-keygen")
    assert "shred -u /run/custody.tmpkey" in gen


def test_volume_is_closed_after_the_ceremony_and_checked():
    rb = harness.load(RB)
    close = harness.step(rb, "close-and-prove-second-token")["command"]
    assert "echo CLOSED" in close and "SECOND_TOKEN_READS" in close
    assert close.rstrip().splitlines()[-2].count("systemd-cryptsetup detach unheaded-buildkey") == 1
    assert harness.index(rb, "sign-a-test-cask") < harness.index(rb, "close-and-prove-second-token")
    check = next(v for v in rb["verification"] if v.get("action", "shell") == "shell")["command"]
    assert "test ! -e /dev/mapper/unheaded-buildkey" in check


def test_audit_watch_and_backup_copy_are_of_the_encrypted_file():
    rb = harness.load(RB)
    assert f"-w {VOLUME} -p rwa -k unheaded-buildkey" in harness.step(rb, "audit-watch")["command"]
    copy = "\n".join(_code(harness.step(rb, "copy-encrypted-file-to-backup-host")["command"]))
    assert VOLUME in copy and MOUNT not in copy and "ssh ${BACKUP_HOST}" in copy


def test_uid_matches_registry_and_sysusers():
    uids = (harness.REPO / "pkg/uids/uids.go").read_text()
    assert re.search(r"\bBuildKey\s*=\s*16737\b", uids)
    sysusers = (harness.REPO / "deploy/sysusers.d/unheaded.conf").read_text()
    assert re.search(r"^u\s+unheaded-buildkey\s+16737\s", sysusers, re.MULTILINE)
    assert "uid 16737" in harness.load(RB)["notes"]


def test_every_path_named_exists():
    text = (harness.REPO / RB).read_text()
    for rel in set(re.findall(r"(?<![\w/])(?:scripts|deploy|runbooks|configs|cmd|tests)/[\w./-]+\.(?:sh|py|yaml|md|conf|service)\b", text)):
        assert (harness.REPO / rel).is_file(), rel
