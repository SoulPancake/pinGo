package limits

import (
	"context"
	"sync"
	"time"

	"github.com/SoulPancake/pinGo/pkg/errors"
)

type RateLimiter interface {
	Allow() bool
	AllowN(n int) bool
	Wait(ctx context.Context) *errors.Error
	WaitN(ctx context.Context, n int) *errors.Error
}

type TokenBucket struct {
	capacity     int64
	tokens       int64
	refillRate   int64
	lastRefill   time.Time
	mu           sync.Mutex
}

func NewTokenBucket(capacity, refillRate int64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (tb *TokenBucket) Allow() bool {
	return tb.AllowN(1)
}

func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= int64(n) {
		tb.tokens -= int64(n)
		return true
	}

	return false
}

func (tb *TokenBucket) Wait(ctx context.Context) *errors.Error {
	return tb.WaitN(ctx, 1)
}

func (tb *TokenBucket) WaitN(ctx context.Context, n int) *errors.Error {
	for {
		if tb.AllowN(n) {
			return nil
		}

		waitTime := tb.calculateWaitTime(int64(n))
		
		select {
		case <-time.After(waitTime):
			continue
		case <-ctx.Done():
			return errors.NewWithCause(errors.TimeoutError, "rate limit wait cancelled", ctx.Err())
		}
	}
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)
	tokensToAdd := int64(elapsed.Seconds()) * tb.refillRate

	if tokensToAdd > 0 {
		tb.tokens += tokensToAdd
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastRefill = now
	}
}

func (tb *TokenBucket) calculateWaitTime(needed int64) time.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()
	deficit := needed - tb.tokens
	if deficit <= 0 {
		return 0
	}

	return time.Duration(deficit/tb.refillRate) * time.Second
}

func (tb *TokenBucket) GetTokens() int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

type SlidingWindowLimiter struct {
	limit      int64
	window     time.Duration
	requests   []time.Time
	mu         sync.Mutex
}

func NewSlidingWindowLimiter(limit int64, window time.Duration) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		limit:    limit,
		window:   window,
		requests: make([]time.Time, 0),
	}
}

func (sw *SlidingWindowLimiter) Allow() bool {
	return sw.AllowN(1)
}

func (sw *SlidingWindowLimiter) AllowN(n int) bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	sw.cleanup(now)

	if int64(len(sw.requests))+int64(n) <= sw.limit {
		for i := 0; i < n; i++ {
			sw.requests = append(sw.requests, now)
		}
		return true
	}

	return false
}

func (sw *SlidingWindowLimiter) Wait(ctx context.Context) *errors.Error {
	return sw.WaitN(ctx, 1)
}

func (sw *SlidingWindowLimiter) WaitN(ctx context.Context, n int) *errors.Error {
	for {
		if sw.AllowN(n) {
			return nil
		}

		select {
		case <-time.After(100 * time.Millisecond):
			continue
		case <-ctx.Done():
			return errors.NewWithCause(errors.TimeoutError, "rate limit wait cancelled", ctx.Err())
		}
	}
}

func (sw *SlidingWindowLimiter) cleanup(now time.Time) {
	cutoff := now.Add(-sw.window)
	
	var validRequests []time.Time
	for _, req := range sw.requests {
		if req.After(cutoff) {
			validRequests = append(validRequests, req)
		}
	}
	
	sw.requests = validRequests
}

func (sw *SlidingWindowLimiter) GetCurrentCount() int64 {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	
	sw.cleanup(time.Now())
	return int64(len(sw.requests))
}

type ConcurrencyLimiter struct {
	limit   int64
	current int64
	mu      sync.Mutex
	cond    *sync.Cond
}

func NewConcurrencyLimiter(limit int64) *ConcurrencyLimiter {
	cl := &ConcurrencyLimiter{
		limit: limit,
	}
	cl.cond = sync.NewCond(&cl.mu)
	return cl
}

func (cl *ConcurrencyLimiter) Acquire(ctx context.Context) *errors.Error {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	for cl.current >= cl.limit {
		done := make(chan struct{})
		go func() {
			cl.cond.Wait()
			close(done)
		}()

		cl.mu.Unlock()
		select {
		case <-done:
			cl.mu.Lock()
		case <-ctx.Done():
			cl.mu.Lock()
			return errors.NewWithCause(errors.TimeoutError, "concurrency limit wait cancelled", ctx.Err())
		}
	}

	cl.current++
	return nil
}

func (cl *ConcurrencyLimiter) Release() {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.current > 0 {
		cl.current--
		cl.cond.Signal()
	}
}

func (cl *ConcurrencyLimiter) TryAcquire() bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if cl.current < cl.limit {
		cl.current++
		return true
	}

	return false
}

func (cl *ConcurrencyLimiter) GetCurrent() int64 {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.current
}

func (cl *ConcurrencyLimiter) GetLimit() int64 {
	return cl.limit
}

type FixedWindowLimiter struct {
	limit      int64
	window     time.Duration
	count      int64
	windowStart time.Time
	mu         sync.Mutex
}

func NewFixedWindowLimiter(limit int64, window time.Duration) *FixedWindowLimiter {
	return &FixedWindowLimiter{
		limit:       limit,
		window:      window,
		windowStart: time.Now(),
	}
}

func (fw *FixedWindowLimiter) Allow() bool {
	return fw.AllowN(1)
}

func (fw *FixedWindowLimiter) AllowN(n int) bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	now := time.Now()
	if now.Sub(fw.windowStart) >= fw.window {
		fw.count = 0
		fw.windowStart = now
	}

	if fw.count+int64(n) <= fw.limit {
		fw.count += int64(n)
		return true
	}

	return false
}

func (fw *FixedWindowLimiter) Wait(ctx context.Context) *errors.Error {
	return fw.WaitN(ctx, 1)
}

func (fw *FixedWindowLimiter) WaitN(ctx context.Context, n int) *errors.Error {
	for {
		if fw.AllowN(n) {
			return nil
		}

		fw.mu.Lock()
		waitTime := fw.window - time.Since(fw.windowStart)
		fw.mu.Unlock()

		if waitTime <= 0 {
			waitTime = 100 * time.Millisecond
		}

		select {
		case <-time.After(waitTime):
			continue
		case <-ctx.Done():
			return errors.NewWithCause(errors.TimeoutError, "rate limit wait cancelled", ctx.Err())
		}
	}
}

func (fw *FixedWindowLimiter) GetCurrentCount() int64 {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	now := time.Now()
	if now.Sub(fw.windowStart) >= fw.window {
		return 0
	}

	return fw.count
}

func (fw *FixedWindowLimiter) GetTimeToReset() time.Duration {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	remaining := fw.window - time.Since(fw.windowStart)
	if remaining < 0 {
		return 0
	}
	return remaining
}

type MultiLimiter struct {
	limiters []RateLimiter
}

func NewMultiLimiter(limiters ...RateLimiter) *MultiLimiter {
	return &MultiLimiter{
		limiters: limiters,
	}
}

func (ml *MultiLimiter) Allow() bool {
	return ml.AllowN(1)
}

func (ml *MultiLimiter) AllowN(n int) bool {
	for _, limiter := range ml.limiters {
		if !limiter.AllowN(n) {
			return false
		}
	}
	return true
}

func (ml *MultiLimiter) Wait(ctx context.Context) *errors.Error {
	return ml.WaitN(ctx, 1)
}

func (ml *MultiLimiter) WaitN(ctx context.Context, n int) *errors.Error {
	for _, limiter := range ml.limiters {
		if err := limiter.WaitN(ctx, n); err != nil {
			return err
		}
	}
	return nil
}