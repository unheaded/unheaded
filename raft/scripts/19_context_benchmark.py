#!/usr/bin/env python3
"""
Zhen Scientist Experiment: Optimal Mistral-7B Context Window

Hypothesis: Mistral-7B-Instruct supports up to 32K context (sliding window attention).
Current -c 2048 is conservative. We test -c 2048/4096/8192/16384/32768 to find
the optimal value for WEST's hardware (AMD RX 7700 XT, 16GB DDR5).

Independent variable: llama-server -c flag (context window size)
Dependent variables: generation speed (tok/s), VRAM usage, first-token latency, response quality

Output: raft/experiments/context_window_benchmark.json
"""
import json
import os
import signal
import subprocess
import time
from pathlib import Path

import requests

# ─── Config ──────────────────────────────────────────────────────────────────

LLAMA_BUILD = Path.home() / 'tmp' / 'unheaded' / 'llama.cpp' / 'build'
MODEL_PATH = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'models' / 'mistral-7b-instruct-q5_k_m.gguf'
OUTPUT_DIR = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'experiments'
INFERENCE_URL = 'http://localhost:20100'
PORT = 20100

CONTEXT_VALUES = [2048, 4096, 8192, 16384]

# Test queries: short (~500 tok), medium (~2000 tok), long (~4000 tok)
TEST_QUERIES = [
    {
        'name': 'short_1',
        'category': 'short',
        'prompt': '<s>[INST] What is Wotan? Answer in 2-3 sentences. [/INST]',
    },
    {
        'name': 'short_2',
        'category': 'short',
        'prompt': '<s>[INST] What protocol does Monad use? Answer in 2-3 sentences. [/INST]',
    },
    {
        'name': 'medium_1',
        'category': 'medium',
        'prompt': '<s>[INST] You are an expert systems architect. Explain a doom-ring architecture for message passing in distributed systems. Cover: ring topology, fault tolerance, message ordering, and typical use cases. Be thorough but concise. [/INST]',
    },
    {
        'name': 'medium_2',
        'category': 'medium',
        'prompt': '<s>[INST] You are a security expert. Explain how an authentication framework works in a microservices architecture. Cover: JWT tokens, RBAC, API keys, middleware patterns, token refresh, and session management. Provide concrete examples. [/INST]',
    },
    {
        'name': 'long_1',
        'category': 'long',
        'prompt': '<s>[INST] You are an expert on distributed systems and networking.\n\n'
                  'Context document 1: IPv6 (Internet Protocol version 6) is the most recent version of the Internet Protocol, '
                  'designed to replace IPv4. IPv6 uses 128-bit addresses, allowing for approximately 3.4×10^38 unique addresses. '
                  'The protocol was developed by the Internet Engineering Task Force (IETF) and first published in RFC 2460 in 1998. '
                  'Key features include simplified header format, improved support for extensions and options, flow labeling capability, '
                  'and authentication/privacy capabilities. IPv6 addresses are represented as eight groups of four hexadecimal digits.\n\n'
                  'Context document 2: eBPF (extended Berkeley Packet Filter) is a technology that allows running sandboxed programs '
                  'in the Linux kernel without changing kernel source code or loading kernel modules. eBPF programs are event-driven '
                  'and run when the kernel or application passes a certain hook point. Pre-defined hooks include system calls, function '
                  'entry/exit, kernel tracepoints, and network events. eBPF has been used for networking, observability, tracing, '
                  'and security. The verifier ensures all eBPF programs are safe to run.\n\n'
                  'Context document 3: gRPC is an open-source remote procedure call framework developed by Google. It uses HTTP/2 '
                  'for transport, Protocol Buffers as the interface description language, and provides features such as authentication, '
                  'bidirectional streaming, flow control, and blocking/nonblocking bindings. gRPC supports multiple programming languages '
                  'and is widely used in microservices architectures for inter-service communication.\n\n'
                  'Using all three context documents, explain how you would design an observability system that combines IPv6 flow labels '
                  'with eBPF tracing and gRPC streaming to achieve real-time network monitoring across a cluster of 100 nodes. [/INST]',
    },
    {
        'name': 'long_2',
        'category': 'long',
        'prompt': '<s>[INST] You are a systems programming expert.\n\n'
                  'Reference 1: The Linux kernel scheduler (CFS - Completely Fair Scheduler) uses a red-black tree to manage process '
                  'scheduling. Each process is assigned a virtual runtime (vruntime) that tracks how much CPU time it has received. '
                  'The scheduler always picks the process with the smallest vruntime. CFS supports scheduling classes including SCHED_NORMAL, '
                  'SCHED_BATCH, SCHED_IDLE, and SCHED_DEADLINE. The O(1) scheduler predecessor was replaced by CFS in kernel 2.6.23.\n\n'
                  'Reference 2: Memory-mapped I/O (MMIO) allows hardware devices to be accessed through the same address space as normal '
                  'memory. In contrast to port-mapped I/O, MMIO uses the same instructions for accessing both memory and devices. This '
                  'simplifies programming but requires careful management of caching and ordering. DMA (Direct Memory Access) controllers '
                  'can transfer data between devices and memory without CPU intervention.\n\n'
                  'Reference 3: The Rust ownership model ensures memory safety without garbage collection. Each value has a single owner, '
                  'and when the owner goes out of scope, the value is dropped. Borrowing rules allow either one mutable reference or '
                  'any number of immutable references at a time. The borrow checker enforces these rules at compile time.\n\n'
                  'Design a zero-copy network packet processing pipeline in Rust that uses AF_XDP sockets, interacts with kernel scheduling '
                  'for priority management, and achieves at least 1M packets/sec on commodity hardware. Address memory safety, buffer '
                  'management, and kernel bypass considerations. [/INST]',
    },
]


