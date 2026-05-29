# Unheaded Jenkins Setup Guide

**Phase 10: S72 Battle Plan — Jenkins Pipeline Scaffolding**

---

## Overview

This directory contains Jenkins pipeline definitions for the Unheaded Kingdom project:

- **Jenkinsfile** — Main pipeline (core build, test, security, SBOM)
- **Jenkinsfile.protocol** — Protocol Foundation (Go + Rust + eBPF)
- **Jenkinsfile.docker** — Container builds & scanning
- **Jenkinsfile.security** — Daily security audit
- **Jenkinsfile.release** — Release builds with SBOM & signing
- **vars/unheadedPipeline.groovy** — Shared library functions

---

## Prerequisites

### Jenkins Installation

```bash
# Docker-based Jenkins (quickstart)
docker run -d \
  --name jenkins \
  -p 8080:8080 -p 50000:50000 \
  -v jenkins_home:/var/jenkins_home \
  -v /var/run/docker.sock:/var/run/docker.sock \
  jenkins/jenkins:lts-alpine

# Access: http://localhost:8080
```

### Required Jenkins Plugins

- **Pipeline** — Declarative and Scripted Pipeline support
- **Docker** — Docker agent and build support
- **Git** — Repository integration
- **GitHub** — GitHub branch source + webhook
- **Cobertura** — Code coverage reports
- **JUnit** — Test result parsing
- **Timestamper** — Build timestamping
- **AnsiColor** — Colorized log output
- **Slack Notifier** — (Optional) Slack integration
- **Email Extension** — (Optional) Email notifications

### Install via Jenkins CLI

```bash
jenkins-cli install-plugin pipeline docker git github cobertura junit timestamper ansicolor slack-plugin

# Restart Jenkins
jenkins-cli restart
```

### Required Tools in Jenkins Environment

```
✓ Go 1.24+
✓ Rust toolchain (stable + nightly)
✓ Docker (for docker builds)
✓ git
✓ curl, wget
✓ jq (for JSON processing)
```

---

## Pipeline Definitions

### 1. Main Jenkinsfile

**Purpose:** Core application build, test, security, SBOM

**Trigger:** Manual or webhook from repository

**Stages:**
1. Dependencies — Download + verify modules
2. Build — Compile binaries
3. Vet — Go static analysis
4. Lint — golangci-lint
5. Test — Unit tests (race + coverage)
6. Security — govulncheck, gosec
7. SBOM — Software Bill of Materials

**Artifacts:**
- `bin/` — Compiled binaries
- `coverage.out` — Test coverage report
- `sbom-results/` — SBOM files
- `gosec-report.json` — Security scan results

**Run:**
```bash
curl -X POST http://jenkins:8080/job/unheaded-main/buildWithParameters
```

### 2. Jenkinsfile.protocol

**Purpose:** Protocol Foundation builds (Go + Rust + eBPF)

**Trigger:** Commits to `theory/protocol-foundation` branch

**Stages:**
1. Go Lint & Test
2. Go Build
3. Go Security
4. Rust Check & Test
5. eBPF Build (nightly)
6. Rust Audit
7. Proto Validation
8. Integration Tests

**Artifacts:**
- Go binaries
- Coverage reports
- Security scan results

**Setup:**
```groovy
@Library('unheaded-shared') _

// In your pipeline
stage('Go') {
  steps {
    script {
      goSetup('1.24')
      testGo('-race -cover')
      coverageGate(60.0)
    }
  }
}
```

### 3. Jenkinsfile.docker

**Purpose:** Container image building with Trivy scanning

**Trigger:** Commits to `main` branch or manual

**Parameters:**
- `IMAGE_TAG` — Docker image tag (default: latest)
- `SCAN_ONLY` — Skip build, run Trivy only
- `PUSH_IMAGE` — Push to registry (default: false)

**Stages:**
1. Setup
2. Build Wotan Image
3. Build Kanban Image
4. Scan with Trivy
5. Push to Registry

**Artifacts:**
- Trivy vulnerability reports (JSON)

**Run:**
```bash
curl -X POST \
  -F 'IMAGE_TAG=v1.0.0' \
  -F 'PUSH_IMAGE=true' \
  http://jenkins:8080/job/unheaded-docker/buildWithParameters
```

