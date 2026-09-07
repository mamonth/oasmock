package eventbus

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
func TestBroker_ResolveSchemaLocal(t *testing.T) {
	t.Parallel()

	broker := NewBroker(nil)
	broker.AddSubscriptions("/v1", []ChannelSubscription{
		{Address: "/v1/alerts", Event: "orderCreated"},
	})

	subs, count := broker.ResolveSubscribers("orderCreated", "/v1")
	assert.Equal(t, 1, count)
	require.Len(t, subs, 1)
	assert.Equal(t, "/v1/alerts", subs[0].Address)
}

/*
Scenario: Schema-local events do not cross schema boundaries
Given a broker with a subscription in schema /a
When a schema-local event is fired from schema /b
Then no subscription is resolved

Related spec scenarios: RS.EVT.5
*/
func TestBroker_ResolveSchemaLocalNoCross(t *testing.T) {
	t.Parallel()

	broker := NewBroker(nil)
	broker.AddSubscriptions("/a", []ChannelSubscription{
		{Address: "/a/alerts", Event: "orderCreated"},
	})

	subs, count := broker.ResolveSubscribers("orderCreated", "/b")
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
func TestBroker_ResolveGlobal(t *testing.T) {
	t.Parallel()

	broker := NewBroker(nil)
	broker.AddSubscriptions("/a", []ChannelSubscription{
		{Address: "/a/alerts", Event: "orderCreated"},
	})

	subs, count := broker.ResolveSubscribers("orderCreated", "", true)
	assert.Equal(t, 1, count)
	require.Len(t, subs, 1)
	assert.Equal(t, "/a/alerts", subs[0].Address)
}

/*
Scenario: Event with no subscribers is accepted
Given a broker with no matching subscription
When an event fires
Then it is accepted with no delivery

Related spec scenarios: RS.EVT.14
*/
func TestBroker_FireNoSubscribers(t *testing.T) {
	t.Parallel()

	broker := NewBroker(nil)
	broker.AddSubscriptions("/v1", []ChannelSubscription{
		{Address: "/v1/alerts", Event: "other"},
	})

	broker.Fire("orderCreated", map[string]any{"id": "1"}, "/v1", false, nil)
}

/*
Scenario: Delayed event delivery schedules
Given an event with a delay
When fire is called
Then the delivery is scheduled and the broker returns immediately

Related spec scenarios: RS.EVT.4, RS.EVT.16
*/
func TestBroker_FireWithDelaySchedules(t *testing.T) {
	t.Parallel()

	delivered := make(chan ChannelSubscription, 1)
	broker := NewBroker(func(sub ChannelSubscription, payload map[string]any) {
		delivered <- sub
	})

	broker.AddSubscriptions("/v1", []ChannelSubscription{
		{Address: "/v1/alerts", Event: "orderCreated"},
	})

	broker.Fire("orderCreated", map[string]any{"id": "1"}, "/v1", false, &DelaySchedule{Ms: 10})

	select {
	case sub := <-delivered:
		assert.Equal(t, "/v1/alerts", sub.Address)
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
func TestBroker_FireImmediate(t *testing.T) {
	t.Parallel()

	var delivered []ChannelSubscription
	broker := NewBroker(func(sub ChannelSubscription, payload map[string]any) {
		delivered = append(delivered, sub)
	})

	broker.AddSubscriptions("/v1", []ChannelSubscription{
		{Address: "/v1/alerts", Event: "orderCreated"},
	})

	broker.Fire("orderCreated", map[string]any{"id": "1"}, "/v1", false, nil)
	require.Len(t, delivered, 1)
	assert.Equal(t, "/v1/alerts", delivered[0].Address)
}

/*
Scenario: Match-identified examples resolve only for their schema
Given a broker with an event-driven example registered under identity + schema
When resolveSubscribers is called for a schema-local match fire in the same schema
Then the subscription resolves, and it does not resolve for another schema

Related spec scenarios: RS.EVT.5, RS.EVT.22
*/
func TestBroker_ResolveMatchIdentified(t *testing.T) {
	t.Parallel()

	broker := NewBroker(nil)
	broker.AddSubscriptions("/v1", []ChannelSubscription{
		{Address: "/v1/alerts", Event: "orderCreated"},
	})

	subs, count := broker.ResolveSubscribers("orderCreated", "/v1")
	assert.Equal(t, 1, count)
	require.Len(t, subs, 1)
	assert.Equal(t, "/v1/alerts", subs[0].Address)

	// A different schema-local fire must not resolve this subscription.
	other, count := broker.ResolveSubscribers("orderCreated", "/v2")
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
func TestBroker_ResolveMatchIdentifiedGlobal(t *testing.T) {
	t.Parallel()

	broker := NewBroker(nil)
	broker.AddSubscriptions("/v1", []ChannelSubscription{
		{Address: "/v1/alerts", Event: "orderCreated"},
	})

	subs, count := broker.ResolveSubscribers("orderCreated", "/anything", true)
	assert.Equal(t, 1, count)
	require.Len(t, subs, 1)
	assert.Equal(t, "/v1/alerts", subs[0].Address)
}

/*
Scenario: hasSubscribers reports emptiness cheaply
Given a broker with and without a matching identity+scope
When hasSubscribers is queried
Then it returns true only when a subscription exists for the identity+scope

Related spec scenarios: RS.EVT.14, RS.EVT.22
*/
func TestBroker_HasSubscribers(t *testing.T) {
	t.Parallel()

	broker := NewBroker(nil)
	broker.AddSubscriptions("/v1", []ChannelSubscription{
		{Address: "/v1/alerts", Event: "levelUp"},
	})

	assert.True(t, broker.HasSubscribers("levelUp", "/v1"))
	assert.True(t, broker.HasSubscribers("levelUp", "/v1", true))

	assert.False(t, broker.HasSubscribers("missing", "/v1"))
	assert.False(t, broker.HasSubscribers("levelUp", "/v2"))
}

/*
Scenario: SubscriptionCount reports identity entries
Given a broker with known subscriptions
When SubscriptionCount is queried
Then it reports the number of registered identities

Related spec scenarios: RS.EVT.5
*/
func TestBroker_SubscriptionCount(t *testing.T) {
	t.Parallel()

	broker := NewBroker(nil)
	assert.Equal(t, 0, broker.SubscriptionCount())
	broker.AddSubscriptions("/v1", []ChannelSubscription{
		{Address: "/v1/a", Event: "ev-a"},
		{Address: "/v1/b", Event: "ev-b"},
		{Address: "/v1/c", Event: "ev-a"}, // same identity as ev-a -> coalesces
	})
	assert.Equal(t, 2, broker.SubscriptionCount())
}

/*
Scenario: hasSubscribers and addSubscriptions are safe under concurrency
Given a broker being mutated and queried from multiple goroutines
When subscriptions are added while hasSubscribers and resolveSubscribers run
Then the broker remains consistent (race detector must stay clean)

Related spec scenarios: RS.EVT.14, RS.EVT.22, RS.MAPI.33
*/
func TestBroker_HasSubscribersConcurrentWithAdds(t *testing.T) {
	broker := NewBroker(nil)
	broker.AddSubscriptions("/v0", []ChannelSubscription{{Address: "/v0/base", Event: "seed"}})

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				schema := fmt.Sprintf("/s%d", (seed+j)%8)
				broker.AddSubscriptions(schema, []ChannelSubscription{{
					Address: schema + "/ch",
					Event:   fmt.Sprintf("ev-%d", (seed+j)%16),
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
				_ = broker.HasSubscribers(fmt.Sprintf("ev-%d", (seed+j)%16), schema)
				_, count := broker.ResolveSubscribers(fmt.Sprintf("ev-%d", (seed+j)%16), schema)
				assert.GreaterOrEqual(t, count, 0)
			}
		}(i)
	}
	wg.Wait()
}
