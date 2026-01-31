//! Pattern Matcher for THE SHIELD
//!
//! High-performance pattern matching for WAF rules.
//! Uses compiled regex with set matching for sub-millisecond evaluation.

use crate::config::RuleConfig;
use crate::rules::parser::{MethodFilter, PathPattern, RateLimit};
use crate::rules::{RuleDecision, RuleEngineError};
use regex::{Regex, RegexSet};
use std::net::IpAddr;

/// Request data for rule evaluation
#[derive(Debug, Clone)]
pub struct RequestData {
    /// HTTP method
    pub method: String,
    /// Request path
    pub path: String,
    /// Query string (without leading ?)
    pub query_string: Option<String>,
    /// Request headers (name, value)
    pub headers: Vec<(String, String)>,
    /// Request body (for POST/PUT)
    pub body: Option<Vec<u8>>,
    /// Client IP address
    pub client_ip: Option<IpAddr>,
}

impl RequestData {
    /// Get full URI (path + query)
    pub fn uri(&self) -> String {
        match &self.query_string {
            Some(q) if !q.is_empty() => format!("{}?{}", self.path, q),
            _ => self.path.clone(),
        }
    }

    /// Get a header value by name (case-insensitive)
    pub fn header(&self, name: &str) -> Option<&str> {
        let name_lower = name.to_lowercase();
        self.headers
            .iter()
            .find(|(n, _)| n.to_lowercase() == name_lower)
            .map(|(_, v)| v.as_str())
    }

    /// Get body as string (if valid UTF-8)
    pub fn body_str(&self) -> Option<&str> {
        self.body.as_ref().and_then(|b| std::str::from_utf8(b).ok())
    }
}

/// Target for pattern matching
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum MatchTarget {
    /// Match against query string
    Query,
    /// Match against path
    Path,
    /// Match against specific header
    Header,
    /// Match against body
    Body,
    /// Match against method
    Method,
    /// Match against full URI
    Uri,
    /// Match against all targets
    All,
}

impl MatchTarget {
    /// Parse target from string
    pub fn from_str(s: &str) -> Self {
        match s.to_lowercase().as_str() {
            "query" => MatchTarget::Query,
            "path" => MatchTarget::Path,
            "header" => MatchTarget::Header,
            "body" => MatchTarget::Body,
            "method" => MatchTarget::Method,
            "uri" => MatchTarget::Uri,
            _ => MatchTarget::All,
        }
    }
}

/// High-performance pattern matcher using regex sets
pub struct PatternMatcher {
    /// Compiled regex set for fast multi-pattern matching
    regex_set: RegexSet,
    /// Individual patterns for capture groups (if needed)
    patterns: Vec<Regex>,
}

impl PatternMatcher {
    /// Create a new pattern matcher from regex patterns
    pub fn new(patterns: &[&str]) -> Result<Self, String> {
        let regex_set = RegexSet::new(patterns)
            .map_err(|e| format!("Failed to compile pattern set: {}", e))?;

        let compiled: Result<Vec<_>, _> = patterns.iter().map(|p| Regex::new(p)).collect();

        let patterns =
            compiled.map_err(|e| format!("Failed to compile individual pattern: {}", e))?;

        Ok(Self {
            regex_set,
            patterns,
        })
    }

    /// Check if any pattern matches the input
    #[inline]
    pub fn is_match(&self, text: &str) -> bool {
        self.regex_set.is_match(text)
    }

    /// Get all matching pattern indices
    pub fn matches(&self, text: &str) -> Vec<usize> {
        self.regex_set.matches(text).iter().collect()
    }

    /// Get the first matching pattern index
    pub fn first_match(&self, text: &str) -> Option<usize> {
        self.regex_set.matches(text).iter().next()
    }
}

