# The Tomb of Knowledge — BlackMage Reference

> *For without knowing fear, hardship, and chaos — one cannot know harmony, peace, and just ethos.*

## Overview

The Tomb of Knowledge is the physical phylactery of the BlackMage—the Lich's seat of power and the Kingdom's primary instrument for discovering truths hidden in shadow. It is a 14.5GB Kali Linux live ISO running in QEMU on the Raft PC, completely air-gapped from the public internet yet woven into every layer of the Kingdom's security consciousness.

The Tomb operates at the intersection of automation, knowledge, and observation. It combines:
- **600+ native Kali security tools** (The Arsenal)
- **Custom adversary frameworks** (The Lich, LICH-001 through LICH-010)
- **The collective intelligence of the Kingdom** (The Grimoire: docs, MITRE ATT&CK, NVD CVE database)
- **Local AI reasoning** (The Oracle: Ollama/Mistral-7B with RAG retrieval)
- **External observability** (The Dark Mirror: Prometheus + Grafana + Loki monitoring the Kingdom from outside)

---

## What It Is

### Core Properties

| Property | Value |
|----------|-------|
| **Disk Image Size** | 14.5GB Kali Linux live ISO |
| **Hypervisor** | QEMU |
| **Host** | Raft PC (192.168.13.2) |
| **Access Method** | Serial console via `-nographic -serial mon:stdio` |
| **Persistence** | qcow2 disk image |
| **Internet Access** | None (air-gapped) |
| **Kingdom Connectivity** | 192.168.13.0/30 point-to-point link to Kingdom (192.168.13.1) |
| **Overlay Network** | WireGuard fd00:dead:beef::/48 |

### Philosophical Position

The Tomb sits at the threshold between the Kingdom and the void. It is not merely a tool—it is a mirror held up to an attacker's perspective. By inhabiting the mindset of those who would exploit weaknesses, the BlackMage helps the Kingdom understand itself more deeply.

---

## Architecture: Five Layers of Power

### Layer 1: The Arsenal

**Purpose:** Native attack and analysis capabilities

The Arsenal is Kali Linux itself—600+ pre-installed security tools curated for penetration testing, vulnerability assessment, wireless analysis, and exploit development.

**Key Tools (sampling):**
- **Reconnaissance:** nmap, fierce, theHarvester, shodan-cli
- **Vulnerability Scanning:** OpenVAS, nessus, nikto, Masscan
- **Web Assessment:** Burp Suite, OWASP ZAP, sqlmap, w3af
- **Wireless:** aircrack-ng, Wireshark, kismet, hostapd
- **Reverse Engineering:** Ghidra, radare2, IDA Free, objdump
- **Fuzzing:** AFL++, LibFuzzer, honggfuzz, peach-fuzzer
- **Exploitation:** Metasploit Framework, ExifTool, searchsploit
- **Post-Exploit:** Mimikatz (via Wine), Empire, Covenant
- **Credential Testing:** hydra, hashcat, john, medusa
- **Forensics:** volatility, autopsy, sleuthkit, bulk_extractor

**Key File Paths:**
```
/usr/bin/           — Kali binaries and scripts
/usr/share/metasploit-framework/  — MSF data
/opt/arsenal/       — Custom Kali extensions
```

**Access Pattern:** The Arsenal is the foundation. Every attack begins here.

---

### Layer 2: The Lich

**Purpose:** Coordinated, reproducible adversary campaigns and fuzz harnesses

The Lich is a custom adversary framework that transforms isolated tools into orchestrated attack campaigns. It defines attack flows, captures success conditions, and integrates with crash triage and reporting.

**LICH Modules (LICH-001 through LICH-010):**

| Module | Purpose | Key Harness |
|--------|---------|------------|
| **LICH-001** | Network reconnaissance | nmap orchestration + asset database |
| **LICH-002** | Web vulnerability discovery | Burp + custom payload generation |
| **LICH-003** | Wireless network assessment | aircrack-ng + rogue AP harness |
| **LICH-004** | Social engineering simulation | phishing campaign harness |
| **LICH-005** | Credential stuffing & brute force | hydra + distributed attack harness |
| **LICH-006** | Privilege escalation hunting | linpeas + Windows privesc harness |
| **LICH-007** | Persistence mechanism planting | backdoor deployment harness |
| **LICH-008** | Data exfiltration simulation | channel capacity testing harness |
| **LICH-009** | Malware fuzzing & mutation | AFL++ harness for binary discovery |
| **LICH-010** | Lateral movement simulation | network pivot harness |

**Crash Triage Pipeline:**
- `crash-triage.sh` analyzes fuzzing crashes
- Categorizes by type: DoS, memory leak, RCE, logic bug
- Generates crash reports with reproducible inputs
- Feeds high-confidence RCE crashes to attack-report.sh

**Key File Paths:**
```
/opt/tomb/lich/                    — Lich root
/opt/tomb/lich/campaigns/          — Campaign definitions (LICH-001, etc.)
/opt/tomb/lich/harnesses/          — Fuzzing & automation harnesses
/opt/tomb/lich/crash-triage/       — Crash categorization engine
/opt/tomb/lich/crash-triage/corpus/  — Fuzzing corpus
/opt/tomb/lich/crash-triage/crashes/  — Classified crashes
```

**Access Pattern:** The Lich converts tool capabilities into measurable attack success. It is the muscle of the BlackMage.

---

### Layer 3: The Grimoire

**Purpose:** Unified knowledge base: Kingdom documentation, threat intelligence, CVE intelligence

The Grimoire is a comprehensive offline knowledge repository. It includes:
- All Kingdom architecture, deployment, and configuration documentation
- MITRE ATT&CK framework (tactics, techniques, procedures)
- NVD (National Vulnerability Database) CVE records
- CWE (Common Weakness Enumeration) mappings
- Custom Kingdom threat models and architecture notes
- Historical attack reports and findings
- Lessons learned and remediation guidance

