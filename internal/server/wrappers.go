package server

import (
	"github.com/mamonth/oasmock/internal/history"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/state"
)

// loaderRouteProvider wraps loader package to implement RouteProvider.
type loaderRouteProvider struct{}

func (p *loaderRouteProvider) BuildRouteMappings(schemas []SchemaInfo) ([]RouteMapping, error) {
	return loader.BuildRouteMappings(schemas)
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
	s.buffer.Add(record)
}

func (s *historyRingBufferStore) GetAll() []RequestRecord {
	return s.buffer.GetAll()
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
