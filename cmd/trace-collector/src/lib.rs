//! THE WHISPERING VOID - Library Root
//!
//! This crate provides the core functionality for eBPF event collection,
//! parsing, and publishing to the Busboy message bus.
//!
//! # Architecture
//!
//! The trace-collector operates as a bridge between kernel-space eBPF programs
//! and the Busboy message bus. It provides:
//!
//! - **BPF Readers**: Zero-copy event reading from ring buffers and perf arrays
//! - **Event Parsing**: Strongly-typed event structures matching eBPF definitions
//! - **Publisher**: Batched, compressed gRPC client for Busboy
//! - **Metrics**: Prometheus-compatible metrics endpoint
//! - **Proto**: Protocol buffer definitions for Busboy communication
//! - **Collector**: High-level event collection orchestration
//!
//! # Modules
//!
//! - `bpf` - Low-level BPF map and syscall interaction
//! - `collector` - High-level event collection orchestration
//! - `config` - Configuration loading and management
//! - `events` - Event types and parsing
//! - `metrics` - Prometheus metrics and HTTP server
//! - `proto` - Protocol buffer definitions
//! - `publisher` - Busboy gRPC client and event batching
//!
//! # Example Usage
//!
//! ```ignore
//! use trace_collector::{Config, Event, BusboyPublisher};
//! use std::time::Duration;
//!
//! let config = Config::default();
//! let publisher = BusboyPublisher::new(
//!     "http://localhost:50051",
//!     1000,
//!     Duration::from_millis(10)
//! ).await?;
//! ```
//!
//! # Performance Targets
//!
//! - Sub-microsecond event read latency
//! - 1M+ events/second throughput
//! - Zero-copy where possible
//! - Lock-free data structures

pub mod bpf;
pub mod collector;
pub mod config;
pub mod events;
pub mod metrics;
pub mod proto;
pub mod publisher;

// Re-export commonly used types
pub use config::Config;
pub use events::{Event, EventBatch, EventData, EventType, LatencyEvent, PacketEvent, SyscallEvent};
pub use publisher::{BusboyPublisher, PublisherState, PublisherStats};

// Re-export collector types
pub use collector::{
    Collector, CollectorBuilder, CollectorConfig, CollectorError, CollectorState, CollectorStats,
    EventFilter, EventProcessor,
};

// Re-export proto types
pub use proto::{
    EventType as ProtoEventType, FlowData, FlowKey, FlowState, HealthRequest, HealthResponse,
    LatencyData, PublishRequest, PublishResponse, SpanId, SubscribeRequest, TraceEvent,
    TraceEventBatch, TraceId,
};

// Re-export metrics types
pub use metrics::{AtomicMetrics, MetricsHttpServer, MetricsServer, ServerConfig, ATOMIC_METRICS};
