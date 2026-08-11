package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dynExample builds a dynamicExample for tests with the given ttl, addedAt and response body.
func dynExample(ttl int, addedAt time.Time, body any) dynamicExample {
	return dynamicExample{
		addedAt: addedAt,
		ttl:     ttl,
		response: struct {
			code    int
			headers map[string]string
			body    any
		}{
			code: 200,
			body: body,
		},
	}
}

/*
Scenario: Selecting dynamic examples with TTL
Given a server with expired and non-expired dynamic examples
When selectDynamicExample is called
Then expired examples are skipped and the first non-expired example is returned

Related spec scenarios: RS.MSC.40, RS.MSC.41
*/
func TestSelectDynamicExampleExpiry(t *testing.T) {
	t.Parallel()

	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{})
	key := "GET /test"
	server.dynamicExamples = map[string][]dynamicExample{
		key: {
			dynExample(1, time.Now().Add(-2*time.Second), "expired"),
			dynExample(3600, time.Now(), "alive"),
		},
	}

	mapping := &RouteMapping{Method: "GET", ChiPattern: "/test"}
	ex, _ := server.selectDynamicExample(mapping, runtime.NewEvaluator())

	require.NotNil(t, ex, "a non-expired example should be selected")
	assert.Equal(t, "alive", ex.response.body, "expired example should be skipped")
}

/*
Scenario: TTL=0 means no expiration
Given a server with a dynamic example that has ttl=0 and old addedAt
When selectDynamicExample is called
Then the example is still selected

Related spec scenarios: RS.MSC.42
*/
func TestSelectDynamicExampleZeroTTLNeverExpires(t *testing.T) {
	t.Parallel()

	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{})
	key := "GET /test"
	server.dynamicExamples = map[string][]dynamicExample{
		key: {
			dynExample(0, time.Now().Add(-10*time.Hour), "no-ttl"),
		},
	}

	mapping := &RouteMapping{Method: "GET", ChiPattern: "/test"}
	ex, _ := server.selectDynamicExample(mapping, runtime.NewEvaluator())

	require.NotNil(t, ex, "example without TTL should never expire")
	assert.Equal(t, "no-ttl", ex.response.body)
}

/*
Scenario: TTL and once combined
Given a server with a one-time dynamic example that has a TTL
When selectDynamicExample is called twice
Then the example is consumed on first match and not returned again

Related spec scenarios: RS.MSC.43
*/
func TestSelectDynamicExampleOnceWithTTL(t *testing.T) {
	t.Parallel()

	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{})
	key := "GET /test"
	ex := dynExample(3600, time.Now(), "once-ttl")
	ex.once = true
	ex.onceID = "once-ttl"
	server.dynamicExamples = map[string][]dynamicExample{key: {ex}}

	mapping := &RouteMapping{Method: "GET", ChiPattern: "/test"}

	first, _ := server.selectDynamicExample(mapping, runtime.NewEvaluator())
	require.NotNil(t, first, "first match should be returned")
	assert.Equal(t, "once-ttl", first.response.body)

	second, _ := server.selectDynamicExample(mapping, runtime.NewEvaluator())
	assert.Nil(t, second, "once example should not be returned again even with valid TTL")
}

/*
Scenario: Sweeping expired examples from storage
Given a server with expired, non-expired, and no-TTL dynamic examples
When sweepExpiredExamples is called
Then only expired examples are removed, non-expired and no-TTL examples remain

Related spec scenarios: RS.MSC.44, RS.MSC.46
*/
func TestSweepExpiredExamples(t *testing.T) {
	t.Parallel()

	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{})
	server.dynamicExamples = map[string][]dynamicExample{
		"GET /a": {
			dynExample(1, time.Now().Add(-2*time.Second), "expired"),
			dynExample(3600, time.Now(), "alive"),
			dynExample(0, time.Time{}, "no-ttl"),
		},
		"GET /b": {
			dynExample(0, time.Time{}, "persistent"),
		},
	}

	server.sweepExpiredExamples()

	server.dyMu.RLock()
	defer server.dyMu.RUnlock()
	got := server.dynamicExamples

	require.Len(t, got["GET /a"], 2, "only the expired example should be removed")
	assert.Equal(t, "alive", got["GET /a"][0].response.body)
	assert.Equal(t, "no-ttl", got["GET /a"][1].response.body)
	require.Len(t, got["GET /b"], 1, "no-TTL examples should be preserved")
}

