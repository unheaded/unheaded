# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/security/build-key-ceremony.yaml (S-IRON-GATE
Step 339). No key is generated and no host is touched: the runbook is
dry-run and its text is checked. The ceremony waits on E1."""
import re

import harness

RB = "runbooks/security/build-key-ceremony.yaml"
ORDER = ["generate-build-key", "audit-watch", "print-public-key", "publish-public-key",
         "sign-a-test-cask", "rotate-quarterly", "revoke"]


def test_dry_run_lists_every_step_in_order():
    names = harness.dry_run(RB)
    assert names == [s["name"] for s in harness.steps(harness.load(RB))]
    assert names == ORDER


def test_draft_until_stevie_approves():
    meta = harness.load(RB)["metadata"]
    assert meta["status"] == "draft" and meta["approved_by"] == ""
    assert meta["risk"] == "critical"
    assert "E1" in harness.load(RB)["notes"]


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
    assert "unheaded-keygen -algo mldsa65 -owner unheaded-buildkey ${KEY_NAME}" in gen


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
    assert "cask-verify verify" in s and "printf 'x' >>" in s
    assert "mktemp -d" in s and 'rm -rf "$D"' in s


def test_rotation_keeps_one_quarter_of_overlap():
    rot = harness.step(harness.load(RB), "rotate-quarterly")["command"]
    assert "build-previous.pubkey" in rot and "--pubkey twice" in rot


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
