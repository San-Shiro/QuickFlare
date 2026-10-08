package supervisor

import "sync"

const defaultLogRingCapacity = 500

// LogRing is a thread-safe fixed-capacity circular buffer storing recent log lines.
type LogRing struct {
	mu       sync.RWMutex
	capacity int
	entries  []string
	start    int
}

// NewLogRing creates a LogRing with the specified capacity.
func NewLogRing(capacity int) *LogRing {
	if capacity <= 0 {
		capacity = defaultLogRingCapacity
	}
	return &LogRing{
		capacity: capacity,
		entries:  make([]string, 0, capacity),
	}
}

// Add appends a line to the ring buffer.
func (r *LogRing) Add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.entries) < r.capacity {
		r.entries = append(r.entries, line)
		return
	}
	r.entries[r.start] = line
	r.start = (r.start + 1) % r.capacity
}

// Entries returns an ordered snapshot of lines from oldest to newest.
func (r *LogRing) Entries() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n := len(r.entries)
	out := make([]string, n)
	if n < r.capacity {
		copy(out, r.entries)
		return out
	}
	copy(out, r.entries[r.start:])
	copy(out[n-r.start:], r.entries[:r.start])
	return out
}

// Clear empties the ring buffer.
func (r *LogRing) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = r.entries[:0]
	r.start = 0
}
