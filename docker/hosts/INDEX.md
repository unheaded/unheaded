# Unheaded Kingdom Docker Deployment - File Index

**Total Files**: 17 | **Total Size**: 164K | **Date**: 2026-02-26

## Navigation Guide

### Start Here (Everyone)
1. **README.md** - Main entry point, overview, quick links
2. **QUICKSTART.md** - 5-minute setup guide (fastest path)

### System Understanding
3. **ARCHITECTURE.md** - Complete system design, services, data flows

### Deployment Guides
4. **host-a/README.md** - Full-stack deployment (25 services)
5. **host-b/README.md** - Minimal deployment (6 services)

### Reference
6. **DEPLOYMENT_SUMMARY.txt** - Checklist and complete reference
7. **INDEX.md** - This file

## Host-A (Forge) - Full Stack - 8 Files

### Core Configuration
- **docker-compose.yml** (1,252 lines)
  - 25 Unheaded Kingdom services
  - 5 telemetry services (Prometheus, VictoriaMetrics, Loki, Grafana, eBPF)
  - 30+ named volumes
  - Resource limits (~40 cpus, ~18GB memory)
  - Security hardening

- **.env.example** (27 lines)
  - Environment variables template
  - WireGuard key placeholders
  - Grafana credentials
  - Telemetry settings

### Telemetry Configuration
- **prometheus.yml** (140 lines)
  - Scrape configs for 30 services
  - 5-second interval
  - Remote write to VictoriaMetrics

- **loki-config.yaml** (40 lines)
  - Log aggregation setup
  - BoltDB+filesystem storage
  - 3-month retention

- **grafana-provisioning/datasources/unheaded.yml** (35 lines)
  - Prometheus datasource
  - VictoriaMetrics datasource
  - Loki datasource

- **grafana-provisioning/dashboards/unheaded.yml** (15 lines)
  - Dashboard provisioning

### Documentation
- **README.md** (450+ lines)
  - Hardware requirements
  - Setup instructions (step-by-step)
  - All 30 services described
  - Health checks
  - Resource limits
  - Networking details
  - Security features
  - Persistent storage
  - Troubleshooting (detailed)
  - Maintenance procedures
  - Integration with Host-B
  - Performance tuning

## Host-B (Outpost) - Minimal - 5 Files

### Core Configuration
- **docker-compose.yml** (430 lines)
  - 6 Unheaded Kingdom services
  - 4 telemetry agents (Prometheus, Promtail, Node-Exporter, WireGuard)
  - 8 named volumes
  - Resource limits (~10.5 cpus, ~2.5GB memory)
  - Security hardening

- **.env.example** (24 lines)
  - Environment variables template
  - Host-A endpoint settings
  - WireGuard configuration

### Telemetry Configuration
- **prometheus.yml** (80 lines)
  - Scrape configs for local services
  - 10-second interval
  - Remote write to Host-A VictoriaMetrics

- **promtail-config.yaml** (45 lines)
  - Log forwarding to Host-A Loki
  - System log scraping
  - Docker log scraping
  - Container metadata extraction

### Documentation
- **README.md** (400+ lines)
  - Hardware requirements
  - Setup instructions (step-by-step)
  - 10 services described
  - WireGuard VPN setup
  - Monitoring & logs
  - Troubleshooting (detailed)
  - Maintenance procedures
  - Integration with Host-A
  - Advanced configuration

## Root Documentation - 4 Files

### **README.md** (Main Entry Point)
- Project overview
- Directory structure
- Quick links
- Common commands
- Troubleshooting quick links
- Support resources

### **QUICKSTART.md** (5-Minute Setup)
- Hardware prerequisites
- WireGuard key generation
- Environment configuration
- Fast deployment steps
- Service access URLs
- Common commands
- Networking setup
- Performance expectations

### **ARCHITECTURE.md** (System Design)
- Overview of both hosts
- Complete service inventory (30)
- Resource allocation
- Networking architecture
- Security architecture
- Service dependencies
- Storage architecture
- Data flow diagrams
- Scaling considerations
- File structure
- Future enhancements

### **DEPLOYMENT_SUMMARY.txt** (Reference)
- Files created checklist
- Service specifications
- Networking configuration
- Security features
- Quick start steps
- Resource allocation summary
- Verification checklist

## Configuration Details

### Docker Compose Features
- Version 3.8 format
- 30+ named volumes (persistence)
- Custom bridge networks (IPv4 + IPv6)
- Health checks (curl/wget)
- Resource limits (CPU/memory)
- Service dependencies
- Security options (no-new-privileges, read-only)
- Capability restrictions (specific services)

### Services Summary
**Host-A**: 30 total
- 1 Foundation (wotan)
- 3 Core Protocol (monad, sophia, anamnesis)
- 2 Core Daemons (shield, daemon)
- 6 API/Dashboard (backend, frontend, protocol-api, trace, gateway, discovery)
- 2 High-Performance (doom, lich)
- 5 Orchestration (captain, micromanager, timeguru, architect, developer)
- 6 Data/Knowledge (kanban, lore, busboy, kingdom, blackmage, moatghost)
- 5 Telemetry (prometheus, victoriametrics, loki, grafana, ebpf-exporter)

