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
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
)

type dynamicExample struct {
	once       bool
	conditions map[string]any
	response   struct {
		code    int
		headers map[string]string
		body    any
	}
}

func (s *Server) selectResponse(mapping *RouteMapping, eval runtime.Evaluator) (string, *openapi3.Response) {
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

func (s *Server) selectMediaType(response *openapi3.Response) (string, *openapi3.MediaType, error) {
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

func (s *Server) generateResponse(example *openapi3.Example, dynExample *dynamicExample, eval runtime.Evaluator, currentStatusCode string) (body []byte, headers map[string]string, statusCode string, err error) {
	if example != nil {
		body, err = s.evaluateExample(example, eval)
		if err != nil {
			return nil, nil, "", fmt.Errorf("failed to evaluate example: %w", err)
		}
		headers = s.evaluateHeaders(example, eval)
		statusCode = currentStatusCode
		return
	}
	// dynExample != nil
	resolvedBody, err := s.evaluateValue(dynExample.response.body, eval)
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
		resolved, err := s.evaluateExpressionInString(v, eval)
		if err == nil {
			headers[k] = resolved
		}
	}
	statusCode = strconv.Itoa(dynExample.response.code)
	return
}

func (s *Server) selectExample(mediaType *openapi3.MediaType, eval runtime.Evaluator, opID string) (*openapi3.Example, string) {
	if mediaType.Examples == nil {
		return nil, ""
	}
	keys := slices.Collect(maps.Keys(mediaType.Examples))
	slices.Sort(keys)
	withParamsMatch, withoutParamsMatch := s.categorizeExamples(mediaType.Examples, keys, eval, opID)

	// First, try examples with params-match
	for _, k := range keys {
		ex, ok := withParamsMatch[k]
		if !ok {
			continue
		}
		pm, _ := extensions.ExtractParamsMatch(ex)
		if s.config.Verbose {
			slog.Debug("Example has x-mock-params-match", "example", k, "params", pm)
		}
		matched, err := extensions.EvaluateParamsMatch(pm, eval)
		if err != nil {
			if s.config.Verbose {
				slog.Debug("Error evaluating params-match", "example", k, "error", err)
			}
			continue
		}
		if s.config.Verbose {
			slog.Debug("Example params-match result", "example", k, "matched", matched)
		}
		if matched {
			if extensions.ExtractOnce(ex) {
				exampleID := opID + ":" + k
				s.markOnceUsed(exampleID)
				if s.config.Verbose {
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
			s.markOnceUsed(exampleID)
			if s.config.Verbose {
				slog.Debug("Marked example as used (x-mock-once)", "example", k)
			}
		}
		if s.config.Verbose {
			slog.Debug("Selecting example (no params-match)", "example", k)
		}
		return ex, k
	}
	return nil, ""
}

func (s *Server) selectDynamicExample(mapping *RouteMapping, eval runtime.Evaluator) (*dynamicExample, string) {
	key := routeKey(mapping.Method, mapping.ChiPattern)
	if s.config.Verbose {
		slog.Debug("selectDynamicExample", "key", key, "numExamples", len(s.dynamicExamples[key]))
	}
	s.dyMu.RLock()
	examples := s.dynamicExamples[key]
	s.dyMu.RUnlock()
	for idx, ex := range examples {
		if s.config.Verbose {
			slog.Debug("selectDynamicExample: checking example",
				"idx", idx,
				"once", ex.once,
				"conditions", len(ex.conditions))
		}
		// Check once flag
		if ex.once {
			onceID := fmt.Sprintf("dynamic:%s:%d", key, idx)
			if s.isOnceUsed(onceID) {
				if s.config.Verbose {
					slog.Debug("selectDynamicExample: example already used", "onceID", onceID)
				}
				continue
			}
		}
		// Evaluate conditions
		if len(ex.conditions) > 0 {
			// Convert to ParamsMatch
			pm := extensions.ParamsMatch(ex.conditions)
			matched, err := extensions.EvaluateParamsMatch(pm, eval)
			if s.config.Verbose {
				slog.Debug("selectDynamicExample: condition evaluation result",
					"matched", matched, "err", err, "conditions", ex.conditions)
			}
			if err != nil || !matched {
				continue
			}
		} else if s.config.Verbose {
			slog.Debug("selectDynamicExample: no conditions, matching")
		}
		// Matched
		if ex.once {
			onceID := fmt.Sprintf("dynamic:%s:%d", key, idx)
			s.markOnceUsed(onceID)
		}
		if s.config.Verbose {
			slog.Debug("selectDynamicExample: returning matched example", "idx", idx)
		}
		return &ex, fmt.Sprintf("dynamic:%d", idx)
	}
	if s.config.Verbose {
		slog.Debug("selectDynamicExample: no matching examples found", "key", key)
	}
	return nil, ""
}

func (s *Server) applyExtensions(example *openapi3.Example, eval runtime.Evaluator, prefix string) {
	// Apply x-mock-set-state
	if stateMap, ok := extensions.ExtractSetState(example); ok {
		s.applySetState(stateMap, eval, prefix)
	}
	// x-mock-headers handled separately in evaluateHeaders
	// x-mock-once is handled in selectExample
}

// markOnceUsed marks an example as used (for x-mock-once).
// The ID should uniquely identify the example (e.g., operation path + method + example key).
func (s *Server) markOnceUsed(id string) {
	s.onceMu.Lock()
	defer s.onceMu.Unlock()
	s.onceExamples[id] = true
}

// isOnceUsed checks if an example has been used.
func (s *Server) isOnceUsed(id string) bool {
	s.onceMu.RLock()
	defer s.onceMu.RUnlock()
	return s.onceExamples[id]
}

func (s *Server) shouldSkipExample(ex *openapi3.Example, exampleKey, opID string) bool {
	if extensions.ExtractSkip(ex) {
		if s.config.Verbose {
			slog.Debug("Example skipped via x-mock-skip", "example", exampleKey)
		}
		return true
	}
	if extensions.ExtractOnce(ex) {
		exampleID := opID + ":" + exampleKey
		if s.isOnceUsed(exampleID) {
			if s.config.Verbose {
				slog.Debug("Example skipped via x-mock-once (already used)", "example", exampleKey)
			}
			return true
		}
	}
	return false
}

// categorizeExamples iterates over keys and categorizes examples into those with and without params-match.
// Returns maps from key to example for each category.
func (s *Server) categorizeExamples(examples openapi3.Examples, keys []string, eval runtime.Evaluator, opID string) (withParamsMatch, withoutParamsMatch map[string]*openapi3.Example) {
	withParamsMatch = make(map[string]*openapi3.Example)
	withoutParamsMatch = make(map[string]*openapi3.Example)
	for _, k := range keys {
		exRef := examples[k]
		if exRef == nil || exRef.Value == nil {
			continue
		}
		ex := exRef.Value
		if s.shouldSkipExample(ex, k, opID) {
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

func (s *Server) evaluateExample(example *openapi3.Example, eval runtime.Evaluator) ([]byte, error) {
	if example.Value == nil {
		return []byte{}, nil
	}
	// Evaluate runtime expressions in the value
	resolved, err := s.evaluateValue(example.Value, eval)
	if err != nil {
		return nil, err
	}
	// Convert to JSON
	return json.Marshal(resolved)
}

func (s *Server) evaluateHeaders(example *openapi3.Example, eval runtime.Evaluator) map[string]string {
	headers := make(map[string]string)

	if headersMap, ok := extensions.ExtractHeaders(example); ok {
		for key, val := range headersMap {
			if str, ok := s.resolveHeaderValue(val, eval); ok {
				headers[key] = str
			}
		}
	}

	return headers
}

// resolveHeaderValue converts a header value (string, []any, or any) to a resolved string.
func (s *Server) resolveHeaderValue(val any, eval runtime.Evaluator) (string, bool) {
	switch v := val.(type) {
	case string:
		resolved, err := s.evaluateValue(v, eval)
		if err != nil {
			if s.config.Verbose {
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
				resolved, err := s.evaluateValue(first, eval)
				if err == nil {
					if str, ok := resolved.(string); ok {
						return str, true
					}
				}
			}
		}
	default:
		// Try to evaluate as runtime expression
		resolved, err := s.evaluateValue(val, eval)
		if err == nil {
			if str, ok := resolved.(string); ok {
				return str, true
			}
		}
	}
	return "", false
}

func getStatusCode(mapping *loader.RouteMapping, response *openapi3.Response) int {
	// TODO: parse status code from mapping (key in Responses map)
	// For now, default to 200
	return 200
}