### 4. Jenkinsfile.security

**Purpose:** Daily scheduled security audit

**Trigger:** Daily at 6 AM UTC (cron: `H 6 * * *`)

**Stages:**
1. Setup
2. Go Vulnerability Check (govulncheck)
3. Go Security Scan (gosec)
4. Rust Audit (cargo-audit)
5. Filesystem Scan (Trivy)
6. Report

**Artifacts:**
- `*-report.json` — Scan results (90-day retention)

**Result:** Archive for compliance & audit trail

### 5. Jenkinsfile.release

**Purpose:** Release builds with SBOM, provenance, checksums

**Trigger:** Manual only (high-risk operation)

**Parameters:**
- `VERSION` — Release version (e.g., v1.2.3)
- `DRY_RUN` — Dry run or live release (default: true)

**Stages:**
1. Validate — Version format check
2. Pre-Release Security — govulncheck scan
3. Build Binaries — Multi-platform compilation
4. Generate SBOM — CycloneDX + SPDX
5. Create Checksums — SHA256
6. Generate Provenance — SLSA placeholder
7. Sign Artifacts — cosign placeholder
8. Push to Registry
9. Create Release Notes

**Artifacts:**
- Binaries (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64)
- SBOM (CycloneDX, SPDX)
- Checksums + signatures
- Provenance
- Release notes

**Run (DRY):**
```bash
curl -X POST \
  -F 'VERSION=v1.2.3' \
  -F 'DRY_RUN=true' \
  http://jenkins:8080/job/unheaded-release/buildWithParameters
```

**Run (LIVE):**
```bash
curl -X POST \
  -F 'VERSION=v1.2.3' \
  -F 'DRY_RUN=false' \
  http://jenkins:8080/job/unheaded-release/buildWithParameters
```

---

## Shared Library Setup

### Installation

1. **Create shared library in Jenkins:**

   **Manage Jenkins → Configure System → Global Pipeline Libraries**

   - Name: `unheaded-shared`
   - Default version: `main`
   - Modern SCM: `Git`
   - Project repository: `https://github.com/unheaded-kingdom/unheaded.git`
   - Library path: `jenkins/`

2. **In pipeline, import:**

   ```groovy
   @Library('unheaded-shared') _

   pipeline {
     stages {
       stage('Build') {
         steps {
           script {
             goSetup('1.24')
             buildGo('v1.0.0', 'bin')
           }
         }
       }
       stage('Security') {
         steps {
           script {
             securityScan()
           }
         }
       }
       stage('SBOM') {
         steps {
           script {
             sbomGenerate('v1.0.0')
           }
         }
       }
     }
   }
   ```

### Available Functions

| Function | Purpose | Example |
|----------|---------|---------|
| `goSetup(version)` | Setup Go + cache modules | `goSetup('1.24')` |
| `rustSetup(stable, nightly)` | Setup Rust toolchains | `rustSetup('stable', 'nightly-2025-06-01')` |
| `securityScan()` | Run govulncheck + gosec | `securityScan()` |
| `sbomGenerate(version)` | Generate SBOM artifacts | `sbomGenerate('v1.0.0')` |
| `coverageGate(threshold)` | Fail if below threshold | `coverageGate(60.0)` |
| `testGo(args)` | Run Go tests | `testGo('-race -cover -timeout 300s')` |
| `lintGo()` | Run golangci-lint | `lintGo()` |
| `buildGo(version, output)` | Build Go binaries | `buildGo('v1.0.0', 'bin')` |
| `archiveResults(artifacts)` | Archive build artifacts | `archiveResults('bin/**/*,coverage.out')` |
| `validateVersion(version)` | Validate semver format | `validateVersion('v1.2.3')` |
| `releaseArtifacts(version)` | Build release artifacts | `releaseArtifacts('v1.2.3')` |

---

## Configuration

### Environment Variables

Set in Jenkins **Manage → Configure System → Global properties** or in pipeline:

```groovy
environment {
  GO_VERSION = '1.24'
  RUST_STABLE = 'stable'
  RUST_NIGHTLY = 'nightly-2025-06-01'
  REGISTRY = 'ghcr.io'
  IMAGE_NAME = "${REGISTRY}/${GIT_REPOSITORY_OWNER}/${GIT_REPOSITORY}"
  GOFLAGS = '-v -mod=readonly'
  CGO_ENABLED = '0'
}
```

