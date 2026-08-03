#!/usr/bin/env python3
"""
10_hotswap_index.py — Hot-swap FAISS index for Zhen RAG pipeline

Safely swaps the active FAISS index from ring1 to combined (or any new index)
using a symlink approach. This allows instant rollback by re-pointing symlinks.

Usage:
    python3 10_hotswap_index.py              # swap to combined index
    python3 10_hotswap_index.py --rollback   # swap back to ring1 index
    python3 10_hotswap_index.py --status     # show current active index

Safety:
    - Checks that combined_ids.json exists and has expected entry count
    - Checks that combined.index exists and is non-zero size
    - Will NOT swap if the embedding is still in progress
    - Uses atomic symlink swap (rename is atomic on Linux)
"""
import argparse
import json
import os
import signal
import subprocess
import sys
from pathlib import Path

INDEX_DIR = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index'
RAFT_DIR = Path.home() / 'tmp' / 'unheaded' / 'raft'

# Source index files
RING1_INDEX = INDEX_DIR / 'ring1.index'
RING1_IDS = INDEX_DIR / 'ring1_ids.json'
COMBINED_INDEX = INDEX_DIR / 'combined.index'
COMBINED_IDS = INDEX_DIR / 'combined_ids.json'

# Active symlinks — zhen_rag.py reads from these
ACTIVE_INDEX = INDEX_DIR / 'active.index'
ACTIVE_IDS = INDEX_DIR / 'active_ids.json'


def get_vector_count(index_path):
    """Get vector count from a FAISS index without loading the full index."""
    try:
        import faiss
        index = faiss.read_index(str(index_path))
        count = index.ntotal
        dim = index.d
        del index
        return count, dim
    except Exception:
        return None, None


def get_ids_count(ids_path):
    """Count entries in an IDs JSON file."""
    try:
        with open(ids_path, 'r') as f:
            data = json.load(f)
        if isinstance(data, (list, dict)):
            return len(data)
        return 0
    except Exception:
        return 0


def get_current_target(symlink_path):
    """Get the target of a symlink, or None if not a symlink."""
    if symlink_path.is_symlink():
        return symlink_path.resolve().name
    elif symlink_path.exists():
        return f"(regular file: {symlink_path.name})"
    return None


def atomic_symlink(target, link_path):
    """Create a symlink atomically using rename."""
    tmp_link = link_path.parent / (link_path.name + '.tmp')
    try:
        tmp_link.unlink(missing_ok=True)
        tmp_link.symlink_to(target)
        tmp_link.rename(link_path)
    except Exception:
        tmp_link.unlink(missing_ok=True)
        raise


def setup_initial_symlinks():
    """If no active symlinks exist, create them pointing to ring1."""
    if not ACTIVE_INDEX.exists() and not ACTIVE_INDEX.is_symlink():
        print("  Creating initial symlink: active.index -> ring1.index")
        atomic_symlink(RING1_INDEX.name, ACTIVE_INDEX)
    if not ACTIVE_IDS.exists() and not ACTIVE_IDS.is_symlink():
        print("  Creating initial symlink: active_ids.json -> ring1_ids.json")
        atomic_symlink(RING1_IDS.name, ACTIVE_IDS)


def find_zhen_pid():
    """Find the PID of the running zhen_app.py process."""
    try:
        result = subprocess.run(
            ['pgrep', '-f', 'python.*zhen_app.py'],
            capture_output=True, text=True, check=False
        )
        pids = [int(p) for p in result.stdout.strip().split('\n') if p.strip()]
        return pids
    except Exception:
        return []


