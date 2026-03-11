package llm

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("llm circuit breaker is open")

// ResilienceConfig controls retry/fallback behavior for LLM requests.
type ResilienceConfig struct {
	FallbackModels          []string
	RetryMaxAttempts        int
	RetryInitialBackoff     time.Duration
	RetryMaxBackoff         time.Duration
	CircuitFailureThreshold int
	CircuitWindow           time.Duration
	CircuitOpenDuration     time.Duration
}

// ResilientClient wraps an LLM client with retry/circuit-breaker/fallback logic.
type ResilientClient struct {
	base      Client
	cfg       ResilienceConfig
	breakers  map[string]*circuitBreaker
	breakerMu sync.Mutex
	rngMu     sync.Mutex
	rng       *rand.Rand
}

func NewResilientClient(base Client, cfg ResilienceConfig) *ResilientClient {
	if cfg.RetryMaxAttempts < 0 {
		cfg.RetryMaxAttempts = 0
	}
	if cfg.RetryInitialBackoff <= 0 {
		cfg.RetryInitialBackoff = 300 * time.Millisecond
	}
	if cfg.RetryMaxBackoff <= 0 {
		cfg.RetryMaxBackoff = 2 * time.Second
	}
	if cfg.RetryInitialBackoff > cfg.RetryMaxBackoff {
		cfg.RetryInitialBackoff = cfg.RetryMaxBackoff
	}
	if cfg.CircuitFailureThreshold <= 0 {
		cfg.CircuitFailureThreshold = 5
	}
	if cfg.CircuitWindow <= 0 {
		cfg.CircuitWindow = 30 * time.Second
	}
	if cfg.CircuitOpenDuration <= 0 {
		cfg.CircuitOpenDuration = 15 * time.Second
	}

	return &ResilientClient{
		base:     base,
		cfg:      cfg,
		breakers: map[string]*circuitBreaker{},
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (c *ResilientClient) Create(ctx context.Context, req Request) (Response, error) {
	if c == nil || c.base == nil {
		return Response{}, errors.New("llm client is not configured")
	}
	return c.callWithResilience(ctx, req, func(callCtx context.Context, callReq Request) (Response, error) {
		return c.base.Create(callCtx, callReq)
	})
}

func (c *ResilientClient) Stream(ctx context.Context, req Request, onDelta func(string)) (Response, error) {
	if c == nil || c.base == nil {
		return Response{}, errors.New("llm client is not configured")
	}
	return c.callWithResilience(ctx, req, func(callCtx context.Context, callReq Request) (Response, error) {
		return c.base.Stream(callCtx, callReq, onDelta)
	})
}

func (c *ResilientClient) callWithResilience(ctx context.Context, req Request, callFn func(context.Context, Request) (Response, error)) (Response, error) {
	models := c.modelChain(req.Model)
	if len(models) == 0 {
		models = []string{req.Model}
	}

	var lastErr error
	for i, model := range models {
		callReq := req
		callReq.Model = model

		resp, err := c.callModelWithRetries(ctx, callReq, callFn)
		if err == nil {
			return resp, nil
		}
		lastErr = fmt.Errorf("model %s failed: %w", model, err)
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		if i == len(models)-1 {
			break
		}
		if !isFallbackEligible(err) {
			return Response{}, lastErr
		}
	}

	if lastErr == nil {
		lastErr = errors.New("llm request failed")
	}
	return Response{}, lastErr
}

func (c *ResilientClient) callModelWithRetries(ctx context.Context, req Request, callFn func(context.Context, Request) (Response, error)) (Response, error) {
	breaker := c.breakerForModel(req.Model)
	attempts := c.cfg.RetryMaxAttempts + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if !breaker.Allow(time.Now()) {
			return Response{}, ErrCircuitOpen
		}

		resp, err := callFn(ctx, req)
		if err == nil {
			breaker.OnSuccess()
			return resp, nil
		}
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}

		breaker.OnFailure(time.Now())
		info := classifyLLMError(err)
		if !info.Retryable || attempt == attempts-1 {
			return Response{}, err
		}

		delay := c.computeBackoff(attempt)
		if !sleepWithContext(ctx, delay) {
			return Response{}, ctx.Err()
		}
	}
	return Response{}, errors.New("llm retries exhausted")
}

