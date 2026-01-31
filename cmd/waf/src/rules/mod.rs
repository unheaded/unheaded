//! WAF Rule Engine for THE SHIELD
//!
//! This module provides the core rule engine that evaluates incoming requests
//! against configured security rules. Designed for sub-millisecond evaluation
//! to handle 100k+ requests per second.

pub mod actions;
pub mod matcher;
pub mod parser;

use crate::config::RuleConfig;
use matcher::{CompiledRule, MatchTarget, RequestData};
use std::collections::HashMap;
use std::net::IpAddr;
use std::sync::Arc;

pub use actions::{Action, RuleAction, RuleDecision};
pub use matcher::PatternMatcher;

/// The rule engine that evaluates requests against security rules
pub struct RuleEngine {
    /// Compiled rules sorted by priority
    rules: Vec<CompiledRule>,

    /// IP allowlist for fast lookup
    ip_allowlist: std::collections::HashSet<IpAddr>,

    /// IP blocklist for fast lookup
    ip_blocklist: std::collections::HashSet<IpAddr>,

    /// CIDR ranges for allowlist (network, prefix_len)
    cidr_allowlist: Vec<(IpAddr, u8)>,

    /// CIDR ranges for blocklist (network, prefix_len)
    cidr_blocklist: Vec<(IpAddr, u8)>,

    /// Built-in SQL injection patterns
    sqli_matcher: Option<PatternMatcher>,

    /// Built-in XSS patterns
    xss_matcher: Option<PatternMatcher>,

    /// Enable built-in protections
    enable_sqli_protection: bool,
    enable_xss_protection: bool,
}

/// Result of rule evaluation
#[derive(Debug, Clone)]
pub struct EvaluationResult {
    /// The decision (allow, block, rate_limit)
    pub decision: RuleDecision,

    /// Rule that triggered this decision (if any)
    pub matched_rule: Option<String>,

    /// Should this request be logged
    pub should_log: bool,

    /// Additional context for logging
    pub context: HashMap<String, String>,
}

impl RuleEngine {
    /// Create a new rule engine from configuration
    pub fn new(
        rule_configs: &[RuleConfig],
        ip_allowlist: &[String],
        ip_blocklist: &[String],
    ) -> Result<Self, RuleEngineError> {
        // Parse and compile rules
        let mut rules: Vec<CompiledRule> = rule_configs
            .iter()
            .filter(|r| r.enabled)
            .map(CompiledRule::try_from)
            .collect::<Result<Vec<_>, _>>()?;

        // Sort by priority (lower = higher priority)
        rules.sort_by_key(|r| r.priority);

        // Parse IP lists
        let (ip_allow_set, cidr_allow) = Self::parse_ip_list(ip_allowlist)?;
        let (ip_block_set, cidr_block) = Self::parse_ip_list(ip_blocklist)?;

        // Compile built-in protection patterns
        let sqli_matcher = Some(Self::compile_sqli_patterns()?);
        let xss_matcher = Some(Self::compile_xss_patterns()?);

        Ok(Self {
            rules,
            ip_allowlist: ip_allow_set,
            ip_blocklist: ip_block_set,
            cidr_allowlist: cidr_allow,
            cidr_blocklist: cidr_block,
            sqli_matcher,
            xss_matcher,
            enable_sqli_protection: true,
            enable_xss_protection: true,
        })
    }

    /// Parse IP list into individual IPs and CIDR ranges
    fn parse_ip_list(
        ips: &[String],
    ) -> Result<(std::collections::HashSet<IpAddr>, Vec<(IpAddr, u8)>), RuleEngineError> {
        let mut ip_set = std::collections::HashSet::new();
        let mut cidr_list = Vec::new();

        for ip_str in ips {
            if ip_str.contains('/') {
                // CIDR notation
                let parts: Vec<&str> = ip_str.split('/').collect();
                if parts.len() != 2 {
                    return Err(RuleEngineError::InvalidIp(ip_str.clone()));
                }
                let addr: IpAddr = parts[0]
                    .parse()
                    .map_err(|_| RuleEngineError::InvalidIp(ip_str.clone()))?;
                let prefix: u8 = parts[1]
                    .parse()
                    .map_err(|_| RuleEngineError::InvalidIp(ip_str.clone()))?;
                cidr_list.push((addr, prefix));
            } else {
                // Single IP
                let addr: IpAddr = ip_str
                    .parse()
                    .map_err(|_| RuleEngineError::InvalidIp(ip_str.clone()))?;
                ip_set.insert(addr);
            }
        }

        Ok((ip_set, cidr_list))
    }

