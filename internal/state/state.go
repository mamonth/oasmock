package state

import (
	"maps"
	"sync"
)

// Manager manages state per namespace.
type Manager struct {
	mu sync.RWMutex
	// namespace -> key -> value (any JSON-serializable value)
	state map[string]map[string]any
}

// NewManager creates a new state manager.
func NewManager() *Manager {
	return &Manager{
		state: make(map[string]map[string]any),
	}
}

// Get returns the value for the given key in the namespace.
func (m *Manager) Get(namespace, key string) (any, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ns, ok := m.state[namespace]
	if !ok {
		return nil, false
	}
	val, ok := ns[key]
	return val, ok
}

// Set sets a key-value pair in the namespace.
func (m *Manager) Set(namespace, key string, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ns, ok := m.state[namespace]
	if !ok {
		ns = make(map[string]any)
		m.state[namespace] = ns
	}
	ns[key] = value
}

// Increment increments a numeric value in the namespace.
// If the key does not exist, it is initialized to delta.
// Returns the new value.
func (m *Manager) Increment(namespace, key string, delta float64) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ns, ok := m.state[namespace]
	if !ok {
		ns = make(map[string]any)
		m.state[namespace] = ns
	}
	old, ok := ns[key]
	if !ok {
		ns[key] = delta
		return delta, nil
	}
	// Try to convert to float64
	var oldNum float64
	switch v := old.(type) {
	case float64:
		oldNum = v
	case int:
		oldNum = float64(v)
	case int64:
		oldNum = float64(v)
	case float32:
		oldNum = float64(v)
	default:
		// If not numeric, treat as delta
		ns[key] = delta
		return delta, nil
	}
	newVal := oldNum + delta
	ns[key] = newVal
	return newVal, nil
}

// Delete removes a key from the namespace.
func (m *Manager) Delete(namespace, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ns, ok := m.state[namespace]
	if !ok {
		return
	}
	delete(ns, key)
	if len(ns) == 0 {
		delete(m.state, namespace)
	}
}

// ClearNamespace removes all state for a namespace.
func (m *Manager) ClearNamespace(namespace string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state, namespace)
}

// GetNamespace returns a copy of all state for a namespace.
func (m *Manager) GetNamespace(namespace string) map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ns, ok := m.state[namespace]
	if !ok {
		return nil
	}
	return maps.Clone(ns)
}

// GetAll returns a copy of all state (for debugging).
func (m *Manager) GetAll() map[string]map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copy := make(map[string]map[string]any, len(m.state))
	for ns, kv := range m.state {
		copy[ns] = maps.Clone(kv)
	}
	return copy
}
