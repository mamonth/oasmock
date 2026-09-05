package server

import (
	"cmp"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
)

func (b *eventBus) deliver(sub channelSubscription, payload map[string]any) {
	b.deliverTo(sub, payload, nil)
}

// deliverTargeted delivers a built-in event to a single candidate connection.
// The connection bucket (if any) is evaluated against that one recipient only;
// with no connection conditions the message is pushed to the recipient alone
// (RS.EVT.24, RS.EXT.26).
func (b *eventBus) deliverTargeted(sub channelSubscription, payload map[string]any, recipient ConsumerInfo) {
	b.deliverTo(sub, payload, &recipient)
}

// deliverTo runs the shared delayed-emission + delivery pipeline for a
// subscription. When target is non-nil, delivery is restricted to that single
// candidate (built-in connect recipient).
func (b *eventBus) deliverTo(sub channelSubscription, payload map[string]any, target *ConsumerInfo) {
	if len(sub.messages) == 0 {
		return
	}
	if sub.delay > 0 {
		ms := sub.delay
		sub.delay = 0
		go func() {
			select {
			case <-b.done:
				return
			case <-time.After(time.Duration(ms) * time.Millisecond):
			}
			b.deliverTo(sub, payload, target)
		}()
		return
	}
	deliverable := sub.messages[0]
	addr := sub.address
	prefix := deliverable.prefix
	eventName := sub.event
	opID := "event:" + cmp.Or(eventName, anyEventIdentity) + ":" + addr

	b.deliverExample(sub, deliverable.spec.Examples, addr, prefix, eventName, payload, opID, target)
}

// stateEnvEvaluator wires the fixed state and environment sources shared by
// every emission path (periodic deliveries have no event/connection context).
func (b *eventBus) stateEnvEvaluator(prefix string) runtime.Evaluator {
	eval := runtime.NewEvaluator()
	eval.AddSource(runtime.SourceState, b.renderer.NewStateSource(prefix))
	eval.AddSource(runtime.SourceEnv, b.renderer.NewEnvSource())
	return eval
}

// eventEvaluator wires the fixed emission sources (state, env, event) plus an
// optional per-connection source into a fresh evaluator.
func (b *eventBus) eventEvaluator(state, env runtime.DataSource, eventName string, payload map[string]any, connection *runtime.ConnectionSource) runtime.Evaluator {
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
func (b *eventBus) renderExample(view *MessageExampleView, eval runtime.Evaluator, prefix, opID string) []byte {
	if extensions.ValueSkip(view) {
		return nil
	}
	if stateMap, ok := extensions.ValueSetState(view); ok {
		b.renderer.ApplySetState(stateMap, eval, prefix)
	}
	body, err := b.renderer.RenderAsyncPayload(view, eval)
	if err != nil {
		if b.verbose {
			slog.Debug("Example delivery render failed", "opID", opID, "err", err)
		}
		return nil
	}
	return body
}

// evaluateConnectionBucket evaluates an example's connection conditions
// against one candidate recipient. An empty bucket matches every candidate.
func (b *eventBus) evaluateConnectionBucket(bucket extensions.ParamsMatch, state, env runtime.DataSource, eventName string, payload map[string]any, candidate ConsumerInfo) (bool, error) {
	if len(bucket) == 0 {
		return true, nil
	}
	eval := b.eventEvaluator(state, env, eventName, payload, connectionSourceFromInfo(candidate))
	return extensions.EvaluateParamsMatch(bucket, eval)
}

// deliverExample runs the shared selection + render + recipient-partition
// pipeline for one subscription's examples. When target is non-nil, delivery
// is restricted to that single candidate (built-in connect recipient).
func (b *eventBus) deliverExample(sub channelSubscription, examples []*loader.MessageExampleSpec, addr, prefix, eventName string, payload map[string]any, opID string, target *ConsumerInfo) {
	// Fixed (non-connection) sources evaluated once per emission.
	state := b.renderer.NewStateSource(prefix)
	env := b.renderer.NewEnvSource()

	for _, example := range examples {
		view := &MessageExampleView{spec: example}
		common, connection := b.partitionedMatch(view)
		var connSource *runtime.ConnectionSource
		if target != nil {
			connSource = connectionSourceFromInfo(*target)
		}
		evaluator := b.eventEvaluator(state, env, eventName, payload, connSource)
		if len(common) > 0 {
			ok, cErr := extensions.EvaluateParamsMatch(common, evaluator)
			if cErr != nil || !ok {
				continue
			}
		}
		body := b.renderExample(view, evaluator, prefix, opID)
		if body == nil {
			continue
		}
		if target != nil {
			// Built-in recipient: evaluate the connection bucket against the
			// single candidate and deliver on match (or immediately when there
			// is no connection bucket).
			ok, okErr := b.evaluateConnectionBucket(connection, state, env, eventName, payload, *target)
			if okErr != nil || !ok {
				continue
			}
			b.notifyPush(addr, target.ConnectionID, body)
			b.bus.PushTo(*target, addr, body)
			continue
		}
		if len(connection) == 0 {
			// Broadcast fast path (RS.EXT.25).
			b.notifyPush(addr, "", body)
			b.bus.SignalRPush(addr, body)
			b.bus.WSBroadcast(addr, body)
			continue
		}
		// Per-connection partition: evaluate the connection bucket against each
		// candidate with its connection context (design D6).
		for _, candidate := range b.bus.Candidates(addr) {
			ok, okErr := b.evaluateConnectionBucket(connection, state, env, eventName, payload, candidate)
			if okErr != nil || !ok {
				continue
			}
			b.notifyPush(addr, candidate.ConnectionID, body)
			b.bus.PushTo(candidate, addr, body)
		}
	}
}

// notifyPush emits a push envelope to the management observer (RS.AMG.25).
func (b *eventBus) notifyPush(channel, connectionID string, body []byte) {
	if b.observer == nil {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		payload = map[string]any{"raw": string(body)}
	}
	env := manageEnvelope{Type: "push"}
	env.Push = &managePushEnvelope{Channel: channel, ConnectionID: connectionID, Payload: payload}
	b.observer(env)
}

// partitionedMatch splits an example's x-mock-match into common conditions
// (evaluated once per emission) and connection conditions (evaluated per
// candidate recipient). A nil/absent match yields empty buckets.
func (b *eventBus) partitionedMatch(view *MessageExampleView) (extensions.ParamsMatch, extensions.ParamsMatch) {
	match, _ := extensions.ValueMatch(view)
	return extensions.PartitionConnectionConditions(extensions.ParamsMatch(match))
}
