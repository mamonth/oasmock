package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
)

// ---------------------------------------------------------------------------
// AsyncAPI message rendering
// ---------------------------------------------------------------------------

// renderAsyncMessage selects a message example from the route's AsyncAPI
// message specs (or an injected dynamic example), evaluates runtime
// expressions, applies x-mock-set-state, and returns the rendered payload
// bytes. It returns the number of produced messages (0 when the route has no
// reply message/examples).
func (e *exampleEngine) renderAsyncMessage(mapping *RouteMapping, in InboundMessage) (int, []byte, error) {
	opID := "async:" + mapping.Protocol + ":" + mapping.Pattern

	// Dynamic examples injected via the management API take priority (8.2).
	evaluator := e.newAsyncEvaluator(mapping, in)
	if dyn, _ := e.registry.selectDynamic(routeKey(mapping.Method, mapping.ChiPattern), evaluator); dyn != nil {
		body, err := e.evaluateValue(dyn.response.body, evaluator)
		if err != nil {
			return 0, nil, err
		}
		jsonBody, merr := json.Marshal(body)
		if merr != nil {
			return 0, nil, merr
		}
		return 1, jsonBody, nil
	}

	return e.RenderMessageSpecs(mapping.Messages, mapping.Prefix, opID, in)
}

// newAsyncEvaluator builds an evaluator for an async exchange with the
// protocol-relevant data sources.
func (e *exampleEngine) newAsyncEvaluator(mapping *RouteMapping, in InboundMessage) runtime.Evaluator {
	eval := runtime.NewEvaluator()
	eval.AddSource(runtime.SourceRequest, e.asyncRequestSource(in))
	eval.AddSource(runtime.SourceMessage, &runtime.MessageSource{Payload: jsonPayload(in.Payload), Headers: in.Headers})
	eval.AddSource(runtime.SourceChannel, &runtime.ChannelSource{Params: in.PathParams})
	eval.AddSource(runtime.SourceState, e.NewStateSource(mapping.Prefix))
	eval.AddSource(runtime.SourceEnv, e.NewEnvSource())
	return eval
}

// renderMessageSpecs renders the first selectable example across the given
// message specs using the shared selection pipeline (design D5). The evaluator
// exposes {$request.*}, {$message.*}, {$channel.*}, {$state.*} and {$env.*}.
func (e *exampleEngine) RenderMessageSpecs(messages []*loader.MessageSpec, prefix, opID string, in InboundMessage) (int, []byte, error) {
	evaluator := runtime.NewEvaluator()
	evaluator.AddSource(runtime.SourceRequest, e.asyncRequestSource(in))
	evaluator.AddSource(runtime.SourceMessage, &runtime.MessageSource{
		Payload: jsonPayload(in.Payload),
		Headers: in.Headers,
	})
	evaluator.AddSource(runtime.SourceChannel, &runtime.ChannelSource{Params: in.PathParams})
	evaluator.AddSource(runtime.SourceState, e.NewStateSource(prefix))
	evaluator.AddSource(runtime.SourceEnv, e.NewEnvSource())

	for _, msg := range messages {
		example, _ := e.SelectAsyncExample(msg, evaluator, opID)
		if example == nil {
			continue
		}
		if stateMap, ok := extensions.ValueSetState(example); ok {
			e.ApplySetState(stateMap, evaluator, prefix)
		}
		body, err := e.RenderAsyncPayload(example, evaluator)
		if err != nil {
			return 0, nil, err
		}
		return 1, body, nil
	}
	return 0, nil, nil
}

// asyncRequestSource adapts an InboundMessage to a runtime data source.
func (e *exampleEngine) asyncRequestSource(in InboundMessage) *runtime.RequestSource {
	headers := make(map[string][]string, len(in.Headers))
	for k, v := range in.Headers {
		headers[k] = []string{v}
	}
	return &runtime.RequestSource{
		PathParams:  in.PathParams,
		QueryParams: nil,
		Headers:     headers,
		Body:        jsonPayload(in.Payload),
		Cookies:     nil,
	}
}

// selectAsyncExample selects a message example using the x-mock-* semantics
// (skip, once, params-match) shared with the OpenAPI pipeline.
func (e *exampleEngine) SelectAsyncExample(message *loader.MessageSpec, evaluator runtime.Evaluator, opID string) (*MessageExampleView, string) {
	if message == nil {
		return nil, ""
	}
	indices := make([]int, 0, len(message.Examples))
	for i := range message.Examples {
		indices = append(indices, i)
	}
	slices.Sort(indices)

	for _, idx := range indices {
		example := message.Examples[idx]
		exampleKey := idxName(message.Name, idx)
		if example == nil {
			continue
		}
		view := &MessageExampleView{spec: example}
		if extensions.ValueSkip(view) {
			continue
		}
		onceID := opID + ":" + exampleKey
		if extensions.ValueOnce(view) && e.registry.isOnceUsed(onceID) {
			continue
		}
		if match, ok := extensions.ValueMatch(view); ok {
			matched, err := extensions.EvaluateParamsMatch(extensions.ParamsMatch(match), evaluator, e.verbose)
			if err != nil || !matched {
				continue
			}
		}
		if extensions.ValueOnce(view) {
			e.registry.markOnceUsed(onceID)
		}
		return view, exampleKey
	}
	return nil, ""
}

// renderAsyncPayload evaluates runtime expressions in a message example's
// payload and returns the JSON bytes.
func (e *exampleEngine) RenderAsyncPayload(example *MessageExampleView, evaluator runtime.Evaluator) ([]byte, error) {
	payload := example.Payload()
	if payload == nil {
		payload = map[string]any{}
	}
	resolved, err := e.evaluateValue(payload, evaluator)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate message payload: %w", err)
	}
	if e.verbose {
		slog.Debug("Rendered AsyncAPI message payload", "payload", resolved)
	}
	return json.Marshal(resolved)
}

// recordAsyncExchange records an AsyncAPI message exchange in the request
// history store (RS.ATM.15).
func (e *exampleEngine) recordAsyncExchange(in InboundMessage, address string, status int, responseBody []byte) {
	headers := make(http.Header, len(in.Headers))
	for k, v := range in.Headers {
		headers.Set(k, v)
	}
	now := time.Now()
	record := RequestRecord{
		ID:        fmt.Sprintf("%d", now.UnixNano()),
		Timestamp: now,
		Method:    "async",
		Path:      address,
		Query:     "",
		Headers:   headers,
		Body:      in.Payload,
		Response: &ResponseRecord{
			StatusCode: status,
			Headers:    http.Header{},
			Body:       responseBody,
			Duration:   0,
		},
	}
	e.historyStore.Add(record)
}

// newStateSource builds a runtime state source for a schema namespace.
func (e *exampleEngine) NewStateSource(prefix string) *runtime.StateSource {
	data := e.stateStore.GetNamespace(prefix)
	if data == nil {
		data = make(map[string]any)
	}
	return &runtime.StateSource{Data: data}
}

// newEnvSource builds a runtime environment-variable source.
func (e *exampleEngine) NewEnvSource() *runtime.EnvSource {
	env := make(map[string]string)
	for _, item := range os.Environ() {
		if key, val, found := strings.Cut(item, "="); found {
			env[key] = val
		}
	}
	return &runtime.EnvSource{Env: env}
}