    /// Compile built-in SQL injection detection patterns
    fn compile_sqli_patterns() -> Result<PatternMatcher, RuleEngineError> {
        // These patterns detect common SQL injection attempts
        // Optimized for low false positives while catching real attacks
        let patterns = vec![
            // Union-based injection
            r"(?i)\bunion\s+(all\s+)?select\b",
            // Classic OR injection
            r"(?i)\bor\s+[\d\w'\"]+\s*=\s*[\d\w'\"]+",
            // Comment injection
            r"(?i)(--|#|/\*)\s*$",
            // Stacked queries
            r";\s*(select|insert|update|delete|drop|truncate|alter)\b",
            // Time-based blind injection
            r"(?i)\b(sleep|benchmark|waitfor|delay)\s*\(",
            // Error-based injection
            r"(?i)\b(extractvalue|updatexml|xmltype)\s*\(",
            // Boolean-based blind injection
            r"(?i)\band\s+[\d\w'\"]+\s*=\s*[\d\w'\"]+",
            // Common SQL keywords in suspicious contexts
            r"(?i)'\s*(or|and)\s+'",
            // Hex encoding bypass
            r"(?i)0x[0-9a-f]+",
            // CHAR() function abuse
            r"(?i)\bchar\s*\(\s*\d+\s*\)",
        ];

        PatternMatcher::new(&patterns).map_err(RuleEngineError::PatternError)
    }

    /// Compile built-in XSS detection patterns
    fn compile_xss_patterns() -> Result<PatternMatcher, RuleEngineError> {
        // XSS detection patterns - catches common attack vectors
        let patterns = vec![
            // Script tags
            r"(?i)<\s*script[^>]*>",
            // Event handlers
            r"(?i)\bon\w+\s*=",
            // JavaScript URLs
            r"(?i)javascript\s*:",
            // Data URLs with script content
            r"(?i)data\s*:[^,]*;base64",
            // SVG with script
            r"(?i)<\s*svg[^>]*\s+on\w+\s*=",
            // Object/embed tags
            r"(?i)<\s*(object|embed|applet)[^>]*>",
            // Expression in style
            r"(?i)expression\s*\(",
            // Import directive
            r"(?i)@import\s",
            // Iframe injection
            r"(?i)<\s*iframe[^>]*>",
            // Form hijacking
            r"(?i)<\s*form[^>]*action\s*=",
            // Meta refresh
            r"(?i)<\s*meta[^>]*http-equiv\s*=\s*['\"]?refresh",
            // Base tag hijacking
            r"(?i)<\s*base[^>]*href\s*=",
        ];

        PatternMatcher::new(&patterns).map_err(RuleEngineError::PatternError)
    }

    /// Enable or disable built-in SQL injection protection
    pub fn set_sqli_protection(&mut self, enabled: bool) {
        self.enable_sqli_protection = enabled;
    }

    /// Enable or disable built-in XSS protection
    pub fn set_xss_protection(&mut self, enabled: bool) {
        self.enable_xss_protection = enabled;
    }

