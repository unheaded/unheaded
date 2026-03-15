#!/usr/bin/env python3
"""
Zhen Integration Tests — Phase 7
Tests inference server, RAG API, web UI, domain knowledge, and load.
"""
import sys
import time
import json
import requests

INFERENCE_URL = "http://localhost:20100"
RAG_URL = "http://localhost:20103"

PASS = "\033[92mPASS\033[0m"
FAIL = "\033[91mFAIL\033[0m"
SKIP = "\033[93mSKIP\033[0m"

results = []


def record(name, passed, detail="", latency=None):
    status = PASS if passed else FAIL
    lat_str = f" [{latency:.2f}s]" if latency is not None else ""
    print(f"  {status}  {name}{lat_str}  {detail}")
    results.append({"name": name, "passed": passed, "latency": latency, "detail": detail})


def test_inference_models():
    """Test 1: GET /v1/models — verify 200 + model name"""
    try:
        t0 = time.time()
        r = requests.get(f"{INFERENCE_URL}/v1/models", timeout=10)
        lat = time.time() - t0
        data = r.json()
        has_model = "data" in data and len(data["data"]) > 0
        model_name = data["data"][0]["id"] if has_model else "N/A"
        record("1. Inference /v1/models", r.status_code == 200 and has_model,
               f"status={r.status_code} model={model_name}", lat)
    except Exception as e:
        record("1. Inference /v1/models", False, str(e))


def test_inference_completion():
    """Test 2: POST /v1/completions — verify response has choices"""
    try:
        t0 = time.time()
        r = requests.post(f"{INFERENCE_URL}/v1/completions", json={
            "model": "mistral",
            "prompt": "Hello, my name is",
            "max_tokens": 32,
            "temperature": 0.1,
        }, timeout=60)
        lat = time.time() - t0
        data = r.json()
        has_choices = "choices" in data and len(data["choices"]) > 0
        preview = data["choices"][0]["text"][:60].replace("\n", " ") if has_choices else "N/A"
        record("2. Inference completion", r.status_code == 200 and has_choices,
               f"preview='{preview}'", lat)
    except Exception as e:
        record("2. Inference completion", False, str(e))


def test_rag_health():
    """Test 3: GET /health — verify rag_ready=true"""
    try:
        t0 = time.time()
        r = requests.get(f"{RAG_URL}/health", timeout=10)
        lat = time.time() - t0
        data = r.json()
        record("3. RAG /health", data.get("rag_ready") is True,
               f"rag_ready={data.get('rag_ready')}", lat)
    except Exception as e:
        record("3. RAG /health", False, str(e))


def test_rag_query():
    """Test 4: POST /api/v1/query 'What is Unheaded?' — answer + sources"""
    try:
        t0 = time.time()
        r = requests.post(f"{RAG_URL}/api/v1/query", json={
            "question": "What is Unheaded?"
        }, timeout=120)
        lat = time.time() - t0
        data = r.json()
        has_answer = bool(data.get("answer", "").strip())
        has_sources = len(data.get("sources", [])) > 0
        preview = data.get("answer", "")[:80].replace("\n", " ")
        record("4. RAG query", r.status_code == 200 and has_answer and has_sources,
               f"answer='{preview}...' sources={len(data.get('sources', []))}", lat)
    except Exception as e:
        record("4. RAG query", False, str(e))


def test_rag_search():
    """Test 5: POST /api/v1/search 'eBPF tracing' — results returned"""
    try:
        t0 = time.time()
        r = requests.post(f"{RAG_URL}/api/v1/search", json={
            "query": "eBPF tracing",
            "k": 5,
        }, timeout=30)
        lat = time.time() - t0
        data = r.json()
        has_results = data.get("total", 0) > 0
        record("5. RAG search", r.status_code == 200 and has_results,
               f"total={data.get('total', 0)}", lat)
    except Exception as e:
        record("5. RAG search", False, str(e))


def test_web_ui():
    """Test 6: GET / — HTML contains 'ZHEN'"""
    try:
        t0 = time.time()
        r = requests.get(f"{RAG_URL}/", timeout=10)
        lat = time.time() - t0
        has_zhen = "ZHEN" in r.text
        record("6. Web UI", r.status_code == 200 and has_zhen,
               f"status={r.status_code} contains_ZHEN={has_zhen}", lat)
    except Exception as e:
        record("6. Web UI", False, str(e))


