#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
#
# cerydwyn-daemon.py — Background daemon for the Cerydwyn
#
# Capabilities:
#   - Monitors Lich crash reports and auto-generates threat assessments
#   - Watches Dark Mirror alerts and provides analysis
#   - Logs suggestions to /opt/tomb/cerydwyn/suggestions/
#   - Runs continuously in the background
#
# Usage:
#   python3 cerydwyn-daemon.py &
#   python3 cerydwyn-daemon.py --config /opt/tomb/cerydwyn/config/rag-config.json
#   python3 cerydwyn-daemon.py --dry-run    # Show what would be analyzed
#
# Systemd:
#   [Service]
#   ExecStart=/usr/bin/python3 /opt/tomb/cerydwyn/cerydwyn-daemon.py
#   Restart=on-failure

import hashlib
import json
import logging
import os
import signal
import sys
import time
from datetime import datetime

# --- Configuration ---
CERYDWYN_ROOT = os.environ.get("CERYDWYN_ROOT", "/opt/tomb/cerydwyn")
GRIMOIRE_ROOT = os.environ.get("GRIMOIRE_ROOT", "/opt/tomb/grimoire")
OLLAMA_URL = os.environ.get("OLLAMA_URL", "http://127.0.0.1:11434")
CERYDWYN_MODEL = os.environ.get("CERYDWYN_MODEL", "cerydwyn-mistral")

LICH_CRASHES_DIR = os.environ.get("LICH_CRASHES_DIR", "/opt/tomb/lich/crashes")
LICH_CAMPAIGNS_DIR = os.environ.get("LICH_CAMPAIGNS_DIR", "/opt/tomb/lich/campaigns")
DARK_MIRROR_ALERTS_DIR = os.environ.get("DARK_MIRROR_ALERTS_DIR", "/opt/tomb/dark-mirror/alerts")
SUGGESTIONS_DIR = os.path.join(CERYDWYN_ROOT, "suggestions")
PROCESSED_DB = os.path.join(CERYDWYN_ROOT, "config", "processed-items.json")
SYSTEM_PROMPT_FILE = os.path.join(CERYDWYN_ROOT, "prompts", "system-cerydwyn.txt")

# Polling intervals (seconds)
CRASH_POLL_INTERVAL = 30
ALERT_POLL_INTERVAL = 60
CAMPAIGN_POLL_INTERVAL = 120
HEARTBEAT_INTERVAL = 300

# --- Logging ---
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [cerydwyn-daemon] %(levelname)s %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
    handlers=[
        logging.StreamHandler(sys.stdout),
        logging.FileHandler(
            os.path.join(CERYDWYN_ROOT, "logs", "cerydwyn-daemon.log"),
            mode="a",
        ) if os.path.isdir(os.path.join(CERYDWYN_ROOT, "logs")) else logging.StreamHandler(),
    ],
)
log = logging.getLogger("cerydwyn-daemon")


class ProcessedTracker:
    """Track which items have already been analyzed to avoid duplicates."""

    def __init__(self, db_path):
        self.db_path = db_path
        self.processed = self._load()

    def _load(self):
        try:
            with open(self.db_path, "r") as f:
                return json.load(f)
        except (FileNotFoundError, json.JSONDecodeError):
            return {"crashes": {}, "alerts": {}, "campaigns": {}}

    def _save(self):
        os.makedirs(os.path.dirname(self.db_path), exist_ok=True)
        with open(self.db_path, "w") as f:
            json.dump(self.processed, f, indent=2)

    def is_processed(self, category, item_hash):
        return item_hash in self.processed.get(category, {})

    def mark_processed(self, category, item_hash, metadata=None):
        if category not in self.processed:
            self.processed[category] = {}
        self.processed[category][item_hash] = {
            "timestamp": datetime.now().isoformat(),
            "metadata": metadata or {},
        }
        self._save()


def file_hash(filepath):
    """Compute SHA-256 hash of a file for deduplication."""
    h = hashlib.sha256()
    try:
        with open(filepath, "rb") as f:
            for chunk in iter(lambda: f.read(8192), b""):
                h.update(chunk)
        return h.hexdigest()[:16]
    except OSError:
        return None


def load_system_prompt():
    """Load the Cerydwyn system prompt."""
    try:
        with open(SYSTEM_PROMPT_FILE, "r") as f:
            return f.read().strip()
    except FileNotFoundError:
        return "You are a security analyst for the Unheaded Kingdom."


