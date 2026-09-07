package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

/*
Scenario: Assigning unique request ids under concurrent load
Given a server with request-history middleware
When many requests are recorded in parallel
Then every record carries a distinct id

Related spec scenarios: RS.MAPI.8
*/
func TestConcurrentRequestsGetUniqueIDs(t *testing.T) {
	srv, err := New(Config{HistorySize: 256}, nil)
	require.NoError(t, err)
	defer func() { _ = srv.Shutdown(context.Background()) }()

	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := srv.requestHistoryMiddleware(stub)

	const n = 64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/req/%d", i), nil)
			handler.ServeHTTP(rec, req)
		}(i)
	}
	wg.Wait()

	records := srv.historyStore.GetAll()
	require.Len(t, records, n)
	seen := make(map[string]bool)
	for _, rec := range records {
		require.NotEmpty(t, rec.ID)
		require.False(t, seen[rec.ID], "duplicate request id %q", rec.ID)
		seen[rec.ID] = true
	}
}
