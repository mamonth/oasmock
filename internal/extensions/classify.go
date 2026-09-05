package extensions

import "fmt"

// TriggerKind classifies how an AsyncAPI message example is driven at load.
type TriggerKind int

const (
	// TriggerReply is a plain sync/async reply example (no event match, no
	// interval; may still carry a request/message/channel match).
	TriggerReply TriggerKind = iota
	// TriggerEvent is event-driven: its x-mock-match references {$event.*}.
	TriggerEvent
	// TriggerPeriodic declares x-mock-interval for recurring emission.
	TriggerPeriodic
)

// Trigger is the classification result for one message example.
type Trigger struct {
	// Kind is the driving mechanism.
	Kind TriggerKind
	// Identity is the event identity for event triggers (the {$event.name}
	// condition value, or the built-in connect/receive kind).
	Identity string
	// Interval is the millisecond cadence for periodic triggers.
	Interval int
	// Delay is the millisecond emission delay for event triggers.
	Delay int
	// Match is the example's x-mock-match conditions (any kind).
	Match map[string]any
}

// ClassifyTrigger classifies a message example from its x-mock-match timing
// extensions (design D3). It rejects mixed match contexts ({$event.*} with
// {$request.*}/{$message.*}/{$channel.*}, RS.EXT.20), dual triggers (interval
// alongside any x-mock-match, RS.EXT.28), non-literal event identities
// (a {$event.name} condition whose value is itself an expression, since the
// produced subscription key could never match a fired identity) and
// declared-but-invalid timing values (a non-positive or fractional
// x-mock-interval or a negative/fractional x-mock-delay, RS.EXT.22-23) instead
// of silently reclassifying the example.
func ClassifyTrigger(ev ExampleValue) (Trigger, error) {
	var trig Trigger
	match, hasMatch := ValueMatch(ev)
	trig.Match = match

	if hasMatch && MatchMixedContext(match) {
		return trig, fmt.Errorf("x-mock-match mixes event conditions with request/message/channel conditions; an example may target a single context")
	}

	if interval, hasInterval := ValueInterval(ev); hasInterval {
		if hasMatch {
			return trig, fmt.Errorf("example declares both x-mock-interval and x-mock-match; a periodically driven example cannot carry a match — use one trigger")
		}
		trig.Kind = TriggerPeriodic
		trig.Interval = interval
		return trig, nil
	}
	// A declared-but-invalid interval is a configuration error, not a silent
	// reclassification to a reply (RS.EXT.22).
	if declaredInterval(ev) {
		return trig, fmt.Errorf("x-mock-interval must be a positive integer")
	}

	if hasDelay(ev) {
		delay, ok := ValueDelay(ev)
		if !ok {
			return trig, fmt.Errorf("x-mock-delay must be an integer number of milliseconds")
		}
		if delay < 0 {
			return trig, fmt.Errorf("x-mock-delay cannot be negative")
		}
		trig.Delay = delay
	}

	if hasMatch && MatchReferencesEvent(match) {
		trig.Kind = TriggerEvent
		if identity, ok := EventIdentity(match); ok {
			if isFullExpression(identity) {
				return trig, fmt.Errorf("{$event.name} identity must be a literal string, not a runtime expression (got %q)", identity)
			}
			trig.Identity = identity
		}
		return trig, nil
	}

	trig.Kind = TriggerReply
	return trig, nil
}

// declaredInterval reports whether the example declares an x-mock-interval
// extension (regardless of value validity).
func declaredInterval(ev ExampleValue) bool {
	if ev == nil {
		return false
	}
	_, ok := ev.Get("x-mock-interval")
	return ok
}

// hasDelay reports whether the example declares an x-mock-delay extension.
func hasDelay(ev ExampleValue) bool {
	if ev == nil {
		return false
	}
	_, ok := ev.Get("x-mock-delay")
	return ok
}
