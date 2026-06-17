package session

import (
	"time"
)

// Rate-limit defaults. C had no throttle; these are Go-only safety values.
const (
	DefaultBurstRate    = 10           // max burst of commands in the window
	DefaultSustainRate  = 30           // max sustained commands per second
	DefaultRateWindow   = time.Second  // sliding window duration
	ThrottleMessage     = "\r\n너무 빠릅니다. 잠시 기다리세요.\r\n"
)

// nowFunc is overridable for testing.
var nowFunc = time.Now

// rateLimiter implements a sliding-window command rate limiter.
// It tracks timestamps of accepted commands within a rolling window.
// If the number of accepted commands in the window exceeds burstRate,
// further commands are rejected until the oldest entry exits the window.
// Additionally, if the sustained rate (total commands in the window
// divided by window duration) exceeds sustainRate, commands are rejected.
//
// For simplicity and testability, this uses a slice of timestamps
// (not a token bucket) because burst and sustained limits are both
// enforced over the same window.
type rateLimiter struct {
	burst   int           // max commands allowed in rapid succession
	sustain int           // max commands per second sustained
	window  time.Duration // sliding window size

	// stamps holds unix-nano timestamps of accepted commands, oldest first.
	// No mutex needed: readLoop is single-goroutine per session.
	stamps []int64
}

func newRateLimiter(burst, sustain int, window time.Duration) *rateLimiter {
	if burst <= 0 {
		burst = DefaultBurstRate
	}
	if sustain <= 0 {
		sustain = DefaultSustainRate
	}
	if window <= 0 {
		window = DefaultRateWindow
	}
	return &rateLimiter{
		burst:   burst,
		sustain: sustain,
		window:  window,
	}
}

// NewRateLimiterForTest creates a rate limiter with custom burst/sustain
// and the default 1-second window. Exported for use by integration tests
// in cmd/muhan-server.
func NewRateLimiterForTest(burst, sustain int) *rateLimiter {
	return newRateLimiter(burst, sustain, DefaultRateWindow)
}

// allow returns true if a command is allowed under the rate limit.
// It prunes expired entries first, then checks burst and sustained limits.
func (r *rateLimiter) allow(now time.Time) bool {
	cutoff := now.Add(-r.window).UnixNano()

	// Prune expired timestamps.
	i := 0
	for i < len(r.stamps) && r.stamps[i] <= cutoff {
		i++
	}
	if i > 0 {
		r.stamps = r.stamps[i:]
	}

	// Check burst limit: no more than `burst` commands in the window.
	if len(r.stamps) >= r.burst {
		return false
	}

	// Check sustained limit: no more than `sustain` commands per second.
	// Since window is typically 1s, this means no more than `sustain`
	// commands total in the window. For other window sizes, scale accordingly.
	maxInWindow := int(int64(r.sustain) * int64(r.window) / int64(time.Second))
	if maxInWindow <= 0 {
		maxInWindow = r.sustain
	}
	if len(r.stamps) >= maxInWindow {
		return false
	}

	r.stamps = append(r.stamps, now.UnixNano())
	return true
}