def test_stats():
    """Test 7: GET /api/v1/stats — index_vectors > 20000"""
    try:
        t0 = time.time()
        r = requests.get(f"{RAG_URL}/api/v1/stats", timeout=10)
        lat = time.time() - t0
        data = r.json()
        vectors = data.get("index_vectors", 0)
        record("7. Stats", r.status_code == 200 and vectors > 20000,
               f"index_vectors={vectors} dimension={data.get('index_dimension')}", lat)
    except Exception as e:
        record("7. Stats", False, str(e))


def test_load():
    """Test 8: Send 3 sequential queries, verify all succeed, report avg latency"""
    queries = [
        "What services does Unheaded provide?",
        "How does Wotan work?",
        "What is the Doom Range?",
    ]
    latencies = []
    all_ok = True
    for i, q in enumerate(queries):
        try:
            t0 = time.time()
            r = requests.post(f"{RAG_URL}/api/v1/query", json={"question": q}, timeout=120)
            lat = time.time() - t0
            latencies.append(lat)
            data = r.json()
            if r.status_code != 200 or not data.get("answer"):
                all_ok = False
        except Exception as e:
            all_ok = False
            latencies.append(0)

    avg_lat = sum(latencies) / len(latencies) if latencies else 0
    record("8. Load test (3 queries)", all_ok,
           f"avg_latency={avg_lat:.2f}s latencies={[round(l, 2) for l in latencies]}", avg_lat)


def test_domain_wotan_port():
    """Test 9: Query 'What port does wotan use?' — answer mentions 18000 or 18001"""
    try:
        t0 = time.time()
        r = requests.post(f"{RAG_URL}/api/v1/query", json={
            "question": "What port does wotan use?"
        }, timeout=120)
        lat = time.time() - t0
        data = r.json()
        answer = data.get("answer", "")
        has_port = "18000" in answer or "18001" in answer
        preview = answer[:100].replace("\n", " ")
        record("9. Domain: Wotan port", r.status_code == 200 and has_port,
               f"mentions_18000/18001={has_port} answer='{preview}...'", lat)
    except Exception as e:
        record("9. Domain: Wotan port", False, str(e))


def test_domain_monad_wire():
    """Test 10: Query 'What is the Monad wire format size?' — answer mentions 20 bytes"""
    try:
        t0 = time.time()
        r = requests.post(f"{RAG_URL}/api/v1/query", json={
            "question": "What is the Monad wire format size?"
        }, timeout=120)
        lat = time.time() - t0
        data = r.json()
        answer = data.get("answer", "")
        has_20 = "20" in answer and ("byte" in answer.lower() or "bytes" in answer.lower())
        preview = answer[:100].replace("\n", " ")
        record("10. Domain: Monad 20 bytes", r.status_code == 200 and has_20,
               f"mentions_20_bytes={has_20} answer='{preview}...'", lat)
    except Exception as e:
        record("10. Domain: Monad 20 bytes", False, str(e))


def main():
    print("=" * 70)
    print("  ZHEN INTEGRATION TESTS — Phase 7")
    print(f"  Inference: {INFERENCE_URL}")
    print(f"  RAG/UI:    {RAG_URL}")
    print(f"  Time:      {time.strftime('%Y-%m-%d %H:%M:%S')}")
    print("=" * 70)
    print()

    test_inference_models()
    test_inference_completion()
    test_rag_health()
    test_rag_query()
    test_rag_search()
    test_web_ui()
    test_stats()
    test_load()
    test_domain_wotan_port()
    test_domain_monad_wire()

    print()
    print("=" * 70)
    passed = sum(1 for r in results if r["passed"])
    failed = sum(1 for r in results if not r["passed"])
    latencies = [r["latency"] for r in results if r["latency"] is not None]
    avg_lat = sum(latencies) / len(latencies) if latencies else 0

    print(f"  RESULTS: {passed}/{len(results)} passed, {failed} failed")
    print(f"  AVG LATENCY: {avg_lat:.2f}s")
    if latencies:
        print(f"  MIN LATENCY: {min(latencies):.2f}s")
        print(f"  MAX LATENCY: {max(latencies):.2f}s")
    print("=" * 70)

    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
