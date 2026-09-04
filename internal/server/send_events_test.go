package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Parsing x-send-events named-event subscriptions
Given an AsyncAPI message example with x-send-events entries
When parseSendEvents is called
Then each named subscription is parsed with its wait

Related spec scenarios: RS.EVT.7, RS.EVT.9, RS.EVT.10
*/
func TestParseSendEvents_Named(t *testing.T) {
	t.Parallel()

	ext := map[string]any{
		"x-send-events": []any{
			map[string]any{"on": "orderCreated", "wait": 50},
			map[string]any{"on": "connect"},
		},
	}

	events, err := parseSendEvents(ext)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "orderCreated", events[0].On)
	assert.Equal(t, 50, events[0].Wait)
	assert.Equal(t, "connect", events[1].On)
	assert.Equal(t, 0, events[1].Wait)
}

/*
Scenario: Parsing x-send-events built-in receive subscription
Given a message example with a flat receive entry
When parseSendEvents is called
Then a receive subscription is parsed

Related spec scenarios: RS.EVT.11
*/
func TestParseSendEvents_FlatReceive(t *testing.T) {
	t.Parallel()

	ext := map[string]any{
		"x-send-events": []any{"receive"},
	}

	events, err := parseSendEvents(ext)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "receive", events[0].On)
}

/*
Scenario: Parsing x-send-events cron built-in
Given a message example with an object cron entry
When parseSendEvents is called
Then the cron subscription carries its wait interval

Related spec scenarios: RS.EVT.10
*/
func TestParseSendEvents_Cron(t *testing.T) {
	t.Parallel()

	ext := map[string]any{
		"x-send-events": []any{
			map[string]any{"on": "cron", "wait": 1000},
		},
	}

	events, err := parseSendEvents(ext)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "cron", events[0].On)
	assert.Equal(t, 1000, events[0].Wait)
}

/*
Scenario: Handling an invalid x-send-events entry
Given a message example with a malformed x-send-events entry
When parseSendEvents is called
Then it returns an error

Related spec scenarios: RS.EVT.7
*/
func TestParseSendEvents_Invalid(t *testing.T) {
	t.Parallel()

	ext := map[string]any{
		"x-send-events": []any{42},
	}
	_, err := parseSendEvents(ext)
	require.Error(t, err)
}