**Host-B**: 10 total
- 1 Foundation (wotan)
- 3 Core Protocol (monad, sophia, anamnesis)
- 2 API (dashboard-backend, gateway)
- 4 Agents (prometheus, promtail, node-exporter, wireguard)

### Resource Allocation
**Host-A**:
- Total: ~40 cpus, ~18GB memory
- Conservative allocation on 16+ cores, 64GB RAM

**Host-B**:
- Total: ~10.5 cpus, ~2.5GB memory
- Conservative allocation on 8 cores, 8GB RAM

### Networking
**Docker Networks**:
- Host-A: 172.20.0.0/16 + fd00:dead:beef:1::/64
- Host-B: 172.21.0.0/16 + fd00:dead:beef:2::/64

**WireGuard VPN**:
- Network: fd00:dead:beef::/48
- Port: 51820 UDP
- Host-A: ::1, Host-B: ::2

### Security
**Default Hardening**:
- no-new-privileges: true
- read-only: true
- /tmp isolation (tmpfs)
- Network isolation

**Capability Restrictions**:
- shield: BPF + NET_ADMIN + SYS_ADMIN + SYS_RESOURCE
- daemon: BPF + NET_ADMIN
- lich: NET_RAW + NET_ADMIN
- blackmage: NET_RAW
- wireguard: NET_ADMIN + SYS_MODULE

## How to Use These Files

### For Quick Deployment
1. Start with: QUICKSTART.md
2. Then follow: host-a/README.md
3. Optional: host-b/README.md

### For Understanding Architecture
1. Read: ARCHITECTURE.md
2. Understand: README.md
3. Deep dive: Individual host READMEs

### For Reference
1. Check: DEPLOYMENT_SUMMARY.txt
2. Look up: Specific service in ARCHITECTURE.md
3. Troubleshoot: Relevant README

### For Configuration
1. Copy: .env.example to .env
2. Edit: WireGuard keys and passwords
3. Review: docker-compose.yml comments
4. Validate: prometheus.yml and other configs

## File Sizes

| File | Size | Lines |
|------|------|-------|
| host-a/docker-compose.yml | 48K | 1,252 |
| host-b/docker-compose.yml | 18K | 430 |
| ARCHITECTURE.md | 16K | 397 |
| host-a/README.md | 20K | 450+ |
| host-b/README.md | 18K | 400+ |
| QUICKSTART.md | 14K | 300+ |
| DEPLOYMENT_SUMMARY.txt | 15K | 400+ |
| README.md | 8K | 200+ |
| prometheus.yml (a) | 5K | 140 |
| loki-config.yaml | 2K | 40 |
| promtail-config.yaml | 2K | 45 |
| .env.example files | 1K | 51 |
| Grafana provisioning | 1K | 50 |
| **Total** | **164K** | **4,500+** |

## Quick Links by Task

### Deploy Host-A
1. QUICKSTART.md - Step-by-step
2. host-a/README.md - Full guide
3. host-a/docker-compose.yml - Reference
4. host-a/.env.example - Config template

### Deploy Host-B
1. QUICKSTART.md - Step-by-step
2. host-b/README.md - Full guide
3. host-b/docker-compose.yml - Reference
4. host-b/.env.example - Config template

### Understand System
1. README.md - Overview
2. ARCHITECTURE.md - Design details
3. DEPLOYMENT_SUMMARY.txt - Checklist

### Monitor & Troubleshoot
1. host-a/README.md - Troubleshooting section
2. host-b/README.md - Troubleshooting section
3. docker-compose.yml - Health checks

### Configure Services
1. .env.example - Environment variables
2. prometheus.yml - Metrics scraping
3. loki-config.yaml - Log storage
4. promtail-config.yaml - Log forwarding

## Key Information

### Hardware Requirements
- **Host-A**: 16+ cores, 64GB RAM, RX 7700 XT GPU (optional)
- **Host-B**: 4-8 cores, 8GB RAM, no GPU

### Port Mappings
- **Host-A**: 60+ ports (services + telemetry)
- **Host-B**: 10+ ports (services + WireGuard)

### Volume Allocation
- **Host-A**: 30 named volumes
- **Host-B**: 8 named volumes

### Documentation Quality
- **Comprehensive**: ~2,000+ lines of guides
- **Step-by-step**: Full setup instructions
- **Troubleshooting**: Detailed solutions
- **Reference**: Complete service inventory

## Support Path

1. **Questions about setup**: QUICKSTART.md
2. **Questions about system**: ARCHITECTURE.md
3. **Questions about Host-A**: host-a/README.md
4. **Questions about Host-B**: host-b/README.md
5. **Questions about files**: This INDEX.md
6. **Questions about configuration**: docker-compose.yml comments

## Next Steps

1. Choose your path: Quick start or deep understanding?
2. Read: Appropriate documentation file
3. Generate: WireGuard keys (if deploying)
4. Configure: .env files
5. Deploy: docker-compose up -d
6. Monitor: Check logs and dashboards
7. Customize: Adjust for your needs

---

**Ready to deploy?** Start with QUICKSTART.md!
