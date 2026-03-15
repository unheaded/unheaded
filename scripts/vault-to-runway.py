#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# ═══════════════════════════════════════════════════════════════════════════
# vault-to-runway.py — MAD SCIENTIST MODEL TIERING
#
# Smart tiering between 2TB HDD (Vault) and 1TB NVMe (Runway).
# Auto-purges oldest models when NVMe gets full. Auto-launches vLLM.
#
# Architecture:
#   Vault  (HDD /mnt/hdd/models)  = Permanent archive, every model ever
#   Runway (NVMe /opt/unheaded/active-model) = Hot cache, active inference
#
# Flow:
#   1. Check NVMe capacity → purge LRU models if > threshold
#   2. rsync model from Vault → Runway (differential, resumable)
#   3. Touch atime on active model (LRU tracking)
#   4. Optionally auto-launch vLLM with the model
#
# Usage:
#   ./vault-to-runway.py <model-name> [--launch] [--purge-only] [--list]
#   ./vault-to-runway.py deepseek-r1-7b --launch
#   ./vault-to-runway.py --list
#   ./vault-to-runway.py --purge-only
#
# Environment:
#   VAULT_PATH     (default: /mnt/hdd/models)
#   RUNWAY_PATH    (default: /opt/unheaded/active-model)
#   THRESHOLD_PCT  (default: 85)
#   VLLM_PORT      (default: 20100)
#   VLLM_IMAGE     (default: rocm/vllm:latest)
# ═══════════════════════════════════════════════════════════════════════════

import os
import sys
import shutil
import subprocess
import time
import json
import argparse
from pathlib import Path
from datetime import datetime, timedelta

# ─────────────────────────────────────────────────────────────────────────
# CONFIGURATION
# ─────────────────────────────────────────────────────────────────────────
VAULT_PATH = os.environ.get("VAULT_PATH", "/mnt/hdd/models")
RUNWAY_PATH = os.environ.get("RUNWAY_PATH", "/opt/unheaded/active-model")
THRESHOLD_PCT = float(os.environ.get("THRESHOLD_PCT", "85"))
VLLM_PORT = int(os.environ.get("VLLM_PORT", "20100"))
VLLM_IMAGE = os.environ.get("VLLM_IMAGE", "rocm/vllm:latest")
VLLM_CONTAINER = "unheaded-vllm"
STATE_FILE = os.path.join(RUNWAY_PATH, ".runway-state.json")

# GPU config for RX 7700 XT (gfx1101)
GPU_ENV = {
    "HSA_OVERRIDE_GFX_VERSION": "11.0.1",
    "PYTORCH_ROCM_ARCH": "gfx1101",
    "HIP_VISIBLE_DEVICES": "0",
    "VLLM_USE_AITER": "1",
}


# ─────────────────────────────────────────────────────────────────────────
# COLORS (because we're mad scientists, not animals)
# ─────────────────────────────────────────────────────────────────────────
class C:
    RED = "\033[0;31m"
    GREEN = "\033[0;32m"
    YELLOW = "\033[1;33m"
    BLUE = "\033[0;34m"
    CYAN = "\033[0;36m"
    MAGENTA = "\033[0;35m"
    BOLD = "\033[1m"
    NC = "\033[0m"


def info(msg):
    print(f"{C.BLUE}[INFO]{C.NC} {msg}")


def ok(msg):
    print(f"{C.GREEN}[OK]{C.NC} {msg}")


def warn(msg):
    print(f"{C.YELLOW}[WARN]{C.NC} {msg}")


def err(msg):
    print(f"{C.RED}[ERROR]{C.NC} {msg}")


def phase(msg):
    print(f"\n{C.MAGENTA}{C.BOLD}=== {msg} ==={C.NC}\n")


# ─────────────────────────────────────────────────────────────────────────
# STATE TRACKING (persist model access times across reboots)
# ─────────────────────────────────────────────────────────────────────────
def load_state() -> dict:
    """Load runway state (model access times, sizes, launch history)."""
    if os.path.exists(STATE_FILE):
        try:
            with open(STATE_FILE, "r") as f:
                return json.load(f)
        except (json.JSONDecodeError, IOError):
            return {"models": {}, "launches": []}
    return {"models": {}, "launches": []}


