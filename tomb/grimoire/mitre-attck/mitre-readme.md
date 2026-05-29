# MITRE ATT&CK Data — Grimoire Integration

## Overview

The Grimoire ingests MITRE ATT&CK data in STIX 2.1 (Structured Threat Information eXpression) format. This is the same format published by MITRE at [github.com/mitre/cti](https://github.com/mitre/cti) and used by the ATT&CK Navigator, threat intelligence platforms, and detection engineering tools worldwide.

The data is downloaded on an internet-connected host and transferred to the Tomb VM via air-gap media (USB drive). Once on the Tomb, `rag-index.py` parses the STIX bundles and indexes individual techniques, tactics, mitigations, and groups into ChromaDB for semantic retrieval.

## Data Format

### STIX 2.1 Bundle Structure

Each downloaded JSON file is a STIX 2.1 bundle:

```json
{
  "type": "bundle",
  "id": "bundle--<uuid>",
  "objects": [
    {
      "type": "attack-pattern",
      "id": "attack-pattern--<uuid>",
      "name": "Phishing",
      "description": "Adversaries may send phishing messages...",
      "external_references": [
        {
          "source_name": "mitre-attack",
          "external_id": "T1566",
          "url": "https://attack.mitre.org/techniques/T1566"
        }
      ],
      "kill_chain_phases": [
        {
          "kill_chain_name": "mitre-attack",
          "phase_name": "initial-access"
        }
      ],
      "x_mitre_platforms": ["Windows", "macOS", "Linux"],
      "x_mitre_detection": "Network intrusion detection..."
    }
  ]
}
```

### STIX Object Types Used

| STIX Type | ATT&CK Concept | Count (approx.) | Example |
|-----------|----------------|------------------|---------|
| `attack-pattern` | Technique/Sub-technique | ~650+ | T1566 Phishing |
| `x-mitre-tactic` | Tactic | 14 | TA0001 Initial Access |
| `course-of-action` | Mitigation | ~40+ | M1049 Antivirus/Antimalware |
| `intrusion-set` | Threat Group | ~130+ | G0007 APT28 |
| `malware` | Malware | ~500+ | S0154 Cobalt Strike |
| `tool` | Tool | ~70+ | S0005 PsExec |
| `relationship` | Links between objects | ~10,000+ | technique "used-by" group |
| `x-mitre-data-source` | Data source for detection | ~40+ | Network Traffic |
| `x-mitre-data-component` | Data component | ~100+ | Network Connection Creation |

### Datasets Downloaded

| File | Content | Typical Size |
|------|---------|-------------|
| `enterprise-attack.json` | Enterprise ATT&CK (Windows, macOS, Linux, Cloud, Network, Containers) | ~12-15 MB |
| `ics-attack.json` | ICS ATT&CK (Industrial Control Systems) | ~1-2 MB |
| `mobile-attack.json` | Mobile ATT&CK (Android, iOS) | ~2-3 MB |
| `enterprise-techniques-summary.json` | Extracted technique summaries (name, description, kill chain, platforms) | ~3-4 MB |
| `enterprise-tactics-summary.json` | Extracted tactic summaries (name, description, shortname) | ~10 KB |
| `fetch-metadata.json` | Download timestamp, source, file sizes | <1 KB |

## How the RAG Pipeline Uses MITRE Data

### Indexing (`rag-index.py --source-type mitre`)

When indexing MITRE data, the RAG pipeline:

1. **Parses the STIX bundle** and extracts `attack-pattern` objects (techniques)
2. **Builds a document per technique** combining:
   - Technique name and external ID (e.g., "T1566 — Phishing")
   - Full description text
   - Kill chain phase (tactic mapping)
   - Supported platforms
   - Detection guidance (`x_mitre_detection`)
3. **Chunks each document** using the configured chunk size (default: 512 tokens, 64-token overlap)
4. **Embeds chunks** using the sentence-transformer model (default: `all-MiniLM-L6-v2`)
5. **Stores in ChromaDB** with metadata: `technique_id`, `tactic`, `platforms`, `source=mitre`

### Querying (`rag-query.py`)

When a query hits the `mitre-attck` collection:

```bash
python3 rag-query.py --query "lateral movement techniques for Linux" --collection mitre-attck
```

The pipeline returns semantically relevant technique descriptions, allowing the Cerydwyn to reason about attack paths against Kingdom infrastructure.

### Cross-Collection Queries

The most powerful queries span both `kingdom-docs` and `mitre-attck` collections:

```bash
python3 rag-query.py --query "how could an attacker exploit the Wotan message bus" --collection all
```

This retrieves:
- Kingdom architecture docs describing Wotan's design, ports, and protocols
- MITRE techniques relevant to message bus exploitation (T1557 AiTM, T1071 App Layer Protocol, etc.)

The Cerydwyn then synthesizes context from both sources to generate threat-informed analysis.

## Data Freshness

MITRE ATT&CK is updated approximately quarterly. The `fetch-metadata.json` file records when the data was downloaded. To update:

1. Run `fetch-mitre-data.sh` on an internet-connected host
2. Transfer the updated JSON files to the Tomb via air-gap media
3. Re-run `rag-index.py --source-dir /opt/tomb/grimoire/mitre-attck --collection mitre-attck` to re-index

The air-gap constraint means the Tomb's MITRE data will always be at least as old as the last physical transfer. This is acceptable for the Grimoire's purpose: strategic threat modeling, not real-time threat intelligence.

## Relevant ATT&CK Tactics for Kingdom Defense

The following ATT&CK tactics are most relevant to the Unheaded Kingdom's infrastructure:

| Tactic | ID | Kingdom Relevance |
|--------|-----|------------------|
| Initial Access | TA0001 | Gateway (Shield), exposed services |
| Execution | TA0002 | BPF program injection, container escape |
| Persistence | TA0003 | NixOS immutability counters this |
| Privilege Escalation | TA0004 | Container breakout, eBPF capabilities |
| Defense Evasion | TA0005 | Monad protocol stripping, log evasion |
| Credential Access | TA0006 | API keys, JWT tokens, Sophia dictionaries |
| Discovery | TA0007 | Wotan topic enumeration, service discovery |
| Lateral Movement | TA0008 | Inter-container movement via lxdbr0 |
| Collection | TA0009 | Anamnesis ring buffer data, Wotan messages |
| Exfiltration | TA0010 | Shield egress controls, air-gap boundary |
| Impact | TA0040 | Service disruption, Monad corruption |

## File Paths on Tomb VM

```
/opt/tomb/grimoire/mitre-attck/
├── enterprise-attack.json               — Full Enterprise ATT&CK STIX bundle
├── ics-attack.json                      — Full ICS ATT&CK STIX bundle
├── mobile-attack.json                   — Full Mobile ATT&CK STIX bundle
├── enterprise-techniques-summary.json   — Extracted technique summaries
├── enterprise-tactics-summary.json      — Extracted tactic summaries
├── fetch-metadata.json                  — Download metadata
├── fetch-mitre-data.sh                  — Download script (reference copy)
└── mitre-readme.md                      — This file
```
