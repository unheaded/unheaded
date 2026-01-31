//! Rule Parser for THE SHIELD
//!
//! Handles parsing of WAF rules from configuration and runtime rule loading.
//! Supports glob-style path patterns and regex patterns for content matching.

use crate::config::{parse_duration, RuleConfig};
use std::time::Duration;

/// Parsed path pattern supporting glob-style matching
#[derive(Debug, Clone)]
pub enum PathPattern {
    /// Exact match
    Exact(String),
    /// Prefix match (e.g., "/api/*")
    Prefix(String),
    /// Suffix match (e.g., "*.json")
    Suffix(String),
    /// Contains match
    Contains(String),
    /// Regex pattern
    Regex(regex::Regex),
    /// Match all
    Any,
}

impl PathPattern {
    /// Parse a path pattern string
    pub fn parse(pattern: &str) -> Result<Self, String> {
        if pattern.is_empty() || pattern == "*" || pattern == "**" {
            return Ok(PathPattern::Any);
        }

        // Check for glob patterns
        let has_star = pattern.contains('*');
        let has_question = pattern.contains('?');

        if !has_star && !has_question {
            // Exact match
            return Ok(PathPattern::Exact(pattern.to_string()));
        }

        // Simple glob patterns
        if pattern.ends_with("/*") || pattern.ends_with("/**") {
            let prefix = pattern.trim_end_matches("/**").trim_end_matches("/*");
            return Ok(PathPattern::Prefix(prefix.to_string()));
        }

        if pattern.starts_with("*.") {
            let suffix = pattern.trim_start_matches('*');
            return Ok(PathPattern::Suffix(suffix.to_string()));
        }

        if pattern.starts_with('*') && pattern.ends_with('*') && pattern.len() > 2 {
            let middle = &pattern[1..pattern.len() - 1];
            if !middle.contains('*') && !middle.contains('?') {
                return Ok(PathPattern::Contains(middle.to_string()));
            }
        }

        // Complex glob - convert to regex
        let regex_pattern = glob_to_regex(pattern)?;
        let regex = regex::Regex::new(&regex_pattern)
            .map_err(|e| format!("Invalid path pattern '{}': {}", pattern, e))?;
        Ok(PathPattern::Regex(regex))
    }

    /// Check if a path matches this pattern
    pub fn matches(&self, path: &str) -> bool {
        match self {
            PathPattern::Exact(p) => path == p,
            PathPattern::Prefix(p) => path.starts_with(p),
            PathPattern::Suffix(s) => path.ends_with(s),
            PathPattern::Contains(c) => path.contains(c),
            PathPattern::Regex(r) => r.is_match(path),
            PathPattern::Any => true,
        }
    }
}

/// Convert a glob pattern to a regex pattern
fn glob_to_regex(glob: &str) -> Result<String, String> {
    let mut regex = String::with_capacity(glob.len() * 2);
    regex.push('^');

    let mut chars = glob.chars().peekable();
    while let Some(c) = chars.next() {
        match c {
            '*' => {
                if chars.peek() == Some(&'*') {
                    chars.next(); // consume second *
                    regex.push_str(".*"); // ** matches everything including /
                } else {
                    regex.push_str("[^/]*"); // * matches everything except /
                }
            }
            '?' => regex.push_str("[^/]"), // ? matches single char except /
            '.' | '+' | '^' | '$' | '(' | ')' | '[' | ']' | '{' | '}' | '|' | '\\' => {
                regex.push('\\');
                regex.push(c);
            }
            _ => regex.push(c),
        }
    }

    regex.push('$');
    Ok(regex)
}

/// Parsed rate limit specification
#[derive(Debug, Clone)]
pub struct RateLimit {
    /// Requests allowed
    pub requests: u32,
    /// Time period
    pub period: Duration,
}

impl RateLimit {
    /// Parse rate limit from rule config
    pub fn from_config(rate: u32, per: &str) -> Result<Self, String> {
        let period = parse_duration(per)?;
        Ok(Self {
            requests: rate,
            period,
        })
    }

    /// Get requests per second
    pub fn requests_per_second(&self) -> f64 {
        self.requests as f64 / self.period.as_secs_f64()
    }
}

/// HTTP method filter
#[derive(Debug, Clone)]
pub enum MethodFilter {
    /// Match any method
    Any,
    /// Match specific methods
    Methods(Vec<String>),
}

