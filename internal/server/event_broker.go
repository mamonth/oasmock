package server

import (
	"log/slog"
	"sync"
	"time"

	"github.com/mamonth/oasmock/internal/loader"
)

// channelSubscription binds an event subscription to a channel address.
type channelSubscription struct {
	// address is the fully-prefixed channel address.
	address string
	// event is the named event, or a built-in trigger ("" when built-in).
	event string
	// schema is the owning schema prefix (empty = global).
	schema string
	// messages carries the message specs whose examples subscribed.
	messages []*messageDeliverable
}

// messageDeliverable is a message spec deliverable when its subscription fires.
type messageDeliverable struct {
	spec   *loader.MessageSpec
	prefix string
}

// delaySchedule describes a delayed delivery.
type delaySchedule struct {
	ms int
}

// eventDeliverer emits a delivered message for a channel subscription.
type eventDeliverer func(sub channelSubscription, payload map[string]any)

// eventBroker decouples OpenAPI event triggers from AsyncAPI consumers
// (design D8). Subscriptions are keyed by event name + schema scope.
type eventBroker struct {
	mu      sync.RWMutex
	byEvent map[string][]channelSubscription // event name -> subscriptions
	deliver eventDeliverer
}

// newEventBroker creates an empty broker. When deliver is nil, fired events
// are accepted without delivery (used by tests).
func newEventBroker(deliver eventDeliverer) *eventBroker {
	return &eventBroker{
		byEvent: make(map[string][]channelSubscription),
		deliver: deliver,
	}
}

// addSubscriptions registers subscriptions for a schema.
func (b *eventBroker) addSubscriptions(schema string, subs []channelSubscription) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range subs {
		if subs[i].event == "" {
			continue
		}
		subs[i].schema = schema
		b.byEvent[subs[i].event] = append(b.byEvent[subs[i].event], subs[i])
	}
}

// resolveSubscribers returns subscriptions matching an event name for the
// given firing schema. When global is true, all schemas' subscriptions match.
func (b *eventBroker) resolveSubscribers(event, firingSchema string, global ...bool) ([]channelSubscription, int) {
	if b == nil {
		return nil, 0
	}
	isGlobal := len(global) > 0 && global[0]
	b.mu.RLock()
	defer b.mu.RUnlock()
	all := b.byEvent[event]
	out := make([]channelSubscription, 0, len(all))
	for _, sub := range all {
		if isGlobal || sub.schema == firingSchema {
			out = append(out, sub)
		}
	}
	return out, len(out)
}

// fire dispatches a named event. A delay schedules delivery on a background
// goroutine; otherwise delivery is synchronous.
func (b *eventBroker) fire(event string, payload map[string]any, firingSchema string, global bool, delay *delaySchedule) {
	if b == nil {
		return
	}
	subs, _ := b.resolveSubscribers(event, firingSchema, global)
	if len(subs) == 0 {
		return
	}
	if delay != nil && delay.ms > 0 {
		go func() {
			time.Sleep(time.Duration(delay.ms) * time.Millisecond)
			b.deliverAll(subs, payload)
		}()
		return
	}
	b.deliverAll(subs, payload)
}

// deliverAll emits a payload to every resolved subscription.
func (b *eventBroker) deliverAll(subs []channelSubscription, payload map[string]any) {
	for _, sub := range subs {
		if b.deliver != nil {
			b.deliver(sub, payload)
		} else {
			slog.Debug("Event delivered (no deliverer)", "event", sub.event, "address", sub.address)
		}
	}
}
