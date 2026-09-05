package server

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: The event bus observer emits an event envelope on fire
Given an eventBus with an observer and a text-only deliverer
When fire is called
Then the observer receives an event envelope

Related spec scenarios: RS.AMG.24
*/
func TestEventBusObserver_EmitsEventOnFire(t *testing.T) {
	t.Parallel()

	var seen atomic.Value
	bus := newEventBus(nil, nil, false)
	bus.setObserver(func(env manageEnvelope) { seen.Store(env) })

	bus.fire("orderCreated", map[string]any{"id": "1"}, "/v1", true, nil)

	env, ok := seen.Load().(manageEnvelope)
	require.True(t, ok)
	assert.Equal(t, "event", env.Type)
	require.NotNil(t, env.Event)
	assert.Equal(t, "orderCreated", env.Event.Name)
	assert.Equal(t, "/v1", env.Event.Schema)
	assert.True(t, env.Event.Global)
}

/*
Scenario: The event bus observer emits a push envelope on a broadcast delivery
Given an eventBus with an observer, a registered event example and a consumer bus
When the event fires and the example delivers
Then the observer receives a push envelope for the channel

Related spec scenarios: RS.AMG.25
*/
func TestEventBusObserver_EmitsPushOnDeliver(t *testing.T) {
	t.Parallel()

	var pushed atomic.Bool
	bus := newEventBus(&stubMessageRenderer{}, &stubConsumerBus{
		candidates: []ConsumerInfo{{ConnectionID: "c1", Channel: "/alerts"}},
		wsPush: func(consumer ConsumerInfo, payload []byte) {
			pushed.Store(true)
		},
	}, false)
	bus.setObserver(func(env manageEnvelope) {
		if env.Type == "push" {
			pushed.Store(true)
		}
	})

	spec := loaderExampleSpecForTest(map[string]any{
		"level": "info",
		"msg":   "hi",
	}, map[string]any{
		"x-mock-match": map[string]any{
			"{$event.name}": "orderCreated",
		},
	})
	trigger, _, err := bus.registerRuntimeExample("ex-1", "/alerts", "/v1", spec)
	require.NoError(t, err)
	assert.Equal(t, extensions.TriggerEvent, trigger)

	bus.fire("orderCreated", map[string]any{"id": "1"}, "/v1", true, nil)
	assert.True(t, pushed.Load())
}

/*
Scenario: The observer emits a push envelope on a periodic delivery
Given an interval-driven example registered in the event bus
When its job delivers at a tick
Then the observer receives a push envelope for the channel

Related spec scenarios: RS.AMG.25, RS.AMG.27
*/
func TestEventBusObserver_EmitsPushOnPeriodicDelivery(t *testing.T) {
	t.Parallel()

	var pushEnvelopes atomic.Int64
	var delivered atomic.Int64
	bus := newEventBus(&stubMessageRenderer{}, &stubConsumerBus{
		wsPush: func(ConsumerInfo, []byte) { delivered.Add(1) },
	}, false)
	bus.setObserver(func(env manageEnvelope) {
		if env.Type == "push" {
			pushEnvelopes.Add(1)
		}
	})

	spec := loaderExampleSpecForTest(map[string]any{"tick": true}, map[string]any{
		"x-mock-interval": 25,
	})
	trigger, _, err := bus.registerRuntimeExample("ex-p", "/alerts", "", spec)
	require.NoError(t, err)
	assert.Equal(t, extensions.TriggerPeriodic, trigger)

	deadline := time.Now().Add(2 * time.Second)
	for pushEnvelopes.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	time.Sleep(5 * time.Millisecond)
	assert.Greater(t, delivered.Load(), int64(0))
	assert.Greater(t, pushEnvelopes.Load(), int64(0), "periodic deliveries must emit a push envelope")
}

/*
Scenario: The observer emits an event envelope for a built-in targeted fire
Given a connect example registered in the event bus
When the connect built-in fires for a recipient
Then the observer receives an event envelope naming the built-in

Related spec scenarios: RS.AMG.24
*/
func TestEventBusObserver_EmitsEventOnFireTargeted(t *testing.T) {
	t.Parallel()

	var seen atomic.Value
	bus := newEventBus(&stubMessageRenderer{}, &stubConsumerBus{
		wsPush: func(ConsumerInfo, []byte) {},
	}, false)
	bus.setObserver(func(env manageEnvelope) {
		if env.Type == "event" {
			seen.Store(env)
		}
	})

	spec := loaderExampleSpecForTest(map[string]any{"msg": "welcome"}, map[string]any{
		"x-mock-match": map[string]any{"{$event.name}": "connect"},
	})
	trigger, _, err := bus.registerRuntimeExample("ex-c", "/alerts", "", spec)
	require.NoError(t, err)
	assert.Equal(t, extensions.TriggerEvent, trigger)

	bus.fireTargeted("connect", map[string]any{"connectionId": "c1"}, "", ConsumerInfo{
		ConnectionID: "c1",
		Channel:      "/alerts",
	})

	env, ok := seen.Load().(manageEnvelope)
	require.True(t, ok)
	assert.Equal(t, "event", env.Type)
	require.NotNil(t, env.Event)
	assert.Equal(t, "connect", env.Event.Name)
}