/// A compiled rule ready for evaluation
pub struct CompiledRule {
    /// Rule identifier
    pub id: String,
    /// Rule priority (lower = higher priority)
    pub priority: i32,
    /// Pattern matcher (if pattern is specified)
    pub pattern: Option<PatternMatcher>,
    /// Match target
    pub target: MatchTarget,
    /// Header name (if target is Header)
    pub header_name: Option<String>,
    /// Path pattern (if specified)
    pub path_pattern: Option<PathPattern>,
    /// Method filter
    pub method_filter: MethodFilter,
    /// Action to take
    pub action: RuleAction,
    /// Whether to log matches
    pub log: bool,
    /// Rate limit (if action is Limit)
    pub rate_limit: Option<RateLimit>,
}

/// Action to take when a rule matches
#[derive(Debug, Clone)]
pub enum RuleAction {
    /// Block the request
    Block { status: u16, message: String },
    /// Allow the request
    Allow,
    /// Log the request but continue
    Log,
    /// Apply rate limiting
    Limit,
}

impl CompiledRule {
    /// Evaluate this rule against a request
    pub fn evaluate(&self, request: &RequestData) -> Option<RuleEvaluationResult> {
        // Check method filter first (fast)
        if !self.method_filter.matches(&request.method) {
            return None;
        }

        // Check path pattern if specified
        if let Some(ref path_pattern) = self.path_pattern {
            if !path_pattern.matches(&request.path) {
                return None;
            }
        }

        // Check content pattern if specified
        if let Some(ref pattern) = self.pattern {
            if !self.check_pattern(pattern, request) {
                return None;
            }
        }

        // Rule matched
        let decision = match &self.action {
            RuleAction::Block { status, message } => RuleDecision::Block {
                status: *status,
                message: message.clone(),
            },
            RuleAction::Allow => RuleDecision::Allow,
            RuleAction::Log => RuleDecision::Log,
            RuleAction::Limit => {
                if let Some(ref rl) = self.rate_limit {
                    RuleDecision::RateLimit {
                        requests: rl.requests,
                        period: rl.period,
                    }
                } else {
                    RuleDecision::Allow
                }
            }
        };

        Some(RuleEvaluationResult {
            rule_id: self.id.clone(),
            decision,
        })
    }

    /// Check pattern against appropriate request data
    fn check_pattern(&self, pattern: &PatternMatcher, request: &RequestData) -> bool {
        match self.target {
            MatchTarget::Query => request
                .query_string
                .as_ref()
                .map(|q| pattern.is_match(q))
                .unwrap_or(false),

            MatchTarget::Path => pattern.is_match(&request.path),

            MatchTarget::Header => {
                if let Some(ref header_name) = self.header_name {
                    request
                        .header(header_name)
                        .map(|v| pattern.is_match(v))
                        .unwrap_or(false)
                } else {
                    // Check all headers
                    request.headers.iter().any(|(_, v)| pattern.is_match(v))
                }
            }

            MatchTarget::Body => request
                .body_str()
                .map(|b| pattern.is_match(b))
                .unwrap_or(false),

            MatchTarget::Method => pattern.is_match(&request.method),

            MatchTarget::Uri => pattern.is_match(&request.uri()),

            MatchTarget::All => {
                // Check all targets
                if let Some(ref q) = request.query_string {
                    if pattern.is_match(q) {
                        return true;
                    }
                }

                if pattern.is_match(&request.path) {
                    return true;
                }

                for (_, v) in &request.headers {
                    if pattern.is_match(v) {
                        return true;
                    }
                }

                if let Some(body) = request.body_str() {
                    if pattern.is_match(body) {
                        return true;
                    }
                }

                false
            }
        }
    }
}

/// Result of evaluating a single rule
#[derive(Debug, Clone)]
pub struct RuleEvaluationResult {
    /// ID of the rule that matched
    pub rule_id: String,
    /// Decision from the rule
    pub decision: RuleDecision,
}

impl TryFrom<&RuleConfig> for CompiledRule {
    type Error = RuleEngineError;

