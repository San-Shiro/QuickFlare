package supervisor

import (
	"fmt"
	"testing"
)

func TestLogRingCapacityAndOrder(t *testing.T) {
	ring := NewLogRing(3)
	if len(ring.Entries()) != 0 {
		t.Fatalf("expected empty ring, got %d", len(ring.Entries()))
	}

	ring.Add("line 1")
	ring.Add("line 2")
	got := ring.Entries()
	if len(got) != 2 || got[0] != "line 1" || got[1] != "line 2" {
		t.Fatalf("unexpected entries before full: %v", got)
	}

	ring.Add("line 3")
	got = ring.Entries()
	if len(got) != 3 || got[0] != "line 1" || got[1] != "line 2" || got[2] != "line 3" {
		t.Fatalf("unexpected entries at capacity: %v", got)
	}

	ring.Add("line 4") // should drop "line 1"
	got = ring.Entries()
	if len(got) != 3 || got[0] != "line 2" || got[1] != "line 3" || got[2] != "line 4" {
		t.Fatalf("unexpected entries after 1 overflow: %v", got)
	}

	ring.Add("line 5") // should drop "line 2"
	got = ring.Entries()
	if len(got) != 3 || got[0] != "line 3" || got[1] != "line 4" || got[2] != "line 5" {
		t.Fatalf("unexpected entries after 2 overflows: %v", got)
	}

	ring.Clear()
	if len(ring.Entries()) != 0 {
		t.Fatalf("expected empty entries after clear, got %d", len(ring.Entries()))
	}
}

func TestLogRingConcurrency(t *testing.T) {
	ring := NewLogRing(100)
	done := make(chan bool)

	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				ring.Add(fmt.Sprintf("g%d: %d", id, j))
				_ = ring.Entries()
			}
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	entries := ring.Entries()
	if len(entries) != 100 {
		t.Fatalf("expected 100 entries, got %d", len(entries))
	}
}
