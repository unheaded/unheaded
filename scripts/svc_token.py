#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""svc_token.py: EdDSA service tokens in Python, byte for byte the tokens Go's
pkg/auth mints (ADR-103 §2, §6), for scripts that call Wotan (run-runbook.py).

- header {"alg":"EdDSA","typ":"JWT","kid":"<kid>"}
- claims {"sub":<name>,"iss":"unheaded","aud":<target>,"exp":<iat+300>,
  "iat":<now>,"roles":["service"]}, compact JWS, base64url without padding
- kid = the first 16 hex characters of SHA-256 over the raw public key
- key file = one "UNHEADED ED25519 SEED" PEM block (the 32-byte seed) as
  unheaded-keygen writes it, refused when group or others can access it

Ed25519 comes from the `cryptography` package (pinned in the Zhen venv,
sbom/python-requirements.txt; Debian's python3-cryptography on the hosts).
It is imported only when a key is loaded, so a script that never mints
never needs it. tests/svctoken/crosslang_test.go runs `mint` below and
checks the token against pkg/auth.

  python3 scripts/svc_token.py mint <key-file> <sub> <aud> <iat>
"""

import base64
import binascii
import hashlib
import json
import os
import re
import stat
import sys
import threading
import time

ISSUER = "unheaded"
WOTAN_AUDIENCE = "wotan"
TOKEN_TTL = 300
REFRESH = 30
DEFAULT_KEY_DIR = "/etc/unheaded/keys"
SEED_PEM_TYPE = "UNHEADED ED25519 SEED"
MAX_KEY_FILE_BYTES = 4096
_KEY_NAME_RE = re.compile(r"^[a-z0-9][a-z0-9_-]{0,63}$")


class KeyFileError(Exception):
    """A key file that is there but unusable (mode, format), or no Ed25519."""


def valid_key_name(name):
    """auth.ValidKeyName: [a-z0-9][a-z0-9_-]*, at most 64 bytes."""
    return isinstance(name, str) and _KEY_NAME_RE.fullmatch(name) is not None


def key_id(public_key):
    """auth.KeyID: first 16 hex of SHA-256 over the 32 raw key bytes."""
    return hashlib.sha256(public_key).hexdigest()[:16]


def _b64(data):
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")


def parse_seed_pem(raw, path="<key>"):
    """The 32-byte seed from exactly one UNHEADED ED25519 SEED block."""
    try:
        text = raw.decode("ascii")
    except UnicodeDecodeError:
        raise KeyFileError(f"{path}: not an {SEED_PEM_TYPE} PEM file") from None
    lines = [ln.rstrip() for ln in text.strip().splitlines()]
    if (len(lines) < 3 or lines[0] != f"-----BEGIN {SEED_PEM_TYPE}-----"
            or lines[-1] != f"-----END {SEED_PEM_TYPE}-----"):
        raise KeyFileError(f"{path}: not an {SEED_PEM_TYPE} PEM file")
    body = lines[1:-1]
    if any(":" in ln or "-----" in ln for ln in body):
        raise KeyFileError(f"{path}: PEM headers or a second block")
    try:
        seed = base64.b64decode("".join(body), validate=True)
    except (binascii.Error, ValueError):
        raise KeyFileError(f"{path}: body is not base64") from None
    if len(seed) != 32:
        raise KeyFileError(f"{path}: seed is not 32 bytes")
    return seed


def read_key_file(path):
    """auth.LoadPrivateKey's checks: a regular file group and others cannot
    access, at most 4096 bytes. FileNotFoundError passes through."""
    with open(path, "rb") as f:
        st = os.fstat(f.fileno())
        if not stat.S_ISREG(st.st_mode):
            raise KeyFileError(f"{path}: not a regular file")
        mode = stat.S_IMODE(st.st_mode)
        if mode & 0o077:
            raise KeyFileError(f"{path} is {mode:04o}: group or others can access it (want 0400 or 0600)")
        raw = f.read(MAX_KEY_FILE_BYTES + 1)
    if len(raw) > MAX_KEY_FILE_BYTES:
        raise KeyFileError(f"{path}: larger than {MAX_KEY_FILE_BYTES} bytes")
    return parse_seed_pem(raw, path)


class Minter:
    """auth.ServiceTokenMinter: this service's tokens, cached per audience
    and replaced 30 s before they expire."""

    def __init__(self, name, seed):
        if not valid_key_name(name):
            raise KeyFileError(f"bad key name {name!r}")
        try:
            from cryptography.hazmat.primitives.asymmetric.ed25519 import (
                Ed25519PrivateKey,
            )
            from cryptography.hazmat.primitives.serialization import (
                Encoding,
                PublicFormat,
            )
        except ImportError:
            raise KeyFileError("the cryptography package is not installed (Ed25519)") from None
        self.name = name
        self._key = Ed25519PrivateKey.from_private_bytes(bytes(seed))
        self.public_key = self._key.public_key().public_bytes(Encoding.Raw, PublicFormat.Raw)
        self.kid = key_id(self.public_key)
        self._cache = {}
        self._lock = threading.Lock()

    def __repr__(self):
        return f"Minter(name={self.name!r}, kid={self.kid!r})"

    def mint_at(self, aud, iat):
        """A fresh token for aud issued at iat (Unix seconds). aud is a
        service identity, held to the key-name rules."""
        if not valid_key_name(aud):
            raise KeyFileError(f"bad audience {aud!r}")
        iat = int(iat)
        # Field order and spelling are Go's json.Marshal of JWTHeader and
        # JWTClaims (nbf, extra and act are omitempty and unset).
        header = json.dumps({"alg": "EdDSA", "typ": "JWT", "kid": self.kid}, separators=(",", ":"))
        claims = json.dumps({"sub": self.name, "iss": ISSUER, "aud": aud, "exp": iat + TOKEN_TTL,
                             "iat": iat, "roles": ["service"]}, separators=(",", ":"))
        signing_input = _b64(header.encode()) + "." + _b64(claims.encode())
        return signing_input + "." + _b64(self._key.sign(signing_input.encode()))

    def mint(self, aud, now=None):
        now = int(time.time()) if now is None else int(now)
        with self._lock:
            hit = self._cache.get(aud)
            if hit and now + REFRESH < hit[1]:
                return hit[0]
            tok = self.mint_at(aud, now)
            self._cache[aud] = (tok, now + TOKEN_TTL)
            return tok

    def authorization(self, aud):
        """Bearer <token> for aud."""
        return "Bearer " + self.mint(aud)


def load(name, path):
    """auth.LoadServiceTokenMinter. FileNotFoundError when there is no key."""
    return Minter(name, read_key_file(path))


def main(argv):
    if len(argv) != 5 or argv[0] != "mint":
        print("usage: svc_token.py mint <key-file> <sub> <aud> <iat>", file=sys.stderr)
        return 2
    _, key_file, sub, aud, iat = argv
    try:
        print(load(sub, key_file).mint_at(aud, int(iat)))
    except (OSError, KeyFileError, ValueError) as e:
        print(f"svc_token: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
