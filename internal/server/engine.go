package server

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
)

// exampleEngine is the shared example-selection and templating core for both
// the OpenAPI and AsyncAPI pipelines. It owns runtime-expression evaluation,
// x-mock-* extension handling, state mutation and async message rendering.
// The event/hub/management subsystems depend on it through narrow interfaces
// instead of on the whole Server.
type exampleEngine struct {
	verbose      bool
	stateStore   StateStore
	historyStore HistoryStore
	registry     *exampleRegistry
}

func newExampleEngine(config Config, deps Dependencies, registry *exampleRegistry) *exampleEngine {
	return &exampleEngine{
		verbose:      config.Verbose,
		stateStore:   deps.StateStore,
		historyStore: deps.HistoryStore,
		registry:     registry,
	}
}

// ---------------------------------------------------------------------------
// Runtime expression evaluation
// ---------------------------------------------------------------------------

func (e *exampleEngine) replaceEmbeddedExpressions(str string, eval runtime.Evaluator) (string, error) {
	var result strings.Builder
	i := 0
	for i < len(str) {
		// Find start of expression "{$"
		start := strings.Index(str[i:], "{$")
		if start == -1 {
			result.WriteString(str[i:])
			break
		}
		start += i
		// Write literal part before expression
		result.WriteString(str[i:start])
		// Find matching '}'
		braceDepth := 1
		j := start + 2
		for j < len(str) && braceDepth > 0 {
			ch := str[j]
			if ch == '{' && j+1 < len(str) && str[j+1] == '$' {
				braceDepth++
				j += 2
				continue
			}
			if ch == '}' {
				braceDepth--
				if braceDepth == 0 {
					break
				}
			}
			j++
		}
		if braceDepth != 0 {
			// Unmatched braces, treat as literal
			result.WriteString(str[start:])
			break
		}
		// j now points at the closing '}'
		end := j
		expr := str[start : end+1]
		// Evaluate expression
		value, err := eval.Evaluate(expr)
		if err != nil {
			// If evaluation fails, keep the original expression
			result.WriteString(expr)
		} else {
			// Convert value to string
			switch v := value.(type) {
			case string:
				result.WriteString(v)
			default:
				b, err := json.Marshal(v)
				if err != nil {
					result.WriteString(expr)
				} else {
					result.Write(b)
				}
			}
		}
		i = end + 1
	}
	return result.String(), nil
}

func (e *exampleEngine) evaluateExpressionInString(str string, eval runtime.Evaluator) (string, error) {
	// First, check if the whole string is a runtime expression (optimization)
	if strings.HasPrefix(str, "{$") && strings.HasSuffix(str, "}") && !strings.Contains(str[2:], "{$") {
		result, err := eval.Evaluate(str)
		if err != nil {
			return "", err
		}
		// Convert result to string
		switch v := result.(type) {
		case string:
			return v, nil
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
	}
	// Otherwise replace embedded expressions
	return e.replaceEmbeddedExpressions(str, eval)
}

func (e *exampleEngine) evaluateValue(val any, eval runtime.Evaluator) (any, error) {
	// Handle strings: they may contain embedded runtime expressions
	if str, ok := val.(string); ok {
		// Check if the whole string is a single runtime expression (no other characters)
		if strings.HasPrefix(str, "{$") && strings.HasSuffix(str, "}") && strings.Count(str, "{$") == 1 {
			return eval.Evaluate(str)
		}
		// Otherwise replace embedded expressions
		return e.replaceEmbeddedExpressions(str, eval)
	}
	// Recursively handle maps and slices
	switch v := val.(type) {
	case map[string]any:
		result := make(map[string]any)
		for k, item := range v {
			resolvedK, err := e.evaluateExpressionInString(k, eval)
			if err != nil {
				return nil, err
			}
			resolvedItem, err := e.evaluateValue(item, eval)
			if err != nil {
				return nil, err
			}
			result[resolvedK] = resolvedItem
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			resolvedItem, err := e.evaluateValue(item, eval)
			if err != nil {
				return nil, err
			}
			result[i] = resolvedItem
		}
		return result, nil
	default:
		// Literal value
		return val, nil
	}
}

// ---------------------------------------------------------------------------
// State mutation (x-mock-set-state)
// ---------------------------------------------------------------------------

func (e *exampleEngine) handleDeleteState(prefix, resolvedKey string) {
	e.stateStore.Delete(prefix, resolvedKey)
	if e.verbose {
		slog.Debug("Deleted state", "key", resolvedKey, "namespace", prefix)
	}
}

func (e *exampleEngine) handleIncrementState(prefix, resolvedKey string, incVal any, eval runtime.Evaluator) error {
	resolvedInc, err := e.evaluateValue(incVal, eval)
	if err != nil {
		if e.verbose {
			slog.Debug("Failed to evaluate increment value", "error", err)
		}
		return err
	}
	// Convert to float64
	var delta float64
	switch v := resolvedInc.(type) {
	case float64:
		delta = v
	case int:
		delta = float64(v)
	case string:
		// Try to parse as number
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			delta = f
		} else {
			if e.verbose {
				slog.Debug("Increment value is not a number", "value", v)
			}
			return fmt.Errorf("increment value is not a number: %s", v)
		}
	default:
		if e.verbose {
			slog.Debug("Increment value has unsupported type", "type", fmt.Sprintf("%T", v))
		}
		return fmt.Errorf("increment value has unsupported type: %T", v)
	}
	newVal, err := e.stateStore.Increment(prefix, resolvedKey, delta)
	if err != nil {
		if e.verbose {
			slog.Debug("Failed to increment state", "key", resolvedKey, "error", err)
		}
		return err
	}
	if e.verbose {
		slog.Debug("Incremented state", "key", resolvedKey, "namespace", prefix, "delta", delta, "newValue", newVal)
	}
	return nil
}