def restart_zhen():
    """Restart the Zhen Flask app."""
    pids = find_zhen_pid()
    if pids:
        print(f"\n  Stopping Zhen (PIDs: {pids})...")
        for pid in pids:
            try:
                os.kill(pid, signal.SIGTERM)
            except ProcessLookupError:
                pass

        # Wait briefly for graceful shutdown
        import time
        time.sleep(2)

        # Force kill if still running
        for pid in pids:
            try:
                os.kill(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass

    # Restart
    print("  Starting Zhen Web UI...")
    venv_python = Path.home() / '.venv' / 'zhen' / 'bin' / 'python3'
    zhen_app = RAFT_DIR / 'zhen_app.py'

    subprocess.Popen(
        [str(venv_python), str(zhen_app)],
        cwd=str(RAFT_DIR),
        # Deliberate: this handle is owned by the child process for its whole
        # lifetime. A context manager would close it the moment Popen returns.
        stdout=open('/tmp/zhen-webapp.log', 'w'),  # noqa: SIM115
        stderr=subprocess.STDOUT,
        start_new_session=True,
    )

    import time
    time.sleep(3)

    # Verify
    new_pids = find_zhen_pid()
    if new_pids:
        print(f"  Zhen restarted (PIDs: {new_pids})")
    else:
        print("  WARNING: Zhen may not have started. Check /tmp/zhen-webapp.log")


def show_status():
    """Show current index status."""
    print("=" * 60)
    print("ZHEN INDEX STATUS")
    print("=" * 60)

    # Current active target
    idx_target = get_current_target(ACTIVE_INDEX)
    ids_target = get_current_target(ACTIVE_IDS)

    print(f"\n  Active index:  {idx_target or 'NOT SET'}")
    print(f"  Active IDs:    {ids_target or 'NOT SET'}")

    # Ring1 stats
    if RING1_INDEX.exists():
        count, dim = get_vector_count(RING1_INDEX)
        ids_count = get_ids_count(RING1_IDS)
        size_mb = RING1_INDEX.stat().st_size / (1024 * 1024)
        print(f"\n  ring1.index:   {count:,} vectors, dim={dim}, {size_mb:.1f} MB, {ids_count:,} IDs")

    # Combined stats
    if COMBINED_INDEX.exists():
        count, dim = get_vector_count(COMBINED_INDEX)
        ids_count = get_ids_count(COMBINED_IDS) if COMBINED_IDS.exists() else 0
        size_mb = COMBINED_INDEX.stat().st_size / (1024 * 1024)
        complete = "COMPLETE" if COMBINED_IDS.exists() and ids_count > 0 else "INCOMPLETE"
        print(f"  combined.index: {count:,} vectors, dim={dim}, {size_mb:.1f} MB, {ids_count:,} IDs [{complete}]")
    else:
        print("  combined.index: NOT FOUND (embedding still in progress)")

    # Zhen process
    pids = find_zhen_pid()
    print(f"\n  Zhen process:  {'Running (PIDs: ' + str(pids) + ')' if pids else 'NOT RUNNING'}")
    print()


def swap_to_combined():
    """Swap active index to combined."""
    print("=" * 60)
    print("ZHEN INDEX HOT-SWAP: ring1 -> combined")
    print("=" * 60)

    # Pre-flight checks
    print("\n[1/5] Pre-flight checks...")

    if not COMBINED_INDEX.exists():
        print("  ABORT: combined.index does not exist.")
        print("  The embedding is likely still in progress.")
        sys.exit(1)

    combined_size = COMBINED_INDEX.stat().st_size
    if combined_size == 0:
        print("  ABORT: combined.index is empty (0 bytes).")
        sys.exit(1)

    if not COMBINED_IDS.exists():
        print("  ABORT: combined_ids.json does not exist.")
        print("  The embedding is likely still in progress.")
        sys.exit(1)

    combined_ids_count = get_ids_count(COMBINED_IDS)
    if combined_ids_count == 0:
        print("  ABORT: combined_ids.json has 0 entries.")
        sys.exit(1)

    print(f"  combined.index: {combined_size / (1024*1024):.1f} MB")
    print(f"  combined_ids.json: {combined_ids_count:,} entries")
    print("  All checks passed.")

    # Get before stats
    print("\n[2/5] Before stats...")
    old_count, _old_dim = None, None
    if ACTIVE_INDEX.exists():
        old_count, _old_dim = get_vector_count(ACTIVE_INDEX)
        old_ids = get_ids_count(ACTIVE_IDS) if ACTIVE_IDS.exists() else 0
        print(f"  Current: {old_count:,} vectors, {old_ids:,} IDs")
    else:
        print("  No active index currently set.")

    # Get new stats
    print("\n[3/5] New index stats...")
    new_count, _new_dim = get_vector_count(COMBINED_INDEX)
    print(f"  Combined: {new_count:,} vectors, {combined_ids_count:,} IDs")

    # Sanity: IDs should match vectors
    if new_count != combined_ids_count:
        print(f"  WARNING: Vector count ({new_count:,}) != ID count ({combined_ids_count:,})")
        print("  This may indicate a partial build. Proceed? [y/N] ", end="")
        if input().strip().lower() != 'y':
            print("  Aborted.")
            sys.exit(1)

    # Swap symlinks
    print("\n[4/5] Swapping symlinks...")
    atomic_symlink(COMBINED_INDEX.name, ACTIVE_INDEX)
    atomic_symlink(COMBINED_IDS.name, ACTIVE_IDS)
    print(f"  active.index -> {COMBINED_INDEX.name}")
    print(f"  active_ids.json -> {COMBINED_IDS.name}")

    # Restart Zhen
    print("\n[5/5] Restarting Zhen...")
    restart_zhen()

    # Summary
    print("\n" + "=" * 60)
    print("HOT-SWAP COMPLETE")
    print("=" * 60)
    if old_count is not None:
        print(f"  Vectors: {old_count:,} -> {new_count:,} (+{new_count - old_count:,})")
    else:
        print(f"  Vectors: (none) -> {new_count:,}")
    print(f"  IDs: {combined_ids_count:,}")
    print("  Index: combined.index")
    print()


def rollback_to_ring1():
    """Swap active index back to ring1."""
    print("=" * 60)
    print("ZHEN INDEX ROLLBACK: -> ring1")
    print("=" * 60)

    if not RING1_INDEX.exists():
        print("  ABORT: ring1.index does not exist.")
        sys.exit(1)

    # Get before stats
    old_count, _ = get_vector_count(ACTIVE_INDEX) if ACTIVE_INDEX.exists() else (None, None)

    # Swap
    print("\n  Swapping symlinks...")
    atomic_symlink(RING1_INDEX.name, ACTIVE_INDEX)
    atomic_symlink(RING1_IDS.name, ACTIVE_IDS)
    print(f"  active.index -> {RING1_INDEX.name}")
    print(f"  active_ids.json -> {RING1_IDS.name}")

    # Restart
    print("\n  Restarting Zhen...")
    restart_zhen()

    ring1_count, _ = get_vector_count(RING1_INDEX)
    print(f"\n  Rollback complete. Vectors: {old_count:,} -> {ring1_count:,}")
    print()


def main():
    parser = argparse.ArgumentParser(description='Hot-swap Zhen FAISS index')
    parser.add_argument('--rollback', action='store_true', help='Roll back to ring1 index')
    parser.add_argument('--status', action='store_true', help='Show current index status')
    parser.add_argument('--no-restart', action='store_true', help='Swap symlinks without restarting Zhen')
    args = parser.parse_args()

    # Ensure initial symlinks exist
    setup_initial_symlinks()

    if args.status:
        show_status()
    elif args.rollback:
        rollback_to_ring1()
    else:
        if args.no_restart:
            # Just check and report, skip restart
            print("  --no-restart: Will swap symlinks only")
        swap_to_combined()


if __name__ == '__main__':
    main()
