package timeout

import (
	"context"
	"sync"
	"time"

	"github.com/SoulPancake/pinGo/pkg/errors"
)

type Timer struct {
	duration time.Duration
	callback func()
	timer    *time.Timer
	mu       sync.Mutex
	active   bool
}

func NewTimer(duration time.Duration, callback func()) *Timer {
	return &Timer{
		duration: duration,
		callback: callback,
	}
}

func (t *Timer) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.active {
		return
	}

	t.timer = time.AfterFunc(t.duration, func() {
		t.mu.Lock()
		t.active = false
		t.mu.Unlock()
		
		if t.callback != nil {
			t.callback()
		}
	})
	
	t.active = true
}

func (t *Timer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.active || t.timer == nil {
		return false
	}

	stopped := t.timer.Stop()
	t.active = false
	return stopped
}

func (t *Timer) Reset(duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.duration = duration
	
	if t.active && t.timer != nil {
		t.timer.Reset(duration)
	}
}

func (t *Timer) IsActive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

type TimeoutGroup struct {
	timers map[string]*Timer
	mu     sync.RWMutex
}

func NewTimeoutGroup() *TimeoutGroup {
	return &TimeoutGroup{
		timers: make(map[string]*Timer),
	}
}

func (tg *TimeoutGroup) AddTimer(name string, duration time.Duration, callback func()) {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	if existing, exists := tg.timers[name]; exists {
		existing.Stop()
	}

	timer := NewTimer(duration, callback)
	tg.timers[name] = timer
	timer.Start()
}

func (tg *TimeoutGroup) RemoveTimer(name string) bool {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	if timer, exists := tg.timers[name]; exists {
		stopped := timer.Stop()
		delete(tg.timers, name)
		return stopped
	}

	return false
}

func (tg *TimeoutGroup) ResetTimer(name string, duration time.Duration) bool {
	tg.mu.RLock()
	timer, exists := tg.timers[name]
	tg.mu.RUnlock()

	if !exists {
		return false
	}

	timer.Reset(duration)
	return true
}

func (tg *TimeoutGroup) StopAll() {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	for _, timer := range tg.timers {
		timer.Stop()
	}

	tg.timers = make(map[string]*Timer)
}

func (tg *TimeoutGroup) Count() int {
	tg.mu.RLock()
	defer tg.mu.RUnlock()
	return len(tg.timers)
}

func WithTimeout(ctx context.Context, timeout time.Duration, fn func(context.Context) *errors.Error) *errors.Error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan *errors.Error, 1)
	
	go func() {
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return errors.New(errors.TimeoutError, "operation timed out")
		}
		return errors.NewWithCause(errors.InternalError, "context cancelled", ctx.Err())
	}
}

func WithDeadline(ctx context.Context, deadline time.Time, fn func(context.Context) *errors.Error) *errors.Error {
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	done := make(chan *errors.Error, 1)
	
	go func() {
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return errors.New(errors.TimeoutError, "deadline exceeded")
		}
		return errors.NewWithCause(errors.InternalError, "context cancelled", ctx.Err())
	}
}

type RetryConfig struct {
	MaxAttempts int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
	}
}

func WithRetry(ctx context.Context, config *RetryConfig, fn func(context.Context) *errors.Error) *errors.Error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr *errors.Error
	delay := config.InitialDelay

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return errors.NewWithCause(errors.TimeoutError, "retry cancelled", ctx.Err())
			}

			delay = time.Duration(float64(delay) * config.Multiplier)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}

		if err := fn(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return errors.NewWithCause(errors.TimeoutError, "retry cancelled", ctx.Err())
		default:
		}
	}

	return lastErr
}

type Circuit struct {
	maxFailures  int
	resetTimeout time.Duration
	failures     int
	lastFailTime time.Time
	state        CircuitState
	mu           sync.RWMutex
}

type CircuitState int

const (
	Closed CircuitState = iota
	Open
	HalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case Closed:
		return "Closed"
	case Open:
		return "Open"
	case HalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *Circuit {
	return &Circuit{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        Closed,
	}
}

func (c *Circuit) Call(ctx context.Context, fn func(context.Context) *errors.Error) *errors.Error {
	if !c.allowRequest() {
		return errors.New(errors.TimeoutError, "circuit breaker is open")
	}

	err := fn(ctx)
	c.recordResult(err == nil)
	return err
}

func (c *Circuit) allowRequest() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case Closed:
		return true
	case Open:
		if time.Since(c.lastFailTime) > c.resetTimeout {
			c.state = HalfOpen
			return true
		}
		return false
	case HalfOpen:
		return true
	default:
		return false
	}
}

func (c *Circuit) recordResult(success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if success {
		c.failures = 0
		c.state = Closed
	} else {
		c.failures++
		c.lastFailTime = time.Now()

		if c.failures >= c.maxFailures {
			c.state = Open
		}
	}
}

func (c *Circuit) GetState() CircuitState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Circuit) GetFailures() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.failures
}

func (c *Circuit) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.failures = 0
	c.state = Closed
	c.lastFailTime = time.Time{}
}