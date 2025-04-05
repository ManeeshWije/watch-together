use crate::types::AppState;
use crate::types::{RateLimiter, SharedRateLimiter, TokenBucket};
use axum::{
    body::Body,
    extract::{ConnectInfo, State},
    http::{Request, StatusCode},
    middleware::Next,
    response::Response,
};
use std::{
    collections::HashMap,
    net::IpAddr,
    sync::Arc,
    time::{Duration, Instant},
};
use tokio::sync::Mutex;

impl TokenBucket {
    pub fn new(capacity: usize, refill_rate: usize) -> Self {
        Self {
            capacity,
            tokens: capacity, // Start with full bucket
            refill_rate,
            last_refill: Instant::now(),
        }
    }

    pub fn take(&mut self) -> bool {
        // Refill tokens based on time elapsed
        self.refill();

        // Check if there are tokens available
        if self.tokens > 0 {
            self.tokens -= 1;
            true
        } else {
            false
        }
    }

    fn refill(&mut self) {
        let now = Instant::now();
        let elapsed = now.duration_since(self.last_refill).as_secs() as usize;

        if elapsed > 0 {
            // Calculate how many tokens to add
            let new_tokens = elapsed * self.refill_rate;
            self.tokens = (self.tokens + new_tokens).min(self.capacity);
            self.last_refill = now;
        }
    }
}

impl RateLimiter {
    pub fn new(capacity: usize, refill_rate: usize) -> Self {
        Self {
            buckets: HashMap::new(),
            capacity,
            refill_rate,
        }
    }

    pub fn check_rate_limit(&mut self, ip: IpAddr) -> bool {
        let bucket = self
            .buckets
            .entry(ip)
            .or_insert_with(|| TokenBucket::new(self.capacity, self.refill_rate));

        bucket.take()
    }
}

// Create a new shared rate limiter
pub fn create_rate_limiter(capacity: usize, refill_rate: usize) -> SharedRateLimiter {
    Arc::new(Mutex::new(RateLimiter::new(capacity, refill_rate)))
}

// Fixed middleware for rate limiting - use Body as the specific type
pub async fn rate_limit_middleware(
    State(app_state): State<AppState>,
    ConnectInfo(addr): ConnectInfo<std::net::SocketAddr>,
    request: Request<Body>,
    next: Next,
) -> Result<Response, StatusCode> {
    // Get the rate limiter from application state
    let mut rate_limiter = app_state.rate_limiter.lock().await;

    // Check if this IP is allowed to make a request
    let ip = addr.ip();
    if !rate_limiter.check_rate_limit(ip) {
        return Err(StatusCode::TOO_MANY_REQUESTS);
    }

    // If we get here, the request is allowed
    let response = next.run(request).await;
    Ok(response)
}

// Helper function to clean up old rate limiters periodically
pub async fn cleanup_rate_limiters(rate_limiter: SharedRateLimiter) {
    let mut interval = tokio::time::interval(Duration::from_secs(3600)); // Run once an hour

    loop {
        interval.tick().await;

        // Remove any rate limiters that haven't been used recently
        let mut limiter = rate_limiter.lock().await;
        let now = Instant::now();
        limiter
            .buckets
            .retain(|_, bucket| now.duration_since(bucket.last_refill) < Duration::from_secs(3600));
    }
}