    /// Evaluate a request against all rules
    ///
    /// Returns immediately on first blocking rule match for performance.
    /// Allow rules are processed in priority order.
    pub fn evaluate(&self, request: &RequestData) -> EvaluationResult {
        let mut context = HashMap::new();

        // Step 1: Check IP allowlist (fast path for trusted IPs)
        if let Some(ref ip) = request.client_ip {
            if self.is_ip_allowed(ip) {
                return EvaluationResult {
                    decision: RuleDecision::Allow,
                    matched_rule: Some("ip_allowlist".to_string()),
                    should_log: false,
                    context,
                };
            }

            // Step 2: Check IP blocklist
            if self.is_ip_blocked(ip) {
                context.insert("blocked_ip".to_string(), ip.to_string());
                return EvaluationResult {
                    decision: RuleDecision::Block {
                        status: 403,
                        message: "Access denied".to_string(),
                    },
                    matched_rule: Some("ip_blocklist".to_string()),
                    should_log: true,
                    context,
                };
            }
        }

        // Step 3: Built-in SQL injection protection
        if self.enable_sqli_protection {
            if let Some(ref matcher) = self.sqli_matcher {
                if self.check_builtin_pattern(matcher, request) {
                    context.insert("attack_type".to_string(), "sqli".to_string());
                    return EvaluationResult {
                        decision: RuleDecision::Block {
                            status: 403,
                            message: "Potential SQL injection detected".to_string(),
                        },
                        matched_rule: Some("builtin_sqli".to_string()),
                        should_log: true,
                        context,
                    };
                }
            }
        }

        // Step 4: Built-in XSS protection
        if self.enable_xss_protection {
            if let Some(ref matcher) = self.xss_matcher {
                if self.check_builtin_pattern(matcher, request) {
                    context.insert("attack_type".to_string(), "xss".to_string());
                    return EvaluationResult {
                        decision: RuleDecision::Block {
                            status: 403,
                            message: "Potential XSS attack detected".to_string(),
                        },
                        matched_rule: Some("builtin_xss".to_string()),
                        should_log: true,
                        context,
                    };
                }
            }
        }

        // Step 5: Evaluate custom rules in priority order
        for rule in &self.rules {
            if let Some(result) = rule.evaluate(request) {
                context.insert("rule_id".to_string(), rule.id.clone());
                return EvaluationResult {
                    decision: result.decision,
                    matched_rule: Some(rule.id.clone()),
                    should_log: rule.log,
                    context,
                };
            }
        }

        // Default: Allow
        EvaluationResult {
            decision: RuleDecision::Allow,
            matched_rule: None,
            should_log: false,
            context,
        }
    }

    /// Check if an IP is in the allowlist
    fn is_ip_allowed(&self, ip: &IpAddr) -> bool {
        if self.ip_allowlist.contains(ip) {
            return true;
        }

        // Check CIDR ranges
        for (network, prefix_len) in &self.cidr_allowlist {
            if Self::ip_in_cidr(ip, network, *prefix_len) {
                return true;
            }
        }

        false
    }

    /// Check if an IP is in the blocklist
    fn is_ip_blocked(&self, ip: &IpAddr) -> bool {
        if self.ip_blocklist.contains(ip) {
            return true;
        }

        // Check CIDR ranges
        for (network, prefix_len) in &self.cidr_blocklist {
            if Self::ip_in_cidr(ip, network, *prefix_len) {
                return true;
            }
        }

        false
    }

    /// Check if an IP is within a CIDR range
    fn ip_in_cidr(ip: &IpAddr, network: &IpAddr, prefix_len: u8) -> bool {
        match (ip, network) {
            (IpAddr::V4(ip), IpAddr::V4(net)) => {
                if prefix_len > 32 {
                    return false;
                }
                let ip_bits = u32::from(*ip);
                let net_bits = u32::from(*net);
                let mask = if prefix_len == 0 {
                    0
                } else {
                    !0u32 << (32 - prefix_len)
                };
                (ip_bits & mask) == (net_bits & mask)
            }
            (IpAddr::V6(ip), IpAddr::V6(net)) => {
                if prefix_len > 128 {
                    return false;
                }
                let ip_bits = u128::from(*ip);
                let net_bits = u128::from(*net);
                let mask = if prefix_len == 0 {
                    0
                } else {
                    !0u128 << (128 - prefix_len)
                };
                (ip_bits & mask) == (net_bits & mask)
            }
            _ => false, // IPv4 and IPv6 don't match
        }
    }

    /// Check built-in patterns against request data
    fn check_builtin_pattern(&self, matcher: &PatternMatcher, request: &RequestData) -> bool {
        // Check query string
        if let Some(ref query) = request.query_string {
            if matcher.is_match(query) {
                return true;
            }
        }

        // Check path
        if matcher.is_match(&request.path) {
            return true;
        }

        // Check body (if present and not too large)
        if let Some(ref body) = request.body {
            // Only check first 64KB of body for performance
            let check_len = std::cmp::min(body.len(), 65536);
            if let Ok(body_str) = std::str::from_utf8(&body[..check_len]) {
                if matcher.is_match(body_str) {
                    return true;
                }
            }
        }

        // Check relevant headers
        for (name, value) in &request.headers {
            let name_lower = name.to_lowercase();
            // Only check headers that might contain user input
            if matches!(
                name_lower.as_str(),
                "referer" | "user-agent" | "cookie" | "x-forwarded-for" | "content-type"
            ) {
                if matcher.is_match(value) {
                    return true;
                }
            }
        }

        false
    }

