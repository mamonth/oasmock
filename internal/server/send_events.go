package server

import (
	"fmt"

	"github.com/mamonth/oasmock/internal/extensions"
)

// xSendEventsKey is the extension key carrying event subscriptions on an
// AsyncAPI message example.
const xSendEventsKey = "x-send-events"

// SendEvent is a single x-send-events subscription entry.
type SendEvent struct {
	// On is a named event or a built-in trigger (connect, receive, cron).
	On string
	// Wait is the optional delay/interval in milliseconds for built-ins.
	Wait int
}

// parseSendEvents parses the x-send-events extension on an AsyncAPI message
// example. Each entry is {on: <event>, wait?: ms} or a bare built-in string
// (RS.EVT.7-11).
func parseSendEvents(ext map[string]any) ([]SendEvent, error) {
	raw, ok := ext[xSendEventsKey]
	if !ok {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("x-send-events must be a list")
	}
	var out []SendEvent
	for _, item := range items {
		switch v := item.(type) {
		case string:
			out = append(out, SendEvent{On: v})
		case map[string]any:
			on, ok := v["on"].(string)
			if !ok || on == "" {
				return nil, fmt.Errorf("x-send-events entry must have an 'on' field")
			}
			ev := SendEvent{On: on}
			if wait, ok := extensions.AsMilliseconds(v["wait"]); ok {
				ev.Wait = wait
			}
			out = append(out, ev)
		default:
			return nil, fmt.Errorf("x-send-events entry must be a string or an object")
		}
	}
	return out, nil
}
