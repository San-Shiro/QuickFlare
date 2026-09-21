package ui

import (
	"strings"
	"testing"
)

// The status bar is one line tall, so the summary has to stay short - and a
// route vanishing on its own is not success.
func TestReconcileSummaryIsShortAndFlagsRemovals(t *testing.T) {
	got, tone := reconcileSummary(2, []string{"gone.example.com"}, nil)
	if !strings.Contains(got, "1 route removed") {
		t.Errorf("removals should be reported, got %q", got)
	}
	if tone == colSuccess {
		t.Error("a route disappearing on its own must not read as success")
	}
	if len(got) > 40 {
		t.Errorf("summary too long for the status strip (%d chars): %q", len(got), got)
	}

	if msg, _ := reconcileSummary(0, nil, nil); msg != "" {
		t.Errorf("nothing to report should say nothing, got %q", msg)
	}
}
