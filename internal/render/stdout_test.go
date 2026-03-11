package render

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"fi-cli/internal/events"
)

type testBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (tb *testBuffer) Write(p []byte) (int, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.b.Write(p)
}

func (tb *testBuffer) String() string {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.b.String()
}

func (tb *testBuffer) Len() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.b.Len()
}

func TestSpinnerLifecycleStopsAndClearsOnFirstVisibleOutput(t *testing.T) {
	var buf testBuffer
	r := NewStdoutRenderer(
		&buf,
		false,
		false,
		true,
		false,
		true,
		StdoutRendererOptions{
			Interactive:     true,
			SpinnerWriter:   &buf,
			SpinnerInterval: 2 * time.Millisecond,
			SpinnerFrames:   []string{".", ".."},
		},
	)

	r.Emit(events.Event{Type: events.RunStarted, Payload: events.RunStartedPayload{}})
	waitForOutput(t, &buf, "thinking")
	r.Emit(events.Event{Type: events.FinalAnswerReady, Payload: events.FinalAnswerPayload{Answer: "done"}})
	_ = r.Close()

	out := buf.String()
	if !strings.Contains(out, "thinking") {
		t.Fatalf("expected spinner output before first visible output, got %q", out)
	}
	if !strings.Contains(out, "thinking .") && !strings.Contains(out, "thinking ..") {
		t.Fatalf("expected spinner to render on the right of the thinking label, got %q", out)
	}
	clearIdx := strings.LastIndex(out, "\x1b[K")
	if clearIdx == -1 {
		t.Fatalf("expected spinner clear sequence, got %q", out)
	}
	tail := out[clearIdx+len("\x1b[K"):]
	if strings.Contains(tail, "thinking") {
		t.Fatalf("expected no spinner residue after clear, got %q", tail)
	}
	if !strings.Contains(tail, "fi: done\n") {
		t.Fatalf("expected final answer after spinner clear, got %q", tail)
	}
}

func TestSpinnerResumesAfterPlanOutput(t *testing.T) {
	var buf testBuffer
	r := NewStdoutRenderer(
		&buf,
		false,
		false,
		false,
		false,
		true,
		StdoutRendererOptions{
			Interactive:     true,
			SpinnerWriter:   &buf,
			SpinnerInterval: 2 * time.Millisecond,
			SpinnerFrames:   []string{".", ".."},
		},
	)

	r.Emit(events.Event{Type: events.RunStarted, Payload: events.RunStartedPayload{}})
	waitForOutput(t, &buf, "thinking")
	r.Emit(events.Event{Type: events.PlanGenerated, Payload: events.PlanGeneratedPayload{Plan: []string{"Inspect repo"}}})
	waitForOutput(t, &buf, "Plan:\n- Inspect repo\n")

	planBlock := "Plan:\n- Inspect repo\n"
	planIdx := strings.LastIndex(buf.String(), planBlock)
	if planIdx == -1 {
		t.Fatalf("expected plan output, got %q", buf.String())
	}
	waitForOutputAfter(t, &buf, "thinking", planIdx+len(planBlock))
	_ = r.Close()
}

func TestSpinnerShowsUnderToolLinesUntilModelDelta(t *testing.T) {
	var buf testBuffer
	r := NewStdoutRenderer(
		&buf,
		false,
		false,
		true,
		false,
		true,
		StdoutRendererOptions{
			Interactive:     true,
			SpinnerWriter:   &buf,
			SpinnerInterval: 2 * time.Millisecond,
			SpinnerFrames:   []string{".", ".."},
		},
	)

	r.Emit(events.Event{Type: events.RunStarted, Payload: events.RunStartedPayload{}})
	waitForOutput(t, &buf, "thinking")

	r.Emit(events.Event{
		Type: events.ToolCallFinished,
		Payload: events.ToolCallFinishedPayload{
			ToolName:   "grep",
			Status:     "success",
			DurationMs: 12,
			LineCount:  4,
			ByteCount:  120,
		},
	})
	waitForOutput(t, &buf, "tool: grep ok (12ms, 4 lines, 120 bytes)")
	toolLine := "tool: grep ok (12ms, 4 lines, 120 bytes)\n"
	toolIdx := strings.LastIndex(buf.String(), toolLine)
	if toolIdx == -1 {
		t.Fatalf("expected tool line, got %q", buf.String())
	}
	waitForOutputAfter(t, &buf, "thinking", toolIdx+len(toolLine))

	beforeDelta := buf.String()
	startAfterDelta := len(beforeDelta)
	r.Emit(events.Event{Type: events.ModelDelta, Payload: events.ModelDeltaPayload{Delta: "hello"}})
	time.Sleep(10 * time.Millisecond)
	r.Emit(events.Event{Type: events.FinalAnswerReady, Payload: events.FinalAnswerPayload{Answer: "hello"}})
	_ = r.Close()

	out := buf.String()
	tail := out[startAfterDelta:]
	clearIdx := strings.LastIndex(tail, "\x1b[K")
	if clearIdx == -1 {
		t.Fatalf("expected spinner clear after model delta, got %q", tail)
	}
	if strings.Contains(tail[clearIdx+len("\x1b[K"):], "thinking") {
		t.Fatalf("spinner should stop once model delta starts, got %q", tail)
	}
	if !strings.Contains(out, "fi: hello\n") {
		t.Fatalf("expected streamed final output, got %q", out)
	}
}

func TestSpinnerDisabledWhenNotInteractive(t *testing.T) {
	var buf testBuffer
	r := NewStdoutRenderer(&buf, false, false, true, false, true)

	r.Emit(events.Event{Type: events.RunStarted, Payload: events.RunStartedPayload{}})
	r.Emit(events.Event{Type: events.FinalAnswerReady, Payload: events.FinalAnswerPayload{Answer: "done"}})
	_ = r.Close()

	out := buf.String()
	if strings.Contains(out, "thinking") || strings.Contains(out, "\x1b[K") {
		t.Fatalf("unexpected spinner output for non-interactive renderer: %q", out)
	}
	if out != "fi: done\n" {
		t.Fatalf("unexpected renderer output: %q", out)
	}
}

func waitForOutput(t *testing.T, buf *testBuffer, marker string) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Millisecond)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), marker) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in %q", marker, buf.String())
}

func waitForOutputAfter(t *testing.T, buf *testBuffer, marker string, start int) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Millisecond)
	for time.Now().Before(deadline) {
		out := buf.String()
		if start < 0 {
			start = 0
		}
		if start > len(out) {
			start = len(out)
		}
		if strings.Contains(out[start:], marker) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q after %d in %q", marker, start, buf.String())
}
