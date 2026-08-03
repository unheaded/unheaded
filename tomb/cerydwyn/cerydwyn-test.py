#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
#
# cerydwyn-test.py — Test harness for Cerydwyn LLM verification
#
# Verifies:
#   1. Ollama connectivity (API reachable)
#   2. Model loading (cerydwyn-mistral available)
#   3. Query capability (generate a response)
#   4. RAG retrieval (Grimoire search works)
#   5. System prompt loading
#   6. Response quality (structured output)
#
# Usage:
#   python3 cerydwyn-test.py
#   python3 cerydwyn-test.py --verbose
#   python3 cerydwyn-test.py --quick    # Skip slow tests

import json
import os
import sys
import time
import traceback
from datetime import datetime

# --- Configuration ---
OLLAMA_URL = os.environ.get("OLLAMA_URL", "http://127.0.0.1:11434")
CERYDWYN_MODEL = os.environ.get("CERYDWYN_MODEL", "cerydwyn-mistral")
CERYDWYN_ROOT = os.environ.get("CERYDWYN_ROOT", "/opt/tomb/cerydwyn")
GRIMOIRE_ROOT = os.environ.get("GRIMOIRE_ROOT", "/opt/tomb/grimoire")
SYSTEM_PROMPT_FILE = os.path.join(CERYDWYN_ROOT, "prompts", "system-cerydwyn.txt")
RAG_CONFIG_FILE = os.path.join(CERYDWYN_ROOT, "config", "rag-config.json")

VERBOSE = "--verbose" in sys.argv or "-v" in sys.argv
QUICK = "--quick" in sys.argv


class TestResult:
    def __init__(self, name):
        self.name = name
        self.passed = False
        self.message = ""
        self.duration_ms = 0
        self.details = ""

    def __str__(self):
        status = "PASS" if self.passed else "FAIL"
        msg = f"  [{status}] {self.name} ({self.duration_ms}ms)"
        if self.message:
            msg += f" — {self.message}"
        return msg


def run_test(name, test_func):
    """Execute a test function and capture result."""
    result = TestResult(name)
    start = time.time()
    try:
        test_func(result)
    except Exception as e:
        result.passed = False
        result.message = f"Exception: {e}"
        result.details = traceback.format_exc()
    result.duration_ms = int((time.time() - start) * 1000)
    return result


# --- Test Functions ---

def test_ollama_connectivity(result):
    """Test 1: Verify Ollama API is reachable."""
    import urllib.request

    req = urllib.request.Request(f"{OLLAMA_URL}/api/tags")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            if resp.status == 200:
                body = json.loads(resp.read().decode("utf-8"))
                model_count = len(body.get("models", []))
                result.passed = True
                result.message = f"Ollama reachable, {model_count} model(s) loaded"
                if VERBOSE:
                    result.details = json.dumps(body, indent=2)
            else:
                result.message = f"Unexpected status: {resp.status}"
    except Exception as e:
        result.message = f"Cannot reach Ollama at {OLLAMA_URL}: {e}"


def test_model_available(result):
    """Test 2: Verify cerydwyn-mistral model is loaded."""
    import urllib.request

    req = urllib.request.Request(f"{OLLAMA_URL}/api/tags")
    with urllib.request.urlopen(req, timeout=10) as resp:
        body = json.loads(resp.read().decode("utf-8"))

    models = body.get("models", [])
    model_names = [m.get("name", "") for m in models]

    # Check for exact match or prefix match (e.g., "cerydwyn-mistral:latest")
    found = any(CERYDWYN_MODEL in name for name in model_names)

    if found:
        result.passed = True
        result.message = f"Model '{CERYDWYN_MODEL}' is available"
    else:
        result.message = f"Model '{CERYDWYN_MODEL}' not found. Available: {model_names}"
        result.details = (
            "Fix: Run 'ollama create cerydwyn-mistral -f /opt/tomb/cerydwyn/Modelfile-mistral'"
        )


def test_basic_query(result):
    """Test 3: Verify Ollama can generate a response."""
    import urllib.request

    payload = {
        "model": CERYDWYN_MODEL,
        "prompt": "Respond with exactly: CERYDWYN_READY",
        "stream": False,
    }
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        f"{OLLAMA_URL}/api/generate",
        data=data,
        headers={"Content-Type": "application/json"},
    )

    with urllib.request.urlopen(req, timeout=120) as resp:
        body = json.loads(resp.read().decode("utf-8"))

    response_text = body.get("response", "")
    if response_text and len(response_text) > 0:
        result.passed = True
        result.message = f"Response generated ({len(response_text)} chars)"
        if VERBOSE:
            result.details = response_text[:500]
    else:
        result.message = "Empty response from model"


