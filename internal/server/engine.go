package server

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mamonth/oasmock/internal/extensions"
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
	if ex, k := e.selectFromBucket(withParamsMatch, keys, eval, opID, true); ex != nil {
		return ex, k
	}
	// No matched params-match examples, try those without params-match
	return e.selectFromBucket(withoutParamsMatch, keys, eval, opID, false)
}

// selectFromBucket returns the first example of a bucket (in key order) that
// passes the optional params-match evaluation, marking x-mock-once examples as
// used. It returns nil when none matches.
func (e *exampleEngine) selectFromBucket(bucket map[string]*openapi3.Example, keys []string, eval runtime.Evaluator, opID string, requireMatch bool) (*openapi3.Example, string) {
	for _, k := range keys {
		ex, ok := bucket[k]
		if !ok {
			continue
		}
		if requireMatch {
			pm, _ := extensions.ExtractParamsMatch(ex)
			matched, err := extensions.EvaluateParamsMatch(pm, eval)
			if err != nil {
				if e.verbose {
					slog.Debug("Error evaluating params-match", "example", k, "error", err)
				}
				continue
			}
			if !matched {
				continue
			}
		}
		if extensions.ExtractOnce(ex) {
			e.registry.markOnceUsed(opID + ":" + k)
			if e.verbose {
				slog.Debug("Marked example as used (x-mock-once)", "example", k)
			}
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
