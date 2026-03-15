package adapters

import (
	"net/http"
	"os"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/history"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/mamonth/oasmock/internal/server"
	"github.com/mamonth/oasmock/internal/state"
)

// LoaderRouteAdapter adapts loader package to server.RouteProvider interface.
type LoaderRouteAdapter struct{}

func (a *LoaderRouteAdapter) BuildRouteMappings(schemas []server.SchemaInfo) ([]server.RouteMapping, error) {
	// Convert server.SchemaInfo to loader.SchemaInfo
	loaderSchemas := make([]loader.SchemaInfo, len(schemas))
	for i, schema := range schemas {
		loaderSchemas[i] = loader.SchemaInfo{
			Spec:   schema.Spec,
			Prefix: schema.Prefix,
		}
	}

	// Call original loader function
	loaderMappings, err := loader.BuildRouteMappings(loaderSchemas)
	if err != nil {
		return nil, err
	}

	// Convert loader.RouteMapping to server.RouteMapping
	mappings := make([]server.RouteMapping, len(loaderMappings))
	for i, lm := range loaderMappings {
		mappings[i] = server.RouteMapping{
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

// StateManagerAdapter adapts state.Manager to server.StateStore interface.
type StateManagerAdapter struct {
	manager *state.Manager
}

func NewStateManagerAdapter(manager *state.Manager) *StateManagerAdapter {
	return &StateManagerAdapter{manager: manager}
}

func (a *StateManagerAdapter) Get(namespace, key string) (any, bool) {
	return a.manager.Get(namespace, key)
}

func (a *StateManagerAdapter) Set(namespace, key string, value any) {
	a.manager.Set(namespace, key, value)
}

func (a *StateManagerAdapter) Increment(namespace, key string, delta float64) (float64, error) {
	return a.manager.Increment(namespace, key, delta)
}

func (a *StateManagerAdapter) Delete(namespace, key string) {
	a.manager.Delete(namespace, key)
}

func (a *StateManagerAdapter) GetNamespace(namespace string) map[string]any {
	return a.manager.GetNamespace(namespace)
}

func (a *StateManagerAdapter) GetAll() map[string]map[string]any {
	return a.manager.GetAll()
}

// HistoryRingBufferAdapter adapts history.RingBuffer to server.HistoryStore interface.
type HistoryRingBufferAdapter struct {
	buffer *history.RingBuffer
}

func NewHistoryRingBufferAdapter(buffer *history.RingBuffer) *HistoryRingBufferAdapter {
	return &HistoryRingBufferAdapter{buffer: buffer}
}

func (a *HistoryRingBufferAdapter) Add(record server.RequestRecord) {
	// Convert server.RequestRecord to history.RequestRecord
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

	a.buffer.Add(historyRecord)
}

func (a *HistoryRingBufferAdapter) GetAll() []server.RequestRecord {
	historyRecords := a.buffer.GetAll()
	records := make([]server.RequestRecord, len(historyRecords))

	for i, hr := range historyRecords {
		record := server.RequestRecord{
			ID:        hr.ID,
			Timestamp: hr.Timestamp,
			Method:    hr.Method,
			Path:      hr.Path,
			Query:     hr.Query,
			Headers:   hr.Headers,
			Body:      hr.Body,
		}

		if hr.Response != nil {
			record.Response = &server.ResponseRecord{
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

func (a *HistoryRingBufferAdapter) Count() int {
	return a.buffer.Count()
}

func (a *HistoryRingBufferAdapter) Capacity() int {
	return a.buffer.Capacity()
}

func (a *HistoryRingBufferAdapter) Clear() {
	a.buffer.Clear()
}

// RuntimeRequestSourceAdapter adapts runtime.RequestSource to server.DataSource.
type RuntimeRequestSourceAdapter struct {
	source *runtime.RequestSource
}

func NewRuntimeRequestSourceAdapter(source *runtime.RequestSource) *RuntimeRequestSourceAdapter {
	return &RuntimeRequestSourceAdapter{source: source}
}

func (a *RuntimeRequestSourceAdapter) Get(path string) (any, bool) {
	return a.source.Get(path)
}

// RuntimeStateSourceAdapter adapts runtime.StateSource to server.DataSource.
type RuntimeStateSourceAdapter struct {
	source *runtime.StateSource
}

func NewRuntimeStateSourceAdapter(source *runtime.StateSource) *RuntimeStateSourceAdapter {
	return &RuntimeStateSourceAdapter{source: source}
}

func (a *RuntimeStateSourceAdapter) Get(path string) (any, bool) {
	return a.source.Get(path)
}

// RuntimeEnvSourceAdapter adapts runtime.EnvSource to server.DataSource.
type RuntimeEnvSourceAdapter struct {
	source *runtime.EnvSource
}

func NewRuntimeEnvSourceAdapter(source *runtime.EnvSource) *RuntimeEnvSourceAdapter {
	return &RuntimeEnvSourceAdapter{source: source}
}

func (a *RuntimeEnvSourceAdapter) Get(path string) (any, bool) {
	return a.source.Get(path)
}

// RuntimeRequestSourceFactory implements server.RequestSourceFactory using runtime package.
type RuntimeRequestSourceFactory struct{}

func (f *RuntimeRequestSourceFactory) NewRequestSource(r *http.Request, pathParams map[string]string) server.DataSource {
	source := &runtime.RequestSource{
		PathParams:  pathParams,
		QueryParams: r.URL.Query(),
		Headers:     r.Header,
		Cookies:     make(map[string]string),
		Body:        nil, // Will be set later if needed
	}

	// Parse cookies
	for _, cookie := range r.Cookies() {
		source.Cookies[cookie.Name] = cookie.Value
	}

	// TODO: Parse request body if needed
	// This is simplified - actual server.newRequestSource has more logic

	return NewRuntimeRequestSourceAdapter(source)
}

// RuntimeStateSourceFactory implements server.StateSourceFactory using runtime package.
type RuntimeStateSourceFactory struct {
	stateStore server.StateStore
}

func NewRuntimeStateSourceFactory(stateStore server.StateStore) *RuntimeStateSourceFactory {
	return &RuntimeStateSourceFactory{stateStore: stateStore}
}

func (f *RuntimeStateSourceFactory) NewStateSource(namespace string) server.DataSource {
	data := f.stateStore.GetNamespace(namespace)
	source := &runtime.StateSource{Data: data}
	return NewRuntimeStateSourceAdapter(source)
}

// RuntimeEnvSourceFactory implements server.EnvSourceFactory using runtime package.
type RuntimeEnvSourceFactory struct{}

func (f *RuntimeEnvSourceFactory) NewEnvSource() server.DataSource {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		if i := index(e, '='); i >= 0 {
			env[e[:i]] = e[i+1:]
		}
	}
	source := &runtime.EnvSource{Env: env}
	return NewRuntimeEnvSourceAdapter(source)
}

// RuntimeExpressionEvaluatorAdapter adapts runtime.Evaluator to server.ExpressionEvaluator.
type RuntimeExpressionEvaluatorAdapter struct {
	eval runtime.Evaluator
}

func NewRuntimeExpressionEvaluatorAdapter(eval runtime.Evaluator) *RuntimeExpressionEvaluatorAdapter {
	return &RuntimeExpressionEvaluatorAdapter{eval: eval}
}

func (a *RuntimeExpressionEvaluatorAdapter) AddSource(name string, source server.DataSource) {
	// We need to adapt server.DataSource to runtime.DataSource
	// This is tricky because we have different DataSource interfaces
	// For now, we'll create a wrapper
	wrapper := &dataSourceWrapper{source: source}
	a.eval.AddSource(name, wrapper)
}

func (a *RuntimeExpressionEvaluatorAdapter) Evaluate(expr string) (any, error) {
	return a.eval.Evaluate(expr)
}

// dataSourceWrapper wraps server.DataSource to implement runtime.DataSource.
type dataSourceWrapper struct {
	source server.DataSource
}

func (w *dataSourceWrapper) Get(path string) (any, bool) {
	return w.source.Get(path)
}

// ExtensionsAdapter adapts extensions package to server.ExtensionProcessor interface.
type ExtensionsAdapter struct{}

func (a *ExtensionsAdapter) ExtractSetState(example *openapi3.Example) (map[string]any, bool) {
	return extensions.ExtractSetState(example)
}

func (a *ExtensionsAdapter) ExtractSkip(example *openapi3.Example) bool {
	return extensions.ExtractSkip(example)
}

func (a *ExtensionsAdapter) ExtractOnce(example *openapi3.Example) bool {
	return extensions.ExtractOnce(example)
}

func (a *ExtensionsAdapter) ExtractParamsMatch(example *openapi3.Example) (map[string]any, bool) {
	return extensions.ExtractParamsMatch(example)
}

func (a *ExtensionsAdapter) EvaluateParamsMatch(params map[string]any, eval server.ExpressionEvaluator) (bool, error) {
	// We need to adapt server.ExpressionEvaluator to runtime.Evaluator
	// This is complex - for now, we'll use a simplified approach
	// In practice, we'd need a full adapter
	// For MVP, we'll return false
	return false, nil
}

func (a *ExtensionsAdapter) ExtractHeaders(example *openapi3.Example) (map[string]any, bool) {
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