def query_ollama(prompt, system_prompt=""):
    """Send a query to Ollama and return the response."""
    try:
        import urllib.request

        payload = {
            "model": CERYDWYN_MODEL,
            "prompt": prompt,
            "stream": False,
        }
        if system_prompt:
            payload["system"] = system_prompt

        data = json.dumps(payload).encode("utf-8")
        req = urllib.request.Request(
            f"{OLLAMA_URL}/api/generate",
            data=data,
            headers={"Content-Type": "application/json"},
        )
        with urllib.request.urlopen(req, timeout=180) as resp:
            body = json.loads(resp.read().decode("utf-8"))
            return body.get("response", "")
    except Exception as e:
        log.error("Ollama query failed: %s", e)
        return None


def write_suggestion(category, title, content, metadata=None):
    """Write a suggestion to the suggestions directory."""
    os.makedirs(SUGGESTIONS_DIR, exist_ok=True)

    timestamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    filename = f"{timestamp}-{category}-{title[:40].replace(' ', '-').replace('/', '_')}.md"
    filepath = os.path.join(SUGGESTIONS_DIR, filename)

    lines = [
        f"# Cerydwyn Suggestion: {title}",
        f"**Category:** {category}",
        f"**Generated:** {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
        f"**Model:** {CERYDWYN_MODEL}",
        "",
    ]

    if metadata:
        lines.append("## Metadata")
        for k, v in metadata.items():
            lines.append(f"- **{k}:** {v}")
        lines.append("")

    lines.append("## Analysis")
    lines.append("")
    lines.append(content)

    with open(filepath, "w") as f:
        f.write("\n".join(lines))

    log.info("Suggestion written: %s", filepath)

    # Update the latest.log symlink/pointer
    latest_path = os.path.join(SUGGESTIONS_DIR, "latest.log")
    try:
        with open(latest_path, "a") as f:
            f.write(f"[{datetime.now().isoformat()}] {category}: {title} -> {filepath}\n")
    except OSError:
        pass

    return filepath


# --- Watchers ---

class CrashWatcher:
    """Monitor Lich crash reports directory for new crashes."""

    def __init__(self, tracker):
        self.tracker = tracker
        self.system_prompt = load_system_prompt()

    def scan(self):
        """Scan crash directories for unprocessed crash files."""
        if not os.path.isdir(LICH_CRASHES_DIR):
            return

        crash_categories = ["rce", "dos", "memory-leak", "info-leak", "unknown"]

        for category in crash_categories:
            category_dir = os.path.join(LICH_CRASHES_DIR, category)
            if not os.path.isdir(category_dir):
                continue

            for entry in os.listdir(category_dir):
                filepath = os.path.join(category_dir, entry)
                if not os.path.isfile(filepath):
                    continue

                fhash = file_hash(filepath)
                if not fhash or self.tracker.is_processed("crashes", fhash):
                    continue

                log.info("New crash detected: %s/%s", category, entry)
                self.analyze_crash(filepath, category, entry, fhash)

    def analyze_crash(self, filepath, category, filename, fhash):
        """Generate a threat assessment for a crash report."""
        try:
            with open(filepath, "r", errors="replace") as f:
                crash_content = f.read(8192)  # Limit to 8KB
        except OSError as e:
            log.error("Cannot read crash file %s: %s", filepath, e)
            return

        prompt = f"""Analyze this Lich fuzzing crash report. The crash was categorized as: {category}

Crash file: {filename}
Crash content:
```
{crash_content}
```

Provide:
1. Root cause analysis (what input triggered the crash)
2. Exploitability assessment (RCE, DoS, info leak?)
3. Affected Kingdom service and component
4. CVSS 3.1 score estimate
5. MITRE ATT&CK TTP mapping
6. CWE classification
7. Recommended fix or mitigation
8. Priority (P0 critical / P1 high / P2 medium / P3 low)"""

        response = query_ollama(prompt, self.system_prompt)
        if response:
            write_suggestion(
                category=f"crash-{category}",
                title=f"Crash Analysis: {filename}",
                content=response,
                metadata={
                    "source_file": filepath,
                    "crash_category": category,
                    "file_hash": fhash,
                },
            )
            self.tracker.mark_processed("crashes", fhash, {"file": filepath, "category": category})
        else:
            log.warning("No Cerydwyn response for crash: %s", filepath)


