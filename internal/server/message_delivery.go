package server

import (
	"cmp"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
)

// messageDelivery renders and delivers subscribed AsyncAPI message examples
// through the MessageRenderer and ConsumerBus contracts. It is the cohesive
// delivery engine behind the event driver (design D8): it owns the sources of
// per-emission rendering (state, env, event, connection), the recipient
// partition, the delayed-emission cancellation and the push/observer side
// effects. It never reaches into Server or the broker/scheduler registries.
type messageDelivery struct {
	renderer MessageRenderer
	bus      ConsumerBus
	verbose  bool

	// observer, when set, is invoked with push envelopes so the management
	// stream can mirror deliveries (RS.AMG.25).
	observer func(env manageEnvelope)
	// done is closed on shutdown so pending delayed emissions no longer run.
	done    chan struct{}
	stopOne sync.Once
}

// newMessageDelivery wires a delivery engine over a renderer and consumer bus.
func newMessageDelivery(renderer MessageRenderer, bus ConsumerBus, verbose bool) *messageDelivery {
	return &messageDelivery{
		renderer: renderer,
		bus:      bus,
		verbose:  verbose,
		done:     make(chan struct{}),
	}
}

// setObserver installs the management-stream push observer.
func (d *messageDelivery) setObserver(observer func(env manageEnvelope)) {
	d.observer = observer
}

// doneChannel returns the delivery shutdown signal so callers that schedule
// work on their own goroutines can cancel it on shutdown.
func (d *messageDelivery) doneChannel() <-chan struct{} {
	if d == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return d.done
}

// shutdown cancels pending delayed emissions so no delivery happens after
// shutdown.
func (d *messageDelivery) shutdown() {
	if d == nil {
		return
	}
	d.stopOne.Do(func() { close(d.done) })
}

// deliver delivers an event payload to a subscription's consumers (broadcast).
func (d *messageDelivery) deliver(sub channelSubscription, payload map[string]any) {
	d.deliverTo(sub, payload, nil)
}

// deliverTargeted delivers a built-in event to a single candidate connection.
// The connection bucket (if any) is evaluated against that one recipient only;
// with no connection conditions the message is pushed to the recipient alone
// (RS.EVT.24, RS.EXT.26).
func (d *messageDelivery) deliverTargeted(sub channelSubscription, payload map[string]any, recipient ConsumerInfo) {
	d.deliverTo(sub, payload, &recipient)
}

// deliverTo runs the shared delayed-emission + delivery pipeline for a
// subscription. When target is non-nil, delivery is restricted to that single
// candidate (built-in connect recipient).
func (d *messageDelivery) deliverTo(sub channelSubscription, payload map[string]any, target *ConsumerInfo) {
	if len(sub.messages) == 0 {
		return
	}
	if sub.delay > 0 {
		ms := sub.delay
		sub.delay = 0
		go func() {
			select {
			case <-d.done:
				return
			case <-time.After(time.Duration(ms) * time.Millisecond):
			}
			d.deliverTo(sub, payload, target)
		}()
		return
	}
	deliverable := sub.messages[0]
	addr := sub.address
	prefix := deliverable.prefix
	eventName := sub.event
	opID := "event:" + cmp.Or(eventName, anyEventIdentity) + ":" + addr

	d.deliverExample(sub, deliverable.spec.Examples, addr, prefix, eventName, payload, opID, target)
}

// stateEnvEvaluator wires the fixed state and environment sources shared by
// every emission path (periodic deliveries have no event/connection context).
func (d *messageDelivery) stateEnvEvaluator(prefix string) runtime.Evaluator {
	eval := runtime.NewEvaluator()
	eval.AddSource(runtime.SourceState, d.renderer.NewStateSource(prefix))
	eval.AddSource(runtime.SourceEnv, d.renderer.NewEnvSource())
	return eval
}

// eventEvaluator wires the fixed emission sources (state, env, event) plus an
// optional per-connection source into a fresh evaluator.
func (d *messageDelivery) eventEvaluator(state, env runtime.DataSource, eventName string, payload map[string]any, connection *runtime.ConnectionSource) runtime.Evaluator {
	eval := runtime.NewEvaluator()
	eval.AddSource(runtime.SourceState, state)
	eval.AddSource(runtime.SourceEnv, env)
	eval.AddSource(runtime.SourceEvent, &runtime.EventSource{Name: eventName, Data: payload})
	if connection != nil {
		eval.AddSource(runtime.SourceConnection, connection)
	}
	return eval
}

