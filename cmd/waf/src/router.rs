//! Request Router for THE SHIELD
//!
//! Routes incoming requests through the WAF pipeline:
//! 1. Parse request
//! 2. Check IP allowlist/blocklist
//! 3. Apply rate limiting
//! 4. Evaluate WAF rules
//! 5. Forward to backend (if allowed)

use crate::metrics::{MetricsRegistry, Timer};
use crate::proxy::ReverseProxy;
use crate::ratelimit::{RateLimitManager, RateLimitResult};
use crate::rules::actions::BlockResponse;
use crate::rules::matcher::RequestData;
use crate::rules::{EvaluationResult, RuleDecision, RuleEngine};
use http_body_util::{BodyExt, Full};
use hyper::body::{Body, Bytes, Incoming};
use hyper::{Method, Request, Response, StatusCode};
use std::net::IpAddr;
use std::sync::Arc;
use std::time::{Duration, Instant};

/// The main request router
pub struct Router {
    /// WAF rule engine
    rule_engine: Arc<RuleEngine>,
    /// Rate limiter
    rate_limiter: Arc<RateLimitManager>,
    /// Reverse proxy
    proxy: Arc<ReverseProxy>,
    /// Metrics registry
    metrics: Arc<MetricsRegistry>,
    /// Maximum body size for inspection
    max_body_size: usize,
    /// Whether to log all requests
    log_all_requests: bool,
}

impl Router {
    /// Create a new router
    pub fn new(
        rule_engine: Arc<RuleEngine>,
        rate_limiter: Arc<RateLimitManager>,
        proxy: Arc<ReverseProxy>,
        metrics: Arc<MetricsRegistry>,
        max_body_size: usize,
    ) -> Self {
        Self {
            rule_engine,
            rate_limiter,
            proxy,
            metrics,
            max_body_size,
            log_all_requests: false,
        }
    }

    /// Set whether to log all requests
    pub fn set_log_all(&mut self, log_all: bool) {
        self.log_all_requests = log_all;
    }

    /// Route a request through the WAF pipeline
    pub async fn route(
        &self,
        request: Request<Incoming>,
        client_ip: Option<IpAddr>,
    ) -> Response<Full<Bytes>> {
        let start = Instant::now();
        let _timer = Timer::start();

        // Track request in flight
        self.metrics.requests_in_flight.inc();

        // Extract request info for logging/metrics
        let method = request.method().clone();
        let path = request.uri().path().to_string();
        let uri = request.uri().to_string();

        // Process the request
        let response = self.process_request(request, client_ip).await;

        // Record metrics
        let status = response.status().as_u16().to_string();
        let duration = start.elapsed();

        self.metrics.requests_in_flight.dec();

        // Record request duration
        let hist = self
            .metrics
            .request_duration
            .with_labels(&[("method", method.as_str()), ("path", &normalize_path(&path))])
            .await;
        hist.observe_duration(duration);

        // Record request count
        let counter = self
            .metrics
            .requests_total
            .with_labels(&[
                ("method", method.as_str()),
                ("path", &normalize_path(&path)),
                ("status", &status),
            ])
            .await;
        counter.inc();

        // Log request if enabled
        if self.log_all_requests {
            log_request(&method, &uri, client_ip, response.status(), duration);
        }

        response
    }

