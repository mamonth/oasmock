package extensions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Classifying an example driven by an event match
Given a message example whose x-mock-match references {$event.*}
When ClassifyTrigger is called
Then the example is classified as event-driven (TriggerEvent)

Related spec scenarios: RS.EXT.20, RS.EXT.28
*/
func TestClassifyTrigger_EventDriven(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{"level": "warn"}, nil, map[string]any{
		"x-mock-match": map[string]any{
			"{$event.name}": "orderCreated",
		},
	})

	trig, err := ClassifyTrigger(ev)
	require.NoError(t, err)
	assert.Equal(t, TriggerEvent, trig.Kind)
	assert.Equal(t, "orderCreated", trig.Identity)
}

/*
Scenario: Classifying an example driven by the interval timing extension
Given a message example declaring x-mock-interval without an event match
When ClassifyTrigger is called
Then the example is classified as periodically driven (TriggerPeriodic)

Related spec scenarios: RS.EXT.22
*/
func TestClassifyTrigger_Periodic(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{"tick": true}, nil, map[string]any{
		"x-mock-interval": float64(1000),
	})

	trig, err := ClassifyTrigger(ev)
	require.NoError(t, err)
	assert.Equal(t, TriggerPeriodic, trig.Kind)
	assert.Equal(t, 1000, trig.Interval)
}

/*
Scenario: Classifying a plain reply example
Given a message example with no event match and no interval
When ClassifyTrigger is called
Then the example is classified as a reply (TriggerReply)

Related spec scenarios: RS.EXT.20
*/
func TestClassifyTrigger_Reply(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{"level": "info"}, nil, map[string]any{
		"x-mock-match": map[string]any{
			"{$request.query.kind}": "alerts",
		},
	})

	trig, err := ClassifyTrigger(ev)
	require.NoError(t, err)
	assert.Equal(t, TriggerReply, trig.Kind)
}

/*
Scenario: Rejecting an example mixing event and reply match contexts
Given a message example whose x-mock-match references both {$event.*} and
{$message.*}
When ClassifyTrigger is called
Then it returns a clear load error

Related spec scenarios: RS.EXT.20
*/
func TestClassifyTrigger_MixedContextRejected(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{}, nil, map[string]any{
		"x-mock-match": map[string]any{
			"{$event.name}":           "orderCreated",
			"{$message.payload.kind}": "order",
		},
	})

	_, err := ClassifyTrigger(ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event")
}

/*
Scenario: Rejecting an example declaring both interval and an event match
Given a message example declaring both x-mock-interval and an event-based match
When ClassifyTrigger is called
Then it returns a clear load error

Related spec scenarios: RS.EXT.28
*/
func TestClassifyTrigger_DualTriggerRejected(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{}, nil, map[string]any{
		"x-mock-interval": float64(500),
		"x-mock-match": map[string]any{
			"{$event.name}": "orderCreated",
		},
	})

	_, err := ClassifyTrigger(ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interval")
}

/*
Scenario: Rejecting an example declaring both interval and a non-event match
Given a message example declaring x-mock-interval and an x-mock-match that is
not event-driven (a reply/connection/literal match)
When ClassifyTrigger is called
Then it returns a clear load error: a periodically driven example has exactly
one trigger and cannot carry a match, which would otherwise be silently dropped

Related spec scenarios: RS.EXT.22, RS.EXT.28
*/
func TestClassifyTrigger_PeriodicWithMatchRejected(t *testing.T) {
	t.Parallel()

	for _, match := range []map[string]any{
		{"{$request.query.kind}": "alerts"},
		{"{$connection.id}": "conn-1"},
		{"fixed": "value"},
	} {
		ev := NewExampleValue(map[string]any{}, nil, map[string]any{
			"x-mock-interval": float64(500),
			"x-mock-match":    match,
		})

		_, err := ClassifyTrigger(ev)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "interval")
		assert.Contains(t, err.Error(), "match")
	}
}

/*
Scenario: Rejecting a non-literal event identity condition value
Given a message example whose {$event.name} condition value is itself a runtime
expression
When ClassifyTrigger is called
Then it returns a clear load error instead of registering a subscription keyed
by the literal expression string (which could never match a fired identity)

Related spec scenarios: RS.EXT.33
*/
func TestClassifyTrigger_NonLiteralIdentityRejected(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{}, nil, map[string]any{
		"x-mock-match": map[string]any{
			"{$event.name}": "{$state.envName}",
		},
	})

	_, err := ClassifyTrigger(ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity")
}