class AlertWatcher:
    """Monitor Dark Mirror alert firing state."""

    def __init__(self, tracker):
        self.tracker = tracker
        self.system_prompt = load_system_prompt()
        self.prometheus_url = "http://127.0.0.1:9090"

    def scan(self):
        """Check Prometheus for firing alerts."""
        try:
            import urllib.request

            req = urllib.request.Request(f"{self.prometheus_url}/api/v1/alerts")
            with urllib.request.urlopen(req, timeout=10) as resp:
                body = json.loads(resp.read().decode("utf-8"))

            alerts = body.get("data", {}).get("alerts", [])
            firing = [a for a in alerts if a.get("state") == "firing"]

            for alert in firing:
                alert_name = alert.get("labels", {}).get("alertname", "unknown")
                alert_hash = hashlib.sha256(
                    json.dumps(alert, sort_keys=True).encode()
                ).hexdigest()[:16]

                if self.tracker.is_processed("alerts", alert_hash):
                    continue

                log.info("New firing alert: %s", alert_name)
                self.analyze_alert(alert, alert_name, alert_hash)

        except Exception as e:
            # Prometheus may not be running; this is expected during startup
            log.debug("Alert scan skipped (Prometheus not reachable): %s", e)

    def scan_alert_rules(self):
        """Scan alert rule files for new or modified rules."""
        if not os.path.isdir(DARK_MIRROR_ALERTS_DIR):
            return

        for entry in os.listdir(DARK_MIRROR_ALERTS_DIR):
            if not entry.endswith((".yml", ".yaml")):
                continue

            filepath = os.path.join(DARK_MIRROR_ALERTS_DIR, entry)
            fhash = file_hash(filepath)
            if not fhash or self.tracker.is_processed("alerts", f"rule-{fhash}"):
                continue

            # Mark as processed but do not generate analysis for rule files
            self.tracker.mark_processed("alerts", f"rule-{fhash}", {"file": filepath})

    def analyze_alert(self, alert, alert_name, alert_hash):
        """Generate analysis for a firing alert."""
        annotations = alert.get("annotations", {})
        labels = alert.get("labels", {})

        prompt = f"""A Dark Mirror alert is firing in the Kingdom monitoring system.

Alert: {alert_name}
Severity: {labels.get('severity', 'unknown')}
Summary: {annotations.get('summary', 'No summary')}
Description: {annotations.get('description', 'No description')}
Labels: {json.dumps(labels, indent=2)}

Analyze this alert from a security perspective:
1. Is this alert indicative of an active attack or a misconfiguration?
2. What Kingdom services are affected?
3. What MITRE ATT&CK techniques could produce this behavior?
4. What immediate actions should the operator take?
5. What evidence should be preserved for forensic analysis?
6. Is this correlated with any recent Lich campaign activity?"""

        response = query_ollama(prompt, self.system_prompt)
        if response:
            write_suggestion(
                category="alert-analysis",
                title=f"Alert: {alert_name}",
                content=response,
                metadata={
                    "alert_name": alert_name,
                    "severity": labels.get("severity", "unknown"),
                    "alert_hash": alert_hash,
                },
            )
            self.tracker.mark_processed("alerts", alert_hash, {
                "name": alert_name,
                "severity": labels.get("severity"),
            })


