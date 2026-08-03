// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! THE SHIELD - WAF/Gateway for the Unheaded Kingdom
//!
//! A high-performance Web Application Firewall and reverse proxy gateway
//! built in pure Rust with minimal dependencies.
//!
//! # Features
//!
//! - HTTP/HTTPS termination with TLS 1.2/1.3 support
//! - Rule-based request filtering (SQL injection, XSS, custom patterns)
//! - IP allowlist/blocklist with CIDR support
//! - Token bucket rate limiting
//! - Circuit breaker for backend protection
//! - Connection pooling for backends
//! - Prometheus-compatible metrics
//!
//! # Performance
//!
//! Designed for 100k+ requests per second with sub-millisecond rule evaluation.
//!
//! # Crate shape
//!
//! This is a library with a thin `main.rs` binary on top, rather than a
//! binary-only crate. The distinction matters for dead-code analysis: in a
//! binary crate `pub` grants no exemption, so an item counts as used only if
//! the shipped binary reaches it. That mislabelled test-exercised code and
//! deliberate extension surface as dead, and drowned real findings in noise.
//!
//! With the split, `pub` means "part of the WAF's API" and `dead_code`
//! recovers its meaning: a warning here is a genuine claim that nothing —
//! binary, test, or downstream caller — uses the item. Keep it that way. Do
//! not reach for `#![allow(dead_code)]`; delete the item or make it `pub`
//! because it is really API.

pub mod circuit;
pub mod config;
pub mod metrics;
pub mod proxy;
pub mod ratelimit;
pub mod router;
pub mod rules;
pub mod server;
pub mod tls;
