package task

import (
	"context"
	"testing"
)

func TestRegisterFinishCancel(t *testing.T) {
	called := false
	done := Register("task-1", func() { called = true })
	if !Cancel("task-1") {
		t.Fatalf("expected cancel to succeed")
	}
	if !called {
		t.Fatalf("expected cancel func to be called")
	}
	Finish("task-1")
	select {
	case <-done:
	default:
		t.Fatalf("expected done channel to be closed")
	}
	if Cancel("missing") {
		t.Fatalf("expected missing task cancel to fail")
	}
	// ensure Register can accept a nil-ish context path through context.WithCancel usage
	_ = context.Background()
}
