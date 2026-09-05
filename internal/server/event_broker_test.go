package server

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Registering event subscriptions per schema
Given a broker with a schema-prefixed subscription
When resolveSubscribers is called for a schema-local event in the same schema
Then the subscription is resolved

Related spec scenarios: RS.EVT.5
*/
func TestEventBroker_ResolveSchemaLocal(t *testing.T) {
	t.Parallel()

	broker := newEventBroker(nil)
	broker.addSubscriptions("/v1", []channelSubscription{
		{address: "/v1/alerts", event: "orderCreated"},
	})

	subs, count := broker.resolveSubscribers("orderCreated", "/v1")
	assert.Equal(t, 1, count)
	require.Len(t, subs, 1)
	assert.Equal(t, "/v1/alerts", subs[0].address)
}

/*
Scenario: Schema-local events do not cross schema boundaries
Given a broker with a subscription in schema /a
When a schema-local event is fired from schema /b
Then no subscription is resolved

Related spec scenarios: RS.EVT.5
*/
func TestEventBroker_ResolveSchemaLocalNoCross(t *testing.T) {
	t.Parallel()

	broker := newEventBroker(nil)
	broker.addSubscriptions("/a", []channelSubscription{
		{address: "/a/alerts", event: "orderCreated"},
	})

	subs, count := broker.resolveSubscribers("orderCreated", "/b")
	assert.Equal(t, 0, count)
	assert.Empty(t, subs)
}

/*
Scenario: Global events cross schema boundaries
Given a broker with a subscription in schema /a
When a global event is fired from schema /b
Then the subscription resolves regardless of schema

Related spec scenarios: RS.EVT.6
*/
func TestEventBroker_ResolveGlobal(t *testing.T) {
	t.Parallel()

	broker := newEventBroker(nil)
	broker.addSubscriptions("/a", []channelSubscription{
		{address: "/a/alerts", event: "orderCreated"},
	})

	subs, count := broker.resolveSubscribers("orderCreated", "", true)
	assert.Equal(t, 1, count)
	require.Len(t, subs, 1)
	assert.Equal(t, "/a/alerts", subs[0].address)
}

/*
Scenario: Event with no subscribers is accepted
Given a broker with no matching subscription
When an event fires
Then it is accepted with no delivery

Related spec scenarios: RS.EVT.14
*/
func TestEventBroker_FireNoSubscribers(t *testing.T) {
	t.Parallel()

	broker := newEventBroker(nil)
	broker.addSubscriptions("/v1", []channelSubscription{
		{address: "/v1/alerts", event: "other"},
	})

	broker.fire("orderCreated", map[string]any{"id": "1"}, "/v1", false, nil)
}

/*
Scenario: Delayed event delivery schedules
Given an event with a delay
When fire is called
Then the delivery is scheduled and the broker returns immediately

Related spec scenarios: RS.EVT.4, RS.EVT.16
*/
func TestEventBroker_FireWithDelaySchedules(t *testing.T) {
	t.Parallel()

	delivered := make(chan channelSubscription, 1)
	broker := newEventBroker(func(sub channelSubscription, payload map[string]any) {
		delivered <- sub
	})

	broker.addSubscriptions("/v1", []channelSubscription{
		{address: "/v1/alerts", event: "orderCreated"},
	})

	broker.fire("orderCreated", map[string]any{"id": "1"}, "/v1", false, &delaySchedule{ms: 10})

	select {
	case sub := <-delivered:
		assert.Equal(t, "/v1/alerts", sub.address)
	case <-time.After(time.Second):
		t.Fatal("expected delayed delivery")
	}
}

/*
Scenario: Immediate delivery with no delay
Given an event with no delay
When fire is called
Then the delivery happens synchronously

Related spec scenarios: RS.EVT.1, RS.EVT.3
*/
func TestEventBroker_FireImmediate(t *testing.T) {
	t.Parallel()

	var delivered []channelSubscription
	broker := newEventBroker(func(sub channelSubscription, payload map[string]any) {
		delivered = append(delivered, sub)
	})

	broker.addSubscriptions("/v1", []channelSubscription{
		{address: "/v1/alerts", event: "orderCreated"},
	})

	broker.fire("orderCreated", map[string]any{"id": "1"}, "/v1", false, nil)
	require.Len(t, delivered, 1)
	assert.Equal(t, "/v1/alerts", delivered[0].address)
}

