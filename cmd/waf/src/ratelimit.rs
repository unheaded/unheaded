//! Rate Limiting for THE SHIELD
//!
//! Implements token bucket rate limiting for protecting backends from abuse.
//! Thread-safe and designed for high-concurrency scenarios (100k+ req/sec).

use std::collections::HashMap;
use std::hash::Hash;
use std::net::IpAddr;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::RwLock;

/// Token bucket rate limiter
///
/// Uses the token bucket algorithm:
/// - Bucket has a maximum capacity (burst)
/// - Tokens are added at a constant rate
/// - Each request consumes one token
/// - If no tokens available, request is rejected
#[derive(Debug)]
pub struct TokenBucket {
    /// Current number of tokens (scaled by 1000 for precision)
    tokens: AtomicU64,
    /// Maximum tokens (burst capacity) scaled by 1000
    max_tokens: u64,
    /// Tokens added per millisecond (scaled by 1000)
    refill_rate: u64,
    /// Last refill time
    last_refill: AtomicU64,
}

impl TokenBucket {
    /// Create a new token bucket
    ///
    /// # Arguments
    /// * `rate` - Requests per second
    /// * `burst` - Maximum burst capacity
    pub fn new(rate: u32, burst: u32) -> Self {
        let max_tokens = (burst as u64) * 1000;
        let refill_rate = (rate as u64 * 1000) / 1000; // tokens per millisecond * 1000

        Self {
            tokens: AtomicU64::new(max_tokens),
            max_tokens,
            refill_rate,
            last_refill: AtomicU64::new(current_time_millis()),
        }
    }

    /// Try to acquire a token
    ///
    /// Returns true if a token was acquired, false if rate limited
    pub fn try_acquire(&self) -> bool {
        self.try_acquire_n(1)
    }

    /// Try to acquire multiple tokens
    pub fn try_acquire_n(&self, n: u32) -> bool {
        let cost = (n as u64) * 1000;
        let now = current_time_millis();

        // Refill tokens
        let last = self.last_refill.load(Ordering::Relaxed);
        let elapsed = now.saturating_sub(last);

        if elapsed > 0 {
            // Try to update last_refill time
            if self
                .last_refill
                .compare_exchange(last, now, Ordering::Relaxed, Ordering::Relaxed)
                .is_ok()
            {
                // Add tokens for elapsed time
                let new_tokens = elapsed * self.refill_rate;
                let mut current = self.tokens.load(Ordering::Relaxed);

                loop {
                    let updated = std::cmp::min(current.saturating_add(new_tokens), self.max_tokens);
                    match self.tokens.compare_exchange_weak(
                        current,
                        updated,
                        Ordering::Relaxed,
                        Ordering::Relaxed,
                    ) {
                        Ok(_) => break,
                        Err(c) => current = c,
                    }
                }
            }
        }

        // Try to consume tokens
        let mut current = self.tokens.load(Ordering::Relaxed);
        loop {
            if current < cost {
                return false;
            }

            match self.tokens.compare_exchange_weak(
                current,
                current - cost,
                Ordering::Relaxed,
                Ordering::Relaxed,
            ) {
                Ok(_) => return true,
                Err(c) => current = c,
            }
        }
    }

    /// Get current token count
    pub fn available(&self) -> u32 {
        (self.tokens.load(Ordering::Relaxed) / 1000) as u32
    }

    /// Get remaining time until a token is available (in milliseconds)
    pub fn retry_after_ms(&self) -> u64 {
        let tokens = self.tokens.load(Ordering::Relaxed);
        if tokens >= 1000 {
            return 0;
        }

        let needed = 1000 - tokens;
        if self.refill_rate == 0 {
            return u64::MAX;
        }

        needed / self.refill_rate + 1
    }
}

/// Get current time in milliseconds since an arbitrary epoch
fn current_time_millis() -> u64 {
    use std::time::SystemTime;
    SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as u64
}

/// Rate limiter entry with expiration tracking
struct RateLimitEntry {
    bucket: TokenBucket,
    last_access: Instant,
}

/// Rate limiter that manages multiple buckets (e.g., per IP or per path)
pub struct RateLimiter<K: Eq + Hash + Clone> {
    /// Buckets per key
    buckets: RwLock<HashMap<K, RateLimitEntry>>,
    /// Default rate (requests per second)
    default_rate: u32,
    /// Default burst size
    default_burst: u32,
    /// Entry expiration time
    expiration: Duration,
}

