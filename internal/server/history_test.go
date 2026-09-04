package server

import (
	"testing"

	"github.com/mamonth/oasmock/internal/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Recording an AsyncAPI exchange in request history
Given a server with a real history store
When recordAsyncExchange is called with an address, payload and response
Then a RequestRecord with the async method marker is added to the history store

Related spec scenarios: RS.ATM.15
*/
func TestServer_RecordAsyncExchange(t *testing.T) {
	t.Parallel()

	srv, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})

	// Use a real ring buffer behind the mock to observe the Add.
	realStore := newHistoryRingBufferStore(history.NewRingBuffer(32))
	srv.historyStore = realStore
	srv.engine.historyStore = realStore

	srv.recordAsyncExchange(InboundMessage{
		Payload:      []byte(`{"id":"1"}`),
		Headers:      map[string]string{"content-type": "application/json"},
		ConnectionID: "conn-1",
	}, "/socket", 200, []byte(`{"id":1}`))

	records := realStore.GetAll()
	require.Len(t, records, 1)
	require.NotNil(t, records[0].Response)
	assert.Equal(t, "async", records[0].Method)
	assert.Contains(t, records[0].Path, "/socket")
	assert.Equal(t, `{"id":"1"}`, string(records[0].Body))
	assert.Equal(t, 200, records[0].Response.StatusCode)
}

/*
Scenario: Recording an AsyncAPI exchange without a response body
Given a server with a real history store
When recordAsyncExchange is called without a response payload
Then the record is still added with the async method marker

Related spec scenarios: RS.ATM.15
*/
func TestServer_RecordAsyncExchange_NoResponse(t *testing.T) {
	t.Parallel()

	srv, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	realStore := newHistoryRingBufferStore(history.NewRingBuffer(32))
	srv.historyStore = realStore
	srv.engine.historyStore = realStore

	srv.recordAsyncExchange(InboundMessage{Payload: []byte("{}")}, "/alerts", 200, nil)

	records := realStore.GetAll()
	require.Len(t, records, 1)
	require.NotNil(t, records[0].Response)
	assert.Equal(t, "async", records[0].Method)
	assert.Nil(t, records[0].Response.Body)
}
