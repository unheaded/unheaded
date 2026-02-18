#!/usr/bin/env bash
#
# Unheaded Project Initialization Script
# Creates complete directory structure and placeholder files
#
# Usage: ./scripts/init-project.sh
#

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# Get project root (script should be in scripts/)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

log_info "Project root: $PROJECT_ROOT"
log_info "Initializing Unheaded project structure..."

# Change to project root
cd "$PROJECT_ROOT"

# Create directory structure
log_info "Creating directories..."

# Service binaries
mkdir -p cmd/unheaded-daemon
mkdir -p cmd/trace-collector
mkdir -p cmd/dashboard-backend
mkdir -p cmd/kanban-app

# Microservices
mkdir -p services/wotan
mkdir -p services/timeguru
mkdir -p services/captain
mkdir -p services/micromanager
mkdir -p services/architect

# eBPF programs
mkdir -p ebpf/src
mkdir -p ebpf/target

# NixOS containers
mkdir -p nix/containers
mkdir -p nix/modules

# Dashboard UI
mkdir -p dashboard/css
mkdir -p dashboard/js
mkdir -p dashboard/assets/images
mkdir -p dashboard/assets/fonts

# Kanban UI
mkdir -p kanban/css
mkdir -p kanban/js
mkdir -p kanban/assets/images
mkdir -p kanban/assets/fonts

# Shared Go packages
mkdir -p pkg/lxd
mkdir -p pkg/state
mkdir -p pkg/telemetry
mkdir -p pkg/wotan-client

# Documentation
mkdir -p docs

# Scripts (already exists but ensure)
mkdir -p scripts

# References
mkdir -p references

# Test directories
mkdir -p tests/unit
mkdir -p tests/integration
mkdir -p tests/e2e

# Build output
mkdir -p bin
mkdir -p build

log_success "Directories created"

# Create placeholder Go files
log_info "Creating Go service placeholders..."

# unheaded-daemon
cat > cmd/unheaded-daemon/main.go <<'EOF'
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("unheaded-daemon starting...")
	// TODO: Implement LXD orchestration
	// TODO: Load eBPF programs
	// TODO: State enforcement
	// TODO: Telemetry reporting
	os.Exit(0)
}
EOF

# dashboard-backend
cat > cmd/dashboard-backend/main.go <<'EOF'
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("dashboard-backend starting...")
	// TODO: Connect to Wotan
	// TODO: WebSocket server
	// TODO: Metrics aggregation
	os.Exit(0)
}
EOF

# kanban-app
cat > cmd/kanban-app/main.go <<'EOF'
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("kanban-app starting...")
	// TODO: HTTP server
	// TODO: Serve static files
	// TODO: Proxy to timeguru API
	os.Exit(0)
}
EOF

log_success "Go service placeholders created"

# Create Rust eBPF placeholders
log_info "Creating Rust eBPF placeholders..."

# Cargo.toml for eBPF
cat > ebpf/Cargo.toml <<'EOF'
[package]
name = "unheaded-ebpf"
version = "0.1.0"
edition = "2021"

[dependencies]
aya = "0.12"
aya-log = "0.2"

[[bin]]
name = "packet_marker"
path = "src/packet_marker.rs"

[[bin]]
name = "flow_tracker"
path = "src/flow_tracker.rs"

[[bin]]
name = "latency_probe"
path = "src/latency_probe.rs"
EOF

# packet_marker.rs
cat > ebpf/src/packet_marker.rs <<'EOF'
// packet_marker.rs
// eBPF program to inject trace_id into packets at XDP layer

fn main() {
    println!("packet_marker eBPF program");
    // TODO: Implement XDP hook
    // TODO: Generate trace_id
    // TODO: Inject into packet metadata
}
EOF

# flow_tracker.rs
cat > ebpf/src/flow_tracker.rs <<'EOF'
// flow_tracker.rs
// eBPF program to track connection flows

fn main() {
    println!("flow_tracker eBPF program");
    // TODO: Track src/dst IP/port
    // TODO: Emit to ring buffer
}
EOF

# latency_probe.rs
cat > ebpf/src/latency_probe.rs <<'EOF'
// latency_probe.rs
// eBPF program to measure RTT

fn main() {
    println!("latency_probe eBPF program");
    // TODO: Measure packet timestamps
    // TODO: Calculate RTT
}
EOF

# trace-collector
cat > cmd/trace-collector/Cargo.toml <<'EOF'
[package]
name = "trace-collector"
version = "0.1.0"
edition = "2021"