def save_state(state: dict):
    """Persist runway state."""
    os.makedirs(os.path.dirname(STATE_FILE), exist_ok=True)
    with open(STATE_FILE, "w") as f:
        json.dump(state, f, indent=2, default=str)


def touch_model(state: dict, model_name: str, size_bytes: int = 0):
    """Update access time for a model (LRU tracking)."""
    state["models"][model_name] = {
        "last_access": datetime.now().isoformat(),
        "size_bytes": size_bytes,
        "access_count": state["models"].get(model_name, {}).get("access_count", 0) + 1,
    }
    save_state(state)


# ─────────────────────────────────────────────────────────────────────────
# DISK OPERATIONS
# ─────────────────────────────────────────────────────────────────────────
def get_disk_usage(path: str) -> tuple:
    """Returns (total_gb, used_gb, free_gb, percent_used)."""
    try:
        total, used, free = shutil.disk_usage(path)
        gb = 1024**3
        return (total / gb, used / gb, free / gb, (used / total) * 100)
    except OSError:
        return (0, 0, 0, 0)


def get_dir_size(path: str) -> int:
    """Get total size of a directory in bytes."""
    total = 0
    for dirpath, _dirnames, filenames in os.walk(path):
        for f in filenames:
            fp = os.path.join(dirpath, f)
            try:
                total += os.path.getsize(fp)
            except OSError:
                pass
    return total


def list_runway_models() -> list:
    """List all models currently on the NVMe runway."""
    if not os.path.exists(RUNWAY_PATH):
        return []
    models = []
    for d in os.listdir(RUNWAY_PATH):
        full = os.path.join(RUNWAY_PATH, d)
        if os.path.isdir(full) and not d.startswith("."):
            size = get_dir_size(full)
            try:
                atime = os.path.getatime(full)
            except OSError:
                atime = 0
            models.append({
                "name": d,
                "path": full,
                "size_bytes": size,
                "size_gb": size / (1024**3),
                "last_access": datetime.fromtimestamp(atime),
            })
    return sorted(models, key=lambda x: x["last_access"])


def list_vault_models() -> list:
    """List all models in the HDD vault."""
    if not os.path.exists(VAULT_PATH):
        return []
    models = []
    for d in os.listdir(VAULT_PATH):
        full = os.path.join(VAULT_PATH, d)
        if os.path.isdir(full):
            size = get_dir_size(full)
            models.append({
                "name": d,
                "path": full,
                "size_bytes": size,
                "size_gb": size / (1024**3),
            })
    return sorted(models, key=lambda x: x["name"])


# ─────────────────────────────────────────────────────────────────────────
# PURGE (LRU eviction)
# ─────────────────────────────────────────────────────────────────────────
def purge_oldest(dry_run: bool = False) -> bool:
    """Evict the least-recently-accessed model from NVMe runway."""
    models = list_runway_models()
    if not models:
        warn("No models on runway to purge!")
        return False

    oldest = models[0]  # sorted by atime ascending
    age = datetime.now() - oldest["last_access"]

    info(f"Purging LRU model: {oldest['name']} ({oldest['size_gb']:.1f}GB, last used {age.days}d ago)")

    if dry_run:
        info(f"[DRY-RUN] Would delete: {oldest['path']}")
        return True

    try:
        shutil.rmtree(oldest["path"])
        ok(f"Purged {oldest['name']} — freed {oldest['size_gb']:.1f}GB")

        # Remove from state
        state = load_state()
        state["models"].pop(oldest["name"], None)
        save_state(state)
        return True
    except OSError as e:
        err(f"Failed to purge {oldest['name']}: {e}")
        return False


def ensure_runway_capacity(needed_gb: float = 0, dry_run: bool = False):
    """Purge models until runway has enough space or is below threshold."""
    total, used, free, pct = get_disk_usage(RUNWAY_PATH)
    info(f"NVMe status: {used:.1f}/{total:.1f}GB used ({pct:.1f}%), {free:.1f}GB free")

    rounds = 0
    max_rounds = 20  # safety valve

    while rounds < max_rounds:
        total, used, free, pct = get_disk_usage(RUNWAY_PATH)

        # Check both threshold AND needed space
        if pct <= THRESHOLD_PCT and free >= needed_gb:
            ok(f"Runway clear: {pct:.1f}% used, {free:.1f}GB free")
            return
        rounds += 1
        warn(f"Runway at {pct:.1f}% (threshold: {THRESHOLD_PCT}%), need {needed_gb:.1f}GB free")
        if not purge_oldest(dry_run=dry_run):
            break

    if rounds >= max_rounds:
        warn(f"Hit max purge rounds ({max_rounds}) — check manually")