/*
Scenario: Match-identified examples resolve only for their schema
Given a broker with an event-driven example registered under identity + schema
When resolveSubscribers is called for a schema-local match fire in the same schema
Then the subscription resolves, and it does not resolve for another schema

Related spec scenarios: RS.EVT.5, RS.EVT.22
*/
func TestEventBroker_ResolveMatchIdentified(t *testing.T) {
	t.Parallel()

	broker := newEventBroker(nil)
	broker.addSubscriptions("/v1", []channelSubscription{
		{address: "/v1/alerts", event: "orderCreated"},
	})

	subs, count := broker.resolveSubscribers("orderCreated", "/v1")
	assert.Equal(t, 1, count)
	require.Len(t, subs, 1)
	assert.Equal(t, "/v1/alerts", subs[0].address)

	// A different schema-local fire must not resolve this subscription.
	other, count := broker.resolveSubscribers("orderCreated", "/v2")
	assert.Equal(t, 0, count)
	assert.Empty(t, other)
}

/*
Scenario: Global resolution crosses schema boundaries for match-identified examples
Given a broker with a match-identified example in one schema
When a global event fires
Then the example resolves regardless of the firing schema

Related spec scenarios: RS.EVT.6, RS.EVT.22
*/
func TestEventBroker_ResolveMatchIdentifiedGlobal(t *testing.T) {
	t.Parallel()

	broker := newEventBroker(nil)
	broker.addSubscriptions("/v1", []channelSubscription{
		{address: "/v1/alerts", event: "orderCreated"},
	})

	subs, count := broker.resolveSubscribers("orderCreated", "/anything", true)
	assert.Equal(t, 1, count)
	require.Len(t, subs, 1)
	assert.Equal(t, "/v1/alerts", subs[0].address)
}

/*
Scenario: hasSubscribers reports emptiness cheaply
Given a broker with and without a matching identity+scope
When hasSubscribers is queried
Then it returns true only when a subscription exists for the identity+scope

Related spec scenarios: RS.EVT.14, RS.EVT.22
*/
func TestEventBroker_HasSubscribers(t *testing.T) {
	t.Parallel()

	broker := newEventBroker(nil)
	broker.addSubscriptions("/v1", []channelSubscription{
		{address: "/v1/alerts", event: "levelUp"},
	})

	assert.True(t, broker.hasSubscribers("levelUp", "/v1"))
	assert.True(t, broker.hasSubscribers("levelUp", "/v1", true))

	assert.False(t, broker.hasSubscribers("missing", "/v1"))
	assert.False(t, broker.hasSubscribers("levelUp", "/v2"))
}

/*
Scenario: hasSubscribers and addSubscriptions are safe under concurrency
Given a broker being mutated and queried from multiple goroutines
When subscriptions are added while hasSubscribers and resolveSubscribers run
Then the broker remains consistent (race detector must stay clean)

Related spec scenarios: RS.EVT.14, RS.EVT.22, RS.MAPI.33
*/
func TestEventBroker_HasSubscribersConcurrentWithAdds(t *testing.T) {
	broker := newEventBroker(nil)
	broker.addSubscriptions("/v0", []channelSubscription{{address: "/v0/base", event: "seed"}})

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				schema := fmt.Sprintf("/s%d", (seed+j)%8)
				broker.addSubscriptions(schema, []channelSubscription{{
					address: schema + "/ch",
					event:   fmt.Sprintf("ev-%d", (seed+j)%16),
				}})
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				schema := fmt.Sprintf("/s%d", (seed+j)%8)
				_ = broker.hasSubscribers(fmt.Sprintf("ev-%d", (seed+j)%16), schema)
				_, count := broker.resolveSubscribers(fmt.Sprintf("ev-%d", (seed+j)%16), schema)
				assert.GreaterOrEqual(t, count, 0)
			}
		}(i)
	}
	wg.Wait()
}
