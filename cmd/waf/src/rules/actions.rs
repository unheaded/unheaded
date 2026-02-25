//! Rule Actions for THE SHIELD
//!
//! Defines the actions that can be taken when a rule matches,
//! and the resulting decisions for request processing.

use std::time::Duration;

/// Actions that can be configured for rules
#[derive(Debug, Clone, PartialEq)]
pub enum Action {
    /// Block the request with a specific status code
    Block,
    /// Allow the request to proceed
    Allow,
    /// Log the request but continue processing
    Log,
    /// Apply rate limiting
    Limit,
}

impl Action {
    /// Parse action from string
    pub fn from_str(s: &str) -> Option<Self> {
        match s.to_lowercase().as_str() {
            "block" => Some(Action::Block),
            "allow" => Some(Action::Allow),
            "log" => Some(Action::Log),
            "limit" => Some(Action::Limit),
            _ => None,
        }
    }

    /// Get string representation
    pub fn as_str(&self) -> &'static str {
        match self {
            Action::Block => "block",
            Action::Allow => "allow",
            Action::Log => "log",
            Action::Limit => "limit",
        }
    }
}

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

impl RuleDecision {
    /// Check if this decision allows the request
    pub fn is_allow(&self) -> bool {
        matches!(self, RuleDecision::Allow | RuleDecision::Log)
    }

    /// Check if this decision blocks the request
    pub fn is_block(&self) -> bool {
        matches!(self, RuleDecision::Block { .. })
    }

    /// Check if this decision requires rate limiting
    pub fn is_rate_limit(&self) -> bool {
        matches!(self, RuleDecision::RateLimit { .. })
    }

    /// Get the status code for blocking (if applicable)
    pub fn block_status(&self) -> Option<u16> {
        match self {
            RuleDecision::Block { status, .. } => Some(*status),
            _ => None,
        }
    }

    /// Get the block message (if applicable)
    pub fn block_message(&self) -> Option<&str> {
        match self {
            RuleDecision::Block { message, .. } => Some(message),
            _ => None,
        }
    }
}

/// Action to take with full context
#[derive(Debug, Clone)]
pub struct RuleAction {
    /// The base action
    pub action: Action,
    /// HTTP status code (for block actions)
    pub status: u16,
    /// Response message (for block actions)
    pub message: String,
    /// Rate limit requests (for limit actions)
    pub rate_requests: Option<u32>,
    /// Rate limit period (for limit actions)
    pub rate_period: Option<Duration>,
}

impl Default for RuleAction {
    fn default() -> Self {
        Self {
            action: Action::Allow,
            status: 200,
            message: String::new(),
            rate_requests: None,
            rate_period: None,
        }
    }
}

impl RuleAction {
    /// Create a block action
    pub fn block(status: u16, message: impl Into<String>) -> Self {
        Self {
            action: Action::Block,
            status,
            message: message.into(),
            rate_requests: None,
            rate_period: None,
        }
    }

    /// Create an allow action
    pub fn allow() -> Self {
        Self {
            action: Action::Allow,
            ..Default::default()
        }
    }

    /// Create a log action
    pub fn log() -> Self {
        Self {
            action: Action::Log,
            ..Default::default()
        }
    }

    /// Create a rate limit action
    pub fn rate_limit(requests: u32, period: Duration) -> Self {
        Self {
            action: Action::Limit,
            status: 429,
            message: "Rate limit exceeded".to_string(),
            rate_requests: Some(requests),
            rate_period: Some(period),
        }
    }

    /// Convert to a rule decision
    pub fn to_decision(&self) -> RuleDecision {
        match self.action {
            Action::Block => RuleDecision::Block {
                status: self.status,
                message: self.message.clone(),
            },
            Action::Allow => RuleDecision::Allow,
            Action::Log => RuleDecision::Log,
            Action::Limit => RuleDecision::RateLimit {
                requests: self.rate_requests.unwrap_or(100),
                period: self.rate_period.unwrap_or(Duration::from_secs(60)),
            },
        }
    }
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

    /// Create a plain text block response
    pub fn text(status: u16, message: &str) -> Self {
        Self {
            status,
            body: message.to_string(),
            content_type: "text/plain",
        }
    }

    /// Create an HTML block response
    pub fn html(status: u16, message: &str) -> Self {
        let body = format!(
            r#"<!DOCTYPE html>
<html>
<head><title>Request Blocked</title></head>
<body>
<h1>{} {}</h1>
<p>{}</p>
<hr>
<p><em>THE SHIELD - Unheaded Kingdom WAF</em></p>
</body>
</html>"#,
            status,
            status_text(status),
            html_escape(message)
        );

        Self {
            status,
            body,
            content_type: "text/html",
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

/// Escape a string for HTML
fn html_escape(s: &str) -> String {
    let mut result = String::with_capacity(s.len());
    for c in s.chars() {
        match c {
            '<' => result.push_str("&lt;"),
            '>' => result.push_str("&gt;"),
            '&' => result.push_str("&amp;"),
            '"' => result.push_str("&quot;"),
            '\'' => result.push_str("&#x27;"),
            c => result.push(c),
        }
    }
    result
}

/// Get status text for HTTP status code
fn status_text(status: u16) -> &'static str {
    match status {
        400 => "Bad Request",
        401 => "Unauthorized",
        403 => "Forbidden",
        404 => "Not Found",
        405 => "Method Not Allowed",
        429 => "Too Many Requests",
        500 => "Internal Server Error",
        502 => "Bad Gateway",
        503 => "Service Unavailable",
        504 => "Gateway Timeout",
        _ => "Error",
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_action_parsing() {
        assert_eq!(Action::from_str("block"), Some(Action::Block));
        assert_eq!(Action::from_str("ALLOW"), Some(Action::Allow));
        assert_eq!(Action::from_str("Log"), Some(Action::Log));
        assert_eq!(Action::from_str("LIMIT"), Some(Action::Limit));
        assert_eq!(Action::from_str("invalid"), None);
    }

    #[test]
    fn test_rule_decision() {
        let allow = RuleDecision::Allow;
        assert!(allow.is_allow());
        assert!(!allow.is_block());

        let block = RuleDecision::Block {
            status: 403,
            message: "Forbidden".to_string(),
        };
        assert!(!block.is_allow());
        assert!(block.is_block());
        assert_eq!(block.block_status(), Some(403));
        assert_eq!(block.block_message(), Some("Forbidden"));

        let rate_limit = RuleDecision::RateLimit {
            requests: 100,
            period: Duration::from_secs(60),
        };
        assert!(rate_limit.is_rate_limit());
    }

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

    #[test]
    fn test_html_escape() {
        assert_eq!(html_escape("<script>"), "&lt;script&gt;");
        assert_eq!(html_escape("a & b"), "a &amp; b");
    }
}