# ─────────────────────────────────────────────────────────────────────────
# TRANSFER (Vault → Runway via rsync)
# ─────────────────────────────────────────────────────────────────────────
def transfer_model(model_name: str, dry_run: bool = False) -> bool:
    """rsync model from HDD vault to NVMe runway (differential, resumable)."""
    vault_dir = os.path.join(VAULT_PATH, model_name)
    runway_dir = os.path.join(RUNWAY_PATH, model_name)

    if not os.path.exists(vault_dir):
        err(f"Model '{model_name}' not found in vault: {vault_dir}")
        info(f"Available in vault:")
        for m in list_vault_models():
            print(f"  {m['name']:40s} {m['size_gb']:.1f}GB")
        return False

    # Check if already on runway and up-to-date
    if os.path.exists(runway_dir):
        vault_size = get_dir_size(vault_dir)
        runway_size = get_dir_size(runway_dir)
        if vault_size == runway_size:
            ok(f"Model '{model_name}' already on runway ({runway_size / (1024**3):.1f}GB)")
            # Touch to update LRU
            os.utime(runway_dir, None)
            state = load_state()
            touch_model(state, model_name, runway_size)
            return True

    # Calculate needed space
    vault_size = get_dir_size(vault_dir)
    needed_gb = vault_size / (1024**3) * 1.1  # 10% headroom

    phase(f"TRANSFERRING: {model_name} ({needed_gb:.1f}GB)")

    # Ensure capacity
    ensure_runway_capacity(needed_gb=needed_gb, dry_run=dry_run)

    if dry_run:
        info(f"[DRY-RUN] Would rsync {vault_dir} → {RUNWAY_PATH}")
        return True

    # rsync: differential, resumable, progress
    info(f"rsync {vault_dir} → {RUNWAY_PATH}")
    try:
        result = subprocess.run(
            [
                "rsync", "-ah", "--progress", "--partial",
                "--info=progress2",
                f"{vault_dir}/",  # trailing slash = contents, not dir itself
                f"{runway_dir}/",
            ],
            check=True,
        )
        ok(f"Transfer complete: {model_name} ({vault_size / (1024**3):.1f}GB)")

        # Touch for LRU tracking
        os.utime(runway_dir, None)
        state = load_state()
        touch_model(state, model_name, vault_size)

        return True
    except subprocess.CalledProcessError as e:
        err(f"rsync failed (exit {e.returncode}) — transfer may be partial, re-run to resume")
        return False
    except FileNotFoundError:
        err("rsync not found — install: apt install rsync")
        return False


# ─────────────────────────────────────────────────────────────────────────
# vLLM AUTO-LAUNCH
# ─────────────────────────────────────────────────────────────────────────
def stop_vllm():
    """Stop existing vLLM container if running."""
    try:
        result = subprocess.run(
            ["docker", "inspect", VLLM_CONTAINER],
            capture_output=True, text=True,
        )
        if result.returncode == 0:
            info(f"Stopping existing vLLM container: {VLLM_CONTAINER}")
            subprocess.run(["docker", "stop", VLLM_CONTAINER], capture_output=True)
            subprocess.run(["docker", "rm", VLLM_CONTAINER], capture_output=True)
    except FileNotFoundError:
        err("Docker not found!")