// renderExample renders a single example's payload against an evaluator,
// honoring x-mock-skip and x-mock-set-state. It returns the rendered body, or
// nil when the example is skipped or rendering fails (verbose-logged).
func (d *messageDelivery) renderExample(view *MessageExampleView, eval runtime.Evaluator, prefix, opID string) []byte {
	if extensions.ValueSkip(view) {
		return nil
	}
	if stateMap, ok := extensions.ValueSetState(view); ok {
		d.renderer.ApplySetState(stateMap, eval, prefix)
	}
	body, err := d.renderer.RenderAsyncPayload(view, eval)
	if err != nil {
		if d.verbose {
			slog.Debug("Example delivery render failed", "opID", opID, "err", err)
		}
		return nil
	}
	return body
}

// evaluateConnectionBucket evaluates an example's connection conditions
// against one candidate recipient. An empty bucket matches every candidate.
func (d *messageDelivery) evaluateConnectionBucket(bucket extensions.ParamsMatch, state, env runtime.DataSource, eventName string, payload map[string]any, candidate ConsumerInfo) (bool, error) {
	if len(bucket) == 0 {
		return true, nil
	}
	eval := d.eventEvaluator(state, env, eventName, payload, connectionSourceFromInfo(candidate))
	return extensions.EvaluateParamsMatch(bucket, eval)
}

// deliverExample runs the shared selection + render + recipient-partition
// pipeline for one subscription's examples. When target is non-nil, delivery
// is restricted to that single candidate (built-in connect recipient).
func (d *messageDelivery) deliverExample(sub channelSubscription, examples []*loader.MessageExampleSpec, addr, prefix, eventName string, payload map[string]any, opID string, target *ConsumerInfo) {
	// Fixed (non-connection) sources evaluated once per emission.
	state := d.renderer.NewStateSource(prefix)
	env := d.renderer.NewEnvSource()

	for _, example := range examples {
		view := &MessageExampleView{spec: example}
		common, connection := d.partitionedMatch(view)
		var connSource *runtime.ConnectionSource
		if target != nil {
			connSource = connectionSourceFromInfo(*target)
		}
		evaluator := d.eventEvaluator(state, env, eventName, payload, connSource)
		if len(common) > 0 {
			ok, cErr := extensions.EvaluateParamsMatch(common, evaluator)
			if cErr != nil || !ok {
				continue
			}
		}
		body := d.renderExample(view, evaluator, prefix, opID)
		if body == nil {
			continue
		}
		if target != nil {
			// Built-in recipient: evaluate the connection bucket against the
			// single candidate and deliver on match (or immediately when there
			// is no connection bucket).
			ok, okErr := d.evaluateConnectionBucket(connection, state, env, eventName, payload, *target)
			if okErr != nil || !ok {
				continue
			}
			d.notifyPush(addr, target.ConnectionID, body)
			d.bus.PushTo(*target, addr, body)
			continue
		}
		if len(connection) == 0 {
			// Broadcast fast path (RS.EXT.25).
			d.notifyPush(addr, "", body)
			d.bus.SignalRPush(addr, body)
			d.bus.WSBroadcast(addr, body)
			continue
		}
		// Per-connection partition: evaluate the connection bucket against each
		// candidate with its connection context (design D6).
		for _, candidate := range d.bus.Candidates(addr) {
			ok, okErr := d.evaluateConnectionBucket(connection, state, env, eventName, payload, candidate)
			if okErr != nil || !ok {
				continue
			}
			d.notifyPush(addr, candidate.ConnectionID, body)
			d.bus.PushTo(candidate, addr, body)
		}
	}
}

// deliverPeriodic renders a periodically driven example against current state
// and environment and broadcasts it to the channel's consumers.
func (d *messageDelivery) deliverPeriodic(address, prefix string, spec *loader.MessageExampleSpec, opID string) {
	view := &MessageExampleView{spec: spec}
	body := d.renderExample(view, d.stateEnvEvaluator(prefix), prefix, opID)
	if body == nil {
		return
	}
	d.notifyPush(address, "", body)
	d.bus.SignalRPush(address, body)
	d.bus.WSBroadcast(address, body)
}

// notifyPush emits a push envelope to the management observer (RS.AMG.25).
func (d *messageDelivery) notifyPush(channel, connectionID string, body []byte) {
	if d.observer == nil {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		payload = map[string]any{"raw": string(body)}
	}
	env := manageEnvelope{Type: "push"}
	env.Push = &managePushEnvelope{Channel: channel, ConnectionID: connectionID, Payload: payload}
	d.observer(env)
}

// partitionedMatch splits an example's x-mock-match into common conditions
// (evaluated once per emission) and connection conditions (evaluated per
// candidate recipient). A nil/absent match yields empty buckets.
func (d *messageDelivery) partitionedMatch(view *MessageExampleView) (extensions.ParamsMatch, extensions.ParamsMatch) {
	match, _ := extensions.ValueMatch(view)
	return extensions.PartitionConnectionConditions(extensions.ParamsMatch(match))
}