    fn try_from(config: &RuleConfig) -> Result<Self, Self::Error> {
        // Compile pattern if present
        let pattern = if let Some(ref p) = config.pattern {
            Some(PatternMatcher::new(&[p]).map_err(RuleEngineError::PatternError)?)
        } else {
            None
        };

        // Parse path pattern if present
        let path_pattern = if let Some(ref path) = config.path {
            Some(PathPattern::parse(path).map_err(RuleEngineError::PatternError)?)
        } else {
            None
        };

        // Parse method filter
        let method_filter = MethodFilter::from_config(config.methods.as_deref());

        // Parse action
        let action = match config.action.as_str() {
            "block" => RuleAction::Block {
                status: config.block_status,
                message: config
                    .block_message
                    .clone()
                    .unwrap_or_else(|| "Request blocked by WAF".to_string()),
            },
            "allow" => RuleAction::Allow,
            "log" => RuleAction::Log,
            "limit" => RuleAction::Limit,
            other => {
                return Err(RuleEngineError::RuleError(format!(
                    "Invalid action: {}",
                    other
                )))
            }
        };

        // Parse rate limit if action is limit
        let rate_limit = if config.action == "limit" {
            if let (Some(rate), Some(per)) = (config.rate, config.per.as_ref()) {
                Some(RateLimit::from_config(rate, per).map_err(RuleEngineError::PatternError)?)
            } else {
                return Err(RuleEngineError::RuleError(
                    "Rate limit rule requires 'rate' and 'per' fields".to_string(),
                ));
            }
        } else {
            None
        };

        Ok(Self {
            id: config.id.clone(),
            priority: config.priority,
            pattern,
            target: MatchTarget::from_str(&config.target),
            header_name: config.header_name.clone(),
            path_pattern,
            method_filter,
            action,
            log: config.log,
            rate_limit,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_pattern_matcher() {
        let matcher = PatternMatcher::new(&[r"(?i)select", r"(?i)union"]).unwrap();

        assert!(matcher.is_match("SELECT * FROM users"));
        assert!(matcher.is_match("1 UNION SELECT 1"));
        assert!(!matcher.is_match("hello world"));
    }

    #[test]
    fn test_request_data() {
        let request = RequestData {
            method: "GET".to_string(),
            path: "/api/users".to_string(),
            query_string: Some("id=1".to_string()),
            headers: vec![("Content-Type".to_string(), "application/json".to_string())],
            body: None,
            client_ip: Some("192.168.1.1".parse().unwrap()),
        };

        assert_eq!(request.uri(), "/api/users?id=1");
        assert_eq!(request.header("content-type"), Some("application/json"));
        assert_eq!(request.header("X-Unknown"), None);
    }

    #[test]
    fn test_match_target() {
        assert_eq!(MatchTarget::from_str("query"), MatchTarget::Query);
        assert_eq!(MatchTarget::from_str("PATH"), MatchTarget::Path);
        assert_eq!(MatchTarget::from_str("unknown"), MatchTarget::All);
    }

    #[test]
    fn test_compiled_rule_evaluation() {
        let config = RuleConfig {
            id: "test-rule".to_string(),
            description: None,
            enabled: true,
            pattern: Some(r"(?i)admin".to_string()),
            target: "path".to_string(),
            header_name: None,
            path: None,
            methods: Some(vec!["GET".to_string()]),
            action: "block".to_string(),
            log: true,
            rate: None,
            per: None,
            block_status: 403,
            block_message: Some("Access denied".to_string()),
            priority: 100,
        };

        let rule = CompiledRule::try_from(&config).unwrap();

        // Should match
        let request1 = RequestData {
            method: "GET".to_string(),
            path: "/admin/dashboard".to_string(),
            query_string: None,
            headers: vec![],
            body: None,
            client_ip: None,
        };
        assert!(rule.evaluate(&request1).is_some());

        // Should not match (wrong method)
        let request2 = RequestData {
            method: "POST".to_string(),
            path: "/admin/dashboard".to_string(),
            query_string: None,
            headers: vec![],
            body: None,
            client_ip: None,
        };
        assert!(rule.evaluate(&request2).is_none());

        // Should not match (path doesn't contain admin)
        let request3 = RequestData {
            method: "GET".to_string(),
            path: "/api/users".to_string(),
            query_string: None,
            headers: vec![],
            body: None,
            client_ip: None,
        };
        assert!(rule.evaluate(&request3).is_none());
    }
}