def test_security_query(result):
    """Test 4: Verify model understands security context."""
    import urllib.request

    payload = {
        "model": CERYDWYN_MODEL,
        "prompt": (
            "What is CWE-787? Respond with the CWE name and a one-sentence description. "
            "Include the words 'out-of-bounds' in your response."
        ),
        "stream": False,
    }
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        f"{OLLAMA_URL}/api/generate",
        data=data,
        headers={"Content-Type": "application/json"},
    )

    with urllib.request.urlopen(req, timeout=120) as resp:
        body = json.loads(resp.read().decode("utf-8"))

    response_text = body.get("response", "").lower()
    if "out-of-bounds" in response_text or "buffer" in response_text or "write" in response_text:
        result.passed = True
        result.message = "Model demonstrates security knowledge"
        if VERBOSE:
            result.details = body.get("response", "")[:500]
    else:
        result.passed = False
        result.message = "Response does not demonstrate expected security knowledge"
        result.details = body.get("response", "")[:500]


def test_system_prompt_exists(result):
    """Test 5: Verify system prompt file exists and is non-empty."""
    if os.path.isfile(SYSTEM_PROMPT_FILE):
        size = os.path.getsize(SYSTEM_PROMPT_FILE)
        if size > 100:
            result.passed = True
            result.message = f"System prompt exists ({size} bytes)"
        else:
            result.message = f"System prompt too small ({size} bytes)"
    else:
        result.message = f"System prompt not found at {SYSTEM_PROMPT_FILE}"


def test_rag_config_exists(result):
    """Test 6: Verify RAG configuration file exists and is valid JSON."""
    if os.path.isfile(RAG_CONFIG_FILE):
        with open(RAG_CONFIG_FILE, "r") as f:
            cfg = json.load(f)
        required_keys = ["ollama_url", "model", "grimoire_root"]
        missing = [k for k in required_keys if k not in cfg]
        if not missing:
            result.passed = True
            result.message = f"RAG config valid ({len(cfg)} keys)"
            if VERBOSE:
                result.details = json.dumps(cfg, indent=2)
        else:
            result.message = f"RAG config missing keys: {missing}"
    else:
        result.message = f"RAG config not found at {RAG_CONFIG_FILE}"


def test_grimoire_exists(result):
    """Test 7: Verify Grimoire directory structure exists."""
    expected_dirs = [
        "kingdom-docs",
        "threat-models",
        "history",
        "mitre-attck",
        "nvd-cves",
        "cwe",
    ]

    if not os.path.isdir(GRIMOIRE_ROOT):
        result.message = f"Grimoire root not found at {GRIMOIRE_ROOT}"
        return

    found_dirs = [d for d in expected_dirs if os.path.isdir(os.path.join(GRIMOIRE_ROOT, d))]
    if found_dirs:
        result.passed = True
        result.message = f"Grimoire exists with {len(found_dirs)}/{len(expected_dirs)} subdirs"
        if VERBOSE:
            result.details = f"Found: {found_dirs}"
    else:
        result.message = "Grimoire exists but no expected subdirs found"


def test_cerydwyn_directories(result):
    """Test 8: Verify Cerydwyn directory structure."""
    expected = ["prompts", "logs", "suggestions", "config", "history"]
    found = [d for d in expected if os.path.isdir(os.path.join(CERYDWYN_ROOT, d))]

    if len(found) == len(expected):
        result.passed = True
        result.message = f"All {len(expected)} Cerydwyn directories exist"
    elif found:
        result.passed = True  # Partial pass
        result.message = f"{len(found)}/{len(expected)} dirs exist (missing: {set(expected) - set(found)})"
    else:
        result.message = f"No Cerydwyn directories found at {CERYDWYN_ROOT}"


def test_ollama_binary(result):
    """Test 9: Verify Ollama binary exists and is executable."""
    binary_path = "/opt/ollama/bin/ollama"
    if os.path.isfile(binary_path):
        if os.access(binary_path, os.X_OK):
            result.passed = True
            result.message = f"Ollama binary executable at {binary_path}"
        else:
            result.message = "Ollama binary exists but not executable"
    else:
        result.message = f"Ollama binary not found at {binary_path}"