func (c *ResilientClient) breakerForModel(model string) *circuitBreaker {
	key := strings.ToLower(strings.TrimSpace(model))
	if key == "" {
		key = "__default__"
	}

	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()
	if breaker, ok := c.breakers[key]; ok {
		return breaker
	}
	breaker := &circuitBreaker{
		failureThreshold: c.cfg.CircuitFailureThreshold,
		window:           c.cfg.CircuitWindow,
		openDuration:     c.cfg.CircuitOpenDuration,
	}
	c.breakers[key] = breaker
	return breaker
}

func (c *ResilientClient) modelChain(primary string) []string {
	seen := map[string]struct{}{}
	var models []string
	add := func(model string) {
		trimmed := strings.TrimSpace(model)
		if trimmed == "" {
			return
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		models = append(models, trimmed)
	}

	add(primary)
	for _, fallback := range c.cfg.FallbackModels {
		add(fallback)
	}
	return models
}

func (c *ResilientClient) computeBackoff(attempt int) time.Duration {
	delay := c.cfg.RetryInitialBackoff
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay >= c.cfg.RetryMaxBackoff {
			delay = c.cfg.RetryMaxBackoff
			break
		}
	}
	if delay > c.cfg.RetryMaxBackoff {
		delay = c.cfg.RetryMaxBackoff
	}
	// 20% jitter to avoid synchronized retries.
	c.rngMu.Lock()
	jitter := 0.8 + (0.4 * c.rng.Float64())
	c.rngMu.Unlock()
	jittered := time.Duration(float64(delay) * jitter)
	if jittered <= 0 {
		return delay
	}
	return jittered
}

type llmErrorInfo struct {
	Retryable       bool
	FallbackAllowed bool
}

func classifyLLMError(err error) llmErrorInfo {
	if err == nil {
		return llmErrorInfo{}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return llmErrorInfo{Retryable: false, FallbackAllowed: false}
	}
	if errors.Is(err, ErrCircuitOpen) {
		return llmErrorInfo{Retryable: false, FallbackAllowed: true}
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "401") || strings.Contains(msg, "403") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "invalid api key") || strings.Contains(msg, "forbidden") {
		return llmErrorInfo{Retryable: false, FallbackAllowed: false}
	}
	if strings.Contains(msg, "400") || strings.Contains(msg, "bad request") || strings.Contains(msg, "invalid request") {
		return llmErrorInfo{Retryable: false, FallbackAllowed: false}
	}
	if strings.Contains(msg, "model") && strings.Contains(msg, "not found") {
		return llmErrorInfo{Retryable: false, FallbackAllowed: true}
	}
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "timeout") || strings.Contains(msg, "temporar") || strings.Contains(msg, "connection") || strings.Contains(msg, "eof") || strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "504") || strings.Contains(msg, "500") {
		return llmErrorInfo{Retryable: true, FallbackAllowed: true}
	}
	return llmErrorInfo{Retryable: false, FallbackAllowed: true}
}

func isFallbackEligible(err error) bool {
	return classifyLLMError(err).FallbackAllowed
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

type circuitBreaker struct {
	mu               sync.Mutex
	failureThreshold int
	window           time.Duration
	openDuration     time.Duration
	failures         []time.Time
	openUntil        time.Time
	halfOpen         bool
}

func (b *circuitBreaker) Allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune(now)

	if now.Before(b.openUntil) {
		return false
	}
	if !b.openUntil.IsZero() && now.After(b.openUntil) && !b.halfOpen {
		b.halfOpen = true
	}
	return true
}

func (b *circuitBreaker) OnSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = nil
	b.openUntil = time.Time{}
	b.halfOpen = false
}

func (b *circuitBreaker) OnFailure(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune(now)

	if b.halfOpen {
		b.openUntil = now.Add(b.openDuration)
		b.halfOpen = false
		b.failures = []time.Time{now}
		return
	}

	b.failures = append(b.failures, now)
	if len(b.failures) >= b.failureThreshold {
		b.openUntil = now.Add(b.openDuration)
	}
}

func (b *circuitBreaker) prune(now time.Time) {
	if b.window <= 0 || len(b.failures) == 0 {
		return
	}
	cutoff := now.Add(-b.window)
	idx := 0
	for idx < len(b.failures) && b.failures[idx].Before(cutoff) {
		idx++
	}
	if idx > 0 {
		b.failures = append([]time.Time(nil), b.failures[idx:]...)
	}
}