impl<K: Eq + Hash + Clone + Send + Sync> RateLimiter<K> {
    /// Create a new rate limiter
    pub fn new(default_rate: u32, default_burst: u32, expiration: Duration) -> Self {
        Self {
            buckets: RwLock::new(HashMap::new()),
            default_rate,
            default_burst,
            expiration,
        }
    }

    /// Check if a request should be allowed
    pub async fn check(&self, key: &K) -> RateLimitResult {
        // Fast path: check existing bucket
        {
            let buckets = self.buckets.read().await;
            if let Some(entry) = buckets.get(key) {
                if entry.bucket.try_acquire() {
                    return RateLimitResult::Allowed {
                        remaining: entry.bucket.available(),
                    };
                } else {
                    return RateLimitResult::Limited {
                        retry_after_ms: entry.bucket.retry_after_ms(),
                    };
                }
            }
        }

        // Slow path: create new bucket
        let mut buckets = self.buckets.write().await;

        // Double-check after acquiring write lock
        if let Some(entry) = buckets.get(key) {
            if entry.bucket.try_acquire() {
                return RateLimitResult::Allowed {
                    remaining: entry.bucket.available(),
                };
            } else {
                return RateLimitResult::Limited {
                    retry_after_ms: entry.bucket.retry_after_ms(),
                };
            }
        }

        // Create new bucket
        let bucket = TokenBucket::new(self.default_rate, self.default_burst);
        let allowed = bucket.try_acquire();
        let remaining = bucket.available();
        let retry_after = bucket.retry_after_ms();

        buckets.insert(
            key.clone(),
            RateLimitEntry {
                bucket,
                last_access: Instant::now(),
            },
        );

        if allowed {
            RateLimitResult::Allowed { remaining }
        } else {
            RateLimitResult::Limited {
                retry_after_ms: retry_after,
            }
        }
    }

    /// Check with a custom rate limit
    pub async fn check_with_limit(&self, key: &K, rate: u32, burst: u32) -> RateLimitResult {
        let mut buckets = self.buckets.write().await;

        let entry = buckets.entry(key.clone()).or_insert_with(|| RateLimitEntry {
            bucket: TokenBucket::new(rate, burst),
            last_access: Instant::now(),
        });

        entry.last_access = Instant::now();

        if entry.bucket.try_acquire() {
            RateLimitResult::Allowed {
                remaining: entry.bucket.available(),
            }
        } else {
            RateLimitResult::Limited {
                retry_after_ms: entry.bucket.retry_after_ms(),
            }
        }
    }

    /// Clean up expired entries
    pub async fn cleanup(&self) {
        let mut buckets = self.buckets.write().await;
        let now = Instant::now();

        buckets.retain(|_, entry| now.duration_since(entry.last_access) < self.expiration);
    }

    /// Get the number of active rate limit entries
    pub async fn entry_count(&self) -> usize {
        self.buckets.read().await.len()
    }
}

/// Result of a rate limit check
#[derive(Debug, Clone, PartialEq)]
pub enum RateLimitResult {
    /// Request is allowed
    Allowed {
        /// Remaining requests in current window
        remaining: u32,
    },
    /// Request is rate limited
    Limited {
        /// Milliseconds until retry is allowed
        retry_after_ms: u64,
    },
}

impl RateLimitResult {
    /// Check if the request is allowed
    pub fn is_allowed(&self) -> bool {
        matches!(self, RateLimitResult::Allowed { .. })
    }

    /// Get retry-after value in seconds (for HTTP header)
    pub fn retry_after_secs(&self) -> Option<u64> {
        match self {
            RateLimitResult::Limited { retry_after_ms } => Some((*retry_after_ms).div_ceil(1000)),
            RateLimitResult::Allowed { .. } => None,
        }
    }
}

/// IP-based rate limiter
pub type IpRateLimiter = RateLimiter<IpAddr>;

/// Path-based rate limiter
pub type PathRateLimiter = RateLimiter<String>;

/// Combined key for IP + path rate limiting
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct IpPathKey {
    pub ip: IpAddr,
    pub path: String,
}

/// IP + Path rate limiter
pub type IpPathRateLimiter = RateLimiter<IpPathKey>;

/// Global rate limiter manager
pub struct RateLimitManager {
    /// Per-IP rate limiter
    pub ip_limiter: Arc<IpRateLimiter>,
    /// Per-path rate limiter
    pub path_limiter: Arc<PathRateLimiter>,
    /// Combined IP+path rate limiter
    pub ip_path_limiter: Arc<IpPathRateLimiter>,
}

