package main

import (
	"fmt"
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	attempts []time.Time
}

var (
	rateLimiters   = map[string]*RateLimiter{}
	rateLimitersMu sync.Mutex
)

// checkRateLimit checks if a request is allowed.
func checkRateLimit(alias string, profileKey string, rateLimitRPM *int) error {
	limit := 60
	if rateLimitRPM != nil {
		if *rateLimitRPM == 0 {
			return nil // unlimited
		}
		limit = *rateLimitRPM
	}

	rateLimitersMu.Lock()
	limiter, exists := rateLimiters[profileKey]
	if !exists {
		limiter = &RateLimiter{}
		rateLimiters[profileKey] = limiter
	}
	rateLimitersMu.Unlock()

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := time.Now()
	oneMinAgo := now.Add(-1 * time.Minute)

	// Clean up old attempts
	var validAttempts []time.Time
	for _, t := range limiter.attempts {
		if t.After(oneMinAgo) {
			validAttempts = append(validAttempts, t)
		}
	}
	limiter.attempts = validAttempts

	if len(limiter.attempts) >= limit {
		return fmt.Errorf("rate limit exceeded for profile %s: max %d requests/min", alias, limit)
	}

	limiter.attempts = append(limiter.attempts, now)
	return nil
}