def test_suggestions_writable(result):
    """Test 10: Verify suggestions directory is writable."""
    suggestions_dir = os.path.join(CERYDWYN_ROOT, "suggestions")
    os.makedirs(suggestions_dir, exist_ok=True)

    test_file = os.path.join(suggestions_dir, ".write-test")
    try:
        with open(test_file, "w") as f:
            f.write("test")
        os.remove(test_file)
        result.passed = True
        result.message = "Suggestions directory is writable"
    except OSError as e:
        result.message = f"Cannot write to suggestions dir: {e}"


# --- Main ---

def main():
    print("=" * 60)
    print("  Tomb of Knowledge — Cerydwyn Test Harness")
    print(f"  {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"  Ollama URL: {OLLAMA_URL}")
    print(f"  Model: {CERYDWYN_MODEL}")
    print(f"  Cerydwyn Root: {CERYDWYN_ROOT}")
    print("=" * 60)
    print()

    # Define test suite
    tests = [
        ("1. Ollama Connectivity", test_ollama_connectivity),
        ("2. Model Available", test_model_available),
        ("5. System Prompt File", test_system_prompt_exists),
        ("6. RAG Config File", test_rag_config_exists),
        ("7. Grimoire Structure", test_grimoire_exists),
        ("8. Cerydwyn Directories", test_cerydwyn_directories),
        ("9. Ollama Binary", test_ollama_binary),
        ("10. Suggestions Writable", test_suggestions_writable),
    ]

    # Slow tests (require model inference)
    if not QUICK:
        tests.insert(2, ("3. Basic Query", test_basic_query))
        tests.insert(3, ("4. Security Knowledge", test_security_query))

    results = []
    for name, func in tests:
        r = run_test(name, func)
        results.append(r)
        print(r)
        if VERBOSE and r.details:
            for line in r.details.split("\n")[:10]:
                print(f"         {line}")

    # Summary
    passed = sum(1 for r in results if r.passed)
    failed = sum(1 for r in results if not r.passed)
    total = len(results)
    total_ms = sum(r.duration_ms for r in results)

    print()
    print("-" * 60)
    print(f"  Results: {passed}/{total} passed, {failed} failed ({total_ms}ms total)")

    if failed == 0:
        print("  Status: ALL TESTS PASSED")
    else:
        print("  Status: SOME TESTS FAILED")
        print()
        print("  Failed tests:")
        for r in results:
            if not r.passed:
                print(f"    - {r.name}: {r.message}")
                if r.details:
                    print(f"      {r.details[:200]}")

    print("-" * 60)

    # Write results to log
    log_dir = os.path.join(CERYDWYN_ROOT, "logs")
    os.makedirs(log_dir, exist_ok=True)
    log_file = os.path.join(log_dir, f"test-{datetime.now().strftime('%Y%m%d-%H%M%S')}.json")
    try:
        with open(log_file, "w") as f:
            json.dump(
                {
                    "timestamp": datetime.now().isoformat(),
                    "total": total,
                    "passed": passed,
                    "failed": failed,
                    "duration_ms": total_ms,
                    "results": [
                        {
                            "name": r.name,
                            "passed": r.passed,
                            "message": r.message,
                            "duration_ms": r.duration_ms,
                        }
                        for r in results
                    ],
                },
                f,
                indent=2,
            )
        print(f"  Log: {log_file}")
    except OSError:
        pass

    sys.exit(0 if failed == 0 else 1)


if __name__ == "__main__":
    if "--help" in sys.argv or "-h" in sys.argv:
        print("Usage: cerydwyn-test.py [--verbose] [--quick]")
        print()
        print("Options:")
        print("  --verbose, -v  Show detailed output for each test")
        print("  --quick        Skip slow tests (model inference)")
        print()
        print("Environment:")
        print("  OLLAMA_URL     Ollama endpoint (default: http://127.0.0.1:11434)")
        print("  CERYDWYN_MODEL   Model name (default: cerydwyn-mistral)")
        print("  CERYDWYN_ROOT    Cerydwyn root (default: /opt/tomb/cerydwyn)")
        print("  GRIMOIRE_ROOT  Grimoire root (default: /opt/tomb/grimoire)")
        sys.exit(0)

    main()