/*
Scenario: Cleaning onceExamples on sweep
Given a consumed one-time example that has expired
When sweepExpiredExamples is called
Then its onceExamples entry is removed

Related spec scenarios: RS.MSC.45
*/
func TestSweepExpiredExamplesCleansOnceExamples(t *testing.T) {
	t.Parallel()

	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{})
	key := "GET /test"
	ex := dynExample(1, time.Now().Add(-2*time.Second), "once-expired")
	ex.once = true
	ex.onceID = "once-expired"
	server.dynamicExamples = map[string][]dynamicExample{key: {ex}}
	server.onceExamples = map[string]bool{ex.onceID: true}

	server.sweepExpiredExamples()

	server.onceMu.RLock()
	_, ok := server.onceExamples[ex.onceID]
	server.onceMu.RUnlock()
	assert.False(t, ok, "onceExamples entry should be removed for swept example")
}

/*
Scenario: Consumed once example is not served again after a preceding example is swept
Given two one-time TTL examples on the same route, both already consumed
When the earlier example expires and is swept, shifting the later example's index
Then the later consumed example is still skipped by the once flag

Related spec scenarios: RS.MSC.43
*/
func TestSweepDoesNotReuseConsumedOnceExample(t *testing.T) {
	t.Parallel()

	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{})
	key := "GET /test"

	expired := dynExample(1, time.Now().Add(-2*time.Second), "expired-once")
	expired.once = true
	expired.onceID = "once-A"

	alive := dynExample(3600, time.Now(), "alive-once")
	alive.once = true
	alive.onceID = "once-B"

	server.dynamicExamples = map[string][]dynamicExample{key: {expired, alive}}

	// Both examples are consumed before the expired one is swept.
	server.markOnceUsed(expired.onceID)
	server.markOnceUsed(alive.onceID)

	// The expired example is swept, so alive shifts from index 1 to index 0.
	server.sweepExpiredExamples()

	mapping := &RouteMapping{Method: "GET", ChiPattern: "/test"}
	ex, _ := server.selectDynamicExample(mapping, runtime.NewEvaluator())
	assert.Nil(t, ex, "consumed once example must not be served again after compaction")
}

/*
Scenario: Adding an example with TTL
Given a server with a matching route mapping
When handleAddExample is called with ttl values (positive, zero, omitted)
Then the example is stored with the given ttl and addedAt set only for positive ttl

Related spec scenarios: RS.MAPI.16, RS.MAPI.18, RS.MSC.39
*/
func TestHandleAddExampleWithTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		reqBody        string
		wantTTL        int
		wantAddedAtSet bool
	}{
		{
			name:           "positive ttl",
			reqBody:        `{"path":"/test","response":{"code":200},"ttl":60}`,
			wantTTL:        60,
			wantAddedAtSet: true,
		},
		{
			name:    "zero ttl",
			reqBody: `{"path":"/test","response":{"code":200},"ttl":0}`,
			wantTTL: 0,
		},
		{
			name:    "omitted ttl",
			reqBody: `{"path":"/test","response":{"code":200}}`,
			wantTTL: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{})
			server.mappings = []RouteMapping{{
				Method:     "GET",
				Path:       "/test",
				Pattern:    "/test",
				ChiPattern: "/test",
			}}
			server.dynamicExamples = make(map[string][]dynamicExample)

			req := httptest.NewRequest("POST", "/_mock/examples", strings.NewReader(tt.reqBody))
			w := httptest.NewRecorder()

			server.handleAddExample(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "expected success response")

			key := "GET /test"
			server.dyMu.RLock()
			defer server.dyMu.RUnlock()
			require.Len(t, server.dynamicExamples[key], 1, "example should be stored")
			stored := server.dynamicExamples[key][0]
			assert.Equal(t, tt.wantTTL, stored.ttl, "ttl should be stored on example")
			if tt.wantAddedAtSet {
				assert.False(t, stored.addedAt.IsZero(), "addedAt should be set for ttl > 0")
			} else {
				assert.True(t, stored.addedAt.IsZero(), "addedAt should be zero for ttl <= 0")
			}
		})
	}
}