impl MethodFilter {
    /// Parse from optional method list
    pub fn from_config(methods: Option<&[String]>) -> Self {
        match methods {
            None => MethodFilter::Any,
            Some(m) if m.is_empty() => MethodFilter::Any,
            Some(m) => MethodFilter::Methods(m.iter().map(|s| s.to_uppercase()).collect()),
        }
    }

    /// Check if a method matches
    pub fn matches(&self, method: &str) -> bool {
        match self {
            MethodFilter::Any => true,
            MethodFilter::Methods(m) => m.iter().any(|allowed| allowed == method),
        }
    }
}

/// Parse and validate a regex pattern
pub fn parse_regex(pattern: &str) -> Result<regex::Regex, String> {
    regex::Regex::new(pattern).map_err(|e| format!("Invalid regex '{}': {}", pattern, e))
}

/// Validate a rule configuration
pub fn validate_rule(config: &RuleConfig) -> Result<(), String> {
    // Validate pattern if present
    if let Some(ref pattern) = config.pattern {
        parse_regex(pattern)?;
    }

    // Validate path pattern if present
    if let Some(ref path) = config.path {
        PathPattern::parse(path)?;
    }

    // Validate rate limit if action is limit
    if config.action == "limit" {
        let rate = config
            .rate
            .ok_or("Rate limit rule requires 'rate' field")?;
        let per = config
            .per
            .as_ref()
            .ok_or("Rate limit rule requires 'per' field")?;
        RateLimit::from_config(rate, per)?;
    }

    // Validate action
    match config.action.as_str() {
        "block" | "allow" | "log" | "limit" => {}
        other => return Err(format!("Invalid action: {}", other)),
    }

    // Validate target
    match config.target.as_str() {
        "query" | "path" | "header" | "body" | "all" | "method" | "uri" => {}
        other => return Err(format!("Invalid target: {}", other)),
    }

    // Validate block status code
    if config.block_status < 100 || config.block_status > 599 {
        return Err(format!("Invalid block status: {}", config.block_status));
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_path_pattern_exact() {
        let pattern = PathPattern::parse("/api/users").unwrap();
        assert!(pattern.matches("/api/users"));
        assert!(!pattern.matches("/api/users/"));
        assert!(!pattern.matches("/api/users/123"));
    }

    #[test]
    fn test_path_pattern_prefix() {
        let pattern = PathPattern::parse("/api/*").unwrap();
        assert!(pattern.matches("/api/users"));
        assert!(pattern.matches("/api/"));
        assert!(!pattern.matches("/other"));
    }

    #[test]
    fn test_path_pattern_suffix() {
        let pattern = PathPattern::parse("*.json").unwrap();
        assert!(pattern.matches("/data.json"));
        assert!(pattern.matches("file.json"));
        assert!(!pattern.matches("/data.xml"));
    }

    #[test]
    fn test_path_pattern_any() {
        let pattern = PathPattern::parse("*").unwrap();
        assert!(pattern.matches("/anything"));
        assert!(pattern.matches(""));

        let pattern2 = PathPattern::parse("**").unwrap();
        assert!(pattern2.matches("/any/path/here"));
    }

    #[test]
    fn test_method_filter() {
        let filter = MethodFilter::from_config(Some(&["GET".to_string(), "POST".to_string()]));
        assert!(filter.matches("GET"));
        assert!(filter.matches("POST"));
        assert!(!filter.matches("DELETE"));

        let any = MethodFilter::from_config(None);
        assert!(any.matches("GET"));
        assert!(any.matches("DELETE"));
    }

    #[test]
    fn test_rate_limit_parsing() {
        let rl = RateLimit::from_config(100, "1m").unwrap();
        assert_eq!(rl.requests, 100);
        assert_eq!(rl.period, Duration::from_secs(60));

        let rps = rl.requests_per_second();
        assert!((rps - 1.666).abs() < 0.01);
    }

    #[test]
    fn test_glob_to_regex() {
        let regex = glob_to_regex("/api/*/users").unwrap();
        let re = regex::Regex::new(&regex).unwrap();
        assert!(re.is_match("/api/v1/users"));
        assert!(!re.is_match("/api/v1/v2/users")); // * doesn't match /
    }
}
