// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//! # API — External Interfaces
//!
//! One transport: gRPC (tonic), loopback by default, no auth or TLS.
//!
//! Operations:
//! - Ingest: submit a new Pu fragment
//! - Surface: request De-ranked fragments for a context
//! - Status: node health, tier counts, gossip stats
//! - Pilgrimage: initiate deep retrieval from Jing
//!
//! The QUIC server that stood beside it was never spawned and is archived in
//! `unheaded-zhend-subsystems` (ADR-152).

pub mod grpc;