[dependencies]
tokio = { version = "1", features = ["full"] }
aya = "0.12"
EOF

mkdir -p cmd/trace-collector/src

cat > cmd/trace-collector/src/main.rs <<'EOF'
// trace-collector
// Reads eBPF ring buffer and publishes to Wotan

fn main() {
    println!("trace-collector starting...");
    // TODO: Read eBPF ring buffer
    // TODO: Connect to Wotan
    // TODO: Publish to network.traces topic
}
EOF

log_success "Rust eBPF placeholders created"

# Create NixOS flake structure
log_info "Creating NixOS flake..."

cat > nix/flake.nix <<'EOF'
{
  description = "Unheaded Alpha - Infrastructure Platform";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }: {
    # Container definitions
    nixosConfigurations = {
      wotan = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [ ./containers/wotan.nix ];
      };

      timeguru = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [ ./containers/timeguru.nix ];
      };

      # TODO: Add other containers
    };
  };
}
EOF

# Container placeholder
cat > nix/containers/wotan.nix <<'EOF'
{ config, pkgs, ... }:

{
  # TODO: Import Wotan service definition
  # TODO: Apply hardening module
  # TODO: Configure networking
}
EOF

# Hardening module
cat > nix/modules/hardening.nix <<'EOF'
{ config, lib, pkgs, ... }:

{
  # Security hardening for all containers
  # TODO: Seccomp filters
  # TODO: Capability restrictions
  # TODO: Filesystem isolation
}
EOF

log_success "NixOS flake created"

# Create dashboard UI
log_info "Creating dashboard UI..."

cat > dashboard/index.html <<'EOF'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Unheaded Dashboard</title>
    <link rel="stylesheet" href="/css/style.css">
</head>
<body>
    <canvas id="particle-canvas"></canvas>

    <nav>
        <h1>Unheaded Dashboard</h1>
        <ul>
            <li><a href="/">Dashboard</a></li>
            <li><a href="/kanban">Kanban</a></li>
        </ul>
    </nav>

    <main>
        <section id="packet-flow">
            <h2>Real-time Packet Flow</h2>
            <div id="flow-viz"></div>
        </section>

        <section id="metrics">
            <h2>System Metrics</h2>
            <div id="metrics-grid"></div>
        </section>
    </main>

    <script src="/js/particles.js"></script>
    <script src="/js/packet-flow.js"></script>
    <script src="/js/metrics.js"></script>
</body>
</html>
EOF

cat > dashboard/css/style.css <<'EOF'
/* Unheaded Dashboard Styles */
/* Inspired by bellis.tech */

* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

body {
    background: #0a0a0a;
    color: #e0e0e0;
    font-family: 'Courier New', monospace;
}

#particle-canvas {
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    z-index: -1;
}

nav {
    padding: 1rem 2rem;
    background: rgba(0, 0, 0, 0.8);
    border-bottom: 1px solid #333;
}

/* TODO: Add more styles */
EOF

cat > dashboard/js/particles.js <<'EOF'
// Particle animation (bellis.tech style)
// TODO: Implement canvas particle animation
console.log('particles.js loaded');
EOF

cat > dashboard/js/packet-flow.js <<'EOF'
// Real-time packet flow visualization
// TODO: WebSocket connection to dashboard-backend
// TODO: D3.js network graph
console.log('packet-flow.js loaded');
EOF

log_success "Dashboard UI created"

# Create Kanban UI
log_info "Creating Kanban UI..."

cat > kanban/index.html <<'EOF'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Unheaded Alpha - Built by Unheaded 🔄</title>
    <link rel="stylesheet" href="/css/kanban.css">
</head>
<body>
    <canvas id="particle-canvas"></canvas>

    <header>
        <h1>Unheaded Alpha - Built by Unheaded 🔄</h1>
        <div class="live-indicator"></div>
    </header>

    <main>
        <div id="kanban-board">
            <div class="column" id="todo">
                <h2>TODO</h2>
                <div class="cards"></div>
            </div>
            <div class="column" id="in-progress">
                <h2>IN PROGRESS</h2>
                <div class="cards"></div>
            </div>
            <div class="column" id="done">
                <h2>DONE</h2>
                <div class="cards"></div>
            </div>
        </div>
    </main>

    <script src="/js/particles.js"></script>
    <script src="/js/timeline-reader.js"></script>
    <script src="/js/board-viz.js"></script>
</body>
</html>
EOF

