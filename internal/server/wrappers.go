package server

import (
	"net/http"
	"os"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/history"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/mamonth/oasmock/internal/state"
)

// loaderRouteProvider wraps loader package to implement RouteProvider.
type loaderRouteProvider struct{}

func (p *loaderRouteProvider) BuildRouteMappings(schemas []SchemaInfo) ([]RouteMapping, error) {
	// Convert SchemaInfo to loader.SchemaInfo
	loaderSchemas := make([]loader.SchemaInfo, len(schemas))
	for i, schema := range schemas {
		loaderSchemas[i] = loader.SchemaInfo{
			Spec:   schema.Spec,
			Prefix: schema.Prefix,
		}
	}

	loaderMappings, err := loader.BuildRouteMappings(loaderSchemas)
	if err != nil {
		return nil, err
	}

	// Convert loader.RouteMapping to RouteMapping
	mappings := make([]RouteMapping, len(loaderMappings))
	for i, lm := range loaderMappings {
		mappings[i] = RouteMapping{
			Method:     lm.Method,
			Path:       lm.Path,
			Pattern:    lm.Pattern,
			Prefix:     lm.Prefix,
			ChiPattern: lm.ChiPattern,
			Operation:  lm.Operation,
			Parameters: lm.Parameters,
			Responses:  lm.Responses,
		}
	}

	return mappings, nil
}

// stateManagerStore wraps state.Manager to implement StateStore.
type stateManagerStore struct {
	manager *state.Manager
}

func newStateManagerStore(manager *state.Manager) *stateManagerStore {
	return &stateManagerStore{manager: manager}
}

func (s *stateManagerStore) Get(namespace, key string) (any, bool) {
	return s.manager.Get(namespace, key)
}

func (s *stateManagerStore) Set(namespace, key string, value any) {
	s.manager.Set(namespace, key, value)
}

func (s *stateManagerStore) Increment(namespace, key string, delta float64) (float64, error) {
	return s.manager.Increment(namespace, key, delta)
}

func (s *stateManagerStore) Delete(namespace, key string) {
	s.manager.Delete(namespace, key)
}

func (s *stateManagerStore) GetNamespace(namespace string) map[string]any {
	return s.manager.GetNamespace(namespace)
}

func (s *stateManagerStore) GetAll() map[string]map[string]any {
	return s.manager.GetAll()
}

// historyRingBufferStore wraps history.RingBuffer to implement HistoryStore.
type historyRingBufferStore struct {
	buffer *history.RingBuffer
}

func newHistoryRingBufferStore(buffer *history.RingBuffer) *historyRingBufferStore {
	return &historyRingBufferStore{buffer: buffer}
}

func (s *historyRingBufferStore) Add(record RequestRecord) {
	historyRecord := history.RequestRecord{
		ID:        record.ID,
		Timestamp: record.Timestamp,
		Method:    record.Method,
		Path:      record.Path,
		Query:     record.Query,
		Headers:   record.Headers,
		Body:      record.Body,
	}

	if record.Response != nil {
		historyRecord.Response = &history.ResponseRecord{
			StatusCode: record.Response.StatusCode,
			Headers:    record.Response.Headers,
			Body:       record.Response.Body,
			Duration:   record.Response.Duration,
		}
	}

	s.buffer.Add(historyRecord)
}

func (s *historyRingBufferStore) GetAll() []RequestRecord {
	historyRecords := s.buffer.GetAll()
	records := make([]RequestRecord, len(historyRecords))

	for i, hr := range historyRecords {
		record := RequestRecord{
			ID:        hr.ID,
			Timestamp: hr.Timestamp,
			Method:    hr.Method,
			Path:      hr.Path,
			Query:     hr.Query,
			Headers:   hr.Headers,
			Body:      hr.Body,
		}

		if hr.Response != nil {
			record.Response = &ResponseRecord{
				StatusCode: hr.Response.StatusCode,
				Headers:    hr.Response.Headers,
				Body:       hr.Response.Body,
				Duration:   hr.Response.Duration,
			}
		}

		records[i] = record
	}

	return records
}

func (s *historyRingBufferStore) Count() int {
	return s.buffer.Count()
}

func (s *historyRingBufferStore) Capacity() int {
	return s.buffer.Capacity()
}