def launch_vllm(model_name: str, dry_run: bool = False) -> bool:
    """Launch vLLM inference server with the specified model."""
    runway_dir = os.path.join(RUNWAY_PATH, model_name)

    if not os.path.exists(runway_dir):
        err(f"Model not on runway: {runway_dir}")
        return False

    phase(f"LAUNCHING vLLM: {model_name} on port {VLLM_PORT}")

    # Build docker run command
    cmd = [
        "docker", "run", "-d",
        "--name", VLLM_CONTAINER,
        "--restart", "unless-stopped",
        "--device=/dev/kfd", "--device=/dev/dri",
        "--group-add", "video", "--group-add", "render",
        "--shm-size=4g",
    ]

    # GPU environment
    for key, val in GPU_ENV.items():
        cmd.extend(["-e", f"{key}={val}"])

    # Mount runway as /models (read-only)
    cmd.extend(["-v", f"{RUNWAY_PATH}:/models:ro"])

    # Port
    cmd.extend(["-p", f"{VLLM_PORT}:{VLLM_PORT}"])

    # Network
    cmd.extend(["--network", "unheaded_unheaded"])

    # Image + args
    cmd.extend([
        VLLM_IMAGE,
        "--model", f"/models/{model_name}",
        "--port", str(VLLM_PORT),
        "--host", "0.0.0.0",
        "--dtype", "half",
        "--gpu-memory-utilization", "0.90",
        "--max-model-len", "8192",
        "--tensor-parallel-size", "1",
    ])

    if dry_run:
        info(f"[DRY-RUN] Would run: {' '.join(cmd)}")
        return True

    # Stop existing
    stop_vllm()

    info("Starting vLLM container...")
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        container_id = result.stdout.strip()[:12]
        ok(f"vLLM started: container {container_id}")

        # Record launch
        state = load_state()
        state["launches"].append({
            "model": model_name,
            "timestamp": datetime.now().isoformat(),
            "port": VLLM_PORT,
            "container": container_id,
        })
        # Keep last 50 launches
        state["launches"] = state["launches"][-50:]
        save_state(state)

        print(f"\n{C.GREEN}{C.BOLD}vLLM is starting up...{C.NC}")
        print(f"  Health:   curl http://localhost:{VLLM_PORT}/health")
        print(f"  Models:   curl http://localhost:{VLLM_PORT}/v1/models")
        print(f"  Chat:     curl http://localhost:{VLLM_PORT}/v1/chat/completions")
        print(f"  Logs:     docker logs -f {VLLM_CONTAINER}")
        print(f"  Stop:     docker stop {VLLM_CONTAINER}")
        print()

        # Wait for health
        info("Waiting for vLLM health check (up to 120s)...")
        for i in range(24):
            time.sleep(5)
            try:
                health = subprocess.run(
                    ["curl", "-sf", f"http://localhost:{VLLM_PORT}/health"],
                    capture_output=True, text=True, timeout=5,
                )
                if health.returncode == 0:
                    ok(f"vLLM healthy after {(i+1)*5}s!")
                    return True
            except (subprocess.TimeoutExpired, FileNotFoundError):
                pass
            sys.stdout.write(f"\r  Waiting... {(i+1)*5}s")
            sys.stdout.flush()

        warn("vLLM didn't respond within 120s — check: docker logs -f unheaded-vllm")
        return True  # Container is running, just slow to load

    except subprocess.CalledProcessError as e:
        err(f"Docker run failed: {e.stderr}")
        return False


# ─────────────────────────────────────────────────────────────────────────
# DISPLAY
# ─────────────────────────────────────────────────────────────────────────
def display_status():
    """Show full status of vault + runway."""
    phase("VAULT (HDD — Deep Storage)")
    vault_models = list_vault_models()
    total_vault_gb = 0
    if vault_models:
        for m in vault_models:
            total_vault_gb += m["size_gb"]
            print(f"  {m['name']:45s} {m['size_gb']:7.1f}GB")
        total, used, free, pct = get_disk_usage(VAULT_PATH)
        print(f"\n  Total models: {len(vault_models)} ({total_vault_gb:.1f}GB)")
        print(f"  Disk: {used:.0f}/{total:.0f}GB ({pct:.1f}%), {free:.0f}GB free")
    else:
        warn(f"No models in vault ({VAULT_PATH})")

    phase("RUNWAY (NVMe — Active Inference)")
    runway_models = list_runway_models()
    total_runway_gb = 0
    if runway_models:
        for m in runway_models:
            total_runway_gb += m["size_gb"]
            age = datetime.now() - m["last_access"]
            age_str = f"{age.days}d" if age.days > 0 else f"{age.seconds//3600}h"
            print(f"  {m['name']:45s} {m['size_gb']:7.1f}GB  (last used: {age_str} ago)")
        total, used, free, pct = get_disk_usage(RUNWAY_PATH)
        print(f"\n  Active models: {len(runway_models)} ({total_runway_gb:.1f}GB)")
        print(f"  NVMe: {used:.0f}/{total:.0f}GB ({pct:.1f}%), {free:.0f}GB free")
        print(f"  Purge threshold: {THRESHOLD_PCT}%")
    else:
        info(f"Runway clear ({RUNWAY_PATH})")

    # vLLM status
    phase("vLLM STATUS")
    try:
        result = subprocess.run(
            ["docker", "inspect", "--format", "{{.State.Status}}", VLLM_CONTAINER],
            capture_output=True, text=True,
        )
        if result.returncode == 0:
            status = result.stdout.strip()
            ok(f"Container: {VLLM_CONTAINER} — {status}")
            if status == "running":
                health = subprocess.run(
                    ["curl", "-sf", f"http://localhost:{VLLM_PORT}/health"],
                    capture_output=True, text=True, timeout=5,
                )
                if health.returncode == 0:
                    ok(f"API healthy on port {VLLM_PORT}")
                else:
                    warn(f"API not responding on port {VLLM_PORT}")
        else:
            info("No vLLM container running")
    except (FileNotFoundError, subprocess.TimeoutExpired):
        info("Docker not available or timeout")