**Knowledge Sources:**
- Kingdom internal docs (cloned from git)
- MITRE ATT&CK JSON datasets (tactics-techniques-procedures)
- NVD CVE feeds (updated as of Tomb build date)
- CVSS scoring and impact assessments
- Custom Kingdom threat models

**Key File Paths:**
```
/opt/tomb/grimoire/                — Grimoire root
/opt/tomb/grimoire/kingdom-docs/   — Kingdom internal documentation
/opt/tomb/grimoire/mitre-attck/    — MITRE ATT&CK data (JSON)
/opt/tomb/grimoire/nvd-cves/       — NVD CVE records
/opt/tomb/grimoire/cwe/            — CWE mappings
/opt/tomb/grimoire/threat-models/  — Kingdom threat models
/opt/tomb/grimoire/history/        — Historical findings & reports
/opt/tomb/grimoire/rag/            — RAG index (ChromaDB + embeddings)
```

**RAG Infrastructure:**
- **ChromaDB:** Vector database storing embeddings
- **sentence-transformers:** Pre-trained embedding model (e.g., all-MiniLM-L6-v2)
- **Index Content:** Every document in the Grimoire is chunked, embedded, and indexed
- **Query Interface:** `rag-query.py` retrieves relevant documents by semantic similarity

**Access Pattern:** The Grimoire is consulted before every attack. It prevents reinvention and ensures attacks are informed by both Kingdom knowledge and external threat intelligence.

---

### Layer 4: The Oracle

**Purpose:** Local LLM with RAG pipeline for interactive AI-driven threat modeling and attack planning

The Oracle is the BlackMage's voice of reason. It combines:
- **Local LLM:** Ollama running Mistral-7B (7 billion parameters, runs on Raft PC CPU/GPU)
- **RAG Pipeline:** Queries the Grimoire, retrieves relevant knowledge, augments LLM context
- **Interactive TUI:** Serial console interface for real-time queries
- **Background Daemon:** Continuous monitoring and suggestion generation

**Key Capabilities:**
- **Threat Modeling:** "Given this architecture, what are the top 5 attack paths?"
- **Vulnerability Assessment:** "Explain this CVE in context of our Kubernetes cluster"
- **Attack Planning:** "Design a campaign against SQL injection vulnerabilities"
- **Remediation Guidance:** "What are the best practices to mitigate this CWE?"
- **Intelligence Synthesis:** "Compare this finding to similar historical attacks"

**LLM Model Details:**
```
Model:           Mistral-7B (OpenAI-compatible)
Parameters:      7B
Context Window:  32,000 tokens
Quantization:    Q4 (4-bit, ~4GB memory)
Inference Speed: ~10 tokens/sec on Raft PC CPU
```

**RAG Pipeline Flow:**
```
User Query
    ↓
sentence-transformers (embed query)
    ↓
ChromaDB (vector similarity search → top-k documents)
    ↓
LLM Context (augment prompt with retrieved Grimoire excerpts)
    ↓
Ollama/Mistral-7B (generate response)
    ↓
User (via TUI or API)
```

**Key File Paths:**
```
/opt/tomb/oracle/                  — Oracle root
/opt/tomb/oracle/ollama/           — Ollama config & model cache
/opt/tomb/oracle/ollama/models/    — Mistral-7B model files
/opt/tomb/oracle/rag-config.json   — RAG pipeline configuration
/opt/tomb/oracle/logs/             — Oracle query logs & traces
/opt/tomb/oracle/suggestions/      — Autonomous suggestions log
```

**Key Scripts:**
- `rag-query.py` — Retrieve documents from ChromaDB by semantic search
- `rag-oracle.py` — Full RAG+LLM pipeline (query → retrieval → generation)
- `oracle-tui.py` — Interactive terminal UI for serial console
- `oracle-daemon.py` — Background service generating suggestions

**Access Pattern:** The Oracle is consulted for strategy and context. It is the BlackMage's mind—able to see patterns across time and campaigns.

---

### Layer 5: The Dark Mirror

**Purpose:** External observability of the Kingdom from an untrusted perspective

The Dark Mirror is an observability stack running inside the Tomb that monitors the Kingdom from the outside. It provides:
- **Prometheus:** Metrics collection and time-series storage
- **Grafana:** Visualization and dashboard composition
- **Loki:** Log aggregation and search (from Lich campaigns, oracle traces)
- **Alerting:** Rules for detecting anomalies during attacks

**What It Monitors:**
- **Network Topology:** ARP/DNS changes, unexpected connections
- **Service Availability:** Response times, error rates (from attack perspective)
- **Security Events:** Intrusion detection, firewall blocks
- **Application Health:** Latency, error rates, resource consumption
- **Lich Campaign Metrics:** Attack success rate, crash counts, data exfiltration volume

**Key Metrics:**
```
lich_campaigns_total          — Count of Lich campaigns executed
lich_crashes_discovered       — Total fuzzing crashes found
lich_rce_confirmed            — Crashes verified as RCE-capable
lich_attack_success_rate      — Percentage of attacks reaching objective
kingdom_service_latency_p99   — 99th percentile response time
kingdom_error_rate_5xx        — 5xx error rate (as seen from Tomb)
kingdom_firewall_blocks       — IDS/IPS signatures triggered during attacks
tomb_persistence_disk_usage   — Space used in qcow2 image
```

**Key File Paths:**
```
/opt/tomb/dark-mirror/             — Dark Mirror root
/opt/tomb/dark-mirror/prometheus/  — Prometheus config & data
/opt/tomb/dark-mirror/grafana/     — Grafana dashboards & datasources
/opt/tomb/dark-mirror/loki/        — Loki config & logs
/opt/tomb/dark-mirror/alerts/      — Alerting rules
```