class CampaignWatcher:
    """Monitor Lich campaign logs for completed campaigns."""

    def __init__(self, tracker):
        self.tracker = tracker
        self.system_prompt = load_system_prompt()

    def scan(self):
        """Scan campaign results for unprocessed campaigns."""
        results_dir = os.path.join(LICH_CAMPAIGNS_DIR, "..", "campaign-results")
        if not os.path.isdir(results_dir):
            # Try alternate location
            results_dir = os.path.join(os.path.dirname(LICH_CRASHES_DIR), "campaign-results")
            if not os.path.isdir(results_dir):
                return

        for entry in sorted(os.listdir(results_dir)):
            campaign_dir = os.path.join(results_dir, entry)
            if not os.path.isdir(campaign_dir):
                continue

            # Use directory name as hash key
            campaign_hash = hashlib.sha256(entry.encode()).hexdigest()[:16]
            if self.tracker.is_processed("campaigns", campaign_hash):
                continue

            log.info("New campaign result: %s", entry)
            self.analyze_campaign(campaign_dir, entry, campaign_hash)

    def analyze_campaign(self, campaign_dir, campaign_name, campaign_hash):
        """Generate analysis for a completed campaign."""
        # Gather campaign artifacts
        artifacts = []
        for root, dirs, files in os.walk(campaign_dir):
            for f in files[:20]:  # Limit files examined
                fpath = os.path.join(root, f)
                try:
                    with open(fpath, "r", errors="replace") as fh:
                        content = fh.read(2048)
                    artifacts.append(f"--- {f} ---\n{content}")
                except OSError:
                    continue

        if not artifacts:
            return

        combined = "\n\n".join(artifacts[:10])

        prompt = f"""A Lich campaign has completed. Analyze the results.

Campaign: {campaign_name}
Artifacts found: {len(artifacts)}

Campaign data:
```
{combined[:6000]}
```

Provide:
1. Campaign summary (what was tested, what was the objective)
2. Key findings (vulnerabilities discovered, attack success/failure)
3. Crash analysis summary (if crashes were found)
4. Kingdom services impacted
5. Severity assessment and risk rating
6. Remediation recommendations prioritized by risk
7. Suggested follow-up campaigns"""

        response = query_ollama(prompt, self.system_prompt)
        if response:
            write_suggestion(
                category="campaign-analysis",
                title=f"Campaign: {campaign_name}",
                content=response,
                metadata={
                    "campaign_dir": campaign_dir,
                    "campaign_name": campaign_name,
                    "artifacts_count": len(artifacts),
                },
            )
            self.tracker.mark_processed("campaigns", campaign_hash, {
                "name": campaign_name,
                "dir": campaign_dir,
            })


# --- Daemon Main Loop ---

class CerydwynDaemon:
    """Main daemon orchestrating all watchers."""

    def __init__(self, dry_run=False):
        self.dry_run = dry_run
        self.running = True
        self.tracker = ProcessedTracker(PROCESSED_DB)
        self.crash_watcher = CrashWatcher(self.tracker)
        self.alert_watcher = AlertWatcher(self.tracker)
        self.campaign_watcher = CampaignWatcher(self.tracker)
        self.last_crash_scan = 0
        self.last_alert_scan = 0
        self.last_campaign_scan = 0
        self.last_heartbeat = 0

        # Signal handlers
        signal.signal(signal.SIGTERM, self._handle_signal)
        signal.signal(signal.SIGINT, self._handle_signal)

    def _handle_signal(self, signum, frame):
        log.info("Received signal %d, shutting down gracefully...", signum)
        self.running = False

    def check_ollama(self):
        """Verify Ollama is reachable."""
        try:
            import urllib.request

            req = urllib.request.Request(f"{OLLAMA_URL}/api/tags")
            with urllib.request.urlopen(req, timeout=5) as resp:
                return resp.status == 200
        except Exception:
            return False

    def heartbeat(self):
        """Log a heartbeat with daemon status."""
        log.info(
            "Heartbeat | crashes_processed=%d alerts_processed=%d campaigns_processed=%d",
            len(self.tracker.processed.get("crashes", {})),
            len(self.tracker.processed.get("alerts", {})),
            len(self.tracker.processed.get("campaigns", {})),
        )

    def run(self):
        """Main loop."""
        log.info("Cerydwyn daemon starting...")
        log.info("  CERYDWYN_ROOT: %s", CERYDWYN_ROOT)
        log.info("  OLLAMA_URL: %s", OLLAMA_URL)
        log.info("  CERYDWYN_MODEL: %s", CERYDWYN_MODEL)
        log.info("  LICH_CRASHES_DIR: %s", LICH_CRASHES_DIR)
        log.info("  DARK_MIRROR_ALERTS_DIR: %s", DARK_MIRROR_ALERTS_DIR)
        log.info("  SUGGESTIONS_DIR: %s", SUGGESTIONS_DIR)

        # Ensure directories exist
        os.makedirs(SUGGESTIONS_DIR, exist_ok=True)
        os.makedirs(os.path.join(CERYDWYN_ROOT, "logs"), exist_ok=True)

        # Initial Ollama health check
        if not self.check_ollama():
            log.warning("Ollama not reachable at %s — daemon will retry on each scan", OLLAMA_URL)

        if self.dry_run:
            log.info("DRY RUN mode — showing what would be scanned")
            self._dry_run_scan()
            return

        while self.running:
            now = time.time()

            # Crash scan
            if now - self.last_crash_scan >= CRASH_POLL_INTERVAL:
                try:
                    self.crash_watcher.scan()
                except Exception as e:
                    log.error("Crash scan error: %s", e)
                self.last_crash_scan = now

            # Alert scan
            if now - self.last_alert_scan >= ALERT_POLL_INTERVAL:
                try:
                    self.alert_watcher.scan()
                    self.alert_watcher.scan_alert_rules()
                except Exception as e:
                    log.error("Alert scan error: %s", e)
                self.last_alert_scan = now

            # Campaign scan
            if now - self.last_campaign_scan >= CAMPAIGN_POLL_INTERVAL:
                try:
                    self.campaign_watcher.scan()
                except Exception as e:
                    log.error("Campaign scan error: %s", e)
                self.last_campaign_scan = now

            # Heartbeat
            if now - self.last_heartbeat >= HEARTBEAT_INTERVAL:
                self.heartbeat()
                self.last_heartbeat = now

            # Sleep briefly between iterations
            time.sleep(5)

        log.info("Cerydwyn daemon stopped.")

    def _dry_run_scan(self):
        """Show what files would be processed without actually analyzing them."""
        log.info("=== Crash files ===")
        if os.path.isdir(LICH_CRASHES_DIR):
            for cat in ["rce", "dos", "memory-leak", "info-leak", "unknown"]:
                cat_dir = os.path.join(LICH_CRASHES_DIR, cat)
                if os.path.isdir(cat_dir):
                    for f in os.listdir(cat_dir):
                        fpath = os.path.join(cat_dir, f)
                        fh = file_hash(fpath)
                        processed = self.tracker.is_processed("crashes", fh) if fh else "N/A"
                        log.info("  [%s] %s/%s (hash=%s, processed=%s)", cat, cat, f, fh, processed)
        else:
            log.info("  (directory not found: %s)", LICH_CRASHES_DIR)

        log.info("=== Alert rules ===")
        if os.path.isdir(DARK_MIRROR_ALERTS_DIR):
            for f in os.listdir(DARK_MIRROR_ALERTS_DIR):
                log.info("  %s", f)
        else:
            log.info("  (directory not found: %s)", DARK_MIRROR_ALERTS_DIR)

        log.info("=== Processed counts ===")
        for cat, items in self.tracker.processed.items():
            log.info("  %s: %d items", cat, len(items))


