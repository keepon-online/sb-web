package systemops

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// SSEEmitter writes ProgressEvent into a Server-Sent Events stream.
//
// It is safe for concurrent emit calls. The HTTP handler must keep the
// response writer alive for the duration of plan execution.
type SSEEmitter struct {
	mu      sync.Mutex
	writer  http.ResponseWriter
	flusher http.Flusher
	closed  bool
}

// NewSSEEmitter prepares the response for SSE streaming.
//
// The caller is responsible for invoking this on a goroutine that owns the
// response writer until plan execution completes.
func NewSSEEmitter(w http.ResponseWriter) (*SSEEmitter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("response writer does not support flushing")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	return &SSEEmitter{writer: w, flusher: flusher}, nil
}

// Emit serializes the event as JSON and writes it as an SSE message.
// The event type is used as the SSE event name so client code can subscribe
// to specific stages.
func (s *SSEEmitter) Emit(event ProgressEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	fmt.Fprintf(s.writer, "event: %s\n", event.Type)
	fmt.Fprintf(s.writer, "data: %s\n\n", payload)
	s.flusher.Flush()
}

// Close marks the emitter as closed; subsequent Emit calls become no-ops.
func (s *SSEEmitter) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}