impl RateLimitManager {
    /// Create a new rate limit manager
    pub fn new(default_rate: u32, default_burst: u32, expiration: Duration) -> Self {
        Self {
            ip_limiter: Arc::new(RateLimiter::new(default_rate, default_burst, expiration)),
            path_limiter: Arc::new(RateLimiter::new(
                default_rate * 10,
                default_burst * 10,
                expiration,
            )),
            ip_path_limiter: Arc::new(RateLimiter::new(default_rate, default_burst, expiration)),
        }
    }

    /// Check rate limit for an IP
    pub async fn check_ip(&self, ip: IpAddr) -> RateLimitResult {
        self.ip_limiter.check(&ip).await
    }

    /// Check rate limit for a path
    pub async fn check_path(&self, path: &str) -> RateLimitResult {
        self.path_limiter.check(&path.to_string()).await
    }

    /// Check rate limit for IP + path combination
    pub async fn check_ip_path(&self, ip: IpAddr, path: &str) -> RateLimitResult {
        let key = IpPathKey {
            ip,
            path: path.to_string(),
        };
        self.ip_path_limiter.check(&key).await
    }

    /// Clean up all limiters
    pub async fn cleanup_all(&self) {
        tokio::join!(
            self.ip_limiter.cleanup(),
            self.path_limiter.cleanup(),
            self.ip_path_limiter.cleanup()
        );
    }

    /// Start background cleanup task
    pub fn start_cleanup_task(self: Arc<Self>, interval: Duration) -> tokio::task::JoinHandle<()> {
        tokio::spawn(async move {
            let mut ticker = tokio::time::interval(interval);
            loop {
                ticker.tick().await;
                self.cleanup_all().await;
            }
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_token_bucket_basic() {
        let bucket = TokenBucket::new(10, 10); // 10 req/sec, burst of 10

        // Should allow first 10 requests immediately
        for _ in 0..10 {
            assert!(bucket.try_acquire());
        }

        // 11th should be rejected
        assert!(!bucket.try_acquire());
    }

    #[test]
    fn test_token_bucket_refill() {
        let bucket = TokenBucket::new(1000, 1); // 1000 req/sec, burst of 1

        // Consume the token
        assert!(bucket.try_acquire());
        assert!(!bucket.try_acquire());

        // Wait for refill (at least 1ms for 1 token at 1000/sec)
        std::thread::sleep(Duration::from_millis(2));

        // Should have a token now
        assert!(bucket.try_acquire());
    }

    #[tokio::test]
    async fn test_rate_limiter() {
        let limiter: RateLimiter<String> =
            RateLimiter::new(10, 5, Duration::from_secs(60));

        // First 5 requests should be allowed (burst)
        for i in 0..5 {
            let result = limiter.check(&"test".to_string()).await;
            assert!(result.is_allowed(), "Request {} should be allowed", i);
        }

        // 6th request should be limited
        let result = limiter.check(&"test".to_string()).await;
        assert!(!result.is_allowed());
    }

    #[tokio::test]
    async fn test_rate_limiter_different_keys() {
        let limiter: RateLimiter<String> =
            RateLimiter::new(1, 1, Duration::from_secs(60));

        // Different keys should have separate buckets
        let result1 = limiter.check(&"key1".to_string()).await;
        let result2 = limiter.check(&"key2".to_string()).await;

        assert!(result1.is_allowed());
        assert!(result2.is_allowed());
    }

    #[tokio::test]
    async fn test_rate_limiter_cleanup() {
        let limiter: RateLimiter<String> =
            RateLimiter::new(10, 10, Duration::from_millis(100));

        // Create some entries
        limiter.check(&"key1".to_string()).await;
        limiter.check(&"key2".to_string()).await;

        assert_eq!(limiter.entry_count().await, 2);

        // Wait for expiration
        tokio::time::sleep(Duration::from_millis(150)).await;

        // Cleanup
        limiter.cleanup().await;

        assert_eq!(limiter.entry_count().await, 0);
    }

    #[test]
    fn test_rate_limit_result() {
        let allowed = RateLimitResult::Allowed { remaining: 5 };
        assert!(allowed.is_allowed());
        assert!(allowed.retry_after_secs().is_none());

        let limited = RateLimitResult::Limited {
            retry_after_ms: 1500,
        };
        assert!(!limited.is_allowed());
        assert_eq!(limited.retry_after_secs(), Some(2));
    }
}
