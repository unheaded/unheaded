# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/stress-cannon.py builds Monads that Shield accepts (S-TRUE-WIRE Steps 211-214, M-183).

Until 2026-10-28 the cannon packed its own 20-byte layout (flags at the hop-count offset, no
checksum), so every packet it sent was an anomaly by construction and STAT_ANOMALIES under
load measured the cannon, not the data plane. These tests pin the v0x01 layout
(pkg/protocol/bpfschema MonadRegister, foundation-06) and the CRC-16/CCITT-FALSE checksum
over octets 0x00-0x11, big-endian at 0x12, with an independent CRC implementation."""
import importlib.util
import pathlib

import pytest

ROOT = pathlib.Path(__file__).resolve().parents[2]


@pytest.fixture(scope="module")
def cannon():
    spec = importlib.util.spec_from_file_location("stress_cannon", ROOT / "scripts" / "stress-cannon.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def crc16_ccitt_false(data: bytes) -> int:
    """Bit-by-bit reference, written independently of the cannon's table."""
    crc = 0xFFFF
    for byte in data:
        crc ^= byte << 8
        for _ in range(8):
            crc = ((crc << 1) ^ 0x1021) & 0xFFFF if crc & 0x8000 else (crc << 1) & 0xFFFF
    return crc


def test_crc_check_value(cannon):
    # The standard CRC-16/CCITT-FALSE check value (monad_common::CrcCcitt doc test).
    assert cannon.crc16_ccitt(b"123456789") == 0x29B1


def test_layout_follows_v01(cannon):
    m = cannon.build_monad_register(flags=cannon.FLAG_TRACED | cannon.FLAG_SAMPLED, src_svc=0x0A, dst_svc=0x0B,
                                    hop_count=3, seq=0x01020304)
    assert len(m) == 20
    assert m[0x00] == 0x01                      # version
    assert (m[0x01], m[0x02], m[0x03]) == (0x0A, 0x0B, 3)  # src, dst, hop count
    assert m[0x05] == 0x00                      # flow action FORWARD (foundation-06)
    assert m[0x07] == cannon.FLAG_TRACED | cannon.FLAG_SAMPLED  # flags live at 0x07
    assert m[0x07] & cannon.FLAG_CUSTOM == 0    # no CUSTOM: the checksum octets are a real CRC
    assert m[0x0E:0x12] == bytes([1, 2, 3, 4])  # seq in the scratch registers


def test_checksum_verifies(cannon):
    for seq in (0, 1, 0xFFFFFFFF, 12345):
        m = cannon.build_monad_register(seq=seq)
        assert int.from_bytes(m[0x12:0x14], "big") == crc16_ccitt_false(m[:0x12]), seq


def test_bad_crc_does_not_verify(cannon):
    m = cannon.build_monad_register(seq=7, bad_crc=True)
    assert int.from_bytes(m[0x12:0x14], "big") != crc16_ccitt_false(m[:0x12])


def test_hbh_option_carries_the_monad(cannon):
    m = cannon.build_monad_register(seq=9)
    hbh = cannon.build_hbh_option(m)
    assert len(hbh) == 24 and hbh[2] == cannon.MONAD_OPT_TYPE and hbh[3] == 20 and hbh[4:24] == m
