package systemops

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSSEEmitter_WritesValidSSE(t *testing.T) {
	rec := httptest.NewRecorder()
	emitter, err := NewSSEEmitter(rec)
	if err != nil {
		t.Fatalf("NewSSEEmitter: %v", err)
	}

	emitter.Emit(ProgressEvent{
		Type:      EventStepStarted,
		PlanName:  "test-plan",
		StepID:    "s1",
		StepTitle: "Step 1",
		Risk:      RiskLevelLow,
		Timestamp: time.Now().UTC(),
	})

	body := rec.Body.String()
	if !strings.Contains(body, "event: step_started") {
		t.Errorf("missing event type line, body=%q", body)
	}
	if !strings.Contains(body, "data: {") {
		t.Errorf("missing data line, body=%q", body)
	}
	if !strings.Contains(body, `"step_id":"s1"`) {
		t.Errorf("payload missing step_id, body=%q", body)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestSSEEmitter_CloseStopsEmissions(t *testing.T) {
	rec := httptest.NewRecorder()
	emitter, err := NewSSEEmitter(rec)
	if err != nil {
		t.Fatalf("NewSSEEmitter: %v", err)
	}

	emitter.Emit(ProgressEvent{Type: EventPlanStarted, PlanName: "p"})
	bodyBeforeClose := rec.Body.Len()

	emitter.Close()
	emitter.Emit(ProgressEvent{Type: EventPlanCompleted, PlanName: "p"})

	if rec.Body.Len() != bodyBeforeClose {
		t.Errorf("emit after Close should be no-op")
	}
}

func TestSSEEmitter_RejectsNonFlusher(t *testing.T) {
	// Wrap a non-flusher response writer
	w := &nonFlusherWriter{header: http.Header{}}
	_, err := NewSSEEmitter(w)
	if err == nil {
		t.Fatal("expected error when writer does not support flushing")
	}
}

type nonFlusherWriter struct {
	header http.Header
}

func (n *nonFlusherWriter) Header() http.Header        { return n.header }
func (n *nonFlusherWriter) Write(p []byte) (int, error) { return len(p), nil }
func (n *nonFlusherWriter) WriteHeader(statusCode int)  {}