    /// Process a request through the WAF pipeline
    async fn process_request(
        &self,
        request: Request<Incoming>,
        client_ip: Option<IpAddr>,
    ) -> Response<Full<Bytes>> {
        let method = request.method().clone();
        let path = request.uri().path().to_string();
        let query = request.uri().query().map(|q| q.to_string());

        // Step 1: Rate limiting (per IP)
        if let Some(ip) = client_ip {
            let rate_result = self.rate_limiter.check_ip(ip).await;
            if let RateLimitResult::Limited { retry_after_ms } = rate_result {
                self.metrics
                    .rate_limited_requests
                    .with_labels(&[("path", &normalize_path(&path))])
                    .await
                    .inc();

                return rate_limit_response(retry_after_ms);
            }
        }

        // Step 2: Collect request data for rule evaluation
        let (parts, body) = request.into_parts();

        // Collect headers
        let headers: Vec<(String, String)> = parts
            .headers
            .iter()
            .filter_map(|(name, value)| {
                value
                    .to_str()
                    .ok()
                    .map(|v| (name.to_string(), v.to_string()))
            })
            .collect();

        // Read body for inspection (limited size)
        let body_bytes = match body.collect().await {
            Ok(collected) => {
                let bytes = collected.to_bytes();
                self.metrics.bytes_received.add(bytes.len() as u64);
                if bytes.len() <= self.max_body_size {
                    Some(bytes.to_vec())
                } else {
                    // Body too large for inspection, but we'll still forward it
                    Some(bytes[..self.max_body_size].to_vec())
                }
            }
            Err(_) => None,
        };

        // Build request data for rule evaluation
        let request_data = RequestData {
            method: method.to_string(),
            path: path.clone(),
            query_string: query,
            headers,
            body: body_bytes.clone(),
            client_ip,
        };

        // Step 3: Evaluate WAF rules
        let rule_timer = Timer::start();
        let eval_result = self.rule_engine.evaluate(&request_data);
        rule_timer.observe_in(&self.metrics.rule_evaluation_duration);

        // Handle rule decision
        match &eval_result.decision {
            RuleDecision::Block { status, message } => {
                // Record blocked request
                if let Some(rule_id) = &eval_result.matched_rule {
                    self.metrics
                        .blocked_requests
                        .with_labels(&[("rule", rule_id), ("action", "block")])
                        .await
                        .inc();
                }

                if eval_result.should_log {
                    log_blocked_request(&request_data, &eval_result);
                }

                return block_response(*status, message, eval_result.matched_rule.as_deref());
            }

            RuleDecision::RateLimit { requests, period } => {
                // Apply rule-specific rate limit
                if let Some(ip) = client_ip {
                    let _key = format!("{}:{}", ip, normalize_path(&path));
                    let result = self
                        .rate_limiter
                        .ip_path_limiter
                        .check_with_limit(
                            &crate::ratelimit::IpPathKey {
                                ip,
                                path: normalize_path(&path),
                            },
                            (*requests as f64 / period.as_secs_f64()) as u32,
                            *requests,
                        )
                        .await;

                    if let RateLimitResult::Limited { retry_after_ms } = result {
                        self.metrics
                            .rate_limited_requests
                            .with_labels(&[("path", &normalize_path(&path))])
                            .await
                            .inc();

                        return rate_limit_response(retry_after_ms);
                    }
                }
            }

            RuleDecision::Log => {
                if eval_result.should_log {
                    log_request_match(&request_data, &eval_result);
                }
            }

            RuleDecision::Allow => {
                // Continue to proxy
            }
        }

        // Step 4: Forward to backend.
        // The body has already been buffered into body_bytes (line 159). Hyper 1.x
        // doesn't allow user code to construct Request<Incoming>, so we hand the
        // proxy a Request<Full<Bytes>> instead — proxy::forward signature was
        // changed to match.
        let body_for_forward = Full::new(Bytes::from(body_bytes.clone().unwrap_or_default()));

        let mut forward_builder = Request::builder()
            .method(method)
            .uri(parts.uri);

        for (name, value) in parts.headers.iter() {
            forward_builder = forward_builder.header(name, value);
        }

        let forward_request = match forward_builder.body(body_for_forward) {
            Ok(mut req) => {
                if let Some(ip) = client_ip {
                    req.extensions_mut().insert(std::net::SocketAddr::new(ip, 0));
                }
                req
            }
            Err(_) => return error_response(500, "Failed to build request"),
        };

        match self.proxy.forward(forward_request).await {
            Ok(response) => {
                self.metrics
                    .bytes_sent
                    .add(response.body().size_hint().lower());
                response
            }
            Err(e) => ReverseProxy::error_response(&e),
        }
    }

    /// Handle internal endpoints (health, metrics, etc.)
    pub async fn handle_internal(&self, path: &str) -> Option<Response<Full<Bytes>>> {
        match path {
            "/health" | "/healthz" => Some(health_response()),
            "/ready" | "/readyz" => Some(ready_response()),
            _ => None,
        }
    }
}

/// Normalize a path for metrics (replace IDs with placeholders)
fn normalize_path(path: &str) -> String {
    // Simple normalization: replace numeric segments with {id}
    let parts: Vec<&str> = path.split('/').collect();
    let normalized: Vec<String> = parts
        .iter()
        .map(|part| {
            if part.chars().all(|c| c.is_ascii_digit()) && !part.is_empty() {
                "{id}".to_string()
            } else if part.len() >= 32 && part.chars().all(|c| c.is_ascii_hexdigit()) {
                // Off-by-one fix 2026-05-09: standard UUID hex (no dashes) is
                // exactly 32 chars; the previous `> 32` check rejected them.
                "{uuid}".to_string()
            } else {
                part.to_string()
            }
        })
        .collect();
    normalized.join("/")
}