def kill_existing_llama():
    """Kill any running llama-server on our port."""
    try:
        result = subprocess.run(['lsof', '-ti', f':{PORT}'], capture_output=True, text=True)
        if result.stdout.strip():
            for pid in result.stdout.strip().split('\n'):
                try:
                    os.kill(int(pid), signal.SIGTERM)
                except (ProcessLookupError, ValueError):
                    pass
            time.sleep(2)
    except Exception:
        pass


def start_llama_server(context_size):
    """Start llama-server with given context size. Returns subprocess."""
    kill_existing_llama()
    time.sleep(1)

    env = os.environ.copy()
    ld_paths = [
        str(LLAMA_BUILD / 'src'),
        str(LLAMA_BUILD / 'ggml' / 'src'),
        str(LLAMA_BUILD / 'ggml' / 'src' / 'ggml-hip'),
        str(LLAMA_BUILD / 'ggml' / 'src' / 'ggml-cpu'),
    ]
    env['LD_LIBRARY_PATH'] = ':'.join(ld_paths) + ':' + env.get('LD_LIBRARY_PATH', '')

    proc = subprocess.Popen(
        [
            str(LLAMA_BUILD / 'bin' / 'llama-server'),
            '-m', str(MODEL_PATH),
            '-ngl', '40',
            '-c', str(context_size),
            '--port', str(PORT),
        ],
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )

    # Wait for model to load (up to 60s)
    for _ in range(30):
        try:
            r = requests.get(f'{INFERENCE_URL}/v1/models', timeout=3)
            if r.status_code == 200:
                return proc
        except Exception:
            pass
        time.sleep(2)

    proc.terminate()
    raise RuntimeError(f"llama-server failed to start with -c {context_size}")


def get_vram_usage():
    """Get VRAM usage from ROCm (amdgpu)."""
    try:
        result = subprocess.run(
            ['rocm-smi', '--showmeminfo', 'vram', '--json'],
            capture_output=True, text=True, timeout=5,
        )
        if result.returncode == 0:
            data = json.loads(result.stdout)
            # Parse rocm-smi JSON output
            for card_id, card_data in data.items():
                if isinstance(card_data, dict):
                    used = card_data.get('VRAM Total Used Memory (B)', 0)
                    total = card_data.get('VRAM Total Memory (B)', 0)
                    if used and total:
                        return {
                            'used_mb': int(used) / (1024 * 1024),
                            'total_mb': int(total) / (1024 * 1024),
                        }
    except Exception:
        pass
    # Fallback: try /sys
    try:
        used = int(open('/sys/class/drm/card1/device/mem_info_vram_used').read().strip())
        total = int(open('/sys/class/drm/card1/device/mem_info_vram_total').read().strip())
        return {'used_mb': used / (1024 * 1024), 'total_mb': total / (1024 * 1024)}
    except Exception:
        return {'used_mb': 0, 'total_mb': 0}


def run_query(prompt, max_tokens=200):
    """Run a single inference query, return timing and response."""
    t0 = time.time()
    try:
        r = requests.post(
            f'{INFERENCE_URL}/v1/completions',
            json={
                'prompt': prompt,
                'max_tokens': max_tokens,
                'temperature': 0.3,
                'top_p': 0.9,
                'stop': ['[INST]', '</s>'],
            },
            timeout=120,
        )
        elapsed = time.time() - t0

        if r.status_code != 200:
            return {
                'status': 'error',
                'status_code': r.status_code,
                'elapsed': elapsed,
                'error': r.text[:200],
            }

        data = r.json()
        answer = data['choices'][0]['text'].strip()
        usage = data.get('usage', {})
        tokens_out = usage.get('completion_tokens', 0)
        tokens_in = usage.get('prompt_tokens', 0)

        return {
            'status': 'ok',
            'answer': answer,
            'tokens_input': tokens_in,
            'tokens_output': tokens_out,
            'tokens_per_sec': tokens_out / elapsed if elapsed > 0 else 0,
            'elapsed': elapsed,
            'first_token_latency': elapsed - (tokens_out * (elapsed / max(tokens_out, 1))),
        }
    except requests.exceptions.Timeout:
        return {'status': 'timeout', 'elapsed': time.time() - t0}
    except Exception as e:
        return {'status': 'error', 'elapsed': time.time() - t0, 'error': str(e)}