# ─────────────────────────────────────────────────────────────────────────
# MAIN
# ─────────────────────────────────────────────────────────────────────────
def main():
    parser = argparse.ArgumentParser(
        description="Mad Scientist Model Tiering — Vault (HDD) ↔ Runway (NVMe)",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s deepseek-r1-7b                  # Transfer model to NVMe
  %(prog)s deepseek-r1-7b --launch         # Transfer + auto-launch vLLM
  %(prog)s --list                           # Show vault + runway status
  %(prog)s --purge-only                     # Free NVMe space (LRU eviction)
  %(prog)s mistral-7b --dry-run --launch    # Preview what would happen

Environment:
  VAULT_PATH     HDD model archive    (default: /mnt/hdd/models)
  RUNWAY_PATH    NVMe active slot     (default: /opt/unheaded/active-model)
  THRESHOLD_PCT  Auto-purge threshold (default: 85)
  VLLM_PORT      Inference port       (default: 20100)
  VLLM_IMAGE     Docker image         (default: rocm/vllm:latest)
        """,
    )
    parser.add_argument("model", nargs="?", help="Model name to transfer to runway")
    parser.add_argument("--launch", action="store_true", help="Auto-launch vLLM after transfer")
    parser.add_argument("--list", action="store_true", help="Show vault + runway status")
    parser.add_argument("--purge-only", action="store_true", help="Only purge LRU models from runway")
    parser.add_argument("--dry-run", action="store_true", help="Preview actions without executing")
    parser.add_argument("--threshold", type=float, default=THRESHOLD_PCT, help="Purge threshold %%")
    parser.add_argument("--stop", action="store_true", help="Stop vLLM container")

    args = parser.parse_args()

    # Override threshold if specified
    global THRESHOLD_PCT
    THRESHOLD_PCT = args.threshold

    print(f"{C.MAGENTA}{C.BOLD}")
    print("╔═══════════════════════════════════════════════╗")
    print("║   VAULT-TO-RUNWAY — Mad Scientist Tiering    ║")
    print("╚═══════════════════════════════════════════════╝")
    print(f"{C.NC}")

    # Ensure directories exist
    os.makedirs(VAULT_PATH, exist_ok=True)
    os.makedirs(RUNWAY_PATH, exist_ok=True)

    if args.list:
        display_status()
        return

    if args.stop:
        stop_vllm()
        ok("vLLM stopped")
        return

    if args.purge_only:
        ensure_runway_capacity(dry_run=args.dry_run)
        return

    if not args.model:
        parser.print_help()
        print(f"\n{C.YELLOW}Hint: Use --list to see available models{C.NC}")
        return

    # Transfer
    success = transfer_model(args.model, dry_run=args.dry_run)

    # Launch vLLM if requested
    if success and args.launch:
        launch_vllm(args.model, dry_run=args.dry_run)
    elif success and not args.launch:
        info(f"Model ready. Launch with: {sys.argv[0]} {args.model} --launch")


if __name__ == "__main__":
    main()