func (e *exampleEngine) handleValueObjectState(prefix, resolvedKey string, valObj any, eval runtime.Evaluator) error {
	resolvedVal, err := e.evaluateValue(valObj, eval)
	if err != nil {
		if e.verbose {
			slog.Debug("Failed to evaluate value object", "error", err)
		}
		return err
	}
	e.stateStore.Set(prefix, resolvedKey, resolvedVal)
	if e.verbose {
		slog.Debug("Set state", "key", resolvedKey, "namespace", prefix, "value", resolvedVal)
	}
	return nil
}

func (e *exampleEngine) handleMapState(prefix, resolvedKey string, m map[string]any, eval runtime.Evaluator) (handled bool, err error) {
	if incVal, hasInc := m["increment"]; hasInc {
		err = e.handleIncrementState(prefix, resolvedKey, incVal, eval)
		return true, err
	}
	if valObj, hasVal := m["value"]; hasVal {
		err = e.handleValueObjectState(prefix, resolvedKey, valObj, eval)
		return true, err
	}
	return false, nil
}

func (e *exampleEngine) handleSimpleState(prefix, resolvedKey string, val any, eval runtime.Evaluator) error {
	resolvedVal, err := e.evaluateValue(val, eval)
	if err != nil {
		if e.verbose {
			slog.Debug("Failed to evaluate value for key", "key", resolvedKey, "error", err)
		}
		return err
	}
	e.stateStore.Set(prefix, resolvedKey, resolvedVal)
	if e.verbose {
		slog.Debug("Set state", "key", resolvedKey, "namespace", prefix, "value", resolvedVal)
	}
	return nil
}

func (e *exampleEngine) ApplySetState(stateMap map[string]any, eval runtime.Evaluator, prefix string) {
	for key, val := range stateMap {
		// Evaluate runtime expressions in key
		resolvedKey, err := e.evaluateExpressionInString(key, eval)
		if err != nil {
			if e.verbose {
				slog.Debug("Failed to evaluate key", "key", key, "error", err)
			}
			continue
		}

		// Handle null value (delete)
		if val == nil {
			e.handleDeleteState(prefix, resolvedKey)
			continue
		}

		// Handle map (increment or value object)
		if m, ok := val.(map[string]any); ok {
			handled, _ := e.handleMapState(prefix, resolvedKey, m, eval)
			if handled {
				// Error already logged inside helpers
				continue
			}
			// Not a recognized map structure, fall through to simple value
		}

		// Simple value (could be runtime expression)
		if err := e.handleSimpleState(prefix, resolvedKey, val, eval); err != nil {
			// Error already logged inside helper
			continue
		}
	}
}

// ---------------------------------------------------------------------------
// OpenAPI example selection & response generation
// ---------------------------------------------------------------------------

func (e *exampleEngine) selectResponse(mapping *RouteMapping, eval runtime.Evaluator) (string, *openapi3.Response) {
	if mapping.Responses == nil {
		return "", nil
	}
	respMap := mapping.Responses.Map()
	if len(respMap) == 0 {
		return "", nil
	}
	// Collect and sort keys for deterministic selection
	keys := make([]string, 0, len(respMap))
	for code := range respMap {
		keys = append(keys, code)
	}
	// Sort keys with custom order: numeric status codes ascending, "default" last
	slices.SortFunc(keys, func(a, b string) int {
		if a == "default" && b == "default" {
			return 0
		}
		if a == "default" {
			return 1 // default after numeric codes
		}
		if b == "default" {
			return -1
		}
		aInt, errA := strconv.Atoi(a)
		bInt, errB := strconv.Atoi(b)
		if errA != nil && errB != nil {
			return strings.Compare(a, b) // fallback lexical
		}
		if errA != nil {
			return 1 // non-numeric after numeric
		}
		if errB != nil {
			return -1
		}
		return cmp.Compare(aInt, bInt)
	})
	// Iterate sorted keys
	for _, code := range keys {
		resp := respMap[code]
		if resp != nil && resp.Value != nil {
			return code, resp.Value
		}
	}
	return "", nil
}

