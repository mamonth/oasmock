package server

import (
	"cmp"
	"fmt"
	"log/slog"
	"time"

	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/loader"
)

// eventBus orchestrates the event driver (design D3). It owns the subscription
// broker and the interval scheduler, and delegates message rendering/delivery
// to the messageDelivery engine. It never reaches into Server.
type eventBus struct {
	broker    *eventBroker
	scheduler *jobScheduler
	delivery  *messageDelivery
	verbose   bool
	// observer, when set, is invoked with event and schedule envelopes so the
	// management stream can mirror fired events and job lifecycle. Push
	// envelopes are emitted by the delivery engine (RS.AMG.24-27).
	observer func(env manageEnvelope)
}

// setObserver installs the management-stream observer on the bus and the
// delivery engine (pushes).
func (b *eventBus) setObserver(observer func(env manageEnvelope)) {
	b.observer = observer
	b.delivery.setObserver(observer)
}

// newEventBus wires a broker whose delivery goes through the messageDelivery
// engine, plus an interval scheduler.
func newEventBus(renderer MessageRenderer, bus ConsumerBus, verbose bool) *eventBus {
	delivery := newMessageDelivery(renderer, bus, verbose)
	b := &eventBus{
		scheduler: newJobScheduler(),
		delivery:  delivery,
		verbose:   verbose,
	}
	b.broker = &eventBroker{
		byEvent: make(map[string][]channelSubscription),
		deliver: delivery.deliver,
		done:    make(chan struct{}),
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
		b.delivery.deliverTargeted(sub, payload, recipient)
	}
}

// doneChannel returns the bus shutdown signal so callers that schedule work on
// their own goroutines (e.g. delayed management pushes) can cancel it when the
// bus shuts down.
func (b *eventBus) doneChannel() <-chan struct{} {
	if b == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return b.delivery.doneChannel()
}

// hasSubscribers reports whether any event-driven example could match an
// event identity within a schema scope (cheap gate for built-in firing).
func (b *eventBus) hasSubscribers(name, schema string) bool {
	return b.broker != nil && b.broker.hasSubscribers(name, schema)
}

// shutdown cancels all periodic interval jobs and cancels pending delayed
// emissions (broker and delivery) so no delivery happens after shutdown.
func (b *eventBus) shutdown() {
	if b == nil {
		return
	}
	if b.scheduler != nil {
		b.scheduler.shutdown()
	}
	b.delivery.shutdown()
	b.broker.stop()
}

// registerEventSubscriptions scans AsyncAPI schemas, classifies each message
// example by trigger kind (event-driven via {$event.*} match, periodic via
// x-mock-interval, or reply), and registers event-driven subscriptions keyed by
// identity + schema. A legacy x-send-events key, if present, is silently
// ignored. Load errors (mixed match contexts, dual triggers) abort schema setup
// (RS.EXT.20, RS.EXT.28).
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
				if err := b.classifyMessageExample(ch.ID, msg.Name, address, prefix, ex, &subs, &periodic); err != nil {
					return err
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

// classifyMessageExample classifies one spec example into an event
// subscription, a periodic registration, or nothing (a plain reply), appending
// to the commit-stage accumulators. Classification is driven purely by the
// example's x-mock-match/x-mock-interval trigger extensions; a legacy
// x-send-events key, if present, is silently ignored (RS.EVT.18 removed).
func (b *eventBus) classifyMessageExample(channelID, msgName, address, prefix string, ex *asyncapi.Example, subs *[]channelSubscription, periodic *[]periodicRegistration) error {
	if ex == nil {
		return nil
	}
	spec := &loader.MessageExampleSpec{
		Name:       ex.Name,
		Headers:    ex.Headers,
		Payload:    ex.Payload,
		Extensions: ex.Extensions,
	}
	trig, err := extensions.ClassifyTrigger(&MessageExampleView{spec: spec})
	if err != nil {
		return fmt.Errorf("channel %q example %q: %w", channelID, ex.Name, err)
	}
	switch trig.Kind {
	case extensions.TriggerEvent:
		*subs = append(*subs, channelSubscription{
			address: address,
			event:   trig.Identity,
			delay:   trig.Delay,
			messages: []*messageDeliverable{{
				spec:   &loader.MessageSpec{Name: msgName, Examples: []*loader.MessageExampleSpec{spec}},
				prefix: prefix,
			}},
		})
	case extensions.TriggerPeriodic:
		*periodic = append(*periodic, periodicRegistration{
			address: address, prefix: prefix, exampleID: ex.Name, spec: spec, interval: trig.Interval,
		})
	case extensions.TriggerReply:
		// Reply examples are served by the channel's normal reply path;
		// nothing to register here. A match that still references
		// {$connection.*} can never evaluate (no connection context in the
		// reply path), so point it out in verbose mode instead of failing
		// silently.
		if b.verbose && extensions.MatchReferencesConnection(trig.Match) {
			slog.Warn("reply example references {$connection.*} which never matches without an event context; remove the connection condition or make the example event-driven",
				"channel", channelID, "example", ex.Name)
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
			b.delivery.deliverPeriodic(address, prefix, spec, opID)
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

// deliver renders the subscribed message with the event payload and emits it
// to the channel's consumers, narrowing to per-connection recipients when the
// example's match references {$connection.*} (design D6, RS.EXT.24-25). An
// example-level x-mock-delay schedules the emission that far after the fire
// (RS.EXT.23).
