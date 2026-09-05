package extensions

import (
	"testing"

	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Matching the event identity via x-mock-match
Given an x-mock-match condition on {$event.name} and a fired event
When the match is evaluated against the event context
Then it matches only when the event identity equals the condition value

Related spec scenarios: RS.EXT.18
*/
func TestEvaluateParamsMatch_EventName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		pm   ParamsMatch
		ev   string
		want bool
	}{
		{name: "identity matches", pm: ParamsMatch{"{$event.name}": "orderCreated"}, ev: "orderCreated", want: true},
		{name: "identity differs", pm: ParamsMatch{"{$event.name}": "orderCreated"}, ev: "orderShipped", want: false},
		{name: "built-in connect", pm: ParamsMatch{"{$event.name}": "connect"}, ev: "connect", want: true},
		{name: "built-in receive", pm: ParamsMatch{"{$event.name}": "receive"}, ev: "receive", want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eval := newMatchEvaluator(nil, &runtime.EventSource{Name: tt.ev, Data: map[string]any{}}, nil, nil)
			got, err := EvaluateParamsMatch(tt.pm, eval)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

/*
Scenario: Matching the event payload via x-mock-match
Given an x-mock-match condition on {$event.<field>} or a JSON-schema on
{$event.data} and a fired event payload
When the match is evaluated against the event context
Then it matches only when the payload satisfies the condition

Related spec scenarios: RS.EXT.19
*/
func TestEvaluateParamsMatch_EventPayload(t *testing.T) {
	t.Parallel()

	payload := map[string]any{"accountId": "acc-1", "amount": 100}

	tests := []struct {
		name string
		pm   ParamsMatch
		want bool
	}{
		{name: "literal field match", pm: ParamsMatch{"{$event.accountId}": "acc-1"}, want: true},
		{name: "literal field mismatch", pm: ParamsMatch{"{$event.accountId}": "acc-9"}, want: false},
		{name: "json schema on data", pm: ParamsMatch{
			"{$event.data}": map[string]any{
				"type":     "object",
				"required": []string{"accountId"},
				"properties": map[string]any{
					"accountId": map[string]any{"type": "string"},
					"amount":    map[string]any{"type": "number", "minimum": 50},
				},
			},
		}, want: true},
		{name: "json schema on data no match", pm: ParamsMatch{
			"{$event.data}": map[string]any{
				"type":     "object",
				"required": []string{"accountId"},
				"properties": map[string]any{
					"accountId": map[string]any{"type": "string"},
					"amount":    map[string]any{"type": "number", "minimum": 1000},
				},
			},
		}, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eval := newMatchEvaluator(nil, &runtime.EventSource{Name: "orderCreated", Data: payload}, nil, nil)
			got, err := EvaluateParamsMatch(tt.pm, eval)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

/*
Scenario: Connecting a connection context to event matching
Given an x-mock-match condition combining event and connection contexts
When the match is evaluated against both contexts
Then it matches only when both conditions hold

Related spec scenarios: RS.EXT.24, RS.EXT.27
*/
func TestEvaluateParamsMatch_EventConnection(t *testing.T) {
	t.Parallel()

	payload := map[string]any{"connectionId": "conn-7"}
	conn := &runtime.ConnectionSource{ID: "conn-7", Channel: "/alerts"}

	pm := ParamsMatch{
		"{$event.name}":         "orderCreated",
		"{$connection.id}":      "{$event.connectionId}",
		"{$connection.channel}": "/alerts",
	}

	eval := newMatchEvaluator(nil, &runtime.EventSource{Name: "orderCreated", Data: payload}, conn, nil)
	got, err := EvaluateParamsMatch(pm, eval)
	require.NoError(t, err)
	assert.True(t, got)
}

/*
Scenario: Failing closed when the event context is unavailable
Given an x-mock-match referencing {$event.*} evaluated without an event context
When the match is evaluated
Then it fails closed (never matches) without a hard error

Related spec scenarios: RS.EXT.29
*/
func TestEvaluateParamsMatch_EventContextUnavailable(t *testing.T) {
	t.Parallel()

	eval := runtime.NewEvaluator()
	eval.AddSource("state", &runtime.StateSource{Data: map[string]any{"k": "v"}})

	pm := ParamsMatch{"{$event.name}": "orderCreated"}
	got, err := EvaluateParamsMatch(pm, eval)
	require.NoError(t, err)
	assert.False(t, got)
}