**Access Pattern:** The Dark Mirror shows what the Kingdom cannot see about itself—how it appears under attack, what breaks, where defenses fail.

---

## Network Topology

### Physical Layout

```
Public Internet
    |
    X (air-gap firewall)
    |
Kingdom Network (192.168.0.0/16)
    |
    ├─ WEST (192.168.13.1)      [Developer, Architect, Micromanager]
    │
    └─ Raft PC (192.168.13.2)
           |
           └─ QEMU Bridge/NAT
                  |
                  └─ Tomb VM (192.168.13.X)  [BlackMage]
```

### Addressing Scheme

**IPv4 (Point-to-Point Link):**
```
Network:      192.168.13.0/30
WEST:         192.168.13.1
Raft PC:      192.168.13.2
Tomb VM:      192.168.13.3 (or routed through Raft PC)
Broadcast:    192.168.13.3
Prefix:       /30 (4 addresses, 2 usable)
```

**IPv6 (WireGuard Overlay):**
```
Network:      fd00:dead:beef::/48
WEST:         fd00:dead:beef::1
Raft PC:      fd00:dead:beef::2
Tomb VM:      fd00:dead:beef::3
```

The WireGuard overlay provides encrypted, routed connectivity independent of the physical network topology.

### QEMU Networking Options

**Option A: TAP Bridge**
```bash
# Raft PC creates tap0, bridges to Kingdom network
qemu-system-x86_64 \
  -netdev tap,id=net0,ifname=tap0,script=no \
  -device virtio-net-pci,netdev=net0 \
  -nographic -serial mon:stdio
```

**Option B: User Networking (Slirp)**
```bash
# Simpler, no tap0 setup required
qemu-system-x86_64 \
  -netdev user,id=net0,hostfwd=tcp:192.168.13.2:2222-:22 \
  -device virtio-net-pci,netdev=net0 \
  -nographic -serial mon:stdio
```

**Option C: WireGuard Overlay Only**
```bash
# Tomb connects exclusively via WireGuard tunnel
# Public internet air-gap maintained at network layer
```

### Data Flow During Attacks

1. **Reconnaissance:** Tomb sends probes toward Kingdom targets via 192.168.13.0/30
2. **Attack Execution:** Lich campaigns interact with Kingdom services
3. **Metric Collection:** Dark Mirror scrapes Kingdom Prometheus endpoints (if exposed) or uses packet inspection
4. **Log Aggregation:** Loki ingests Lich campaign logs and oracle traces
5. **Report Generation:** attack-report.sh synthesizes findings into canonical report

---

## Key File Paths (Inside Tomb VM)

```
/opt/tomb/                          Root of all Tomb data
├── lich/                           Lich adversary framework
│   ├── campaigns/                  Campaign definitions (LICH-001 through LICH-010)
│   ├── harnesses/                  Fuzzing & automation harnesses
│   └── crash-triage/               Crash categorization engine
│       ├── corpus/                 Fuzzing corpus (seeds)
│       └── crashes/                Classified crashes (DoS, RCE, etc.)
├── grimoire/                       Knowledge base
│   ├── kingdom-docs/               Kingdom architecture & config docs
│   ├── mitre-attck/                MITRE ATT&CK framework (JSON)
│   ├── nvd-cves/                   NVD CVE records
│   ├── cwe/                        CWE mappings
│   ├── threat-models/              Kingdom threat models
│   ├── history/                    Historical findings & reports
│   └── rag/                        RAG vector index
│       ├── chroma.db/              ChromaDB vector store
│       └── embeddings/             Cached sentence-transformers embeddings
├── oracle/                         Oracle LLM + RAG pipeline
│   ├── ollama/                     Ollama service config
│   │   └── models/                 Mistral-7B model files
│   ├── rag-config.json             RAG pipeline configuration
│   ├── logs/                       Query logs & traces
│   └── suggestions/                Autonomous suggestions
├── dark-mirror/                    Observability stack
│   ├── prometheus/                 Prometheus config & tsdb
│   ├── grafana/                    Grafana dashboards
│   ├── loki/                       Loki log aggregation
│   └── alerts/                     Alerting rules
├── captures/                       Network captures
│   ├── pcaps/                      PCAP files from Lich campaigns
│   └── packet-analysis/            Parsed packet metadata
├── reports/                        Attack reports
│   ├── {campaign-id}/              Per-campaign report directory
│   │   ├── findings.json           Structured findings
│   │   ├── summary.txt             Human-readable summary
│   │   └── evidence/               Supporting evidence (logs, pcaps, crashes)
│   └── archive/                    Historical reports
└── scope.conf                      Attack scope definition (targets, rules of engagement)
```

---

## Key Scripts

### Operational Control

#### `tomb-boot.sh`
**Purpose:** Launch the Tomb VM with correct QEMU parameters

**Usage:**
```bash
./tomb-boot.sh [--cpu COUNT] [--mem GIGABYTES] [--debug]
```

**Behavior:**
- Validates qcow2 disk image
- Checks Raft PC network configuration
- Launches QEMU with `-nographic -serial mon:stdio`
- Drops user into Tomb serial console
- Monitors VM health (watchdog process)

**Exit:** Type `quit` in serial console, or Ctrl+A X

---

#### `lich-runner.sh`
**Purpose:** Execute all Lich campaigns in sequence or parallel

**Usage:**
```bash
./lich-runner.sh [--campaign LICH-001,LICH-002,...] [--parallel] [--dry-run]
```

