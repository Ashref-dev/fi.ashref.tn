package llm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fnClient struct {
	createFn func(context.Context, Request) (Response, error)
	streamFn func(context.Context, Request, func(string)) (Response, error)
}

func (c *fnClient) Create(ctx context.Context, req Request) (Response, error) {
	if c.createFn == nil {
		return Response{}, errors.New("create not implemented")
	}
	return c.createFn(ctx, req)
}

func (c *fnClient) Stream(ctx context.Context, req Request, onDelta func(string)) (Response, error) {
	if c.streamFn == nil {
		return Response{}, errors.New("stream not implemented")
	}
	return c.streamFn(ctx, req, onDelta)
}

func TestResilientClientRetriesRetryableCreateErrors(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	base := &fnClient{createFn: func(ctx context.Context, req Request) (Response, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		switch calls {
		case 1:
			return Response{}, errors.New("429 rate limit")
		case 2:
			return Response{}, errors.New("timeout")
		default:
			return Response{Content: "ok"}, nil
		}
	}}
	client := NewResilientClient(base, ResilienceConfig{
		RetryMaxAttempts:        2,
		RetryInitialBackoff:     1 * time.Millisecond,
		RetryMaxBackoff:         2 * time.Millisecond,
		CircuitFailureThreshold: 10,
		CircuitWindow:           1 * time.Minute,
		CircuitOpenDuration:     1 * time.Second,
	})

	resp, err := client.Create(context.Background(), Request{Model: "primary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}

func TestResilientClientDoesNotRetryNonRetryableCreateError(t *testing.T) {
	calls := 0
	base := &fnClient{createFn: func(ctx context.Context, req Request) (Response, error) {
		calls++
		return Response{}, errors.New("401 unauthorized")
	}}
	client := NewResilientClient(base, ResilienceConfig{
		RetryMaxAttempts:        3,
		RetryInitialBackoff:     1 * time.Millisecond,
		RetryMaxBackoff:         2 * time.Millisecond,
		CircuitFailureThreshold: 10,
		CircuitWindow:           1 * time.Minute,
		CircuitOpenDuration:     1 * time.Second,
	})

	_, err := client.Create(context.Background(), Request{Model: "primary"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 attempt, got %d", calls)
	}
}

func TestResilientClientFallsBackToNextModel(t *testing.T) {
	var models []string
	base := &fnClient{createFn: func(ctx context.Context, req Request) (Response, error) {
		models = append(models, req.Model)
		if req.Model == "primary" {
			return Response{}, errors.New("model not found")
		}
		if req.Model == "fallback-a" {
			return Response{Content: "fallback-success"}, nil
		}
		return Response{}, errors.New("unexpected model")
	}}
	client := NewResilientClient(base, ResilienceConfig{
		FallbackModels:          []string{"fallback-a", "fallback-b"},
		RetryMaxAttempts:        0,
		RetryInitialBackoff:     1 * time.Millisecond,
		RetryMaxBackoff:         2 * time.Millisecond,
		CircuitFailureThreshold: 10,
		CircuitWindow:           1 * time.Minute,
		CircuitOpenDuration:     1 * time.Second,
	})

	resp, err := client.Create(context.Background(), Request{Model: "primary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "fallback-success" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(models) != 2 || models[0] != "primary" || models[1] != "fallback-a" {
		t.Fatalf("unexpected model sequence: %+v", models)
	}
}

func TestResilientClientCircuitBreakerOpens(t *testing.T) {
	calls := 0
	base := &fnClient{createFn: func(ctx context.Context, req Request) (Response, error) {
		calls++
		return Response{}, errors.New("503 service unavailable")
	}}
	client := NewResilientClient(base, ResilienceConfig{
		RetryMaxAttempts:        0,
		RetryInitialBackoff:     1 * time.Millisecond,
		RetryMaxBackoff:         2 * time.Millisecond,
		CircuitFailureThreshold: 1,
		CircuitWindow:           1 * time.Minute,
		CircuitOpenDuration:     1 * time.Hour,
	})

	_, err := client.Create(context.Background(), Request{Model: "primary"})
	if err == nil {
		t.Fatalf("expected first call error")
	}
	_, err = client.Create(context.Background(), Request{Model: "primary"})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected backend to be called once, got %d", calls)
	}
}

func TestResilientClientFallbackWhenPrimaryCircuitOpen(t *testing.T) {
	var calls []string
	base := &fnClient{createFn: func(ctx context.Context, req Request) (Response, error) {
		calls = append(calls, req.Model)
		if req.Model == "primary" {
			return Response{}, errors.New("503 service unavailable")
		}
		if req.Model == "fallback-a" {
			return Response{Content: "ok-from-fallback"}, nil
		}
		return Response{}, errors.New("unexpected model")
	}}

	client := NewResilientClient(base, ResilienceConfig{
		FallbackModels:          []string{"fallback-a"},
		RetryMaxAttempts:        0,
		RetryInitialBackoff:     1 * time.Millisecond,
		RetryMaxBackoff:         2 * time.Millisecond,
		CircuitFailureThreshold: 1,
		CircuitWindow:           1 * time.Minute,
		CircuitOpenDuration:     1 * time.Hour,
	})

	firstResp, err := client.Create(context.Background(), Request{Model: "primary"})
	if err != nil {
		t.Fatalf("expected first call to recover with fallback, got %v", err)
	}
	if firstResp.Content != "ok-from-fallback" {
		t.Fatalf("unexpected first response: %+v", firstResp)
	}

	resp, err := client.Create(context.Background(), Request{Model: "primary"})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got %v", err)
	}
	if resp.Content != "ok-from-fallback" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(calls) != 3 || calls[0] != "primary" || calls[1] != "fallback-a" || calls[2] != "fallback-a" {
		t.Fatalf("unexpected call sequence: %+v", calls)
	}
}

func TestResilientClientRetriesStream(t *testing.T) {
	calls := 0
	var deltas []string
	base := &fnClient{streamFn: func(ctx context.Context, req Request, onDelta func(string)) (Response, error) {
		calls++
		if calls == 1 {
			return Response{}, errors.New("timeout")
		}
		if onDelta != nil {
			onDelta("ok")
		}
		return Response{Content: "ok"}, nil
	}}
	client := NewResilientClient(base, ResilienceConfig{
		RetryMaxAttempts:        1,
		RetryInitialBackoff:     1 * time.Millisecond,
		RetryMaxBackoff:         2 * time.Millisecond,
		CircuitFailureThreshold: 10,
		CircuitWindow:           1 * time.Minute,
		CircuitOpenDuration:     1 * time.Second,
	})

	resp, err := client.Stream(context.Background(), Request{Model: "primary"}, func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if calls != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls)
	}
	if len(deltas) != 1 || deltas[0] != "ok" {
		t.Fatalf("unexpected deltas: %+v", deltas)
	}
}