func (e *exampleEngine) selectMediaType(response *openapi3.Response) (string, *openapi3.MediaType, error) {
	if response.Content == nil {
		return "", nil, fmt.Errorf("no media type defined for response")
	}
	// Collect keys for deterministic selection
	keys := make([]string, 0, len(response.Content))
	for mt := range response.Content {
		keys = append(keys, mt)
	}
	if len(keys) == 0 {
		return "", nil, fmt.Errorf("no media type defined for response")
	}
	slices.Sort(keys)
	// Select first media type after sorting
	mt := keys[0]
	obj := response.Content[mt]
	return mt, obj, nil
}

func (e *exampleEngine) generateResponse(example *openapi3.Example, dynExample *dynamicExample, eval runtime.Evaluator, currentStatusCode string) (body []byte, headers map[string]string, statusCode string, err error) {
	if example != nil {
		body, err = e.evaluateExample(example, eval)
		if err != nil {
			return nil, nil, "", fmt.Errorf("failed to evaluate example: %w", err)
		}
		headers = e.evaluateHeaders(example, eval)
		statusCode = currentStatusCode
		return
	}
	// dynExample != nil
	resolvedBody, err := e.evaluateValue(dynExample.response.body, eval)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to evaluate dynamic example body: %w", err)
	}
	body, err = json.Marshal(resolvedBody)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to marshal response body: %w", err)
	}
	headers = dynExample.response.headers
	// Evaluate runtime expressions in header values
	for k, v := range headers {
		resolved, err := e.evaluateExpressionInString(v, eval)
		if err == nil {
			headers[k] = resolved
		}
	}
	statusCode = strconv.Itoa(dynExample.response.code)
	return
}

func (e *exampleEngine) selectExample(mediaType *openapi3.MediaType, eval runtime.Evaluator, opID string) (*openapi3.Example, string) {
	if mediaType.Examples == nil {
		return nil, ""
	}
	keys := slices.Collect(maps.Keys(mediaType.Examples))
	slices.Sort(keys)
	withParamsMatch, withoutParamsMatch := e.categorizeExamples(mediaType.Examples, keys, eval, opID)

	// First, try examples with params-match
	for _, k := range keys {
		ex, ok := withParamsMatch[k]
		if !ok {
			continue
		}
		pm, _ := extensions.ExtractParamsMatch(ex)
		if e.verbose {
			slog.Debug("Example has x-mock-params-match", "example", k, "params", pm)
		}
		matched, err := extensions.EvaluateParamsMatch(pm, eval)
		if err != nil {
			if e.verbose {
				slog.Debug("Error evaluating params-match", "example", k, "error", err)
			}
			continue
		}
		if e.verbose {
			slog.Debug("Example params-match result", "example", k, "matched", matched)
		}
		if matched {
			if extensions.ExtractOnce(ex) {
				exampleID := opID + ":" + k
				e.registry.markOnceUsed(exampleID)
				if e.verbose {
					slog.Debug("Marked example as used (x-mock-once)", "example", k)
				}
			}
			return ex, k
		}
	}

	// No matched params-match examples, try those without params-match
	for _, k := range keys {
		ex, ok := withoutParamsMatch[k]
		if !ok {
			continue
		}
		if extensions.ExtractOnce(ex) {
			exampleID := opID + ":" + k
			e.registry.markOnceUsed(exampleID)
			if e.verbose {
				slog.Debug("Marked example as used (x-mock-once)", "example", k)
			}
		}
		if e.verbose {
			slog.Debug("Selecting example (no params-match)", "example", k)
		}
		return ex, k
	}
	return nil, ""
}

func (e *exampleEngine) applyExtensions(example *openapi3.Example, eval runtime.Evaluator, prefix string) {
	// Apply x-mock-set-state
	if stateMap, ok := extensions.ExtractSetState(example); ok {
		e.ApplySetState(stateMap, eval, prefix)
	}
	// x-mock-headers handled separately in evaluateHeaders
	// x-mock-once is handled in selectExample
}

func (e *exampleEngine) shouldSkipExample(ex *openapi3.Example, exampleKey, opID string) bool {
	if extensions.ExtractSkip(ex) {
		if e.verbose {
			slog.Debug("Example skipped via x-mock-skip", "example", exampleKey)
		}
		return true
	}
	if extensions.ExtractOnce(ex) {
		exampleID := opID + ":" + exampleKey
		if e.registry.isOnceUsed(exampleID) {
			if e.verbose {
				slog.Debug("Example skipped via x-mock-once (already used)", "example", exampleKey)
			}
			return true
		}
	}
	return false
}