/*
Scenario: TTL field validation — negative value
Given a server with a matching route mapping
When handleAddExample is called with a negative ttl
Then the server responds with HTTP 400

Related spec scenarios: RS.MAPI.17
*/
func TestHandleAddExampleRejectsNegativeTTL(t *testing.T) {
	t.Parallel()

	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{})
	server.mappings = []RouteMapping{{
		Method:     "GET",
		Path:       "/test",
		Pattern:    "/test",
		ChiPattern: "/test",
	}}
	server.dynamicExamples = make(map[string][]dynamicExample)

	req := httptest.NewRequest("POST", "/_mock/examples", strings.NewReader(`{"path":"/test","response":{"code":200},"ttl":-1}`))
	w := httptest.NewRecorder()

	server.handleAddExample(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "negative ttl should be rejected")
}

/*
Scenario: Sweep starts on server startup and stops on server shutdown
Given a newly created server
When the background sweep runs
Then it removes expired examples from storage
And when the server shuts down the sweep is stopped

Related spec scenarios: RS.MSC.48, RS.MSC.49
*/
func TestTTLSweepStartsAndStops(t *testing.T) {
	t.Parallel()

	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{})
	require.NotNil(t, server.sweepCancel, "sweep should be initialized on server creation")

	// Verify the background sweep goroutine runs: add an expired example and
	// expect it to be removed by the background sweep.
	key := "GET /test"
	server.dynamicExamples = map[string][]dynamicExample{
		key: {dynExample(1, time.Now().Add(-2*time.Second), "expired")},
	}

	require.Eventually(t, func() bool {
		server.dyMu.RLock()
		defer server.dyMu.RUnlock()
		_, ok := server.dynamicExamples[key]
		return !ok
	}, 3*time.Second, 50*time.Millisecond, "sweep goroutine should remove the expired example")

	// Shutdown cancels the sweep.
	require.NoError(t, server.Shutdown(context.Background()))
	assert.Equal(t, context.Canceled, server.sweepCtx.Err(), "sweep context should be cancelled on shutdown")
}

/*
Scenario: Concurrent selection and TTL sweep must not race
Given a route populated with expired and non-expired dynamic examples
When selectDynamicExample and sweepExpiredExamples run concurrently
Then the sweep must not mutate the slice being iterated by selection
And no data race is reported by the race detector

This is a race-regression test: it MUST be run with the race detector
(`go test -race`). Before the fix, sweepExpiredExamples compacted the example
slice in place (reusing the backing array via `examples[:0]`), racing with
selectDynamicExample, which iterates the slice after copying its header under
RLock. The fresh-slice allocation in sweepExpiredExamples removes the race.

Related spec scenarios: RS.MSC.41, RS.MSC.44
*/
func TestConcurrentSelectAndSweepNoDataRace(t *testing.T) {
	server, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{})

	key := "GET /test"
	examples := make([]dynamicExample, 0, 64)
	for i := range 64 {
		if i%2 == 0 {
			examples = append(examples, dynExample(1, time.Now().Add(-2*time.Second), i))
		} else {
			examples = append(examples, dynExample(3600, time.Now(), i))
		}
	}
	server.dynamicExamples = map[string][]dynamicExample{key: examples}

	mapping := &RouteMapping{Method: "GET", ChiPattern: "/test"}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					server.selectDynamicExample(mapping, runtime.NewEvaluator())
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				server.sweepExpiredExamples()
			}
		}
	}()

	time.Sleep(2 * time.Second)
	close(stop)
	wg.Wait()
}
