package server

import (
	"log/slog"
	"sync"
	"time"

	"github.com/mamonth/oasmock/internal/loader"
)

// anyEventIdentity is the broker key for match-identified examples that do not
// pin an identity ({$event.name}) — they evaluate against every fired event.
const anyEventIdentity = "*"

// channelSubscription binds an event subscription to a channel address.
type channelSubscription struct {
	// address is the fully-prefixed channel address.
	address string
	// event is the match identity: the {$event.name} condition value, a
	// built-in trigger (connect/receive), or "" for payload-only matches.
	event string
	// delay is the per-example x-mock-delay (ms) applied before an event-driven
	// emission (RS.EXT.23).
	delay int
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
// (design D8). Subscriptions are keyed by match identity + schema scope.
type eventBroker struct {
	mu      sync.RWMutex
	byEvent map[string][]channelSubscription // identity -> subscriptions
	deliver eventDeliverer
	// done is closed on shutdown so pending delayed fires no longer deliver.
	done    chan struct{}
	stopOne sync.Once
}

// newEventBroker creates an empty broker. When deliver is nil, fired events
// are accepted without delivery (used by tests).
func newEventBroker(deliver eventDeliverer) *eventBroker {
	return &eventBroker{
		byEvent: make(map[string][]channelSubscription),
		deliver: deliver,
		done:    make(chan struct{}),
	}
}

// stop cancels any pending delayed deliveries.
func (b *eventBroker) stop() {
	if b == nil {
		return
	}
	b.stopOne.Do(func() { close(b.done) })
}

// sanitizeIdentity maps a subscription identity to a broker key. An empty
// identity becomes the wildcard key ("*") so payload-only matches evaluate
// against every fired event.
func sanitizeIdentity(identity string) string {
	if identity == "" {
		return anyEventIdentity
	}
	return identity
}

// addSubscriptions registers subscriptions for a schema.
func (b *eventBroker) addSubscriptions(schema string, subs []channelSubscription) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range subs {
		subs[i].schema = schema
		key := sanitizeIdentity(subs[i].event)
		b.byEvent[key] = append(b.byEvent[key], subs[i])
	}
}

// removeRuntimeExample removes the runtime event-driven subscription registered
// under a deliverable named "runtime-<id>" for a schema scope.
func (b *eventBroker) removeRuntimeExample(schema, id string) {
	if b == nil {
		return
	}
	target := "runtime-" + id
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, subs := range b.byEvent {
		kept := subs[:0]
		for _, sub := range subs {
			if sub.schema == schema && hasDeliverableNamed(sub, target) {
				continue
			}
			kept = append(kept, sub)
		}
		if len(kept) == 0 {
			delete(b.byEvent, key)
		} else {
			b.byEvent[key] = kept
		}
	}
}

// hasDeliverableNamed reports whether a subscription carries a deliverable
// whose message spec name equals target.
func hasDeliverableNamed(sub channelSubscription, target string) bool {
	for _, d := range sub.messages {
		if d.spec != nil && d.spec.Name == target {
			return true
		}
	}
	return false
}

// resolveSubscribers returns subscriptions matching an event name for the
// given firing schema. When global is true, all schemas' subscriptions match.
// Wildcard subscriptions (payload-only matches) always resolve.
func (b *eventBroker) resolveSubscribers(event, firingSchema string, global ...bool) ([]channelSubscription, int) {
	if b == nil {
		return nil, 0
	}
	isGlobal := len(global) > 0 && global[0]
	b.mu.RLock()
	defer b.mu.RUnlock()
	all := append([]channelSubscription{}, b.byEvent[event]...)
	all = append(all, b.byEvent[anyEventIdentity]...)
	out := make([]channelSubscription, 0, len(all))
	for _, sub := range all {
		if isGlobal || sub.schema == firingSchema {
			out = append(out, sub)
		}
	}
	return out, len(out)
}

// hasSubscribers is a cheap membership check for hot paths such as built-in
// trigger firing: it reports whether any subscription exists for an event
// identity and schema scope (global when global is true).
func (b *eventBroker) hasSubscribers(event, firingSchema string, global ...bool) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	isGlobal := len(global) > 0 && global[0]
	// Copy before concat so the wildcard entries are never appended into the
	// live byEvent slice's backing array (which would race addSubscriptions).
	all := append([]channelSubscription{}, b.byEvent[event]...)
	all = append(all, b.byEvent[anyEventIdentity]...)
	for _, sub := range all {
		if isGlobal || sub.schema == firingSchema {
			return true
		}
	}
	return false
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
			select {
			case <-b.done:
				return
			case <-time.After(time.Duration(delay.ms) * time.Millisecond):
			}
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