def main():
    dry_run = "--dry-run" in sys.argv
    config_file = None

    for i, arg in enumerate(sys.argv[1:], 1):
        if arg == "--config" and i < len(sys.argv) - 1:
            config_file = sys.argv[i + 1]
        elif arg in ("--help", "-h"):
            print("Usage: cerydwyn-daemon.py [--dry-run] [--config PATH]")
            print()
            print("Options:")
            print("  --dry-run    Show what would be analyzed without querying Cerydwyn")
            print("  --config     Path to rag-config.json (default: /opt/tomb/cerydwyn/config/rag-config.json)")
            print()
            print("Environment:")
            print("  OLLAMA_URL             Ollama endpoint (default: http://127.0.0.1:11434)")
            print("  CERYDWYN_MODEL           Model name (default: cerydwyn-mistral)")
            print("  CERYDWYN_ROOT            Cerydwyn root (default: /opt/tomb/cerydwyn)")
            print("  LICH_CRASHES_DIR       Lich crashes dir (default: /opt/tomb/lich/crashes)")
            print("  DARK_MIRROR_ALERTS_DIR Alert rules dir (default: /opt/tomb/dark-mirror/alerts)")
            sys.exit(0)

    if config_file and os.path.isfile(config_file):
        with open(config_file, "r") as f:
            cfg = json.load(f)
        # Override globals from config
        global OLLAMA_URL, CERYDWYN_MODEL, GRIMOIRE_ROOT
        OLLAMA_URL = cfg.get("ollama_url", OLLAMA_URL)
        CERYDWYN_MODEL = cfg.get("model", CERYDWYN_MODEL)
        GRIMOIRE_ROOT = cfg.get("grimoire_root", GRIMOIRE_ROOT)

    daemon = CerydwynDaemon(dry_run=dry_run)
    daemon.run()


if __name__ == "__main__":
    main()
