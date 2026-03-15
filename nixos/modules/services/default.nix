# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# default.nix — Unheaded Kingdom systemd service modules
#
# Import all 25 services across the armor layers

{
  imports = [
    # Layer 1: Core Services
    ./monad.nix                    # Monad register (20-byte IPv6 HbH extension header)
    ./sophia.nix                   # Sophia BPF dictionary maps
    ./wotan.nix                    # Wotan message bus & event broker (NATS)
    ./anamnesis.nix                # Anamnesis event log & stream
    ./shield.nix                   # Shield eBPF policy enforcement
    
    # Layer 2: Control Plane
    ./unheaded-daemon.nix          # Unheaded daemon Cuirass control plane reconciler
    
    # Layer 3: Frontend & Dashboard
    ./dashboard-backend.nix        # Dashboard backend Cape & Cloak
    ./dashboard-frontend.nix       # Dashboard frontend UI static serve
    
    # Layer 4: Protocol & API
    ./protocol-api.nix             # Protocol API wire format validation & Gauntlets
    ./trace-collector.nix          # Trace collector Zipkin-compatible
    ./gateway.nix                  # Gateway API gateway & ingress
    
    # Layer 5: Infrastructure Services
    ./service-discovery.nix        # Service discovery registry & Greaves
    ./doom.nix                     # DOOM BPF compute substrate
    
    # Layer 6: Red Team & Security
    ./lich.nix                     # Lich automated adversary & red team continuous
    
    # Layer 7: Strategy & Leadership
    ./captain.nix                  # Captain executive strategy
    ./micromanager.nix             # Micromanager engineering leadership
    ./timeguru.nix                 # Timeguru timeline & roadmap
    ./architect.nix                # Architect infrastructure architecture
    ./developer.nix                # Developer development assistance
    
    # Layer 8: Coordination & Organization
    ./kanban.nix                   # Kanban board UI & API
    ./lore.nix                     # Lore naming & mythology
    ./busboy.nix                   # Busboy coordinator & glue
    ./kingdom.nix                  # Kingdom hierarchy reference
    
    # Layer 9: Security & Compliance
    ./blackmage.nix                # Blackmage security & offensive
    ./moatghost.nix                # Moatghost compliance & audit
  ];
}
