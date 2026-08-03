#!/usr/bin/env python3
"""
zhen_scheduler.py — Zhenai Proactive Scheduler

Runs alongside zhen_app.py. Executes scheduled runbooks and heartbeat
health checks. Makes Zhenai proactive — it reaches out first.

Usage:
  python3 zhen_scheduler.py                    # Start scheduler daemon
  python3 zhen_scheduler.py --list             # List scheduled jobs
  python3 zhen_scheduler.py --run-once <name>  # Run one job immediately

ADR: ADR-028 Zhenai Scheduler & Heartbeat
"""

import json
import logging
import os
import subprocess
import sys
import time
from pathlib import Path

import schedule

logging.basicConfig(level=logging.INFO, format='%(asctime)s [%(levelname)s] %(message)s')
log = logging.getLogger('zhenai-scheduler')

PROJECT_ROOT = Path.home() / 'tmp' / 'unheaded'
RUNBOOKS_DIR = PROJECT_ROOT / 'runbooks'
RUNNER = PROJECT_ROOT / 'scripts' / 'run-runbook.py'
SCHEDULE_FILE = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'schedule.json'

# Default schedules — Zhenai's autonomous duties
DEFAULT_SCHEDULES = [
    {'name': 'service-health-sweep', 'interval': '30m', 'enabled': True},
    {'name': 'log-rotation', 'interval': 'daily', 'time': '03:00', 'enabled': False},
    {'name': 'postgresql-backup-restore', 'interval': 'daily', 'time': '02:00', 'enabled': False},
    {'name': 'prometheus-scrape-check', 'interval': '1h', 'enabled': False},
]


def find_runbook(name):
    """Find a runbook YAML file by name."""
    import glob
    matches = glob.glob(str(RUNBOOKS_DIR / '**' / f'{name}.yaml'), recursive=True)
    return matches[0] if matches else None


def execute_runbook(name, dry_run=False):
    """Execute a runbook via run-runbook.py."""
    rb_path = find_runbook(name)
    if not rb_path:
        log.error(f'Runbook not found: {name}')
        return False

    cmd = ['python3', str(RUNNER)]
    if dry_run:
        cmd.append('--dry-run')
    cmd.append(rb_path)

    log.info(f'Executing runbook: {name} {"(dry-run)" if dry_run else ""}')
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=300)
        if result.returncode == 0:
            log.info(f'Runbook {name}: SUCCESS')
        else:
            log.warning(f'Runbook {name}: FAILED (exit {result.returncode})')
            if result.stderr:
                log.warning(f'  stderr: {result.stderr[:500]}')
        return result.returncode == 0
    except subprocess.TimeoutExpired:
        log.error(f'Runbook {name}: TIMEOUT (300s)')
        return False
    except Exception as e:
        log.error(f'Runbook {name}: ERROR — {e}')
        return False


def heartbeat():
    """Lightweight health pulse — check critical services and system resources."""
    import urllib.request

    services = {'wotan': 18000, 'zhenai': 20103, 'inference': 20100}
    issues = []

    for name, port in services.items():
        try:
            urllib.request.urlopen(f'http://localhost:{port}/health', timeout=2)
        except Exception:
            issues.append(f'{name} (:{port}) is DOWN')

    # Check disk space
    try:
        st = os.statvfs('/')
        pct_used = (1 - st.f_bavail / st.f_blocks) * 100
        if pct_used > 85:
            issues.append(f'Disk / is {pct_used:.0f}% full')
    except Exception:
        pass

    # Check swap usage
    try:
        with open('/proc/meminfo') as f:
            mem = {}
            for line in f:
                parts = line.split()
                if len(parts) >= 2:
                    mem[parts[0].rstrip(':')] = int(parts[1])
        swap_total = mem.get('SwapTotal', 0)
        swap_free = mem.get('SwapFree', 0)
        if swap_total > 0:
            swap_pct = (1 - swap_free / swap_total) * 100
            if swap_pct > 50:
                issues.append(f'Swap is {swap_pct:.0f}% used')
    except Exception:
        pass

    if issues:
        log.warning(f'HEARTBEAT ALERT: {len(issues)} issues — ' + '; '.join(issues))
    else:
        log.info('HEARTBEAT: all clear')

    return len(issues) == 0


def load_schedules():
    """Load schedule config from JSON file, or create with defaults."""
    if SCHEDULE_FILE.exists():
        with open(SCHEDULE_FILE) as f:
            return json.load(f)
    # Create default
    save_schedules(DEFAULT_SCHEDULES)
    return DEFAULT_SCHEDULES


def save_schedules(schedules):
    """Save schedule config to JSON file."""
    with open(SCHEDULE_FILE, 'w') as f:
        json.dump(schedules, f, indent=2)


def setup_schedules(schedules):
    """Register all enabled schedules with the scheduler."""
    schedule.clear()

    # Always run heartbeat every 5 minutes
    schedule.every(5).minutes.do(heartbeat)
    log.info('Heartbeat: every 5 minutes')

    for s in schedules:
        if not s.get('enabled', False):
            continue

        name = s['name']
        interval = s.get('interval', '1h')

        if interval.endswith('m'):
            minutes = int(interval[:-1])
            schedule.every(minutes).minutes.do(execute_runbook, name)
            log.info(f'Scheduled: {name} every {minutes}m')
        elif interval.endswith('h'):
            hours = int(interval[:-1])
            schedule.every(hours).hours.do(execute_runbook, name)
            log.info(f'Scheduled: {name} every {hours}h')
        elif interval == 'daily':
            at_time = s.get('time', '00:00')
            schedule.every().day.at(at_time).do(execute_runbook, name)
            log.info(f'Scheduled: {name} daily at {at_time}')
        elif interval == 'weekly':
            schedule.every().week.do(execute_runbook, name)
            log.info(f'Scheduled: {name} weekly')


def add_schedule(name, interval, time_str=None):
    """Add a new scheduled job."""
    schedules = load_schedules()
    # Remove existing with same name
    schedules = [s for s in schedules if s['name'] != name]
    entry = {'name': name, 'interval': interval, 'enabled': True}
    if time_str:
        entry['time'] = time_str
    schedules.append(entry)
    save_schedules(schedules)
    return entry


def remove_schedule(name):
    """Remove a scheduled job."""
    schedules = load_schedules()
    schedules = [s for s in schedules if s['name'] != name]
    save_schedules(schedules)


def main():
    args = sys.argv[1:]

    if '--list' in args:
        schedules = load_schedules()
        print(f'Scheduled jobs ({len(schedules)}):')
        for s in schedules:
            status = 'ENABLED' if s.get('enabled') else 'disabled'
            print(f'  {s["name"]}: {s.get("interval", "?")} [{status}]')
        return

    if '--run-once' in args:
        idx = args.index('--run-once')
        name = args[idx + 1] if idx + 1 < len(args) else None
        if not name:
            print('Usage: --run-once <runbook-name>')
            sys.exit(1)
        success = execute_runbook(name)
        sys.exit(0 if success else 1)

    # Daemon mode
    log.info('============================================')
    log.info('ZHENAI SCHEDULER — Proactive Operations')
    log.info('============================================')

    schedules = load_schedules()
    setup_schedules(schedules)

    log.info(f'Running {len(schedule.get_jobs())} jobs. Ctrl+C to stop.')

    try:
        while True:
            schedule.run_pending()
            time.sleep(10)
    except KeyboardInterrupt:
        log.info('Scheduler stopped.')


if __name__ == '__main__':
    main()
