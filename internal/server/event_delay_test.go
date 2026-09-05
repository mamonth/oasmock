package server

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// delayWindow is the tolerated band around a declared x-mock-delay: the
// delivery should land not before the declared delay and well within the wait
// deadline (never sooner, but also not effectively never).
type delayWindow struct {
	min time.Duration
	max time.Duration
}

func assertWithinDelayWindow(t *testing.T, elapsed time.Duration, window delayWindow) {
	t.Helper()
	assert.GreaterOrEqual(t, elapsed, window.min, "x-mock-delay must delay emission")
	assert.Less(t, elapsed, window.max, "x-mock-delay emission must not be excessively late")
}

/*
Scenario: Delayed event emission for an x-mock-delay example
Given an event-driven example declaring x-mock-delay 60
When the event fires
Then the message is emitted at least the declared delay after the fire

Related spec scenarios: RS.EXT.23
*/
func TestEventBus_DelayedEmissionDelaysDelivery(t *testing.T) {
	t.Parallel()

	var pushed atomic.Int64
	bus := newEventBus(&stubMessageRenderer{}, &stubConsumerBus{
		wsPush: func(ConsumerInfo, []byte) { pushed.Add(1) },
	}, false)
	bus.setObserver(func(env manageEnvelope) {
		if env.Type == "push" {
			pushed.Add(1)
		}
	})

	spec := loaderExampleSpecForTest(map[string]any{"ring": "{$event.tag}"}, map[string]any{
		"x-mock-match": map[string]any{"{$event.name}": "orderCreated"},
		"x-mock-delay": float64(60),
	})
	trigger, _, err := bus.registerRuntimeExample("ex-1", "/alerts", "", spec)
	require.NoError(t, err)
	assert.Equal(t, extensions.TriggerEvent, trigger)

	start := time.Now()
	bus.fire("orderCreated", map[string]any{"tag": "hi"}, "", true, nil)
	elapsed, delivered := waitForPush(t, &pushed, start)
	require.True(t, delivered, "expected a delayed delivery")
	assertWithinDelayWindow(t, elapsed, delayWindow{
		min: 30 * time.Millisecond,
		max: 2 * time.Second,
	})
}

/*
Scenario: Shutdown cancels pending delayed emissions
Given an event-driven example with a large x-mock-delay
When the event fires and the bus shuts down before the delay elapses
Then no delivery occurs after shutdown

Related spec scenarios: RS.AMG.30
*/
func TestEventBus_ShutdownCancelsDelayedEmission(t *testing.T) {
	t.Parallel()

	var pushed atomic.Int64
	bus := newEventBus(&stubMessageRenderer{}, &stubConsumerBus{
		wsPush: func(ConsumerInfo, []byte) { pushed.Add(1) },
	}, false)

	spec := loaderExampleSpecForTest(map[string]any{"ring": "{$event.tag}"}, map[string]any{
		"x-mock-match": map[string]any{"{$event.name}": "orderCreated"},
		"x-mock-delay": float64(500),
	})
	_, _, err := bus.registerRuntimeExample("ex-1", "/alerts", "", spec)
	require.NoError(t, err)

	bus.fire("orderCreated", map[string]any{"tag": "hi"}, "", true, nil)
	// Shut down long before the 500ms delay elapses.
	bus.shutdown()

	time.Sleep(700 * time.Millisecond)
	assert.Zero(t, pushed.Load(), "no delivery may occur after shutdown")
}

/*
Scenario: Delayed connect built-in emission
Given a connect example declaring x-mock-delay 60 on an event-driven match
When the connect built-in fires for a recipient
Then the message is delivered to the recipient at least the delay after the fire

Related spec scenarios: RS.EVT.24
*/
func TestEventBus_ConnectBuiltInDelayed(t *testing.T) {
	t.Parallel()

	var pushed atomic.Int64
	bus := newEventBus(&stubMessageRenderer{}, &stubConsumerBus{
		wsPush: func(ConsumerInfo, []byte) { pushed.Add(1) },
	}, false)

	spec := loaderExampleSpecForTest(map[string]any{"msg": "welcome"}, map[string]any{
		"x-mock-match": map[string]any{"{$event.name}": "connect"},
		"x-mock-delay": float64(60),
	})
	trigger, _, err := bus.registerRuntimeExample("ex-c", "/alerts", "", spec)
	require.NoError(t, err)
	assert.Equal(t, extensions.TriggerEvent, trigger)

	start := time.Now()
	bus.fireTargeted("connect", map[string]any{"connectionId": "c1"}, "", ConsumerInfo{
		ConnectionID: "c1",
		Channel:      "/alerts",
	})
	elapsed, delivered := waitForPush(t, &pushed, start)
	require.True(t, delivered, "expected a delayed delivery")
	assertWithinDelayWindow(t, elapsed, delayWindow{
		min: 30 * time.Millisecond,
		max: 2 * time.Second,
	})
}
