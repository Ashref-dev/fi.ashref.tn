package render

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"ag-cli/internal/events"
)

// StdoutRenderer streams events to a plain text writer.
type StdoutRenderer struct {
	w                  io.Writer
	mu                 sync.Mutex
	verbose            bool
	quiet              bool
	noPlan             bool
	printedFinalHeader bool
	sawDelta           bool
	endedWithNewline   bool
}

// NewStdoutRenderer creates a renderer for plain text streaming.
func NewStdoutRenderer(w io.Writer, verbose bool, quiet bool, noPlan bool) *StdoutRenderer {
	return &StdoutRenderer{w: w, verbose: verbose, quiet: quiet, noPlan: noPlan}
}

func (r *StdoutRenderer) Emit(event events.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch event.Type {
	case events.RunStarted:
		if payload, ok := event.Payload.(events.RunStartedPayload); ok {
			if r.quiet {
				return
			}
			fmt.Fprintf(r.w, "fi v%s | repo: %s | model: %s | run: %s\n", payload.Version, payload.RepoRoot, payload.Model, payload.RunID)
			fmt.Fprintf(r.w, "Started: %s\n", payload.StartedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
	case events.PlanGenerated:
		if payload, ok := event.Payload.(events.PlanGeneratedPayload); ok {
			if r.quiet || r.noPlan {
				return
			}
			fmt.Fprintln(r.w, "\nPlan:")
			for _, item := range payload.Plan {
				fmt.Fprintf(r.w, "- %s\n", item)
			}
		}
	case events.ToolCallStarted:
		if payload, ok := event.Payload.(events.ToolCallStartedPayload); ok {
			if r.quiet {
				return
			}
			fmt.Fprintf(r.w, "\nTool: %s (started)\n", payload.ToolName)
			if r.verbose {
				fmt.Fprintf(r.w, "Input: %v\n", payload.Input)
			}
		}
	case events.ToolCallFinished, events.ToolCallFailed:
		if payload, ok := event.Payload.(events.ToolCallFinishedPayload); ok {
			if r.quiet {
				return
			}
			fmt.Fprintf(r.w, "Tool: %s (%s, %dms, lines=%d, bytes=%d, truncated=%t)\n", payload.ToolName, payload.Status, payload.DurationMs, payload.LineCount, payload.ByteCount, payload.Truncated)
			if r.verbose && payload.Preview != "" {
				fmt.Fprintln(r.w, "Preview:")
				for _, line := range strings.Split(payload.Preview, "\n") {
					fmt.Fprintf(r.w, "  %s\n", line)
				}
			}
		}
	case events.ModelDelta:
		if payload, ok := event.Payload.(events.ModelDeltaPayload); ok {
			if !r.printedFinalHeader {
				if !r.quiet {
					fmt.Fprintln(r.w, "\nFinal Answer:")
				}
				r.printedFinalHeader = true
			}
			if payload.Delta != "" {
				fmt.Fprint(r.w, payload.Delta)
				r.sawDelta = true
				r.endedWithNewline = strings.HasSuffix(payload.Delta, "\n")
			}
		}
	case events.FinalAnswerReady:
		if payload, ok := event.Payload.(events.FinalAnswerPayload); ok {
			if r.sawDelta {
				if !r.endedWithNewline {
					fmt.Fprintln(r.w)
				}
				return
			}
			if !r.printedFinalHeader {
				if !r.quiet {
					fmt.Fprintln(r.w, "\nFinal Answer:")
				}
				r.printedFinalHeader = true
			}
			fmt.Fprintln(r.w, payload.Answer)
		}
	case events.RunError:
		if payload, ok := event.Payload.(events.RunErrorPayload); ok {
			fmt.Fprintf(r.w, "\nError: %s\n", payload.Message)
		}
	}
}

func (r *StdoutRenderer) Close() error {
	return nil
}
