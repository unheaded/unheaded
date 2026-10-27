# TOMB OF KNOWLEDGE - PHASES 6-8: THE ORACLE & THE DARK MIRROR

## Document Overview

This directory contains the complete strategic directive for deploying Phases 6-8 of the TOMB OF KNOWLEDGE battle plan, authored by **The Unheaded Warmonger**.

### Primary Document
- **S75-TOMB-BATTLE-PLAN-part3.md** (2,224 lines, 64KB)
  - The canonical, highly detailed battle plan
  - Exact bash commands for every step
  - Verification gates at each checkpoint
  - Debug branches for troubleshooting
  - Systemd service configurations
  - Python scripts for Oracle, RAG pipeline, and monitoring

### Supporting Documents
- **PHASE-6-8-SUMMARY.txt** (211 lines, 6.4KB)
  - Executive summary of each phase
  - Quick reference for verification commands
  - Troubleshooting guide
  - Deployment timeline estimates
  - Next phases roadmap

## Phase Breakdown

### PHASE 6: THE ORACLE BASE (Steps 186-215)
**Layer 4a — Local LLM Setup**

Delivers an offline Mistral-7B LLM running on the Raft PC (192.168.13.2), accessible via CLI and systemd service.

**Key Deliverables:**
- Ollama v0.1.33 binary installed to /opt/ollama/
- Mistral-7B-Instruct-v0.1.Q4_K_M (4.4GB) model loaded
- TinyLlama-1.1B (1.1GB) fallback model available
- oracle.sh CLI wrapper script
- oracle-test.py validation harness
- System prompt with Kingdom protocol knowledge
- Query logging to /opt/tomb/oracle/logs/
- Systemd service for auto-restart

**Dependencies:**
- Kali Linux VM (Phases 0-5 complete)
- 8GB+ disk space in /opt/tomb/
- 4GB+ available RAM
- Python 3.8+

**Performance Target:**
- 2+ tokens/second on QEMU hardware
- <100ms latency for protocol queries
- Mistral quality with low hallucination rate

**Exit Criteria:**
- oracle.sh returns multi-paragraph technical responses
- Test queries about CRC-16, Sophia, Wotan answered accurately
- Logs persist to /opt/tomb/oracle/logs/oracle-queries.log
- Systemd service auto-restarts on failure

---

### PHASE 7: ORACLE RAG PIPELINE INTEGRATION (Steps 216-240)
**Layer 4b — Context-Aware Security Analysis**

Wires the Oracle LLM to the Grimoire (ChromaDB RAG index), enabling context-aware responses augmented with Kingdom architecture knowledge.

**Key Deliverables:**
- rag-oracle.py integration pipeline
  - Queries ChromaDB for top-5 relevant chunks
  - Injects context into Oracle system prompt
  - Streams responses back to user
  - Logs all queries and responses
- oracle-daemon.py persistent service
  - Listens on Unix socket (/tmp/oracle.sock)
  - Handles concurrent queries with threading
  - JSON-based request/response protocol
  - Systemd auto-restart on failure
- oracle-tui.py Terminal User Interface
  - Chat-like interface for serial console
  - Chat history persistence
  - Special commands: /search, /lich, /threat, /history
  - readline support for editing
- oracle-benchmark.py performance suite
  - Measures latency across query types
  - Calculates tokens/second throughput
  - Stress tests daemon concurrency

**Dependencies:**
- Phase 6 (Ollama) fully operational
- Grimoire populated with Kingdom documentation
- ChromaDB index built and indexed

**Performance Target:**
- RAG retrieval: <2 seconds for top-5 chunks
- LLM query: <5 seconds for 200-word response
- Throughput: 1+ tokens/second (stream mode)
- Concurrent queries: 5+ simultaneous without degradation

**Exit Criteria:**
- rag-oracle.py successfully retrieves and injects context
- oracle-daemon.py listening on /tmp/oracle.sock
- oracle-tui.py runs interactively on serial console
- Responses include explicit citations to Grimoire sources
- Query logs persist with timestamps and RAG chunk counts

---

### PHASE 8: THE DARK MIRROR — OBSERVABILITY STACK (Steps 241-275)
**Layer 5 — Kingdom Monitoring from the Tomb**

Deploys a three-component observability stack (Prometheus + Grafana + Loki) monitoring the Kingdom from outside its walls.

**Key Deliverables:**
- **Prometheus v2.48.0** monitoring system
  - Scrapes Kingdom Prometheus metrics (:9090)
  - Scrapes Kingdom API gateway metrics (:8080)
  - Scrapes Kingdom service mesh (:15000)
  - Scrapes Tomb node_exporter (:9100)
  - Alert rules for:
    - Service availability (down alerts)
    - eBPF program load failures
    - Lich crash detection
    - Unusual traffic patterns (>10k pkt/sec)
    - High latency (p99 >1s)
  - 30-day retention on /var/lib/prometheus

