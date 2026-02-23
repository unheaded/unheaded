#!/usr/bin/env python3
"""WS3 Phase 3: Analyze delay profiling results.

Parses log files from delay-profile.sh and generates a summary table
with throughput rates and estimated FPS for each delay setting.

Usage:
    python3 scripts/ws3/analyze-delays.py
    python3 scripts/ws3/analyze-delays.py --results-dir /tmp/ws3-results
    python3 scripts/ws3/analyze-delays.py --json
"""
import argparse
import glob
import json
import re
import sys

# Doom parameters.
INSNS_PER_PACKET = 4080
INSNS_PER_FRAME = 1_470_000  # ~1.47M insns per frame (measured in S31).


def parse_inject_log(path):
    """Parse an inject.py log file, returning metrics dict or None."""
    try:
        with open(path) as f:
            content = f.read()
    except OSError:
        return None

    result = {
        "has_fault": "ROM FAULT" in content or "*** ROM" in content,
        "pps": 0.0,
        "elapsed": 0.0,
        "total_insns": 0,
    }

    # Extract PPS: "Injected NNN packets in X.Xs (YYY pkt/s, ~ZZZ insns)"
    pps_match = re.search(r"(\d+(?:\.\d+)?)\s+pkt/s", content)
    if pps_match:
        result["pps"] = float(pps_match.group(1))

    elapsed_match = re.search(r"in\s+([\d.]+)s", content)
    if elapsed_match:
        result["elapsed"] = float(elapsed_match.group(1))

    insns_match = re.search(r"~([\d,]+)\s+insns", content)
    if insns_match:
        result["total_insns"] = int(insns_match.group(1).replace(",", ""))

    return result


def estimate_fps(pps):
    """Estimate internal Doom FPS from injection PPS."""
    if pps <= 0:
        return 0.0
    insns_per_sec = pps * INSNS_PER_PACKET
    return insns_per_sec / INSNS_PER_FRAME


def main():
    parser = argparse.ArgumentParser(description="Analyze WS3 delay profiling results")
    parser.add_argument("--results-dir", default="/tmp/ws3-results",
                        help="Directory containing delay test logs")
    parser.add_argument("--json", action="store_true",
                        help="Output as JSON")
    args = parser.parse_args()

    # Find all delay test log files.
    pattern = f"{args.results_dir}/delay-test-*us.log"
    log_files = sorted(glob.glob(pattern))

    if not log_files:
        print(f"No delay test logs found in {args.results_dir}/", file=sys.stderr)
        print(f"Run scripts/ws3/delay-profile.sh first.", file=sys.stderr)
        sys.exit(1)

    results = {}
    for path in log_files:
        match = re.search(r"delay-test-([A-F])-(\d+)us\.log$", path)
        if not match:
            continue
        test_id = match.group(1)
        delay_us = int(match.group(2))
        metrics = parse_inject_log(path)
        if metrics:
            metrics["delay_us"] = delay_us
            results[test_id] = metrics

    if args.json:
        print(json.dumps(results, indent=2))
        return

    # Print results table.
    print("\n=== Delay Profile Results ===\n")
    print(f"{'Test':<6} {'Delay(us)':<10} {'PPS':<10} {'Status':<12} {'Est. FPS':<10}")
    print("-" * 50)

    for test_id in sorted(results.keys()):
        r = results[test_id]
        status = "FAULT" if r["has_fault"] else "OK"
        fps = estimate_fps(r["pps"])
        print(f"{test_id:<6} {r['delay_us']:<10} {r['pps']:<10.0f} {status:<12} {fps:<10.1f}")

    print()

    # Find threshold.
    sorted_tests = sorted(results.items(), key=lambda x: x[1]["delay_us"])
    faulted = [t for t in sorted_tests if t[1]["has_fault"]]
    ok_tests = [t for t in sorted_tests if not t[1]["has_fault"]]

    print("=== Analysis ===\n")

    if faulted:
        first_fault = min(faulted, key=lambda x: x[1]["delay_us"])
        fault_delay = first_fault[1]["delay_us"]
        safe_tests = [t for t in ok_tests if t[1]["delay_us"] > fault_delay]
        if safe_tests:
            min_safe = min(safe_tests, key=lambda x: x[1]["delay_us"])
            print(f"THRESHOLD FOUND: {min_safe[1]['delay_us']}us (safe) -> "
                  f"{fault_delay}us (FAULT)")
        else:
            print(f"FAULT at {fault_delay}us -- no safe delay found above it")
    else:
        if ok_tests:
            fastest = min(ok_tests, key=lambda x: x[1]["delay_us"])
            print(f"No faults detected across all delays.")
            print(f"Fastest safe delay: {fastest[1]['delay_us']}us "
                  f"({fastest[1]['pps']:.0f} pps, ~{estimate_fps(fastest[1]['pps']):.1f} fps)")
            print(f"\nH1 CONFIRMED: Python socket.send() is the bottleneck, not XDP.")
        else:
            print("No results to analyze.")

    # Throughput summary.
    if ok_tests:
        best = max(ok_tests, key=lambda x: x[1]["pps"])
        print(f"\nBest throughput: {best[1]['pps']:.0f} pps at {best[1]['delay_us']}us "
              f"(~{estimate_fps(best[1]['pps']):.1f} fps)")

        if estimate_fps(best[1]["pps"]) >= 15:
            print("  -> Target 15 fps ACHIEVABLE with Python injector at this delay")
        else:
            print("  -> Target 15 fps NOT achievable with Python at any delay")
            print("  -> Proceed to Phase 5 (Go injector) for higher throughput")


if __name__ == "__main__":
    main()
