// Package eventbus owns the pure event-subscription broker and the per-example
// interval scheduler. Both coordinators are decoupled from HTTP, SignalR and
// the Server; delivery and management-stream observation stay in internal/server.
package eventbus

import (
	"log/slog"
	"sync"
	"time"

	"github.com/mamonth/oasmock/internal/loader"
)

// AnyEventIdentity is the broker key for match-identified examples that do not
// pin an identity ({$event.name}) — they evaluate against every fired event.
const AnyEventIdentity = "*"

// anyEventIdentity is the unexported alias used internally so match-identified
// examples never collide with user-chosen event identities.
const anyEventIdentity = AnyEventIdentity

// ChannelSubscription binds an event subscription to a channel address.
type ChannelSubscription struct {
	// Address is the fully-prefixed channel address.
	Address string
	// Event is the match identity: the {$event.name} condition value, a
	// built-in trigger (connect/receive), or "" for payload-only matches.
	Event string
	// Delay is the per-example x-mock-delay (ms) applied before an event-driven
	// emission (RS.EXT.23).
	Delay int
	// Schema is the owning schema prefix (empty = global).
	Schema string
	// Messages carries the message specs whose examples subscribed.
	Messages []*MessageDeliverable
}

// MessageDeliverable is a message spec deliverable when its subscription fires.
type MessageDeliverable struct {
	Spec   *loader.MessageSpec
	Prefix string
}

// DelaySchedule describes a delayed delivery.
type DelaySchedule struct {
	Ms int
}

// EventDeliverer emits a delivered message for a channel subscription.
type EventDeliverer func(sub ChannelSubscription, payload map[string]any)

// Broker decouples OpenAPI event triggers from AsyncAPI consumers. Subscriptions
// are keyed by match identity + schema scope.
type Broker struct {
	mu      sync.RWMutex
	byEvent map[string][]ChannelSubscription // identity -> subscriptions
	deliver EventDeliverer
	// done is closed on shutdown so pending delayed fires no longer deliver.
	done    chan struct{}
	stopOne sync.Once
}

// NewBroker creates an empty broker. When deliver is nil, fired events are
// accepted without delivery (used by tests).
func NewBroker(deliver EventDeliverer) *Broker {
	return &Broker{
		byEvent: make(map[string][]ChannelSubscription),
		deliver: deliver,
		done:    make(chan struct{}),
	}
}

// Stop cancels any pending delayed deliveries.
func (b *Broker) Stop() {
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

// AddSubscriptions registers subscriptions for a schema.
func (b *Broker) AddSubscriptions(schema string, subs []ChannelSubscription) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range subs {
		subs[i].Schema = schema
		key := sanitizeIdentity(subs[i].Event)
		b.byEvent[key] = append(b.byEvent[key], subs[i])
	}
}

// RemoveRuntimeExample removes the runtime event-driven subscription registered
// under a deliverable named "runtime-<id>" for a schema scope.
func (b *Broker) RemoveRuntimeExample(schema, id string) {
	if b == nil {
		return
	}
	target := "runtime-" + id
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, subs := range b.byEvent {
		kept := subs[:0]
		for _, sub := range subs {
			if sub.Schema == schema && hasDeliverableNamed(sub, target) {
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

// SubscriptionCount reports how many event identities are registered, for tests
// asserting an absent subscription set.
func (b *Broker) SubscriptionCount() int {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.byEvent)
}

// hasDeliverableNamed reports whether a subscription carries a deliverable
// whose message spec name equals target.
func hasDeliverableNamed(sub ChannelSubscription, target string) bool {
	for _, d := range sub.Messages {
		if d.Spec != nil && d.Spec.Name == target {
			return true
		}
	}
	return false
}

// ResolveSubscribers returns subscriptions matching an event name for the
// given firing schema. When global is true, all schemas' subscriptions match.
// Wildcard subscriptions (payload-only matches) always resolve.
func (b *Broker) ResolveSubscribers(event, firingSchema string, global ...bool) ([]ChannelSubscription, int) {
	if b == nil {
		return nil, 0
	}
	isGlobal := len(global) > 0 && global[0]
	b.mu.RLock()
	defer b.mu.RUnlock()
	all := append([]ChannelSubscription{}, b.byEvent[event]...)
	all = append(all, b.byEvent[anyEventIdentity]...)
	out := make([]ChannelSubscription, 0, len(all))
	for _, sub := range all {
		if isGlobal || sub.Schema == firingSchema {
			out = append(out, sub)
		}
	}
	return out, len(out)
}

// HasSubscribers is a cheap membership check for hot paths such as built-in
// trigger firing: it reports whether any subscription exists for an event
// identity and schema scope (global when global is true).
func (b *Broker) HasSubscribers(event, firingSchema string, global ...bool) bool {
	if b == nil {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	isGlobal := len(global) > 0 && global[0]
	// Copy before concat so the wildcard entries are never appended into the
	// live byEvent slice's backing array (which would race AddSubscriptions).
	all := append([]ChannelSubscription{}, b.byEvent[event]...)
	all = append(all, b.byEvent[anyEventIdentity]...)
	for _, sub := range all {
		if isGlobal || sub.Schema == firingSchema {
			return true
		}
	}
	return false
}

// Fire dispatches a named event. A delay schedules delivery on a background
// goroutine; otherwise delivery is synchronous.
func (b *Broker) Fire(event string, payload map[string]any, firingSchema string, global bool, delay *DelaySchedule) {
	if b == nil {
		return
	}
	subs, _ := b.ResolveSubscribers(event, firingSchema, global)
	if len(subs) == 0 {
		return
	}
	if delay != nil && delay.Ms > 0 {
		go func() {
			select {
			case <-b.done:
				return
			case <-time.After(time.Duration(delay.Ms) * time.Millisecond):
			}
			b.deliverAll(subs, payload)
		}()
		return
	}
	b.deliverAll(subs, payload)
}

// deliverAll emits a payload to every resolved subscription.
func (b *Broker) deliverAll(subs []ChannelSubscription, payload map[string]any) {
	for _, sub := range subs {
		if b.deliver != nil {
			b.deliver(sub, payload)
		} else {
			slog.Debug("Event delivered (no deliverer)", "event", sub.Event, "address", sub.Address)
		}
	}
}
