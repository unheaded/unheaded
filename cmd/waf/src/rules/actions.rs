// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Rule Actions for THE SHIELD
//!
//! Defines the actions that can be taken when a rule matches,
//! and the resulting decisions for request processing.
//!
//! # Removed 2026-08-03
//!
//! This module used to also define an `Action` enum and a `RuleAction`
//! struct. Both were dead, and the `RuleAction` name was actively dangerous:
//! `matcher.rs` defines a *different* `RuleAction` — an enum, not a struct —
//! and that one is the type the enforcement path actually uses
//! (`matcher.rs` maps it to [`RuleDecision`], `router.rs` acts on the result).
//! Two same-named types of different kinds in sibling modules is a trap for
//! whoever edits next, so the unused copy is gone.
//!
//! Also removed: the five `RuleDecision` predicates (`is_allow`, `is_block`,
//! `is_rate_limit`, `block_status`, `block_message`), which had no caller
//! outside this file\'s own tests — `router.rs` matches on the enum directly;
//! and `BlockResponse::{text, html}`, which nothing constructed. Deleting
//! `html` took `html_escape` and `status_text` with it. **If an HTML block
//! response is ever reintroduced, the escaping must come back with it** — the
//! message it renders is attacker-influenced.

use std::time::Duration;

/// The decision resulting from rule evaluation
#[derive(Debug, Clone)]
pub enum RuleDecision {
    /// Allow the request
    Allow,

    /// Block the request
    Block {
        /// HTTP status code to return
        status: u16,
        /// Message to include in response
        message: String,
    },

    /// Log the request (but allow it)
    Log,

    /// Apply rate limiting
    RateLimit {
        /// Maximum requests
        requests: u32,
        /// Time period
        period: Duration,
    },
}

/// Response builder for blocked requests
pub struct BlockResponse {
    /// HTTP status code
    pub status: u16,
    /// Response body
    pub body: String,
    /// Content type
    pub content_type: &'static str,
}

impl BlockResponse {
    /// Create a JSON block response
    pub fn json(status: u16, message: &str, rule_id: Option<&str>) -> Self {
        let body = match rule_id {
            Some(id) => format!(
                r#"{{"error":"blocked","message":"{}","rule":"{}"}}"#,
                escape_json(message),
                escape_json(id)
            ),
            None => format!(
                r#"{{"error":"blocked","message":"{}"}}"#,
                escape_json(message)
            ),
        };

        Self {
            status,
            body,
            content_type: "application/json",
        }
    }
}

/// Escape a string for JSON
fn escape_json(s: &str) -> String {
    let mut result = String::with_capacity(s.len());
    for c in s.chars() {
        match c {
            '"' => result.push_str("\\\""),
            '\\' => result.push_str("\\\\"),
            '\n' => result.push_str("\\n"),
            '\r' => result.push_str("\\r"),
            '\t' => result.push_str("\\t"),
            c if c.is_control() => {
                result.push_str(&format!("\\u{:04x}", c as u32));
            }
            c => result.push(c),
        }
    }
    result
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_block_response_json() {
        let response = BlockResponse::json(403, "Access denied", Some("rule-1"));
        assert_eq!(response.status, 403);
        assert_eq!(response.content_type, "application/json");
        assert!(response.body.contains("Access denied"));
        assert!(response.body.contains("rule-1"));
    }

    #[test]
    fn test_escape_json() {
        assert_eq!(escape_json("hello"), "hello");
        assert_eq!(escape_json("he\"llo"), "he\\\"llo");
        assert_eq!(escape_json("line1\nline2"), "line1\\nline2");
    }
}
