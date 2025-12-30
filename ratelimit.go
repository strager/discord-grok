package main

import (
	"sync"
	"time"
)

type RateLimiter struct {
	requestsPerMinute int
	users             sync.Map // map[string]*userState
}

type userState struct {
	mu       sync.Mutex
	requests []time.Time
}

func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	rl := &RateLimiter{
		requestsPerMinute: requestsPerMinute,
	}
	go rl.cleanup()
	return rl
}

// Allow checks if the user can make a request
func (rl *RateLimiter) Allow(userID string) bool {
	now := time.Now()
	windowStart := now.Add(-time.Minute)

	val, _ := rl.users.LoadOrStore(userID, &userState{})
	state := val.(*userState)

	state.mu.Lock()
	defer state.mu.Unlock()

	// Remove old requests outside the window
	validRequests := make([]time.Time, 0, len(state.requests))
	for _, t := range state.requests {
		if t.After(windowStart) {
			validRequests = append(validRequests, t)
		}
	}
	state.requests = validRequests

	// Check if under limit
	if len(state.requests) >= rl.requestsPerMinute {
		return false
	}

	// Record this request
	state.requests = append(state.requests, now)
	return true
}

// cleanup periodically removes inactive users to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		cutoff := time.Now().Add(-5 * time.Minute)
		rl.users.Range(func(key, value any) bool {
			state := value.(*userState)
			state.mu.Lock()
			// If all requests are old, remove this user
			allOld := true
			for _, t := range state.requests {
				if t.After(cutoff) {
					allOld = false
					break
				}
			}
			state.mu.Unlock()
			if allOld {
				rl.users.Delete(key)
			}
			return true
		})
	}
}