/// Create a block response
fn block_response(status: u16, message: &str, rule_id: Option<&str>) -> Response<Full<Bytes>> {
    let block = BlockResponse::json(status, message, rule_id);

    Response::builder()
        .status(StatusCode::from_u16(status).unwrap_or(StatusCode::FORBIDDEN))
        .header("Content-Type", block.content_type)
        .header("X-Shield-Block", "true")
        .body(Full::new(Bytes::from(block.body)))
        .unwrap()
}

/// Create a rate limit response
fn rate_limit_response(retry_after_ms: u64) -> Response<Full<Bytes>> {
    let retry_after = retry_after_ms.div_ceil(1000); // Round up to seconds

    let body = format!(
        r#"{{"error":"rate_limited","message":"Too many requests","retry_after":{}}}"#,
        retry_after
    );

    Response::builder()
        .status(StatusCode::TOO_MANY_REQUESTS)
        .header("Content-Type", "application/json")
        .header("Retry-After", retry_after.to_string())
        .header("X-Shield-Rate-Limited", "true")
        .body(Full::new(Bytes::from(body)))
        .unwrap()
}

/// Create an error response
fn error_response(status: u16, message: &str) -> Response<Full<Bytes>> {
    let body = format!(r#"{{"error":"{}","message":"{}"}}"#, status, message);

    Response::builder()
        .status(StatusCode::from_u16(status).unwrap_or(StatusCode::INTERNAL_SERVER_ERROR))
        .header("Content-Type", "application/json")
        .body(Full::new(Bytes::from(body)))
        .unwrap()
}

/// Create a health check response
fn health_response() -> Response<Full<Bytes>> {
    Response::builder()
        .status(StatusCode::OK)
        .header("Content-Type", "application/json")
        .body(Full::new(Bytes::from(r#"{"status":"healthy"}"#)))
        .unwrap()
}

/// Create a readiness check response
fn ready_response() -> Response<Full<Bytes>> {
    Response::builder()
        .status(StatusCode::OK)
        .header("Content-Type", "application/json")
        .body(Full::new(Bytes::from(r#"{"status":"ready"}"#)))
        .unwrap()
}

/// Log a request
fn log_request(
    method: &Method,
    uri: &str,
    client_ip: Option<IpAddr>,
    status: StatusCode,
    duration: Duration,
) {
    let ip_str = client_ip
        .map(|ip| ip.to_string())
        .unwrap_or_else(|| "-".to_string());

    eprintln!(
        "{} {} {} {} {:.3}ms",
        ip_str,
        method,
        uri,
        status.as_u16(),
        duration.as_secs_f64() * 1000.0
    );
}

/// Log a blocked request
fn log_blocked_request(request: &RequestData, result: &EvaluationResult) {
    let ip_str = request
        .client_ip
        .map(|ip| ip.to_string())
        .unwrap_or_else(|| "-".to_string());

    let rule_id = result
        .matched_rule
        .as_deref()
        .unwrap_or("unknown");

    eprintln!(
        "[BLOCKED] {} {} {} rule={} context={:?}",
        ip_str, request.method, request.path, rule_id, result.context
    );
}

/// Log a rule match (for log-only rules)
fn log_request_match(request: &RequestData, result: &EvaluationResult) {
    let ip_str = request
        .client_ip
        .map(|ip| ip.to_string())
        .unwrap_or_else(|| "-".to_string());

    let rule_id = result
        .matched_rule
        .as_deref()
        .unwrap_or("unknown");

    eprintln!(
        "[MATCH] {} {} {} rule={} context={:?}",
        ip_str, request.method, request.path, rule_id, result.context
    );
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_normalize_path() {
        assert_eq!(normalize_path("/api/users/123"), "/api/users/{id}");
        assert_eq!(normalize_path("/api/v1/items"), "/api/v1/items");
        assert_eq!(
            normalize_path("/api/users/550e8400e29b41d4a716446655440000"),
            "/api/users/{uuid}"
        );
    }

    #[test]
    fn test_block_response() {
        let response = block_response(403, "Forbidden", Some("rule-1"));
        assert_eq!(response.status(), StatusCode::FORBIDDEN);
        assert_eq!(
            response.headers().get("Content-Type").unwrap(),
            "application/json"
        );
    }

    #[test]
    fn test_rate_limit_response() {
        let response = rate_limit_response(1500);
        assert_eq!(response.status(), StatusCode::TOO_MANY_REQUESTS);
        assert_eq!(response.headers().get("Retry-After").unwrap(), "2");
    }

    #[test]
    fn test_health_response() {
        let response = health_response();
        assert_eq!(response.status(), StatusCode::OK);
    }
}
