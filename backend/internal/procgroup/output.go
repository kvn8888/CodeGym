package procgroup

import (
	"bytes"
	"sync"
)

// CappedBuffer captures at most a fixed number of bytes from a child
// process's output and reports when more arrived. It is safe for concurrent
// writes so stdout and stderr can share one buffer. When the ceiling is
// exceeded the first time, the optional overflow callback runs once; callers
// use it to cancel the command context, which terminates the process group
// instead of letting the child keep producing unbounded output.
type CappedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	remaining int
	truncated bool
	overflow  func()
}

// NewCappedBuffer returns a buffer that keeps at most limit bytes. A
// nonpositive limit keeps nothing and reports overflow on the first write.
func NewCappedBuffer(limit int, overflow func()) *CappedBuffer {
	if limit < 0 {
		limit = 0
	}
	return &CappedBuffer{remaining: limit, overflow: overflow}
}

// Write keeps the bytes that still fit and reports every byte as consumed so
// the child's writer is not blocked. Overflow never returns an error; the
// caller detects it with Truncated.
func (b *CappedBuffer) Write(payload []byte) (int, error) {
	b.mu.Lock()
	keep := len(payload)
	if keep > b.remaining {
		keep = b.remaining
	}
	if keep > 0 {
		_, _ = b.buffer.Write(payload[:keep])
		b.remaining -= keep
	}
	overflowed := len(payload) > keep
	firstOverflow := overflowed && !b.truncated
	if firstOverflow {
		b.truncated = true
	}
	callback := b.overflow
	b.mu.Unlock()

	if firstOverflow && callback != nil {
		callback()
	}
	return len(payload), nil
}

// String returns the captured prefix. It never includes more than the limit.
func (b *CappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

// Truncated reports whether any bytes were dropped.
func (b *CappedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

// Len returns the number of captured bytes.
func (b *CappedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Len()
}