    /// Get the number of active rules
    pub fn rule_count(&self) -> usize {
        self.rules.len()
    }

    /// Get rule IDs for debugging
    pub fn rule_ids(&self) -> Vec<&str> {
        self.rules.iter().map(|r| r.id.as_str()).collect()
    }
}

/// Errors that can occur in the rule engine
#[derive(Debug)]
pub enum RuleEngineError {
    /// Invalid IP address or CIDR notation
    InvalidIp(String),
    /// Pattern compilation error
    PatternError(String),
    /// Rule configuration error
    RuleError(String),
}

impl std::fmt::Display for RuleEngineError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            RuleEngineError::InvalidIp(ip) => write!(f, "Invalid IP address: {}", ip),
            RuleEngineError::PatternError(msg) => write!(f, "Pattern error: {}", msg),
            RuleEngineError::RuleError(msg) => write!(f, "Rule error: {}", msg),
        }
    }
}

impl std::error::Error for RuleEngineError {}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_request(path: &str, query: Option<&str>) -> RequestData {
        RequestData {
            method: "GET".to_string(),
            path: path.to_string(),
            query_string: query.map(|s| s.to_string()),
            headers: vec![],
            body: None,
            client_ip: Some("192.168.1.100".parse().unwrap()),
        }
    }

    #[test]
    fn test_ip_blocklist() {
        let engine = RuleEngine::new(
            &[],
            &[],
            &["192.168.1.100".to_string()],
        )
        .unwrap();

        let request = create_test_request("/api/test", None);
        let result = engine.evaluate(&request);

        assert!(matches!(result.decision, RuleDecision::Block { .. }));
        assert_eq!(result.matched_rule, Some("ip_blocklist".to_string()));
    }

    #[test]
    fn test_ip_allowlist() {
        let engine = RuleEngine::new(
            &[],
            &["192.168.1.100".to_string()],
            &[],
        )
        .unwrap();

        let request = create_test_request("/api/test", None);
        let result = engine.evaluate(&request);

        assert!(matches!(result.decision, RuleDecision::Allow));
        assert_eq!(result.matched_rule, Some("ip_allowlist".to_string()));
    }

    #[test]
    fn test_cidr_matching() {
        let engine = RuleEngine::new(
            &[],
            &[],
            &["192.168.1.0/24".to_string()],
        )
        .unwrap();

        let request = create_test_request("/api/test", None);
        let result = engine.evaluate(&request);

        assert!(matches!(result.decision, RuleDecision::Block { .. }));
    }

    #[test]
    fn test_sqli_detection() {
        let engine = RuleEngine::new(&[], &[], &[]).unwrap();

        // Test SQL injection in query string
        let request = create_test_request("/search", Some("q=1' OR '1'='1"));
        let result = engine.evaluate(&request);

        assert!(matches!(result.decision, RuleDecision::Block { .. }));
        assert_eq!(result.matched_rule, Some("builtin_sqli".to_string()));
    }

    #[test]
    fn test_xss_detection() {
        let engine = RuleEngine::new(&[], &[], &[]).unwrap();

        // Test XSS in query string
        let request = create_test_request("/search", Some("q=<script>alert(1)</script>"));
        let result = engine.evaluate(&request);

        assert!(matches!(result.decision, RuleDecision::Block { .. }));
        assert_eq!(result.matched_rule, Some("builtin_xss".to_string()));
    }

    #[test]
    fn test_clean_request() {
        let engine = RuleEngine::new(&[], &[], &[]).unwrap();

        let request = create_test_request("/api/users", Some("page=1&limit=10"));
        let result = engine.evaluate(&request);

        assert!(matches!(result.decision, RuleDecision::Allow));
        assert!(result.matched_rule.is_none());
    }
}
