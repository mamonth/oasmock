package history

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Creating a new ring buffer with various capacities
Given capacity values (zero, positive)
When a ring buffer is created
Then it should not be nil and report correct capacity and zero count

Related spec scenarios: RS.MSC.31, RS.MSC.32
*/
func TestNewRingBuffer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		capacity int
		wantCap  int
	}{
		{"zero capacity", 0, 0},
		{"capacity 1", 1, 1},
		{"capacity 10", 10, 10},
		{"capacity 100", 100, 100},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rb := NewRingBuffer(tt.capacity)
			require.NotNil(t, rb, "NewRingBuffer should not return nil")
			assert.Equal(t, tt.wantCap, rb.Capacity(), "Capacity() should match expected")
			assert.Zero(t, rb.Count(), "Count() should be zero for new buffer")
		})
	}
}

/*
Scenario: Adding records to ring buffer and retrieving all
Given a ring buffer with capacity 3
When records are added up to and beyond capacity
Then GetAll returns correct records, oldest records are overwritten on overflow

Related spec scenarios: RS.MSC.31, RS.MSC.32
*/
func TestRingBufferAddAndGetAll(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(3)

	// Add first record
	record1 := RequestRecord{
		ID:        "1",
		Timestamp: time.Now(),
		Method:    "GET",
		Path:      "/test",
	}
	rb.Add(record1)

	records := rb.GetAll()
	require.Len(t, records, 1, "GetAll() should return 1 record")
	assert.Equal(t, "1", records[0].ID, "First record ID should be '1'")

	// Add second record
	record2 := RequestRecord{
		ID:        "2",
		Timestamp: time.Now(),
		Method:    "POST",
		Path:      "/test",
	}
	rb.Add(record2)

	records = rb.GetAll()
	require.Len(t, records, 2, "GetAll() should return 2 records")
	assert.Equal(t, "1", records[0].ID, "First record ID should be '1'")
	assert.Equal(t, "2", records[1].ID, "Second record ID should be '2'")

	// Fill buffer
	record3 := RequestRecord{ID: "3"}
	rb.Add(record3)

	records = rb.GetAll()
	require.Len(t, records, 3, "GetAll() should return 3 records")

	// Overflow buffer - should overwrite oldest
	record4 := RequestRecord{ID: "4"}
	rb.Add(record4)

	records = rb.GetAll()
	require.Len(t, records, 3, "GetAll() should return 3 records after overflow")
	// Should now contain records 2, 3, 4 (1 overwritten)
	assert.Equal(t, "2", records[0].ID, "After overflow, first record ID should be '2'")
	assert.Equal(t, "3", records[1].ID, "After overflow, second record ID should be '3'")
	assert.Equal(t, "4", records[2].ID, "After overflow, third record ID should be '4'")
}

/*
Scenario: Clearing all records from ring buffer
Given a ring buffer with several records
When Clear is called
Then count becomes zero and GetAll returns empty slice

Related spec scenarios: RS.MSC.31
*/
func TestRingBufferClear(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(5)

	// Add some records
	for i := 0; i < 3; i++ {
		rb.Add(RequestRecord{ID: string(rune('0' + i))})
	}

	assert.Equal(t, 3, rb.Count(), "Count() should be 3 after adding records")

	rb.Clear()

	assert.Zero(t, rb.Count(), "Count() should be zero after Clear")

	records := rb.GetAll()
	assert.Empty(t, records, "GetAll() after Clear should return empty slice")
}

/*
Scenario: Zero‑capacity ring buffer behavior
Given a ring buffer with zero capacity
When records are added, retrieved, or cleared
Then operations are no‑ops, count stays zero, GetAll returns nil

Related spec scenarios: RS.MSC.32
*/
func TestRingBufferZeroCapacity(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(0)

	// Adding to zero-capacity buffer should do nothing
	rb.Add(RequestRecord{ID: "test"})
	assert.Zero(t, rb.Count(), "Count() for zero-capacity buffer should be zero")
	assert.Zero(t, rb.Capacity(), "Capacity() should be zero")
	records := rb.GetAll()
	assert.Nil(t, records, "GetAll() for zero-capacity buffer should return nil")

	// Clear should not panic
	rb.Clear()
}

/*
Scenario: Concurrent access to ring buffer
Given a ring buffer with capacity 100
When one goroutine adds records while another reads concurrently
Then the buffer stays within capacity and no data races occur

Related spec scenarios: RS.MSC.31
*/
func TestRingBufferConcurrentAccess(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100)
	done := make(chan bool)

	// Start goroutine that adds records
	go func() {
		for i := 0; i < 1000; i++ {
			rb.Add(RequestRecord{ID: string(rune('A' + (i % 26)))})
		}
		done <- true
	}()

	// Concurrently read
	for i := 0; i < 100; i++ {
		records := rb.GetAll()
		_ = records
		time.Sleep(time.Microsecond)
	}

	<-done

	// Should have at most 100 records (capacity)
	assert.LessOrEqual(t, rb.Count(), 100, "Count() should be <= capacity")
}

/*
Scenario: Request record with response data
Given a request record containing a response
When fields are inspected
Then they contain expected values (ID, status code, duration, etc.)

Related spec scenarios: RS.MSC.31
*/
func TestRequestRecordWithResponse(t *testing.T) {
	t.Parallel()

	req := RequestRecord{
		ID:        "req1",
		Timestamp: time.Now(),
		Method:    "GET",
		Path:      "/users",
		Headers:   http.Header{"Content-Type": []string{"application/json"}},
		Body:      []byte(`{"test": true}`),
		Response: &ResponseRecord{
			StatusCode: 200,
			Headers:    http.Header{"X-Custom": []string{"value"}},
			Body:       []byte(`{"ok": true}`),
			Duration:   100 * time.Millisecond,
		},
	}

	// Basic sanity check
	assert.Equal(t, "req1", req.ID, "Request ID should be 'req1'")
	assert.Equal(t, 200, req.Response.StatusCode, "Response status code should be 200")
	assert.Equal(t, 100*time.Millisecond, req.Response.Duration, "Response duration should be 100ms")
}