- **Grafana v10.2.0** dashboard platform
  - Kingdom topology dashboard
  - Service health dashboard
  - Monad traffic visualization
  - mTLS certificate coverage tracking
  - Responsive UI accessible on :3000
  - Prometheus datasource auto-configured

- **Loki v2.9.2** log aggregation
  - Ingests Kingdom service logs
  - eBPF program events
  - Lich crash reports
  - Security audit trail
  - 30-day retention on /var/lib/loki

- **node_exporter v1.7.0** Tomb health
  - CPU/memory/disk metrics
  - Network interface statistics
  - File descriptor monitoring

- **dark-mirror.sh** control script
  - Unified status dashboard
  - Service start/stop/restart
  - Logs aggregation
  - Kingdom connectivity verification

**Dependencies:**
- 5GB+ disk space in /var/lib/
- Kingdom Prometheus reachable at 192.168.13.1:9090
- Prometheus binary transfer to Tomb

**Performance Target:**
- <30s scrape interval
- <2MB/min metric ingestion
- Alert evaluation <30s
- Grafana dashboard load <5s

**Exit Criteria:**
- Prometheus scrapes all 5 targets (Kingdom + Tomb)
- Grafana dashboards load with live data
- Loki receives Kingdom logs via promtail
- dark-mirror.sh status shows all services active
- Alerts trigger on simulated service failures

---

## Quick Start Guide

### 1. Pre-Flight Checks
```bash
# Verify prerequisites before starting any phase
df -h /opt/tomb/
free -h
curl -s http://192.168.13.1:9090/api/v1/query | head -5
python3 --version
```

### 2. Execute Phase 6
```bash
# Follow S75-TOMB-BATTLE-PLAN-part3.md steps 186-215
# Key milestones:
#  - Step 187-189: Install Ollama binary
#  - Step 191-193: Transfer model weights
#  - Step 194-196: Load Mistral model
#  - Step 200-202: Test Oracle functionality
```

### 3. Execute Phase 7
```bash
# Follow S75-TOMB-BATTLE-PLAN-part3.md steps 216-240
# Key milestones:
#  - Step 217: Deploy rag-oracle.py pipeline
#  - Step 218-222: Deploy oracle-daemon and TUI
#  - Step 223-225: Test end-to-end RAG integration
```

### 4. Execute Phase 8
```bash
# Follow S75-TOMB-BATTLE-PLAN-part3.md steps 241-275
# Key milestones:
#  - Step 242-246: Deploy Prometheus stack
#  - Step 247-252: Deploy Grafana and Loki
#  - Step 253-257: Deploy dashboards and alerts
```

## Verification Commands

### Phase 6 Verification
```bash
# Oracle service status
systemctl status ollama

# Test Oracle
/opt/tomb/oracle/oracle.sh "What are Kingdom vulnerabilities?"

# Benchmark performance
python3 /opt/tomb/oracle/oracle-test.py
```

### Phase 7 Verification
```bash
# RAG pipeline test
/opt/tomb/oracle/rag-oracle.py "Explain Monad CRC-16"

# Daemon socket check
test -S /tmp/oracle.sock && echo "Socket active"

# TUI interactive test
echo "/threat" | /opt/tomb/oracle/oracle-tui.py
```

### Phase 8 Verification
```bash
# Dark Mirror status
/opt/tomb/dark-mirror.sh status

# Prometheus targets
curl http://localhost:9090/api/v1/targets | jq '.data.activeTargets | length'

# Grafana health
curl -u admin:changeme http://localhost:3000/api/health

# Loki ingestion
curl 'http://localhost:3100/loki/api/v1/query?query={job="kingdom-*"}'
```

## Troubleshooting

### Ollama Service Down
```bash
sudo systemctl restart ollama
journalctl -u ollama -n 50
netstat -tuln | grep 11434
```

### RAG Queries Failing
```bash
python3 /opt/tomb/rag/rag-query.py "test" --limit 1
ls -la /opt/tomb/grimoire/
du -sh /opt/tomb/rag/chroma/
```

### Prometheus Not Scraping Kingdom
```bash
curl http://192.168.13.1:9090/metrics
curl http://localhost:9090/api/v1/targets
grep 192.168.13.1 /opt/prometheus/config/prometheus.yml
```

### Grafana Access Issues
```bash
curl -u admin:changeme http://localhost:3000/api/health
tail -50 /opt/grafana/logs/grafana.log
ps aux | grep grafana
```

## File Structure