/*
Scenario: An event-driven match without an {$event.name} pin is a wildcard
Given a match referencing the event context only through a condition value
(no {$event.name} identity condition)
When ClassifyTrigger is called
Then the example is event-driven with an empty (wildcard) identity that
evaluates against every fired event

Related spec scenarios: RS.EXT.34
*/
func TestClassifyTrigger_EventWithoutIdentityIsWildcard(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{}, nil, map[string]any{
		"x-mock-match": map[string]any{
			"{$connection.id}": "{$event.connectionId}",
		},
	})

	trig, err := ClassifyTrigger(ev)
	require.NoError(t, err)
	assert.Equal(t, TriggerEvent, trig.Kind)
	assert.Equal(t, "", trig.Identity, "no {$event.name} condition means a wildcard identity")
}

/*
Scenario: A declared fractional x-mock-interval is a load error
Given a message example declaring x-mock-interval with a fractional millisecond
value
When ClassifyTrigger is called
Then it returns a clear load error instead of silently truncating to an integer

Related spec scenarios: RS.EXT.35
*/
func TestClassifyTrigger_FractionalIntervalRejected(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{}, nil, map[string]any{
		"x-mock-interval": float64(1.5),
	})

	_, err := ClassifyTrigger(ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "positive")
}

/*
Scenario: A declared fractional x-mock-delay is a load error
Given a message example declaring x-mock-delay with a fractional millisecond
value
When ClassifyTrigger is called
Then it returns a clear load error instead of silently ignoring the delay

Related spec scenarios: RS.EXT.36
*/
func TestClassifyTrigger_FractionalDelayRejected(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{}, nil, map[string]any{
		"x-mock-match": map[string]any{
			"{$event.name}": "connect",
		},
		"x-mock-delay": float64(1.5),
	})

	_, err := ClassifyTrigger(ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delay")
}

/*
Scenario: Delayed event-driven example carries the delay
Given an event-driven message example declaring x-mock-delay
When ClassifyTrigger is called
Then the delay is captured on the trigger

Related spec scenarios: RS.EXT.23
*/
func TestClassifyTrigger_EventWithDelay(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{}, nil, map[string]any{
		"x-mock-match": map[string]any{
			"{$event.name}": "connect",
		},
		"x-mock-delay": float64(150),
	})

	trig, err := ClassifyTrigger(ev)
	require.NoError(t, err)
	assert.Equal(t, TriggerEvent, trig.Kind)
	assert.Equal(t, 150, trig.Delay)
}

/*
Scenario: A declared but non-positive x-mock-interval is a load error
Given a message example declaring x-mock-interval with a zero or negative value
When ClassifyTrigger is called
Then it returns a clear load error instead of silently reclassifying the example
as a reply

Related spec scenarios: RS.EXT.22
*/
func TestClassifyTrigger_InvalidIntervalRejected(t *testing.T) {
	t.Parallel()

	for _, interval := range []any{0, -10} {
		ev := NewExampleValue(map[string]any{}, nil, map[string]any{
			"x-mock-interval": interval,
		})

		_, err := ClassifyTrigger(ev)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "positive")
	}
}

/*
Scenario: A declared but non-numeric x-mock-interval is a load error
Given a message example declaring x-mock-interval as a non-numeric value
When ClassifyTrigger is called
Then it returns a clear load error

Related spec scenarios: RS.EXT.22
*/
func TestClassifyTrigger_NonNumericIntervalRejected(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{}, nil, map[string]any{
		"x-mock-interval": "soon",
	})

	_, err := ClassifyTrigger(ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "positive")
}

/*
Scenario: A negative x-mock-delay is a load error
Given a message example declaring a negative x-mock-delay
When ClassifyTrigger is called
Then it returns a clear load error

Related spec scenarios: RS.EXT.23
*/
func TestClassifyTrigger_NegativeDelayRejected(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{}, nil, map[string]any{
		"x-mock-match": map[string]any{
			"{$event.name}": "connect",
		},
		"x-mock-delay": float64(-5),
	})

	_, err := ClassifyTrigger(ev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delay")
}
