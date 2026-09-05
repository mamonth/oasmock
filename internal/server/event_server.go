package server

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
)

// eventBus is the pure-fabrication coordinator behind the event driver
// (design D8). It owns the event broker and renders + delivers subscribed
// messages through the MessageRenderer and ConsumerBus contracts, so it never
// reaches into Server.
type eventBus struct {
	broker    *eventBroker
	renderer  MessageRenderer
	bus       ConsumerBus
	scheduler *jobScheduler
	verbose   bool
	// wait sleeps for a delayed emission (design D4/D5); injectable so tests
	// stay hermetic.
	wait func(time.Duration)
	// observer, when set, is invoked with every emitted envelope so the
	// management stream can mirror fired events and deliveries (RS.AMG.24-25).
	observer func(env manageEnvelope)
}

// setObserver installs the management-stream observer.
func (b *eventBus) setObserver(observer func(env manageEnvelope)) {
	b.observer = observer
}

// newEventBus wires a broker whose delivery goes through the renderer and
// consumer bus.
func newEventBus(renderer MessageRenderer, bus ConsumerBus, verbose bool) *eventBus {
	b := &eventBus{
		renderer:  renderer,
		bus:       bus,
		scheduler: newJobScheduler(),
		verbose:   verbose,
		wait:      time.Sleep,
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
	if b.observer != nil {
		env := manageEnvelope{Type: "event"}
		env.Event = &manageEventEnvelope{Name: name, Schema: firingSchema, Global: global, Payload: payload}
		b.observer(env)
	}
	b.broker.fire(name, payload, firingSchema, global, delay)
}

// fireTargeted fires a built-in event scoped to a single recipient connection
// (the connecting consumer for the connect built-in, RS.EVT.24). Delivery is
// narrow: the common match is evaluated once, the connection match (if any)
// against the single candidate, and a match delivers to that candidate alone.
func (b *eventBus) fireTargeted(name string, payload map[string]any, firingSchema string, recipient ConsumerInfo) {
	if b == nil || b.broker == nil {
		return
	}
	if b.observer != nil {
		env := manageEnvelope{Type: "event"}
		env.Event = &manageEventEnvelope{Name: name, Schema: firingSchema, Global: false, Payload: payload}
		b.observer(env)
	}
	subs, _ := b.broker.resolveSubscribers(name, firingSchema)
	if len(subs) == 0 {
		return
	}
	for _, sub := range subs {
		b.deliverTargeted(sub, payload, recipient)
	}
}

// hasSubscribers reports whether any event-driven example could match an
// event identity within a schema scope (cheap gate for built-in firing).
func (b *eventBus) hasSubscribers(name, schema string) bool {
	return b.broker != nil && b.broker.hasSubscribers(name, schema)
}

// shutdown cancels all periodic interval jobs.
func (b *eventBus) shutdown() {
	if b == nil || b.scheduler == nil {
		return
	}
	b.scheduler.shutdown()
}

// registerEventSubscriptions scans AsyncAPI schemas, classifies each message
// example by trigger kind (event-driven via {$event.*} match, periodic via
// x-mock-interval, or reply), and registers event-driven subscriptions keyed by
// identity + schema. Legacy x-send-events entries are mapped to the unified
// form with a verbose deprecation note (RS.EVT.18). Load errors (mixed match
// contexts, dual triggers) abort schema setup (RS.EXT.20, RS.EXT.28).
func (b *eventBus) registerEventSubscriptions(schemas []SchemaInfo) error {
	if b == nil || b.broker == nil {
		return nil
	}
	for _, schema := range schemas {
		if schema.Kind != loader.KindAsyncAPI || schema.Async == nil {
			continue
		}
		if err := b.registerSchema(schema.Prefix, schema.Async); err != nil {
			return err
		}
	}
	return nil
}

// registerSchema classifies and registers every event-driven example of one
// AsyncAPI schema, and schedules its periodically driven examples. Classifying
// is two-phase so a late classification error aborts the whole schema without
// leaking already-started interval jobs: every example is validated first, and
// only when all pass are subscriptions and scheduler jobs committed (design
// D4, RS.EXT.20/22/28).
func (b *eventBus) registerSchema(prefix string, doc *asyncapi.Document) error {
	var subs []channelSubscription
	var periodic []periodicRegistration
	for _, ch := range doc.Channels {
		address := asyncAddressWithPrefix(prefix, ch.Address)
		for _, msg := range ch.Messages {
			for _, ex := range msg.Examples {
				if ex == nil {
					continue
				}
				derived, err := b.derivedExamples(ex)
				if err != nil {
					return err
				}
				for _, spec := range derived {
					trig, err := extensions.ClassifyTrigger(&MessageExampleView{spec: spec})
					if err != nil {
						return fmt.Errorf("channel %q example %q: %w", ch.ID, ex.Name, err)
					}
					switch trig.Kind {
					case extensions.TriggerEvent:
						subs = append(subs, channelSubscription{
							address: address,
							event:   trig.Identity,
							delay:   trig.Delay,
							messages: []*messageDeliverable{{
								spec:   &loader.MessageSpec{Name: msg.Name, Examples: []*loader.MessageExampleSpec{spec}},
								prefix: prefix,
							}},
						})
					case extensions.TriggerPeriodic:
						periodic = append(periodic, periodicRegistration{
							address: address, prefix: prefix, exampleID: ex.Name, spec: spec, interval: trig.Interval,
						})
					case extensions.TriggerReply:
						// Reply examples are served by the channel's normal
						// reply path; nothing to register here. A match that
						// still references {$connection.*} can never evaluate
						// (no connection context in the reply path), so point it
						// out in verbose mode instead of failing silently.
						if b.verbose && extensions.MatchReferencesConnection(trig.Match) {
							slog.Warn("reply example references {$connection.*} which never matches without an event context; remove the connection condition or make the example event-driven",
								"channel", ch.ID, "example", ex.Name)
						}
					}
				}
			}
		}
	}
	// Commit phase: nothing above has side effects, so a classification error
	// from any example leaves the eventBus untouched.
	b.broker.addSubscriptions(prefix, subs)
	for _, p := range periodic {
		if _, err := b.registerPeriodic(p.address, p.prefix, p.exampleID, p.spec, p.interval); err != nil {
			return err
		}
	}
	return nil
}

// periodicRegistration is a validated periodically driven example awaiting
// scheduler registration after the classification pass of registerSchema.
type periodicRegistration struct {
	address   string
	prefix    string
	exampleID string
	spec      *loader.MessageExampleSpec
	interval  int
}

// registerRuntimeExample registers a dynamically added async example (POST
// /_mock/examples with match or interval, RS.MAPI.24-26). Event-driven
// examples subscribe by identity; periodic examples schedule a delivery job.
// It returns the example trigger kind (TriggerEvent or TriggerPeriodic) and
// the scheduler job id ("" when none).
func (b *eventBus) registerRuntimeExample(id, address, prefix string, spec *loader.MessageExampleSpec) (extensions.TriggerKind, string, error) {
	if b == nil || b.broker == nil {
		return 0, "", fmt.Errorf("event broker not initialized")
	}
	trig, err := extensions.ClassifyTrigger(&MessageExampleView{spec: spec})
	if err != nil {
		return 0, "", err
	}
	switch trig.Kind {
	case extensions.TriggerEvent:
		b.broker.addSubscriptions(prefix, []channelSubscription{{
			address: address,
			event:   trig.Identity,
			delay:   trig.Delay,
			messages: []*messageDeliverable{{
				spec:   &loader.MessageSpec{Name: "runtime-" + id, Examples: []*loader.MessageExampleSpec{spec}},
				prefix: prefix,
			}},
		}})
		return extensions.TriggerEvent, "", nil
	case extensions.TriggerPeriodic:
		if _, err := b.registerPeriodic(address, prefix, id, spec, trig.Interval); err != nil {
			return 0, "", err
		}
		return extensions.TriggerPeriodic, fmt.Sprintf("interval-%s-%s-%s", prefix, address, id), nil
	default:
		return 0, "", fmt.Errorf("unsupported runtime trigger: async examples require an {$event.*} match or an interval")
	}
}

// removeEventSubscription unregisters a runtime event-driven subscription by
// its example id (deliverable spec name "runtime-<id>").
func (b *eventBus) removeEventSubscription(prefix, id string) {
	if b == nil || b.broker == nil {
		return
	}
	b.broker.removeRuntimeExample(prefix, id)
}

// registerPeriodic schedules a scheduler job delivering a periodically driven
// example at its cadence, notifying management observers (RS.AMG.27). The job
// id is scoped by prefix, address and example identity so distinct examples
// never collide.
func (b *eventBus) registerPeriodic(address, prefix, exampleID string, spec *loader.MessageExampleSpec, interval int) (string, error) {
	if interval <= 0 {
		return "", fmt.Errorf("x-mock-interval must be a positive integer")
	}
	jobID := fmt.Sprintf("interval-%s-%s-%s", prefix, address, exampleID)
	opID := "event:interval:" + address
	job := b.scheduler.add(&scheduledJob{
		id:        jobID,
		interval:  time.Duration(interval) * time.Millisecond,
		exampleID: exampleID,
		channel:   address,
		deliver: func() {
			b.deliverPeriodic(address, prefix, spec, opID)
		},
	})
	go b.scheduler.run(job)
	if b.observer != nil {
		env := manageEnvelope{Type: "schedule"}
		env.Schedule = &manageScheduleEnvelope{Action: "started", ExampleID: exampleID, Channel: address, Interval: interval}
		b.observer(env)
	}
	return jobID, nil
}

// removeIntervalJob cancels a runtime interval job by its scheduler id,
// emitting a stopped envelope carrying the same identity as the started one.
func (b *eventBus) removeIntervalJob(jobID string) {
	if b == nil || b.scheduler == nil || jobID == "" {
		return
	}
	job, cancelled := b.scheduler.cancel(jobID)
	if cancelled && b.observer != nil {
		env := manageEnvelope{Type: "schedule"}
		env.Schedule = &manageScheduleEnvelope{
			Action:    "stopped",
			ExampleID: cmp.Or(job.exampleID, jobID),
			Channel:   job.channel,
			Interval:  int(job.interval.Milliseconds()),
		}
		b.observer(env)
	}
}

// deliverPeriodic renders a periodically driven example against current state
// and environment and broadcasts it to the channel's consumers.
func (b *eventBus) deliverPeriodic(address, prefix string, spec *loader.MessageExampleSpec, opID string) {
	view := &MessageExampleView{spec: spec}
	body := b.renderExample(view, b.stateEnvEvaluator(prefix), prefix, opID)
	if body == nil {
		return
	}
	b.notifyPush(address, "", body)
	b.bus.SignalRPush(address, body)
	b.bus.WSBroadcast(address, body)
}

// derivedExamples maps one spec example into the example specs to register.
// An example without x-send-events maps to itself. A legacy x-send-events
// example maps through the deprecation shim: each entry becomes the unified
// form ({on} → {$event.name} match, {on: cron, wait} → x-mock-interval) with a
// verbose-mode deprecation note (RS.EVT.18).
func (b *eventBus) derivedExamples(ex *asyncapi.Example) ([]*loader.MessageExampleSpec, error) {
	events, err := parseSendEvents(ex.Extensions)
	if err != nil {
		return nil, fmt.Errorf("example %q: %w", ex.Name, err)
	}
	if len(events) == 0 {
		return []*loader.MessageExampleSpec{{
			Name:       ex.Name,
			Headers:    ex.Headers,
			Payload:    ex.Payload,
			Extensions: ex.Extensions,
		}}, nil
	}
	out := make([]*loader.MessageExampleSpec, 0, len(events))
	for _, ev := range events {
		ext := cloneExtensions(ex.Extensions)
		delete(ext, xSendEventsKey)
		if ev.On == "cron" {
			if ev.Wait <= 0 {
				return nil, fmt.Errorf("example %q: x-send-events {on: cron} requires a positive wait interval (use x-mock-interval)", ex.Name)
			}
			if b.verbose {
				slog.Warn("x-send-events is deprecated; use x-mock-interval", "example", ex.Name)
			}
			ext["x-mock-interval"] = ev.Wait
		} else {
			if b.verbose {
				slog.Warn("x-send-events is deprecated; use x-mock-match: {'{$event.name}': <name>}", "example", ex.Name)
			}
			match, _ := ext["x-mock-match"].(map[string]any)
			if match == nil {
				match = make(map[string]any)
			}
			match["{$event.name}"] = ev.On
			ext["x-mock-match"] = match
			if ev.On == "connect" || ev.On == "receive" {
				if ev.On == "connect" && ev.Wait > 0 {
					ext["x-mock-delay"] = ev.Wait
				}
			}
		}
		out = append(out, &loader.MessageExampleSpec{
			Name:       ex.Name,
			Headers:    ex.Headers,
			Payload:    ex.Payload,
			Extensions: ext,
		})
	}
	return out, nil
}

// cloneExtensions deep-copies an example's extension map.
func cloneExtensions(ext map[string]any) map[string]any {
	out := make(map[string]any, len(ext))
	for k, v := range ext {
		out[k] = v
	}
	return out
}

// deliver renders the subscribed message with the event payload and emits it
// to the channel's consumers, narrowing to per-connection recipients when the
// example's match references {$connection.*} (design D6, RS.EXT.24-25). An
// example-level x-mock-delay schedules the emission that far after the fire
// (RS.EXT.23).
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
			b.wait(time.Duration(ms) * time.Millisecond)
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
	eval.AddSource("state", b.renderer.NewStateSource(prefix))
	eval.AddSource("env", b.renderer.NewEnvSource())
	return eval
}

// eventEvaluator wires the fixed emission sources (state, env, event) plus an
// optional per-connection source into a fresh evaluator.
func (b *eventBus) eventEvaluator(state, env runtime.DataSource, eventName string, payload map[string]any, connection *runtime.ConnectionSource) runtime.Evaluator {
	eval := runtime.NewEvaluator()
	eval.AddSource("state", state)
	eval.AddSource("env", env)
	eval.AddSource("event", &runtime.EventSource{Name: eventName, Data: payload})
	if connection != nil {
		eval.AddSource("connection", connection)
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
