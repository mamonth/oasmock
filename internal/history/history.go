package history

import (
	"net/http"
	"sync"
	"time"
)

// RequestRecord captures details of an HTTP request served by the mock.
type RequestRecord struct {
	ID        string          `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	Method    string          `json:"method"`
	Path      string          `json:"path"`
	Query     string          `json:"query,omitempty"`
	Headers   http.Header     `json:"headers"`
	Body      []byte          `json:"body,omitempty"`
	Response  *ResponseRecord `json:"response,omitempty"`
}

// ResponseRecord captures details of the HTTP response.
type ResponseRecord struct {
	StatusCode int           `json:"statusCode"`
	Headers    http.Header   `json:"headers"`
	Body       []byte        `json:"body,omitempty"`
	Duration   time.Duration `json:"duration"`
}

// RingBuffer is a fixed‑size circular buffer for request records.
type RingBuffer struct {
	mu      sync.RWMutex
	records []RequestRecord
	head    int // index of the oldest record
	tail    int // index where next record will be inserted
	size    int
	count   int
}

// NewRingBuffer creates a new ring buffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		records: make([]RequestRecord, capacity),
		size:    capacity,
	}
}

// Add adds a request record to the buffer, overwriting the oldest if full.
func (b *RingBuffer) Add(r RequestRecord) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.size == 0 {
		return
	}
	b.records[b.tail] = r
	b.tail = (b.tail + 1) % b.size
	if b.count < b.size {
		b.count++
	} else {
		// buffer full, move head forward
		b.head = (b.head + 1) % b.size
	}
}

// GetAll returns all records in chronological order (oldest first).
func (b *RingBuffer) GetAll() []RequestRecord {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.count == 0 {
		return nil
	}
	result := make([]RequestRecord, b.count)
	if b.head < b.tail {
		copy(result, b.records[b.head:b.tail])
	} else {
		n := copy(result, b.records[b.head:])
		copy(result[n:], b.records[:b.tail])
	}
	return result
}

// Clear removes all records.
func (b *RingBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.head = 0
	b.tail = 0
	b.count = 0
}

// Capacity returns the buffer capacity.
func (b *RingBuffer) Capacity() int {
	return b.size
}

// Count returns the current number of records.
func (b *RingBuffer) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}