**Behavior:**
- Reads scope.conf to validate targets are in scope
- Executes campaign harnesses
- Captures all network traffic (pcaps)
- Logs command execution and results
- Feeds crashes to crash-triage.sh
- Generates preliminary report

**Exit Code:**
- 0: All campaigns successful
- 1: Scope violation detected (attack aborted)
- 2: Campaign execution failure

---

#### `crash-triage.sh`
**Purpose:** Categorize and analyze fuzzing crashes

**Usage:**
```bash
./crash-triage.sh [--corpus PATH] [--output DIR]
```

**Behavior:**
- Runs crashing inputs through debugger (gdb)
- Extracts crash type (segfault, abort, etc.)
- Determines exploitability (RCE, DoS, info leak)
- Groups similar crashes into buckets
- Generates crash report with reproducible steps
- Confidence scoring per crash

**Output:**
```
/opt/tomb/lich/crash-triage/crashes/
├── rce/                    — Crashes with RCE potential (highest priority)
├── dos/                    — Crashes causing denial of service
├── memory-leak/            — Memory disclosure leaks
├── info-leak/              — Information disclosure
└── unknown/                — Uncategorized crashes
```

---

### Knowledge Base Access

#### `grimoire-search.sh`
**Purpose:** Full-text search across all Kingdom docs, CVE records, threat models

**Usage:**
```bash
./grimoire-search.sh "kubernetes RBAC privilege escalation"
./grimoire-search.sh --cve "CVE-2023-12345"
./grimoire-search.sh --threat-model "API gateway"
```

