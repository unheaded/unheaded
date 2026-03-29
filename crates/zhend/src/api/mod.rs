//! # API — External Interfaces
//!
//! Two transports, one truth:
//! - gRPC (tonic) — internal Kingdom services, streaming, deadlines
//! - QUIC/HTTP3 (quinn) — external access, connection migration, 0-RTT
//!
//! Both expose the same operations:
//! - Ingest: submit a new Pu fragment
//! - Surface: request De-ranked fragments for a context
//! - Status: node health, strata snapshot, gossip stats
//! - Pilgrimage: initiate deep retrieval from Jing

pub mod grpc;
pub mod quic;
