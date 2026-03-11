package render

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"fi-cli/internal/events"
)

const defaultSpinnerInterval = 80 * time.Millisecond

// Based on the widely-used "dots" spinner from cli-spinners.
var defaultSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// StdoutRendererOptions configures interactive behavior.
type StdoutRendererOptions struct {
	Interactive     bool
	SpinnerWriter   io.Writer
	SpinnerInterval time.Duration
	SpinnerFrames   []string
}

// StdoutRenderer streams events to a plain text writer.
type StdoutRenderer struct {
	w                  io.Writer
	mu                 sync.Mutex
	verbose            bool
	quiet              bool
	noPlan             bool
	showHeader         bool
	showTools          bool
	printedFinalHeader bool
	sawDelta           bool
	endedWithNewline   bool

	spinnerEnabled  bool
	spinnerWriter   io.Writer
	spinnerInterval time.Duration
	spinnerFrames   []string
	spinnerFrameIdx int
	spinnerActive   bool
	spinnerDrawn    bool
	spinnerStop     chan struct{}
	spinnerDone     bool
}

// NewStdoutRenderer creates a renderer for plain text streaming.
func NewStdoutRenderer(w io.Writer, verbose bool, quiet bool, noPlan bool, showHeader bool, showTools bool, opts ...StdoutRendererOptions) *StdoutRenderer {
	opt := StdoutRendererOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	spinnerWriter := opt.SpinnerWriter
	if spinnerWriter == nil {
		spinnerWriter = w
	}
	spinnerInterval := opt.SpinnerInterval
	if spinnerInterval <= 0 {
		spinnerInterval = defaultSpinnerInterval
	}
	spinnerFrames := opt.SpinnerFrames
	if len(spinnerFrames) == 0 {
		spinnerFrames = append([]string(nil), defaultSpinnerFrames...)
	}
	return &StdoutRenderer{
		w:               w,
		verbose:         verbose,
		quiet:           quiet,
		noPlan:          noPlan,
		showHeader:      showHeader,
		showTools:       showTools,
		spinnerEnabled:  opt.Interactive,
		spinnerWriter:   spinnerWriter,
		spinnerInterval: spinnerInterval,
		spinnerFrames:   spinnerFrames,
	}
}

func (r *StdoutRenderer) Emit(event events.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch event.Type {
	case events.RunStarted:
		if payload, ok := event.Payload.(events.RunStartedPayload); ok {
			if !r.quiet && r.showHeader {
				fmt.Fprintf(r.w, "fi-cli v%s | repo: %s | model: %s | run: %s\n", payload.Version, payload.RepoRoot, payload.Model, payload.RunID)
				fmt.Fprintf(r.w, "Started: %s\n", payload.StartedAt.Format("2006-01-02T15:04:05Z07:00"))
			}
			r.resumeSpinnerLocked()
		}
	case events.PlanGenerated:
		if payload, ok := event.Payload.(events.PlanGeneratedPayload); ok {
			if r.quiet || r.noPlan {
				return
			}
			r.pauseSpinnerLocked()
			fmt.Fprintln(r.w, "\nPlan:")
			for _, item := range payload.Plan {
				fmt.Fprintf(r.w, "- %s\n", item)
			}
			r.resumeSpinnerLocked()
		}
	case events.ToolCallStarted:
		if payload, ok := event.Payload.(events.ToolCallStartedPayload); ok {
			if r.quiet || !r.showTools || !r.verbose {
				return
			}
			r.pauseSpinnerLocked()
			fmt.Fprintf(r.w, "tool: %s start\n", payload.ToolName)
			fmt.Fprintf(r.w, "input: %v\n", payload.Input)
			r.resumeSpinnerLocked()
		}
	case events.ToolCallFinished, events.ToolCallFailed:
		if payload, ok := event.Payload.(events.ToolCallFinishedPayload); ok {
			if r.quiet || !r.showTools {
				return
			}
			r.pauseSpinnerLocked()
			status := payload.Status
			if status == "success" {
				status = "ok"
			} else if status == "error" {
				status = "err"
			}
			trunc := ""
			if payload.Truncated {
				trunc = ", truncated"
			}
			fmt.Fprintf(r.w, "tool: %s %s (%dms, %d lines, %d bytes%s)\n", payload.ToolName, status, payload.DurationMs, payload.LineCount, payload.ByteCount, trunc)
			if r.verbose && payload.Preview != "" {
				fmt.Fprintln(r.w, "preview:")
				for _, line := range strings.Split(payload.Preview, "\n") {
					fmt.Fprintf(r.w, "  %s\n", line)
				}
			}
			r.resumeSpinnerLocked()
		}
	case events.ModelDelta:
		if payload, ok := event.Payload.(events.ModelDeltaPayload); ok {
			r.stopSpinnerLocked()
			r.spinnerDone = true
			if !r.printedFinalHeader {
				fmt.Fprint(r.w, "fi: ")
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
				r.stopSpinnerLocked()
				r.spinnerDone = true
				return
			}
			r.stopSpinnerLocked()
			r.spinnerDone = true
			if !r.printedFinalHeader {
				fmt.Fprint(r.w, "fi: ")
				r.printedFinalHeader = true
			}
			fmt.Fprintln(r.w, payload.Answer)
		}
	case events.RunError:
		if payload, ok := event.Payload.(events.RunErrorPayload); ok {
			r.stopSpinnerLocked()
			r.spinnerDone = true
			fmt.Fprintf(r.w, "\nError: %s\n", payload.Message)
		}
	case events.RunFinished:
		r.stopSpinnerLocked()
		r.spinnerDone = true
	}
}

func (r *StdoutRenderer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopSpinnerLocked()
	r.spinnerDone = true
	return nil
}

func (r *StdoutRenderer) startSpinnerLocked() {
	if !r.spinnerEnabled || r.spinnerDone || r.spinnerActive || len(r.spinnerFrames) == 0 {
		return
	}
	r.spinnerActive = true
	r.spinnerFrameIdx = 0
	r.spinnerStop = make(chan struct{})
	go r.runSpinner(r.spinnerStop)
}

func (r *StdoutRenderer) pauseSpinnerLocked() {
	if !r.spinnerActive {
		return
	}
	r.spinnerActive = false
	if r.spinnerStop != nil {
		close(r.spinnerStop)
		r.spinnerStop = nil
	}
	if r.spinnerDrawn {
		fmt.Fprint(r.spinnerWriter, "\r\x1b[K")
		r.spinnerDrawn = false
	}
}

func (r *StdoutRenderer) stopSpinnerLocked() {
	r.pauseSpinnerLocked()
}

func (r *StdoutRenderer) resumeSpinnerLocked() {
	r.startSpinnerLocked()
}

func (r *StdoutRenderer) runSpinner(stop <-chan struct{}) {
	r.drawSpinnerFrame()
	ticker := time.NewTicker(r.spinnerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			r.drawSpinnerFrame()
		}
	}
}

func (r *StdoutRenderer) drawSpinnerFrame() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.spinnerActive || len(r.spinnerFrames) == 0 {
		return
	}
	frame := r.spinnerFrames[r.spinnerFrameIdx%len(r.spinnerFrames)]
	r.spinnerFrameIdx++
	fmt.Fprintf(r.spinnerWriter, "\rthinking %s", frame)
	r.spinnerDrawn = true
}
