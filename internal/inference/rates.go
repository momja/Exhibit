package inference

import (
	"math"
	"sync"
	"time"
)

// RateConfig bounds how fast calls may arrive. These are belts against a
// runaway loop in artifact code, not the spend limit: a call refused here was
// never sent and cost nothing. Zero fields take the defaults.
type RateConfig struct {
	// PerViewerPerMinute and PerArtifactPerMinute are sustained rates;
	// Burst is the bucket size for both.
	PerViewerPerMinute   int
	PerArtifactPerMinute int
	Burst                int
	// ViewerConcurrency bounds one viewer's calls in flight.
	ViewerConcurrency int
}

func (c RateConfig) withDefaults() RateConfig {
	if c.PerViewerPerMinute <= 0 {
		c.PerViewerPerMinute = 30
	}
	if c.PerArtifactPerMinute <= 0 {
		c.PerArtifactPerMinute = 60
	}
	if c.Burst <= 0 {
		c.Burst = 10
	}
	if c.ViewerConcurrency <= 0 {
		c.ViewerConcurrency = 4
	}
	return c
}

type bucket struct {
	tokens float64
	at     time.Time
}

type rateLimiter struct {
	cfg RateConfig
	now func() time.Time

	mu        sync.Mutex
	viewers   map[int64]*bucket
	artifacts map[string]*bucket
	inFlight  map[int64]int
}

func newRateLimiter(cfg RateConfig, now func() time.Time) *rateLimiter {
	return &rateLimiter{cfg: cfg.withDefaults(), now: now,
		viewers: map[int64]*bucket{}, artifacts: map[string]*bucket{}, inFlight: map[int64]int{}}
}

// take refills b and reports the wait until one token is available (zero
// when one was taken).
func (r *rateLimiter) take(b *bucket, perMinute int, commit bool) time.Duration {
	now := r.now()
	rate := float64(perMinute) / 60
	b.tokens = math.Min(float64(r.cfg.Burst), b.tokens+now.Sub(b.at).Seconds()*rate)
	b.at = now
	if b.tokens >= 1 {
		if commit {
			b.tokens--
		}
		return 0
	}
	return time.Duration((1 - b.tokens) / rate * float64(time.Second))
}

// admit charges one call to the viewer's and the artifact's buckets and
// holds a concurrency slot until release is called. Nothing is charged when
// either refuses.
func (r *rateLimiter) admit(viewerID int64, artifactID string, _ Kind) (release func(), err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inFlight[viewerID] >= r.cfg.ViewerConcurrency {
		return nil, &Error{Code: CodeRateLimited, Message: "Too many AI requests at once. Try again in a moment.", RetryAfter: time.Second}
	}
	vb := r.viewers[viewerID]
	if vb == nil {
		vb = &bucket{tokens: float64(r.cfg.Burst), at: r.now()}
		r.viewers[viewerID] = vb
	}
	ab := r.artifacts[artifactID]
	if ab == nil {
		ab = &bucket{tokens: float64(r.cfg.Burst), at: r.now()}
		r.artifacts[artifactID] = ab
	}
	wait := max(r.take(vb, r.cfg.PerViewerPerMinute, false), r.take(ab, r.cfg.PerArtifactPerMinute, false))
	if wait > 0 {
		return nil, &Error{Code: CodeRateLimited, Message: "Too many AI requests. Try again shortly.", RetryAfter: wait.Round(time.Second) + time.Second}
	}
	r.take(vb, r.cfg.PerViewerPerMinute, true)
	r.take(ab, r.cfg.PerArtifactPerMinute, true)
	r.inFlight[viewerID]++
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.inFlight[viewerID]--; r.inFlight[viewerID] <= 0 {
				delete(r.inFlight, viewerID)
			}
		})
	}, nil
}

// defaultModel is the BYOK fallback when INFERENCE_MODEL /
// INFERENCE_DECISION_MODEL does not resolve for the viewer's own provider.
// Each must exist in the pinned Pi catalog; Resolve checks before use, so a
// stale entry makes the kind unavailable rather than failing a call.
func defaultModel(kind Kind, provider string) string {
	switch kind {
	case KindComplete:
		return map[string]string{
			"openrouter":   "openai/gpt-4.1-nano",
			"openai":       "gpt-4.1-nano",
			"anthropic":    "claude-haiku-4-5",
			"google":       "gemini-2.5-flash-lite",
			"exhibit-mock": "exhibit-mock-1",
		}[provider]
	case KindDecide:
		return map[string]string{
			"openrouter": "typesafe/jev-1.14",
		}[provider]
	}
	return ""
}
