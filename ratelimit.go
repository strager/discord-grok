package main

import (
	"sync"
	"time"
)

type RateLimiter struct {
	users          sync.Map // map[string]*userState
	globalMu       sync.Mutex
	globalRequests []time.Time
}

type userState struct {
	mu       sync.Mutex
	requests []time.Time
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{}
	go rl.cleanup()
	return rl
}

// Allow checks if the user can make a request
func (rl *RateLimiter) Allow(userID string) bool {
	now := time.Now()
	userWindowStart := now.Add(-5 * time.Minute)
	globalWindowStart := now.Add(-10 * time.Minute)

	// Check global limit first (5 per 10 minutes)
	rl.globalMu.Lock()
	validGlobal := make([]time.Time, 0, len(rl.globalRequests))
	for _, t := range rl.globalRequests {
		if t.After(globalWindowStart) {
			validGlobal = append(validGlobal, t)
		}
	}
	rl.globalRequests = validGlobal
	if len(rl.globalRequests) >= 5 {
		rl.globalMu.Unlock()
		return false
	}

	// Check per-user limit (1 per 5 minutes)
	val, _ := rl.users.LoadOrStore(userID, &userState{})
	state := val.(*userState)

	state.mu.Lock()
	validUser := make([]time.Time, 0, len(state.requests))
	for _, t := range state.requests {
		if t.After(userWindowStart) {
			validUser = append(validUser, t)
		}
	}
	state.requests = validUser
	if len(state.requests) >= 1 {
		state.mu.Unlock()
		rl.globalMu.Unlock()
		return false
	}

	// Record this request in both trackers
	state.requests = append(state.requests, now)
	state.mu.Unlock()
	rl.globalRequests = append(rl.globalRequests, now)
	rl.globalMu.Unlock()
	return true
}

// cleanup periodically removes inactive users to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Minute)

		// Clean up global requests
		rl.globalMu.Lock()
		validGlobal := make([]time.Time, 0, len(rl.globalRequests))
		for _, t := range rl.globalRequests {
			if t.After(cutoff) {
				validGlobal = append(validGlobal, t)
			}
		}
		rl.globalRequests = validGlobal
		rl.globalMu.Unlock()

		// Clean up inactive users
		rl.users.Range(func(key, value any) bool {
			state := value.(*userState)
			state.mu.Lock()
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
