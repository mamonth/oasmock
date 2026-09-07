package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

func filterRecords(records []RequestRecord, query url.Values) []RequestRecord {
	filtered := make([]RequestRecord, 0, len(records))
	for _, rec := range records {
		if recordMatchesFilter(rec, query) {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}

// recordMatchesFilter reports whether a record satisfies every filter in query.
func recordMatchesFilter(rec RequestRecord, query url.Values) bool {
	return matchesStringFilter(rec.Path, query.Get("path")) &&
		matchesStringFilter(rec.Method, query.Get("method")) &&
		matchesTimeFilter(rec.Timestamp.UnixMilli(), "time_from", query, false) &&
		matchesTimeFilter(rec.Timestamp.UnixMilli(), "time_till", query, true)
}

// matchesStringFilter reports whether the value equals the query filter, or the
// filter is empty (unset filters always match).
func matchesStringFilter(value, filter string) bool {
	return filter == "" || value == filter
}

// matchesTimeFilter reports whether the timestamp satisfies a millisecond
// range filter. When after is true the timestamp must not exceed the bound;
// otherwise it must not precede it. Malformed bounds are ignored.
func matchesTimeFilter(ts int64, key string, query url.Values, after bool) bool {
	raw := query.Get(key)
	if raw == "" {
		return true
	}
	bound, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return true
	}
	if after {
		return ts <= bound
	}
	return ts >= bound
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

// maxRequestsPage is the upper bound for GET /_mock/requests pagination
// (RS.MAPI.7, RS.MAPI.12).
const maxRequestsPage = 1000

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
	if limit <= 0 || limit > maxRequestsPage {
		limit = maxRequestsPage
	}
	paginated := paginateRecords(filtered, offset, limit)

	// Convert to API response
	items := recordsToAPIResponse(paginated)
	writeJSON(w, http.StatusOK, map[string]any{
		"data": items,
	})
}

// newExampleID returns a time-unique example id in the given namespace. The
// namespace prefix keeps runtime-async ids ("rtex-") disjoint from sync
// dynamic-example ids ("dynex-"), so DELETE /_mock/examples/{id} never has to
