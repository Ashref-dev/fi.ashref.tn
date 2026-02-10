package render

import "fi-cli/internal/events"

// Renderer emits events to an output target.
type Renderer interface {
	Emit(events.Event)
	Close() error
}
