package server

import (
	"log/slog"
	"strings"

	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/loader"
)

// eventBus is the pure-fabrication coordinator behind the event driver
// (design D8). It owns the event broker and renders + delivers subscribed
// messages through the MessageRenderer and ConsumerBus contracts, so it never
// reaches into Server.
type eventBus struct {
	broker   *eventBroker
	renderer MessageRenderer
	bus      ConsumerBus
	verbose  bool
}

// newEventBus wires a broker whose delivery goes through the renderer and
// consumer bus.
func newEventBus(renderer MessageRenderer, bus ConsumerBus, verbose bool) *eventBus {
	b := &eventBus{
		renderer: renderer,
		bus:      bus,
		verbose:  verbose,
	}
	b.broker = &eventBroker{
		byEvent: make(map[string][]channelSubscription),
		deliver: b.deliver,
	}
	return b
}

// fire dispatches a named event, reusing the broker's delay semantics.
func (b *eventBus) fire(name string, payload map[string]any, firingSchema string, global bool, delay *delaySchedule) {
	if b == nil || b.broker == nil {
		return
	}
	b.broker.fire(name, payload, firingSchema, global, delay)
}

// registerEventSubscriptions scans AsyncAPI schemas and registers x-send-events
// subscriptions for every message example, keyed by event name and schema.
func (b *eventBus) registerEventSubscriptions(schemas []SchemaInfo) {
	if b == nil || b.broker == nil {
		return
	}
	for _, schema := range schemas {
		if schema.Kind != loader.KindAsyncAPI || schema.Async == nil {
			continue
		}
		subs := collectSchemaSubscriptions(schema.Prefix, schema.Async)
		b.broker.addSubscriptions(schema.Prefix, subs)
	}
}

// deliver renders the subscribed message with the event payload and emits it
// to the channel's consumers (ws broadcast or SignalR open streams).
func (b *eventBus) deliver(sub channelSubscription, payload map[string]any) {
	if len(sub.messages) == 0 {
		return
	}
	deliverable := sub.messages[0]
	opID := "event:" + sub.event + ":" + sub.address

	count, body, err := b.renderer.RenderMessageSpecsWithEvent([]*loader.MessageSpec{deliverable.spec}, deliverable.prefix, opID, payload)
	if err != nil {
		if b.verbose {
			slog.Debug("Event delivery render failed", "event", sub.event, "err", err)
		}
		return
	}
	if count == 0 {
		return
	}

	// Broadcast to SignalR open streams first (RS.EVT.13), then raw ws.
	b.bus.SignalRPush(sub.address, body)
	b.bus.WSBroadcast(sub.address, body)
}

// hubForAddress finds the SignalR hub owning a channel address.
func (s *Server) hubForAddress(address string) *signalRHub {
	return s.hubMgr.hubForAddress(address)
}

// collectSchemaSubscriptions extracts channel subscriptions declared via
// x-send-events on the schema's message examples. Each subscription carries a
// message spec restricted to the examples that subscribed to that event, so
// delivery renders exactly the templated subscribed message.
func collectSchemaSubscriptions(prefix string, doc *asyncapi.Document) []channelSubscription {
	var subs []channelSubscription
	for _, ch := range doc.Channels {
		address := asyncAddressWithPrefix(prefix, ch.Address)
		for _, msg := range ch.Messages {
			byEvent := groupSubscribedExamples(msg)
			for event, examples := range byEvent {
				spec := loader.NewMessageSpec(msg)
				if spec == nil {
					continue
				}
				spec.Examples = examples
				subs = append(subs, channelSubscription{
					address:  address,
					event:    event,
					messages: []*messageDeliverable{{spec: spec, prefix: prefix}},
				})
			}
		}
	}
	return subs
}

// groupSubscribedExamples returns, per event, the message examples carrying a
// subscription to that event (as loader example specs).
func groupSubscribedExamples(msg *asyncapi.Message) map[string][]*loader.MessageExampleSpec {
	out := make(map[string][]*loader.MessageExampleSpec)
	for _, ex := range msg.Examples {
		if ex == nil {
			continue
		}
		events, err := parseSendEvents(ex.Extensions)
		if err != nil {
			continue
		}
		spec := &loader.MessageExampleSpec{
			Name:       ex.Name,
			Headers:    ex.Headers,
			Payload:    ex.Payload,
			Extensions: ex.Extensions,
		}
		for _, ev := range events {
			out[ev.On] = append(out[ev.On], spec)
		}
	}
	return out
}

// asyncAddressWithPrefix applies a schema prefix to a channel address.
func asyncAddressWithPrefix(prefix, address string) string {
	addr := "/" + strings.Trim(address, "/")
	if prefix == "" {
		return addr
	}
	return "/" + strings.Trim(prefix, "/") + addr
}

// broadcast sends a payload to every connected consumer of a channel address.
func (a *wsProtocolAdapter) broadcast(address string, payload []byte) {
	if a == nil || a.registry == nil {
		return
	}
	for _, ws := range a.registry.connections(address) {
		ws.writer.write(payload)
	}
}

// renderMessageSpecsWithEvent evaluates message specs with an event payload
// registered in the evaluator as {$event.*}.
func (s *Server) renderMessageSpecsWithEvent(messages []*loader.MessageSpec, prefix, opID string, payload map[string]any) (int, []byte, error) {
	return s.engine.RenderMessageSpecsWithEvent(messages, prefix, opID, payload)
}