cat > kanban/js/timeline-reader.js <<'EOF'
// Reads timeline from timeguru API
// TODO: Fetch /api/v1/timeline
// TODO: Parse JSON
// TODO: Update board
console.log('timeline-reader.js loaded');
EOF

log_success "Kanban UI created"

# Create shared Go packages
log_info "Creating shared Go packages..."

cat > pkg/wotan-client/client.go <<'EOF'
package wotanClient

// Client for connecting to Wotan message bus
type Client struct {
	// TODO: gRPC connection
}

// NewClient creates a new Wotan client
func NewClient(addr string) (*Client, error) {
	// TODO: Connect to Wotan
	return &Client{}, nil
}
EOF

cat > pkg/lxd/client.go <<'EOF'
package lxd

// Client for LXD orchestration
type Client struct {
	// TODO: LXD connection
}

// NewClient creates a new LXD client
func NewClient() (*Client, error) {
	// TODO: Connect to LXD
	return &Client{}, nil
}
EOF

log_success "Shared packages created"

# Create documentation placeholders
log_info "Creating documentation placeholders..."

cat > docs/MICROSERVICES.md <<'EOF'
# Microservices Architecture

## Service Catalog

### timeguru
- Timeline tracking and milestone management
- Maintains timeline.md with JSON/YAML mirrors
- REST API: GET/POST /api/v1/timeline

### captain
- Strategic decisions and vision alignment
- REST API: GET/POST /api/v1/strategy

### micromanager
- Task breakdown and QA oversight
- REST API: GET/POST /api/v1/tasks

### architect
- Infrastructure design and tech decisions
- REST API: GET/POST /api/v1/designs

## Communication Pattern

All services communicate via Wotan (message bus).

TODO: Add detailed service specifications
EOF

cat > docs/SECURITY.md <<'EOF'
# Security Architecture

## Principles

1. Zero customer data access (architectural isolation)
2. Container hardening (seccomp, capabilities)
3. Network policies (explicit allow, default deny)
4. TLS 1.3 minimum
5. eBPF safety (kernel verified)

## Container Security

TODO: Document hardening specifications

## Network Security

TODO: Document network policies

## Secrets Management

TODO: Document SOPS + age usage
EOF

log_success "Documentation placeholders created"

# Create reference files
log_info "Creating reference file structure..."

# timeline.json and timeline.yaml will be auto-generated
touch references/timeline.json
touch references/timeline.yaml

# Add .gitkeep for empty dirs
find . -type d -empty -exec touch {}/.gitkeep \;

log_success "Reference files created"

# Create .gitignore
log_info "Creating .gitignore..."

cat > .gitignore <<'EOF'
# Build outputs
/bin/
/build/
/target/
*.o
*.so

# Go
vendor/
*.test
*.prof

# Rust
Cargo.lock
target/

# Node
node_modules/
npm-debug.log

# IDE
.vscode/
.idea/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Nix
result
result-*

# Secrets
*.key
*.pem
secrets/

# Logs
*.log
logs/

# Temporary
tmp/
.tmp/
EOF

log_success ".gitignore created"

# Summary
echo ""
log_success "========================================"
log_success "  Unheaded Project Initialized!"
log_success "========================================"
echo ""
echo "Directory structure created:"
echo "  ✓ cmd/          (4 services)"
echo "  ✓ services/     (5 microservices)"
echo "  ✓ ebpf/         (3 programs)"
echo "  ✓ nix/          (containers + modules)"
echo "  ✓ dashboard/    (UI)"
echo "  ✓ kanban/       (Meta moment UI)"
echo "  ✓ pkg/          (4 shared packages)"
echo "  ✓ docs/         (4 docs)"
echo "  ✓ scripts/      (automation)"
echo "  ✓ references/   (source of truth)"
echo ""
echo "Placeholder files created:"
echo "  ✓ Go services (main.go)"
echo "  ✓ Rust eBPF (*.rs + Cargo.toml)"
echo "  ✓ NixOS flake"
echo "  ✓ Dashboard HTML/CSS/JS"
echo "  ✓ Kanban HTML/CSS/JS"
echo "  ✓ Shared packages"
echo "  ✓ Documentation"
echo ""
echo "Next steps:"
echo "  1. Review structure: tree -L 2"
echo "  2. Initialize Go modules: cd services/timeguru && go mod init"
echo "  3. Start building: Pick a service and implement!"
echo ""
log_info "Run 'make help' for build commands"
echo ""
