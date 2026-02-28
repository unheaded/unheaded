//! WebSocket Server for Real-time Trace Streaming
//!
//! THE WHISPERING VOID speaks directly to the dashboard.
//!
//! This module provides a WebSocket server that streams trace events
//! to connected dashboard clients in real-time.
//!
//! ## Features
//!
//! - Real-time trace event streaming
//! - Subscription filtering (by service, event type)
//! - Automatic reconnection handling
//! - Binary and JSON message formats
//! - Heartbeat/ping-pong keepalive

pub mod handler;
pub mod protocol;
pub mod server;

pub use handler::WebSocketHandler;
pub use protocol::{ClientMessage, ServerMessage, Subscription, TraceUpdate};
pub use server::{WebSocketConfig, WebSocketServer, WebSocketStats};