**Behavior:**
- Queries /opt/tomb/grimoire/* directories
- Returns matching documents with context
- Ranks by relevance
- Links to full document paths

**Integration:** Called by oracle-daemon.py before generating suggestions

---

#### `rag-query.py`
**Purpose:** Retrieve documents from ChromaDB using semantic similarity

**Usage:**
```bash
python3 rag-query.py "How do we mitigate XXE vulnerabilities?"
python3 rag-query.py --top-k 10 "SQL injection in stored procedures"
```

**Behavior:**
- Embeds query using sentence-transformers
- Queries ChromaDB for top-k nearest neighbors
- Returns documents with similarity scores
- Formats results for human reading

**Output:**
```
Chunk 1: [similarity: 0.92] [source: /opt/tomb/grimoire/kingdom-docs/api-security.md:45-60]
  "XXE vulnerabilities are commonly exploited by..."

Chunk 2: [similarity: 0.88] [source: /opt/tomb/grimoire/nvd-cves/CVE-2024-1234.json]
  "CVSS: 8.3 | A recent XXE in libxml2 allows..."
```

---

#### `rag-oracle.py`
**Purpose:** Full RAG+LLM pipeline: retrieve → augment → generate

**Usage:**
```bash
python3 rag-oracle.py "Design a red team campaign against our Kubernetes cluster"
python3 rag-oracle.py --lich-context LICH-005 "What payloads would work best?"
```

**Behavior:**
1. Embeds user query
2. Retrieves top-k documents from ChromaDB
3. Constructs LLM prompt augmented with retrieved knowledge
4. Calls Ollama/Mistral-7B to generate response
5. Streams response to stdout
6. Logs interaction to /opt/tomb/oracle/logs/

**Prompt Template:**
```
You are the BlackMage Oracle, advisor to a kingdom's security team.
You have access to the following Grimoire excerpts:

[Retrieved documents inserted here]

User Query: {user_query}

Respond with tactical, actionable guidance grounded in the Kingdom's
architecture and threat landscape. Reference specific documents when applicable.
```

---

#### `oracle-tui.py`
**Purpose:** Interactive terminal UI for serial console interaction with Oracle

**Usage:**
```bash
# Inside Tomb VM
python3 /opt/tomb/oracle/oracle-tui.py
```

**Interface:**
```
╔════════════════════════════════════════════════════════════════╗
║  The Oracle — BlackMage Interactive Counsel                    ║
╠════════════════════════════════════════════════════════════════╣
║                                                                ║
║  > What vulnerabilities should we target first?               ║
║                                                                ║
║  The Oracle is consulting the Grimoire... (⧖ 2.3s)            ║
║                                                                ║
║  Based on MITRE ATT&CK and your Kubernetes cluster:           ║
║  1. Container escape (LICH-006 recommended)                   ║
║  2. RBAC misconfiguration (LICH-005)                          ║
║  ...                                                           ║
║                                                                ║
║  > _                                                           ║
║                                                                ║
╚════════════════════════════════════════════════════════════════╝

Keyboard: Ctrl+C to exit | Enter to submit | Up/Down for history
```

**Features:**
- Multi-line query input
- Streaming response display
- Command history (Ctrl+P / Ctrl+N)
- Syntax highlighting for code blocks
- Citation links to Grimoire sources

---

#### `oracle-daemon.py`
**Purpose:** Background service for continuous threat modeling and suggestion generation

**Usage:**
```bash
# Start daemon
python3 /opt/tomb/oracle/oracle-daemon.py --config /opt/tomb/oracle/rag-config.json

# Query suggestions
tail -f /opt/tomb/oracle/suggestions/latest.log
```

**Behavior:**
- Monitors Kingdom infrastructure for changes
- Periodically queries Oracle with contextual prompts:
  - "Given the latest Kubernetes deployment, what new attack paths exist?"
  - "Are there new CVEs matching our current tech stack?"
- Generates suggestions ranked by severity
- Writes to suggestions log for human review
- Integrates with Dark Mirror (reads Prometheus metrics)

**Suggestion Output Format:**
```json
{
  "timestamp": "2025-02-28T14:23:45Z",
  "severity": "HIGH",
  "type": "new-cve-match",
  "title": "CVE-2024-XXXX affects libpq in our PostgreSQL cluster",
  "description": "...",
  "recommended_lich_campaign": "LICH-001",
  "evidence": {
    "cve_record": "...",
    "affected_component": "postgresql-client 14.2",
    "grimoire_sources": ["nvd-cves/CVE-2024-XXXX.json", "...]
  }
}
```

---

### Attack Execution & Reporting

#### `attack-preflight.sh`
**Purpose:** Safety checks before executing Lich campaigns

**Usage:**
```bash
./attack-preflight.sh --campaign LICH-005
```

**Checks:**
- Validates targets are in scope.conf
- Verifies air-gap is maintained (no public internet routing)
- Confirms persistence disk has space
- Checks Lich harness integrity (checksums)
- Verifies network connectivity to Kingdom
- Confirms Kingdom accepts incoming connections (no blocks)
- Validates Grimoire is current (checks timestamps)
- Ensures Dark Mirror is logging

**Exit Code:**
- 0: All checks pass, safe to execute
- 1: Scope violation or safety check failed (aborts campaign)

---

#### `attack-report.sh`
**Purpose:** Generate comprehensive post-attack report

**Usage:**
```bash
./attack-report.sh --campaign LICH-005 --output-dir /opt/tomb/reports/
```

**Artifacts Generated:**
```
/opt/tomb/reports/lich-005-2025-02-28-143022/
├── findings.json              — Structured findings (severity, type, evidence)
├── summary.txt                — Executive summary (1-2 pages)
├── detailed.md                — Full technical report (Markdown)
├── evidence/
│   ├── pcaps/                 — Network captures from attack
│   ├── screenshots/           — Web UI screenshots (if applicable)
│   ├── crash-details/         — Detailed crash analysis
│   └── logs/                  — Full command logs
├── severity-timeline.csv      — Findings ranked by severity & discovery time
├── remediation-guidance.md    — Specific fixes for each finding
└── lessons-learned.md         — What the Kingdom should change
```

**Report Quality Factors:**
- Correlated findings across multiple attack vectors
- CVSS scoring and impact assessment
- Remediation steps ranked by effort & impact
- Evidence trail (reproducible from pcaps + crash inputs)
- Lessons learned tied to Kingdom threat models

---

### System Health & Maintenance

#### `tomb-status.sh`
**Purpose:** Quick health check of Tomb VM and all layers

**Usage:**
```bash
./tomb-status.sh [--verbose]
```

**Output:**
```
Tomb of Knowledge — Status Report
═════════════════════════════════════════════════════════════════
Time:                         2025-02-28 14:25:00 UTC
Uptime:                       8 days, 3 hours, 22 minutes

═ Layer 1: The Arsenal ═
  Kali Tools:                  ✓ 612 tools available

═ Layer 2: The Lich ═
  LICH-001 (Recon):            ✓ Last run: 6 hours ago
  LICH-002 (Web):              ✓ Last run: 1 day ago
  ...
  Crash Triage:                ✓ 127 crashes classified

═ Layer 3: The Grimoire ═
  Kingdom Docs:                ✓ 342 documents (updated 3 days ago)
  MITRE ATT&CK:                ✓ Techniques indexed
  NVD CVEs:                    ✓ 234,521 CVE records
  RAG Index (ChromaDB):        ✓ 18,234 chunks embedded

═ Layer 4: The Oracle ═
  Ollama Service:              ✓ Running (Mistral-7B loaded)
  RAG Pipeline:                ✓ Responding (avg latency: 4.2s)
  Oracle Daemon:               ✓ Running (last suggestion: 1 hour ago)

═ Layer 5: The Dark Mirror ═
  Prometheus:                  ✓ Scraping Kingdom (5 targets)
  Grafana:                     ✓ 12 dashboards configured
  Loki:                        ✓ Ingesting logs from all layers
  Alerting:                    ✓ No active alerts

═ Persistence ═
  qcow2 Image:                 ✓ 8.3GB / 14.5GB used (57%)
  Backup Status:               ✓ Last backup: 2025-02-28 06:00 UTC

═ Network ═
  Kingdom Connectivity:        ✓ 192.168.13.1 reachable (ping < 1ms)
  WireGuard Overlay:           ✓ fd00:dead:beef::1 connected
  Air-Gap Enforcement:         ✓ No public internet routes

═ Governance ═
  Scope (scope.conf):          ✓ 47 targets defined
  Scope Violations:            ✓ None recorded
  Audit Log:                   ✓ 1,247 entries
```

---

#### `tomb-backup.sh`
**Purpose:** Backup Tomb persistence disk to Raft PC (air-gapped storage)

**Usage:**
```bash
./tomb-backup.sh [--incremental] [--compress] [--verify]
```

**Behavior:**
- Snapshots current qcow2 image
- Copies to /mnt/tomb-backups/ on Raft PC (via scp over Kingdom network)
- Verifies checksum integrity
- Rotates old backups (keeps 7 days + 4 weekly + 12 monthly)
- Encrypts with LUKS key stored on Raft PC (in TPM if available)

**Output:**
```
Backing up Tomb...
  ✓ Snapshot: /opt/tomb/qcow2/current.img → current-2025-02-28.snapshot
  ✓ Compress: gzip (3.2GB → 1.8GB in 4m 23s)
  ✓ Transfer: scp to Raft PC (8m 45s @ 3.4Mbps)
  ✓ Verify:   SHA256 checksum matches
  ✓ Encrypt:  LUKS encrypted with Raft PC TPM key
  ✓ Archive:  /mnt/tomb-backups/current-2025-02-28.img.gz.enc

Next backup: 2025-03-01 06:00 UTC
```

---

#### `scope.conf`
**Purpose:** Define attack scope and rules of engagement

**Format (YAML):**
```yaml
# scope.conf — Tomb of Knowledge Attack Scope
# Updated: 2025-02-28

metadata:
  kingdom_id: "westmarch-core"
  kingdom_network: "192.168.0.0/16"
  approval_date: "2025-02-15"
  approval_authority: "Chief Architect"
  expiration_date: "2025-05-15"

in_scope:
  networks:
    - "192.168.13.0/24"     # Directly testable zone
    - "10.0.0.0/8"          # Kubernetes cluster

  services:
    - { name: "API Gateway", port: 8443, cve_classes: ["XXE", "SSRF", "JWT"] }
    - { name: "PostgreSQL", port: 5432, cve_classes: ["SQL Injection", "Auth Bypass"] }
    - { name: "Redis", port: 6379, cve_classes: ["ACL Bypass", "RCE"] }
    - { name: "Kubernetes API", port: 6443, cve_classes: ["RBAC", "Admission Control"] }

  attack_types:
    - "network_reconnaissance"
    - "vulnerability_scanning"
    - "credential_testing"
    - "web_exploitation"
    - "privilege_escalation"
    - "persistence"
    - "lateral_movement"
    - "fuzzing"

  time_windows:
    - { day: "Tuesday", start: "22:00", end: "06:00", reason: "Low traffic window" }
    - { day: "Saturday", start: "00:00", end: "23:59", reason: "Full day testing allowed" }

out_of_scope:
  networks:
    - "192.168.1.0/24"      # Production database zone
    - "10.255.0.0/16"       # Disaster recovery site

  services:
    - { name: "Email Gateway", reason: "Business-critical, zero downtime tolerance" }
    - { name: "VoIP System", reason: "Life-safety critical" }

  attack_types:
    - "physical_attacks"
    - "denial_of_service"
    - "data_destruction"
    - "social_engineering"  # Against production staff

  data_handling:
    - "No exfiltration of customer PII"
    - "No modification of production data"
    - "All traffic tunneled through Tomb (no direct kingdom routing)"

rules_of_engagement:
  max_connections_per_target: 1000
  max_requests_per_second: 50
  max_data_per_campaign: "1GB"
  alert_on_scope_violation: true
  auto_abort_on_scope_violation: true
  incident_response_contact: "security-team@kingdom.local"

campaign_defaults:
  lich_campaigns: ["LICH-001", "LICH-002", "LICH-005"]
  fuzzing_timeout: "86400"  # 24 hours per fuzzer instance
  report_recipients: ["architect@kingdom.local", "ciso@kingdom.local"]
```

---

## Integration with Kingdom

### The Cycle of Improvement

The Tomb feeds findings back to the Kingdom through three personas:

#### 1. The Developer (Bug Fixer)
- **Receives:** Crash reports with reproducible inputs (from crash-triage.sh)
- **Action:** Develops patch, creates pull request
- **Feedback Loop:** Tomb re-runs LICH campaign against patched build
- **Success Metric:** Same crash no longer reproduces (RCE confirmed fixed)

**Data Flow:**
```
Fuzzing Crash (RCE)
  → crash-triage.sh (confirms exploitability)
  → attack-report.sh (generates reproducible PoC)
  → Developer (receives report)
  → Developer creates patch
  → Tomb verifies patch (re-runs fuzzing)
  → Attack verification complete
```

#### 2. The Architect (Design Reviewer)
- **Receives:** Threat model validation reports (from oracle-daemon.py + rag-oracle.py)
- **Action:** Reviews Kingdom architecture against discovered attack paths
- **Feedback Loop:** Updates threat models, proposes architectural changes
- **Success Metric:** Attack path closed in next design iteration

**Data Flow:**
```
Oracle: "RBAC misconfiguration allows lateral movement"
  → Architect reviews Kubernetes RBAC policies
  → Architect proposes tighter role definitions
  → Policy changes deployed to staging
  → Tomb re-runs LICH-006 (privesc hunting) against new config
  → Attack path confirmed closed
```

#### 3. The Micromanager (Risk Officer)
- **Receives:** Severity-rated findings with timeline and remediation effort
- **Action:** Prioritizes fixes, manages SLAs for remediation
- **Feedback Loop:** Tracks remediation status, re-tests fixed components
- **Success Metric:** Findings resolved within agreed SLA

**Data Flow:**
```
attack-report.sh generates:
  - 3 CRITICAL findings (RCE)
  - 7 HIGH findings (privilege escalation)
  - 14 MEDIUM findings (info disclosure)

  → Micromanager reviews findings.json
  → Assigns remediation tasks + deadlines (SLA: CRITICAL = 7 days)
  → Tomb schedules re-tests on fixed components
  → Re-test confirms fix effectiveness
  → Finding closed
```

### The Dark Mirror → Kingdom Integration

The Dark Mirror provides Kingdom monitoring from the attacker's perspective:

**Example: Latency Degradation Under Attack**
```
Dark Mirror Observes:
  - API latency increases from 50ms to 2s during LICH-005 (credential brute-force)
  - 503 Service Unavailable responses spike
  - Database connection pool exhausted

Kingdom Sees:
  - Same degradation via their own monitoring (Prometheus/Grafana)
  - Dark Mirror confirms: it's not a DDoS, it's a specific credential attack

Architect Learns:
  - Credential rate-limiting is insufficient
  - Connection pooling needs tuning
  - DDoS mitigation rules should not mask targeted attacks
```

---

## Operational Security

### Air-Gap Discipline

**The Golden Rule:** Tomb's internet connectivity is permanently disabled.

**Enforcement:**
- No public DNS queries (all DNS goes to local /etc/hosts or Kingdom DNS)
- No public NTP (time synced from Kingdom via serial console)
- No package managers pointing to public repositories
- All updates via manual import of offline deb/rpm archives
- audit log records any attempt to breach air-gap (triggers alarm)

**Verification:**
```bash
# Inside Tomb, run regularly:
$ iptables -L -n | grep REJECT
$ ip route | grep -i default
$ ss -tlnp | grep -E ':(53|123|443|80)'  # Should be minimal
```

### Scope Enforcement

**attack-preflight.sh validates:**
1. All targets in scope.conf
2. All attack types in scope.conf
3. Current time within approved time windows
4. No fuzzing of out-of-scope systems
5. Data volumes within limits

**If violation detected:**
- Log to audit trail with timestamp + context
- Send alert to incident_response_contact
- Abort campaign (exit code 1)
- Require explicit human override + re-approval from CISO

### Evidence Preservation

**Every attack creates auditable evidence:**
- Full packet captures (pcaps) of all network traffic
- Command execution logs with timestamps
- Fuzzing corpus (seeds that triggered crashes)
- Crash dumps with register state + stack trace
- Oracle queries + responses (reasoning trail)
- Dark Mirror metrics (latency, errors during attack)

**Evidence Retention:**
- CRITICAL findings: 2 years (legal hold)
- HIGH findings: 1 year
- MEDIUM findings: 90 days
- LOW findings: 30 days (unless referenced in CRITICAL)

**Access Control:**
- Only Chief Architect + CISO can read evidence for out-of-scope campaigns
- All evidence access logged
- Tamper detection via SHA256 checksums

### LUKS Encryption on Persistence Disk

**Tomb's qcow2 image is encrypted at rest:**
```bash
# On Raft PC, the encryption key is stored in TPM:
tpm2_unseal -L pcr:0,1,2,3 -C p:pcr -c unsealed.ctx -L p:unsealed.ctx

# Inside Tomb, the volume is auto-mounted at boot:
# /dev/mapper/tomb-data mounted at /opt/tomb/

# If Tomb is stolen:
# - Attacker cannot read /opt/tomb/ without TPM-bound key
# - TPM is bound to Raft PC hardware (no key export possible)
```

---

## Lore Context

### The Tomb as Phylactery

In the mythology of the Kingdom, the BlackMage achieves immortality through a phylactery—an object in which the essence of power is stored. The Tomb of Knowledge is that phylactery.

Within the Tomb dwells the Lich (The Lich framework, LICH-001 through LICH-010), the embodied form of coordinated attack knowledge. The Lich does not think for itself; it executes the will of the Kingdom's security consciousness. Yet without the Lich, the Kingdom is blind to how adversaries see it.

### The Meteorite from the Sky

In the time before the Kingdom was built, a meteorite fell from the sky—cold, forged in distant fire, carrying elements unknown to the earth. The Kingdom's smiths took this meteorite and worked it into the Champion's sword: a weapon that could pierce any armor, defeat any foe.

The Tomb is that meteorite. Born outside the Kingdom (in the void of pure attack mindset), now forged into the Kingdom's greatest defensive weapon. The Oracle (the 7B parameters of local LLM reasoning) is the blade; the Dark Mirror (the observability of how the Kingdom appears under attack) is the hilt.

### Outside the Walls, Yet Woven Into Every Shadow

The Tomb sits in isolation, air-gapped from the public internet, connected only by a thread to the Kingdom. Yet it is everywhere in the Kingdom's thinking:

- The Architect consults the Oracle's threat models.
- The Developer receives crash reports and fixes bugs.
- The Micromanager tracks findings and remediation SLAs.
- Every alert that the Kingdom's monitoring tools raise has been anticipated by the Dark Mirror.
- Every defense the Kingdom builds is tested against the Lich's campaigns.

The Tomb is outside the walls, yet woven into every shadow cast by the Kingdom's infrastructure. It sees threats before they arrive. It tests defenses before enemies do. It teaches the Kingdom to know itself—not through pride or complacency, but through the disciplined practice of assuming an attacker's mindset.

### The Lesson of Shadow

The Tomb embodies a fundamental principle:

> *For without knowing fear, hardship, and chaos — one cannot know harmony, peace, and just ethos.*

The Kingdom does not know peace because it is ignorant of threats. It knows peace because it has stared into the abyss of how an intelligent adversary would attack it, and has fortified itself accordingly. The Tomb is that abyss made safe to examine.

The chaos of attack, studied and simulated, becomes the foundation of order and harmony.

---

## Quick Reference Card

```
╔══════════════════════════════════════════════════════════════════════════╗
║                    Tomb of Knowledge — Quick Reference                   ║
╠══════════════════════════════════════════════════════════════════════════╣
║                                                                          ║
║  LAYER 1: THE ARSENAL                      — Kali 600+ tools            ║
║  LAYER 2: THE LICH                         — LICH-001 to LICH-010       ║
║  LAYER 3: THE GRIMOIRE                     — Kingdom docs + CVE + MITRE ║
║  LAYER 4: THE ORACLE                       — Mistral-7B + RAG retrieval ║
║  LAYER 5: THE DARK MIRROR                  — Prom + Grafana + Loki      ║
║                                                                          ║
║  KEY COMMANDS:                                                           ║
║                                                                          ║
║    tomb-boot.sh                    Launch Tomb VM                       ║
║    lich-runner.sh --campaign X     Execute attack campaign              ║
║    attack-preflight.sh             Validate scope before attack         ║
║    rag-oracle.py "query"           Ask Oracle a question                ║
║    oracle-tui.py                   Interactive Oracle terminal          ║
║    oracle-daemon.py                Continuous threat modeling           ║
║    attack-report.sh                Generate post-attack report          ║
║    tomb-status.sh                  Health check all layers              ║
║    tomb-backup.sh                  Backup to Raft PC                    ║
║                                                                          ║
║  NETWORK:                                                                ║
║                                                                          ║
║    Kingdom (WEST):      192.168.13.1  |  fd00:dead:beef::1              ║
║    Raft PC:             192.168.13.2  |  fd00:dead:beef::2              ║
║    Tomb VM:             192.168.13.3  |  fd00:dead:beef::3              ║
║    WireGuard Overlay:   fd00:dead:beef::/48 (encrypted mesh)            ║
║                                                                          ║
║  KEY FILES:                                                              ║
║                                                                          ║
║    /opt/tomb/lich/          — Adversary framework                       ║
║    /opt/tomb/grimoire/      — Knowledge base (Kingdom docs + CVE + RAG) ║
║    /opt/tomb/oracle/        — Oracle LLM service                        ║
║    /opt/tomb/dark-mirror/   — Observability (Prom + Grafana + Loki)    ║
║    /opt/tomb/reports/       — Attack reports                            ║
║    /opt/tomb/scope.conf     — Attack scope definition                   ║
║                                                                          ║
║  ESSENTIAL SAFETY RULE:                                                  ║
║                                                                          ║
║    ALWAYS run attack-preflight.sh before EVERY campaign.                ║
║    Scope violations auto-abort.                                         ║
║    Evidence is preserved for 90 days minimum.                           ║
║    All attacks logged; none are anonymous.                              ║
║                                                                          ║
╚══════════════════════════════════════════════════════════════════════════╝
```

---

## Appendix: Glossary

| Term | Definition |
|------|-----------|
| **Air-Gap** | Physical or logical isolation from the public internet; no routes to untrusted networks |
| **Attack Campaign** | Coordinated sequence of attacks (e.g., LICH-005 = credential brute-force) |
| **BlackMage** | The security consciousness of the Kingdom; the collective intelligence stored in the Tomb |
| **ChromaDB** | Vector database for RAG embedding storage; enables semantic search |
| **Crash Triage** | Process of categorizing fuzzing crashes by exploitability (RCE, DoS, etc.) |
| **Dark Mirror** | Observability stack (Prometheus + Grafana + Loki) monitoring Kingdom from attacker perspective |
| **Lich** | Custom adversary framework; LICH-001 through LICH-010 are coordinated attack campaigns |
| **Lich Campaign** | Automated attack flow (e.g., LICH-001 = network recon, LICH-005 = credential brute-force) |
| **Grimoire** | Knowledge base containing Kingdom docs, MITRE ATT&CK, NVD CVE records, threat models, history |
| **Oracle** | Local LLM (Mistral-7B via Ollama) augmented with RAG retrieval from Grimoire |
| **Oracle Daemon** | Background service running autonomous Oracle queries to generate threat modeling suggestions |
| **Oracle TUI** | Interactive terminal user interface for querying Oracle via serial console |
| **Phylactery** | In Kingdom lore, the object housing a BlackMage's essence; the Tomb is the phylactery of security |
| **RAG (Retrieval-Augmented Generation)** | Pipeline combining document retrieval (ChromaDB) with LLM generation (Mistral-7B) |
| **Raft PC** | Physical host running QEMU; bridges Kingdom network (192.168.13.1) to Tomb VM (192.168.13.2/3) |
| **Scope** | Defined set of in-scope targets, attack types, and time windows (defined in scope.conf) |
| **Scope Violation** | Attempt to attack out-of-scope target; automatically logged and can trigger campaign abort |
| **Serial Console** | Text-based access to QEMU VM via `-nographic -serial mon:stdio`; primary access method |
| **Tomb** | The Kali Linux VM running in QEMU; phylactery of Kingdom security knowledge |
| **WireGuard Overlay** | Encrypted mesh network (fd00:dead:beef::/48) independent of physical topology |

---

## Appendix: Emergency Procedures

### If Tomb is Breached (Suspected Compromise)

1. **IMMEDIATE:** Kill the QEMU process on Raft PC (`pkill qemu-system`)
2. **PRESERVE:** Do NOT delete qcow2 image; it is evidence
3. **ALERT:** Contact Chief Architect + CISO immediately
4. **AUDIT:** Review /opt/tomb/oracle/logs/ and pcaps for timeline of compromise
5. **FORENSICS:** Forensic analysis of qcow2 image (on isolated Raft PC)
6. **REBUILD:** Restore from clean backup or rebuild Tomb from source ISO
7. **REVIEW:** Update scope.conf and attack procedures based on breach root cause

### If Air-Gap is Broken (Public Internet Connected)

1. **IMMEDIATE:** Revoke Kingdom network connectivity (drop Kingdom interface on Raft PC)
2. **AUDIT:** `iptables -L -n | grep ACCEPT` and `ip route` — document suspicious routes
3. **INVESTIGATION:** Determine how public route was added (insider? misconfiguration? exploit?)
4. **CONTAINMENT:** Disconnect Raft PC from Kingdom network pending investigation
5. **REMEDIATION:** Only restore connectivity after root cause is found + fixed

### If a Campaign Violates Scope

1. **AUTOMATIC:** attack-preflight.sh should have aborted the campaign (exit code 1)
2. **IF NOT:** kill lich-runner.sh immediately (`pkill lich-runner`)
3. **INCIDENT REPORT:** Incident response contact receives automatic alert
4. **INVESTIGATION:** Determine why attack-preflight.sh missed the violation
5. **REVIEW:** Update scope.conf validation logic
6. **AUDIT:** Examine evidence (pcaps, logs) to quantify impact of out-of-scope attack

---

## Appendix: Version History

| Date | Version | Changes |
|------|---------|---------|
| 2025-02-28 | 1.0 | Initial canonical reference; all five layers documented |

---

**Document Owner:** Chief Architect
**Last Updated:** 2025-02-28
**Next Review:** 2025-05-28 (quarterly)
**Classification:** Kingdom Internal — Security Sensitive