### Docker Agent Configuration

**Required:** Jenkins running with Docker socket access

```groovy
agent {
  docker {
    image 'golang:1.25-alpine'
    args '-v /var/run/docker.sock:/var/run/docker.sock'
  }
}
```

### Credentials Setup

1. **GitHub Token** (for private repos)
   - **Manage Jenkins → Manage Credentials**
   - Add: **Secret text**
   - ID: `github-token`
   - Secret: Your GitHub PAT

2. **Container Registry** (for image pushes)
   - Add: **Username with password**
   - ID: `ghcr-credentials`
   - Username: `<github-username>`
   - Password: `<github-token>`

3. **cosign Private Key** (for artifact signing)
   - Add: **Secret file**
   - ID: `cosign-key`
   - File: `cosign.key`

### GitHub Webhook

1. **Repository → Settings → Webhooks**
2. **Add webhook:**
   - Payload URL: `http://jenkins:8080/github-webhook/`
   - Content type: `application/json`
   - Events: Push, Pull request
   - Active: ✓

---

## Running Pipelines

### Web UI

1. Go to `http://jenkins:8080/`
2. Click job name (e.g., `unheaded-main`)
3. Click **Build** or **Build with Parameters**
4. Enter parameters (if required)
5. Click **Build**

### Command Line

```bash
# Trigger main pipeline
curl -X POST http://jenkins:8080/job/unheaded-main/build

# Trigger with parameters
curl -X POST \
  -u admin:token \
  http://jenkins:8080/job/unheaded-release/buildWithParameters \
  -F 'VERSION=v1.2.3' \
  -F 'DRY_RUN=false'

# Get build status
curl http://jenkins:8080/job/unheaded-main/lastBuild/api/json | jq '.result'
```

### Jenkins CLI

```bash
# Download Jenkins CLI
curl http://jenkins:8080/jnlpJars/jenkins-cli.jar -o jenkins-cli.jar

# Trigger job
java -jar jenkins-cli.jar -s http://jenkins:8080 \
  build unheaded-main \
  -p VERSION=v1.0.0

# Get logs
java -jar jenkins-cli.jar -s http://jenkins:8080 \
  console unheaded-main <build-number>
```

---

## Monitoring & Troubleshooting

### View Build Logs

```
http://jenkins:8080/job/unheaded-main/<build-number>/console
```

### Common Issues

#### Docker: "Cannot connect to daemon"

**Cause:** Jenkins process lacks Docker socket access

**Solution:**
```bash
# Add jenkins user to docker group
sudo usermod -aG docker jenkins
sudo systemctl restart jenkins
```

#### Out of Memory

**Cause:** Large builds exceed heap size

**Solution:** Increase Jenkins heap
```bash
export JAVA_OPTS="-Xmx2048m"
jenkins --webroot=/var/cache/jenkins/war
```

#### Builds timeout

**Cause:** Long-running stage exceeds timeout

**Solution:** Adjust in pipeline:
```groovy
options {
  timeout(time: 60, unit: 'MINUTES')  // Increase timeout
}
```

---

## Integration with GitHub Actions

### Parallel CI/CD

- **GitHub Actions:** PRs, commits to `main/develop`
- **Jenkins:** Protocol builds, releases, container scans

Both integrate with same branch protection rules.

---

## Next Steps

1. **Import pipelines** into Jenkins
2. **Configure credentials** (GitHub, registry, cosign)
3. **Setup GitHub webhook** for auto-trigger
4. **Test on develop** branch before production
5. **Monitor builds** via Jenkins dashboard + Slack

---

## References

- [Jenkins Declarative Pipeline](https://www.jenkins.io/doc/book/pipeline/syntax/)
- [Jenkins Shared Libraries](https://www.jenkins.io/doc/book/pipeline/shared-libraries/)
- [Jenkins Docker Plugin](https://plugins.jenkins.io/docker-plugin/)
- [Groovy DSL Documentation](https://groovy-lang.org/documentation.html)
- S72 Battle Plan — Phases 8-10

---

**Last Updated:** 2026-02-27
**Status:** OPERATIONAL