func (s *historyRingBufferStore) Clear() {
	s.buffer.Clear()
}

// runtimeDataSourceWrapper wraps runtime.DataSource to implement DataSource.
type runtimeDataSourceWrapper struct {
	source runtime.DataSource
}

func (w *runtimeDataSourceWrapper) Get(path string) (any, bool) {
	return w.source.Get(path)
}

// runtimeRequestSourceFactory implements RequestSourceFactory using runtime package.
type runtimeRequestSourceFactory struct{}

func (f *runtimeRequestSourceFactory) NewRequestSource(r *http.Request, pathParams map[string]string) DataSource {
	source := &runtime.RequestSource{
		PathParams:  pathParams,
		QueryParams: r.URL.Query(),
		Headers:     r.Header,
		Cookies:     make(map[string]string),
		Body:        nil,
	}

	// Parse cookies
	for _, cookie := range r.Cookies() {
		source.Cookies[cookie.Name] = cookie.Value
	}

	return &runtimeDataSourceWrapper{source: source}
}

// runtimeStateSourceFactory implements StateSourceFactory using runtime package.
type runtimeStateSourceFactory struct {
	stateStore StateStore
}

func newRuntimeStateSourceFactory(stateStore StateStore) *runtimeStateSourceFactory {
	return &runtimeStateSourceFactory{stateStore: stateStore}
}

func (f *runtimeStateSourceFactory) NewStateSource(namespace string) DataSource {
	data := f.stateStore.GetNamespace(namespace)
	source := &runtime.StateSource{Data: data}
	return &runtimeDataSourceWrapper{source: source}
}

// runtimeEnvSourceFactory implements EnvSourceFactory using runtime package.
type runtimeEnvSourceFactory struct{}

func (f *runtimeEnvSourceFactory) NewEnvSource() DataSource {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		if i := index(e, '='); i >= 0 {
			env[e[:i]] = e[i+1:]
		}
	}
	source := &runtime.EnvSource{Env: env}
	return &runtimeDataSourceWrapper{source: source}
}

// runtimeExpressionEvaluatorWrapper wraps runtime.Evaluator to implement ExpressionEvaluator.
type runtimeExpressionEvaluatorWrapper struct {
	eval runtime.Evaluator
}

func newRuntimeExpressionEvaluatorWrapper(eval runtime.Evaluator) *runtimeExpressionEvaluatorWrapper {
	return &runtimeExpressionEvaluatorWrapper{eval: eval}
}

func (w *runtimeExpressionEvaluatorWrapper) AddSource(name string, source DataSource) {
	// Convert DataSource to runtime.DataSource
	runtimeSource := &runtimeDataSourceWrapper{source: source}
	w.eval.AddSource(name, runtimeSource)
}

func (w *runtimeExpressionEvaluatorWrapper) Evaluate(expr string) (any, error) {
	return w.eval.Evaluate(expr)
}

// extensionsProcessorWrapper wraps extensions package to implement ExtensionProcessor.
type extensionsProcessorWrapper struct{}

func (w *extensionsProcessorWrapper) ExtractSetState(example *openapi3.Example) (map[string]any, bool) {
	return extensions.ExtractSetState(example)
}

func (w *extensionsProcessorWrapper) ExtractSkip(example *openapi3.Example) bool {
	return extensions.ExtractSkip(example)
}

func (w *extensionsProcessorWrapper) ExtractOnce(example *openapi3.Example) bool {
	return extensions.ExtractOnce(example)
}

func (w *extensionsProcessorWrapper) ExtractParamsMatch(example *openapi3.Example) (map[string]any, bool) {
	return extensions.ExtractParamsMatch(example)
}

func (w *extensionsProcessorWrapper) EvaluateParamsMatch(params map[string]any, eval ExpressionEvaluator) (bool, error) {
	// We need to convert the params to extensions.ParamsMatch
	// and use the extensions.EvaluateParamsMatch function
	// But extensions.EvaluateParamsMatch expects runtime.Evaluator
	// This is complex - need proper adapter
	// For now, return false (will be implemented later)
	_ = params
	_ = eval
	return false, nil
}

func (w *extensionsProcessorWrapper) ExtractHeaders(example *openapi3.Example) (map[string]any, bool) {
	return extensions.ExtractHeaders(example)
}

// Helper function
func index(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
