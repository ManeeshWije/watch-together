package ratelimiter

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiter struct to manage client-specific rate limiters
type RateLimiter struct {
	mu        sync.Mutex
	clients   map[string]*rate.Limiter
	rateLimit rate.Limit
	burst     int
}

// NewRateLimiter initializes a RateLimiter
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		clients:   make(map[string]*rate.Limiter),
		rateLimit: r,
		burst:     b,
	}
}

// GetLimiter retrieves or creates a rate limiter for a specific client
func (rl *RateLimiter) GetLimiter(clientID string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if limiter, exists := rl.clients[clientID]; exists {
		return limiter
	}

	limiter := rate.NewLimiter(rl.rateLimit, rl.burst)
	rl.clients[clientID] = limiter
	return limiter
}

// Middleware to enforce rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use the client's IP address as the unique identifier
		clientID := r.RemoteAddr
		limiter := rl.GetLimiter(clientID)

		if !limiter.Allow() {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