func (e *exampleEngine) categorizeExamples(examples openapi3.Examples, keys []string, eval runtime.Evaluator, opID string) (withParamsMatch, withoutParamsMatch map[string]*openapi3.Example) {
	withParamsMatch = make(map[string]*openapi3.Example)
	withoutParamsMatch = make(map[string]*openapi3.Example)
	for _, k := range keys {
		exRef := examples[k]
		if exRef == nil || exRef.Value == nil {
			continue
		}
		ex := exRef.Value
		if e.shouldSkipExample(ex, k, opID) {
			continue
		}
		if _, ok := extensions.ExtractParamsMatch(ex); ok {
			withParamsMatch[k] = ex
		} else {
			withoutParamsMatch[k] = ex
		}
	}
	return
}

func (e *exampleEngine) evaluateExample(example *openapi3.Example, eval runtime.Evaluator) ([]byte, error) {
	if example.Value == nil {
		return []byte{}, nil
	}
	// Evaluate runtime expressions in the value
	resolved, err := e.evaluateValue(example.Value, eval)
	if err != nil {
		return nil, err
	}
	// Convert to JSON
	return json.Marshal(resolved)
}

func (e *exampleEngine) evaluateHeaders(example *openapi3.Example, eval runtime.Evaluator) map[string]string {
	headers := make(map[string]string)

	if headersMap, ok := extensions.ExtractHeaders(example); ok {
		for key, val := range headersMap {
			if str, ok := e.resolveHeaderValue(val, eval); ok {
				headers[key] = str
			}
		}
	}

	return headers
}

func (e *exampleEngine) resolveHeaderValue(val any, eval runtime.Evaluator) (string, bool) {
	switch v := val.(type) {
	case string:
		resolved, err := e.evaluateValue(v, eval)
		if err != nil {
			if e.verbose {
				slog.Debug("Failed to evaluate header value", "headerValue", v, "error", err)
			}
			return "", false
		}
		if str, ok := resolved.(string); ok {
			return str, true
		}
		// Convert to JSON string
		b, err := json.Marshal(resolved)
		if err != nil {
			return "", false
		}
		return string(b), true
	case []any:
		// Multiple header values - join with comma (except for Set-Cookie which should be separate headers)
		// For simplicity, just take the first value for now
		if len(v) > 0 {
			if first, ok := v[0].(string); ok {
				resolved, err := e.evaluateValue(first, eval)
				if err == nil {
					if str, ok := resolved.(string); ok {
						return str, true
					}
				}
			}
		}
	default:
		// Try to evaluate as runtime expression
		resolved, err := e.evaluateValue(val, eval)
		if err == nil {
			if str, ok := resolved.(string); ok {
				return str, true
			}
		}
	}
	return "", false
}

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
	eval.AddSource("request", e.asyncRequestSource(in))
	eval.AddSource("message", &runtime.MessageSource{Payload: jsonPayload(in.Payload), Headers: in.Headers})
	eval.AddSource("channel", &runtime.ChannelSource{Params: in.PathParams})
	eval.AddSource("state", e.NewStateSource(mapping.Prefix))
	eval.AddSource("env", e.NewEnvSource())
	return eval
}

// renderMessageSpecs renders the first selectable example across the given
// message specs using the shared selection pipeline (design D5). The evaluator
// exposes {$request.*}, {$message.*}, {$channel.*}, {$state.*} and {$env.*}.
func (e *exampleEngine) RenderMessageSpecs(messages []*loader.MessageSpec, prefix, opID string, in InboundMessage) (int, []byte, error) {
	evaluator := runtime.NewEvaluator()
	evaluator.AddSource("request", e.asyncRequestSource(in))
	evaluator.AddSource("message", &runtime.MessageSource{
		Payload: jsonPayload(in.Payload),
		Headers: in.Headers,
	})
	evaluator.AddSource("channel", &runtime.ChannelSource{Params: in.PathParams})
	evaluator.AddSource("state", e.NewStateSource(prefix))
	evaluator.AddSource("env", e.NewEnvSource())

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

// renderMessageSpecsWithEvent evaluates message specs with an event payload
// registered in the evaluator as {$event.*}.
func (e *exampleEngine) RenderMessageSpecsWithEvent(messages []*loader.MessageSpec, prefix, opID string, payload map[string]any) (int, []byte, error) {
	evaluator := runtime.NewEvaluator()
	evaluator.AddSource("state", e.NewStateSource(prefix))
	evaluator.AddSource("env", e.NewEnvSource())
	evaluator.AddSource("event", &runtime.EventSource{Data: payload})

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
			matched, err := extensions.EvaluateParamsMatch(extensions.ParamsMatch(match), evaluator)
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
