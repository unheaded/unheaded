# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/upc-mbc-patch-opcode.py rewrites one opcode in a copy of an MBC image (S-STAGE-2C Phase 4).

The tool makes the hostile image that proves an atomic opcode goes through
translate_address, and that bootctl's opcode scan refuses it. These tests pin
what it changes (one opcode byte, after the marker) and what it never does
(touch its input, write in place)."""
import pathlib
import struct
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "upc-mbc-patch-opcode.py"

LD, XCHG, MOV = 0x30, 0x3D, 0x20
MARKER = 0x00800400


def insn(opcode, dst, src, imm):
    return (opcode << 24) | (dst << 20) | (src << 16) | imm


# An LD before the marker, the marker literal, a MOV, then two LDs.
IMAGE = [insn(LD, 1, 2, 0x0010), MARKER, insn(MOV, 3, 4, 0), insn(LD, 5, 6, 0x0004), insn(LD, 7, 8, 0x0008)]


def write(path, words):
    path.write_bytes(struct.pack(f"<{len(words)}I", *words))


def read(path):
    raw = path.read_bytes()
    return list(struct.unpack(f"<{len(raw) // 4}I", raw))


def run(*args):
    return subprocess.run([sys.executable, str(SCRIPT), *map(str, args)], capture_output=True, text=True, check=False)


def test_the_first_matching_instruction_after_the_marker_changes(tmp_path):
    src, out = tmp_path / "probe.mbc", tmp_path / "probe_x.mbc"
    write(src, IMAGE)
    r = run(src, "--word-of-marker", hex(MARKER), "--from", hex(LD), "--to", hex(XCHG), "--out", out)
    assert r.returncode == 0, r.stderr
    got = read(out)
    assert got[3] == insn(XCHG, 5, 6, 0x0004)  # opcode changed; dst, src and imm16 kept
    assert got[:3] == IMAGE[:3] and got[4] == IMAGE[4]  # the LD before the marker and the later one stay
    assert "word 3" in r.stdout


def test_a_masked_marker_matches_an_instruction_whatever_its_registers(tmp_path):
    addi = 0x1D
    image = [insn(LD, 1, 2, 0x0400), insn(addi, 13, 0, 0x0400), insn(LD, 13, 13, 0), insn(LD, 9, 9, 0)]
    src, out = tmp_path / "probe.mbc", tmp_path / "probe_x.mbc"
    write(src, image)
    r = run(src, "--word-of-marker", "0x1D000400", "--marker-mask", "0xFF00FFFF",
            "--from", hex(LD), "--to", hex(XCHG), "--out", out)
    assert r.returncode == 0, r.stderr
    got = read(out)
    assert got[2] == insn(XCHG, 13, 13, 0)
    assert got[0] == image[0] and got[1] == image[1] and got[3] == image[3]


def test_the_input_is_never_changed(tmp_path):
    src, out = tmp_path / "probe.mbc", tmp_path / "probe_x.mbc"
    write(src, IMAGE)
    run(src, "--word-of-marker", hex(MARKER), "--from", hex(LD), "--to", hex(XCHG), "--out", out)
    assert read(src) == IMAGE


def test_writing_in_place_is_refused(tmp_path):
    src = tmp_path / "probe.mbc"
    write(src, IMAGE)
    r = run(src, "--word-of-marker", hex(MARKER), "--from", hex(LD), "--to", hex(XCHG), "--out", src)
    assert r.returncode == 2
    assert read(src) == IMAGE


def test_a_missing_marker_is_an_error_and_writes_nothing(tmp_path):
    src, out = tmp_path / "probe.mbc", tmp_path / "probe_x.mbc"
    write(src, IMAGE)
    r = run(src, "--word-of-marker", "0x12345678", "--from", hex(LD), "--to", hex(XCHG), "--out", out)
    assert r.returncode == 1
    assert "marker" in r.stderr
    assert not out.exists()


def test_no_instruction_after_the_marker_is_an_error(tmp_path):
    src, out = tmp_path / "probe.mbc", tmp_path / "probe_x.mbc"
    write(src, IMAGE[:3])  # the only LD is before the marker
    r = run(src, "--word-of-marker", hex(MARKER), "--from", hex(LD), "--to", hex(XCHG), "--out", out)
    assert r.returncode == 1
    assert not out.exists()


def test_a_ragged_image_is_refused(tmp_path):
    src, out = tmp_path / "probe.mbc", tmp_path / "probe_x.mbc"
    src.write_bytes(b"\x00" * 6)
    r = run(src, "--word-of-marker", hex(MARKER), "--from", hex(LD), "--to", hex(XCHG), "--out", out)
    assert r.returncode == 1
    assert not out.exists()
