package server

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/xeipuuv/gojsonschema"
)

var addExampleRequestSchema = gojsonschema.NewGoLoader(map[string]any{
	"type":     "object",
	"required": []string{"path", "response"},
	"properties": map[string]any{
		"path": map[string]any{"type": "string"},
		"method": map[string]any{
			"type": "string",
			"enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		},
		"once":     map[string]any{"type": "boolean"},
		"validate": map[string]any{"type": "boolean"},
		"conditions": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"response": map[string]any{
			"type":     "object",
			"required": []string{"code"},
			"properties": map[string]any{
				"code": map[string]any{"type": "integer"},
				"headers": map[string]any{
					"type":                 "object",
					"additionalProperties": map[string]any{"type": "string"},
				},
				"body": map[string]any{
					"type": []any{"string", "number", "boolean", "object", "array"},
				},
			},
		},
	},
})

func validateAddExampleRequest(rawJSON []byte) error {
	loader := gojsonschema.NewBytesLoader(rawJSON)
	result, err := gojsonschema.Validate(addExampleRequestSchema, loader)
	if err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}
	if !result.Valid() {
		var errStrs []string
		for _, desc := range result.Errors() {
			errStrs = append(errStrs, desc.String())
		}
		return fmt.Errorf("invalid request: %s", strings.Join(errStrs, "; "))
	}
	return nil
}

// filterRecords filters request records based on query parameters.
func filterRecords(records []RequestRecord, query url.Values) []RequestRecord {
	filtered := make([]RequestRecord, 0, len(records))
	for _, rec := range records {
		// Filter by path
		if path := query.Get("path"); path != "" && rec.Path != path {
			continue
		}
		// Filter by method
		if method := query.Get("method"); method != "" && rec.Method != method {
			continue
		}
		// Filter by time_from (milliseconds since epoch)
		if timeFromStr := query.Get("time_from"); timeFromStr != "" {
			if timeFrom, err := strconv.ParseInt(timeFromStr, 10, 64); err == nil {
				if rec.Timestamp.UnixMilli() < timeFrom {
					continue
				}
			}
		}
		// Filter by time_till
		if timeTillStr := query.Get("time_till"); timeTillStr != "" {
			if timeTill, err := strconv.ParseInt(timeTillStr, 10, 64); err == nil {
				if rec.Timestamp.UnixMilli() > timeTill {
					continue
				}
			}
		}
		filtered = append(filtered, rec)
	}
	return filtered
}

// paginateRecords applies offset and limit pagination to records.
func paginateRecords(records []RequestRecord, offset, limit int) []RequestRecord {
	if offset < 0 {
		offset = 0
	}
	if offset > len(records) {
		offset = len(records)
	}
	if limit < 0 {
		limit = 0
	}
	end := offset + limit
	if end > len(records) {
		end = len(records)
	}
	return records[offset:end]
}

// recordsToAPIResponse converts request records to API response format.
func recordsToAPIResponse(records []RequestRecord) []map[string]any {
	items := make([]map[string]any, len(records))
	for i, rec := range records {
		var body any
		if len(rec.Body) > 0 {
			// Try to unmarshal as JSON, else keep as string
			var jsonBody any
			if err := json.Unmarshal(rec.Body, &jsonBody); err == nil {
				body = jsonBody
			} else {
				body = string(rec.Body)
			}
		}
		headers := make(map[string]string)
		for k, v := range rec.Headers {
			if len(v) > 0 {
				headers[k] = v[0]
			}
		}
		items[i] = map[string]any{
			"ts":      rec.Timestamp.UnixMilli(),
			"url":     rec.Path + "?" + rec.Query,
			"method":  rec.Method,
			"body":    body,
			"headers": headers,
		}
	}
	return items
}

func (s *Server) handleGetRequests(w http.ResponseWriter, r *http.Request) {
	records := s.historyStore.GetAll()
	query := r.URL.Query()

	// Filtering
	filtered := filterRecords(records, query)

	// Pagination
	offset, _ := strconv.Atoi(query.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	paginated := paginateRecords(filtered, offset, limit)

	// Convert to API response
	items := recordsToAPIResponse(paginated)
	response := map[string]any{
		"data": items,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil && s.config.Verbose {
		slog.Debug("Failed to encode response", "err", err)
	}
}

func (s *Server) handleAddExample(w http.ResponseWriter, r *http.Request) {
	// Read the raw body for validation
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		if s.config.Verbose {
			slog.Debug("Failed to read request body", "err", err)
		}
		http.Error(w, `{"error":"Failed to read request body"}`, http.StatusBadRequest)
		return
	}
	// Validate against OpenAPI schema
	if err := validateAddExampleRequest(bodyBytes); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	// Decode into struct
	var req struct {
		Path       string         `json:"path"`
		Method     string         `json:"method"`
		Once       bool           `json:"once"`
		Validate   bool           `json:"validate"`
		Conditions map[string]any `json:"conditions"`
		Response   struct {
			Code    int               `json:"code"`
			Headers map[string]string `json:"headers"`
			Body    any               `json:"body"`
		} `json:"response"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Path == "" || req.Response.Code == 0 {
		http.Error(w, `{"error":"Missing required fields"}`, http.StatusBadRequest)
		return
	}
	req.Method = cmp.Or(req.Method, DefaultMethod)
	// Find a route mapping that matches the path
	var targetMapping *RouteMapping
	for i := range s.mappings {
		mapping := &s.mappings[i]
		if mapping.Pattern == req.Path && mapping.Method == req.Method {
			targetMapping = mapping
			break
		}
	}
	if targetMapping == nil {
		http.Error(w, `{"error":"No matching route found"}`, http.StatusBadRequest)
		return
	}
	// TODO: validate response body against OpenAPI schema if req.Validate is true
	// (skipped for now)
	// Create dynamic example
	example := dynamicExample{
		once:       req.Once,
		conditions: req.Conditions,
	}
	example.response.code = req.Response.Code
	example.response.headers = req.Response.Headers
	example.response.body = req.Response.Body
	// Generate a simple ID
	id := fmt.Sprintf("dynex-%d", time.Now().UnixNano())
	// Store under mapping key
	key := routeKey(targetMapping.Method, targetMapping.ChiPattern)
	if s.config.Verbose {
		slog.Debug("handleAddExample: storing dynamic example",
			"key", key,
			"path", req.Path,
			"method", req.Method,
			"chiPattern", targetMapping.ChiPattern,
			"pattern", targetMapping.Pattern,
			"numExamples", len(s.dynamicExamples[key])+1)
	}
	s.dyMu.Lock()
	s.dynamicExamples[key] = append(s.dynamicExamples[key], example)
	s.dyMu.Unlock()
	// Respond with success
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Example added",
		"id":      id,
	}); err != nil && s.config.Verbose {
		slog.Debug("Failed to encode success response", "err", err)
	}
}