def run_benchmark():
    """Run the full context window benchmark."""
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    results = {
        'experiment': 'context_window_benchmark',
        'model': 'mistral-7b-instruct-q5_k_m',
        'gpu': 'AMD RX 7700 XT',
        'timestamp': time.strftime('%Y-%m-%dT%H:%M:%S'),
        'context_values': CONTEXT_VALUES,
        'runs': [],
    }

    for ctx_size in CONTEXT_VALUES:
        print(f"\n{'='*60}")
        print(f"Testing context size: {ctx_size}")
        print(f"{'='*60}")

        run_data = {
            'context_size': ctx_size,
            'vram_before': get_vram_usage(),
            'queries': [],
            'server_started': False,
        }

        try:
            proc = start_llama_server(ctx_size)
            run_data['server_started'] = True
            run_data['vram_after_load'] = get_vram_usage()

            for tq in TEST_QUERIES:
                print(f"  Running: {tq['name']} ({tq['category']})...")
                result = run_query(tq['prompt'])
                result['name'] = tq['name']
                result['category'] = tq['category']
                run_data['queries'].append(result)

                status = result['status']
                if status == 'ok':
                    print(f"    {result['tokens_per_sec']:.1f} tok/s | {result['elapsed']:.1f}s | {result['tokens_output']} tokens")
                else:
                    print(f"    {status}: {result.get('error', 'unknown')[:80]}")

            proc.terminate()
            proc.wait(timeout=10)

        except Exception as e:
            run_data['error'] = str(e)
            print(f"  ERROR: {e}")

        results['runs'].append(run_data)

        # Save partial results after each context size
        partial_file = OUTPUT_DIR / 'context_window_benchmark_partial.json'
        with open(partial_file, 'w') as f:
            json.dump(results, f, indent=2)

    # Analyze: find optimal context size
    best = analyze_results(results)
    results['recommendation'] = best

    # Save
    output_file = OUTPUT_DIR / 'context_window_benchmark.json'
    with open(output_file, 'w') as f:
        json.dump(results, f, indent=2)

    print(f"\n{'='*60}")
    print(f"Results saved to: {output_file}")
    print(f"Recommendation: -c {best['context_size']}")
    print(f"Reason: {best['reason']}")
    print(f"{'='*60}")

    return results


def analyze_results(results):
    """Analyze benchmark results and recommend optimal context size."""
    summaries = []

    for run in results['runs']:
        if not run['server_started']:
            continue

        ok_queries = [q for q in run['queries'] if q['status'] == 'ok']
        if not ok_queries:
            summaries.append({
                'context_size': run['context_size'],
                'avg_tok_s': 0,
                'success_rate': 0,
                'avg_elapsed': 0,
            })
            continue

        avg_tok_s = sum(q['tokens_per_sec'] for q in ok_queries) / len(ok_queries)
        avg_elapsed = sum(q['elapsed'] for q in ok_queries) / len(ok_queries)
        success_rate = len(ok_queries) / len(run['queries'])
        vram_used = run.get('vram_after_load', {}).get('used_mb', 0)

        summaries.append({
            'context_size': run['context_size'],
            'avg_tok_s': avg_tok_s,
            'success_rate': success_rate,
            'avg_elapsed': avg_elapsed,
            'vram_mb': vram_used,
            'queries_ok': len(ok_queries),
            'queries_total': len(run['queries']),
        })

    if not summaries:
        return {'context_size': 2048, 'reason': 'No successful runs — keeping default'}

    # Filter to runs with >80% success rate
    viable = [s for s in summaries if s['success_rate'] >= 0.8]
    if not viable:
        viable = summaries

    # Pick the largest context size that maintains >90% of baseline speed
    baseline_speed = next((s['avg_tok_s'] for s in viable if s['context_size'] == 2048), 0)
    if baseline_speed == 0:
        # No baseline — just pick best success rate × largest context
        best = max(viable, key=lambda s: s['success_rate'] * s['context_size'])
        return {
            'context_size': best['context_size'],
            'reason': f"Best success rate ({best['success_rate']:.0%}) at this context size",
            'summary': best,
        }

    # Find knee point: largest context where speed >= 80% of baseline
    speed_threshold = baseline_speed * 0.80
    candidates = [s for s in viable if s['avg_tok_s'] >= speed_threshold]
    if candidates:
        best = max(candidates, key=lambda s: s['context_size'])
        pct = best['avg_tok_s'] / baseline_speed * 100
        return {
            'context_size': best['context_size'],
            'reason': f"Largest context with acceptable speed ({best['avg_tok_s']:.1f} tok/s = {pct:.0f}% of baseline {baseline_speed:.1f})",
            'summary': best,
        }

    # All degraded too much — pick 2048
    return {
        'context_size': 2048,
        'reason': f'All larger contexts degraded speed below 80% of baseline ({baseline_speed:.1f} tok/s)',
        'summary': summaries[0],
    }


if __name__ == '__main__':
    print("Zhen Scientist Experiment: Optimal Mistral-7B Context Window")
    print(f"Testing context sizes: {CONTEXT_VALUES}")
    print(f"Model: {MODEL_PATH.name}")
    print("GPU: AMD RX 7700 XT")
    print()
    run_benchmark()