```
/opt/tomb/
├── oracle/                          # Phase 6 & 7
│   ├── oracle.sh                   # CLI wrapper [Phase 6]
│   ├── oracle-test.py              # Test harness [Phase 6]
│   ├── rag-oracle.py               # RAG pipeline [Phase 7]
│   ├── oracle-daemon.py            # Socket service [Phase 7]
│   ├── oracle-tui.py               # Terminal UI [Phase 7]
│   ├── oracle-benchmark.py         # Benchmark suite [Phase 7]
│   ├── oracle-status.sh            # Status dashboard [Phase 6]
│   ├── prompts/
│   │   └── system-oracle.txt       # System prompt [Phase 6]
│   ├── logs/                       # Query logs [Phase 6 & 7]
│   ├── cache/                      # Response cache [Phase 7]
│   └── config/
│       └── ollama-tuning.conf      # Performance tuning [Phase 6]
│
├── dark-mirror.sh                  # Monitoring control [Phase 8]
│
├── grimoire/                       # Knowledge base (Phases 0-5)
│   └── [documentation files]
│
└── rag/                            # RAG index (Phases 0-5)
    ├── rag-query.py
    └── chroma/                     # ChromaDB indices

/opt/ollama/                        # Phase 6
├── bin/
│   └── ollama
├── models/                         # Model binaries
└── config/

/opt/prometheus/                    # Phase 8
├── bin/prometheus
├── config/
│   ├── prometheus.yml
│   └── alert-rules.yml
└── data/                           # Time series database

/opt/grafana/                       # Phase 8
├── bin/grafana-server
├── conf/custom.ini
├── dashboards/                     # Dashboard JSON files
└── logs/

/opt/loki/                          # Phase 8
├── bin/loki
├── config/loki.yml
└── data/                           # Log storage

/var/lib/
├── prometheus/                     # Metrics storage
├── grafana/                        # Grafana db
├── loki/                           # Log storage
└── ollama/models/                  # Model cache
```

## Deployment Timeline

**Phase 6:** 6-8 hours
- Binary downloads/transfer: 1-2h
- Ollama setup and testing: 2-3h
- Model transfer and validation: 2-3h

**Phase 7:** 4-6 hours
- Script development and debugging: 2-3h
- Service setup and testing: 1-2h
- Benchmarking and optimization: 1h

**Phase 8:** 4-6 hours
- Binary downloads/transfer: 2h
- Service configuration: 1-2h
- Dashboard creation and testing: 1-2h

**Total:** 48-72 hours of active deployment

## Architecture Overview

```
THE UNHEADED KINGDOM (192.168.13.1)
├── EAST Node 1 (Monad + Sophia)
├── EAST Node 2 (Monad + Sophia)
├── WEST Node 1 (Monad + Sophia)
├── WEST Node 2 (Monad + Sophia)
└── Prometheus :9090 (metrics export)

              ↓ (network bridge 192.168.13.0/30)

THE TOMB OF KNOWLEDGE (192.168.13.2)
├── Layer 4a: Oracle (Ollama + Mistral-7B)
│   ├── Service: Ollama :11434 (LLM inference)
│   ├── CLI: oracle.sh
│   └── Service: oracle-daemon on /tmp/oracle.sock
│
├── Layer 4b: RAG Pipeline
│   ├── rag-oracle.py (retrieval + injection)
│   └── oracle-tui.py (interactive chat)
│
├── Layer 5: Dark Mirror (Observability)
│   ├── Prometheus :9090 (metrics aggregation)
│   ├── Grafana :3000 (dashboards)
│   └── Loki :3100 (log aggregation)
│
└── Supporting Systems:
    ├── Grimoire (ChromaDB knowledge base)
    ├── Lich (crash report analysis)
    └── node_exporter :9100 (VM metrics)
```

## Success Criteria

### Phase 6 Success
- [ ] Ollama service running and responding
- [ ] Mistral-7B model loaded successfully
- [ ] oracle.sh returns >100-word responses
- [ ] Performance >2 tokens/sec
- [ ] Logs populating in oracle/logs/

### Phase 7 Success
- [ ] rag-oracle.py retrieves context from ChromaDB
- [ ] oracle-daemon listening on /tmp/oracle.sock
- [ ] oracle-tui.py works on serial console
- [ ] Responses cite Grimoire sources
- [ ] Concurrent queries handled gracefully

### Phase 8 Success
- [ ] Prometheus scrapes 5+ targets
- [ ] Grafana dashboards load with live data
- [ ] Loki receives and displays logs
- [ ] Alerts fire on simulated failures
- [ ] dark-mirror.sh shows all services active

## Next Phases

- **Phase 9:** Advanced Threat Modeling (attack tree construction)
- **Phase 10:** Automated penetration testing (protocol fuzzing)
- **Phase 11:** Incident response automation
- **Phase 12:** Full Kingdom red team exercise

## Contact & Authority

**Authored by:** The Unheaded Warmonger
**For:** The Unheaded Kingdom Security Operations
**Date:** 2026-02-28
**Classification:** Strategic Deployment Directive

---

**The Unheaded Warmonger's Final Word:**

"The Tomb holds the Oracle. The Oracle sees through the Grimoire. The Dark Mirror watches all. Together, they guard the Kingdom.

Deploy with precision. Verify at every step. The Unheaded Kingdom's security rests on your shoulders.

In darkness, we fortify. In silence, we defend. In knowledge, we prevail."

*End Directive.*
