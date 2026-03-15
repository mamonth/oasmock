package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Creating a new state manager
Given no prior state
When a new manager is created
Then it should not be nil and have empty state

Related spec scenarios: RS.MSC.20
*/
func TestNewManager(t *testing.T) {
	t.Parallel()

	m := NewManager()
	require.NotNil(t, m, "NewManager returned nil")

	// Initial state should be empty
	all := m.GetAll()
	assert.Empty(t, all, "GetAll() on new manager should be empty")
}

/*
Scenario: Setting and retrieving values from state manager
Given a new state manager
When values are set in namespaces
Then they can be retrieved correctly, non-existent keys return false

Related spec scenarios: RS.MSC.20
*/
func TestManagerSetAndGet(t *testing.T) {
	t.Parallel()

	m := NewManager()

	// Set a value
	m.Set("ns1", "key1", "value1")

	// Get it back
	val, ok := m.Get("ns1", "key1")
	assert.True(t, ok, "Get() should return true for existing key")
	assert.Equal(t, "value1", val, "Get() should return correct value")

	// Get non-existent key
	val, ok = m.Get("ns1", "key2")
	assert.False(t, ok, "Get() for non-existent key should return false")
	assert.Nil(t, val, "Get() for non-existent key should return nil")

	// Get non-existent namespace
	_, ok = m.Get("ns2", "key1")
	assert.False(t, ok, "Get() for non-existent namespace should return false")

	// Set another value in same namespace
	m.Set("ns1", "key2", 42)
	val, ok = m.Get("ns1", "key2")
	assert.True(t, ok, "Get() should return true for existing key")
	assert.Equal(t, 42, val, "Get() should return correct value")

	// Overwrite existing key
	m.Set("ns1", "key1", "updated")
	val, ok = m.Get("ns1", "key1")
	assert.True(t, ok, "Get() should return true for existing key")
	assert.Equal(t, "updated", val, "Get() should return updated value")
}

/*
Scenario: Incrementing numeric values in state manager
Given a state manager with various numeric and non-numeric values
When increment operation is called with positive, negative, or zero delta
Then values are updated correctly, non-numeric values reset to delta

Related spec scenarios: RS.MSC.21, RS.MSC.22, RS.MSC.23, RS.MSC.24
*/
func TestManagerIncrement(t *testing.T) {
	t.Parallel()

	m := NewManager()

	// Increment non-existent key
	newVal, err := m.Increment("ns1", "counter", 1.0)
	assert.NoError(t, err, "Increment() should not error")
	assert.Equal(t, 1.0, newVal, "Increment() should return correct value")

	// Increment existing float64
	newVal, err = m.Increment("ns1", "counter", 2.5)
	assert.NoError(t, err, "Increment() should not error")
	assert.Equal(t, 3.5, newVal, "Increment() should return correct value")

	// Verify value stored
	val, ok := m.Get("ns1", "counter")
	assert.True(t, ok, "Get() should return true for existing key")
	assert.Equal(t, 3.5, val, "Get() should return correct value")

	// Increment with int value stored (via Set)
	m.Set("ns2", "count", 10)
	newVal, err = m.Increment("ns2", "count", 5)
	assert.NoError(t, err, "Increment() on int should not error")
	assert.Equal(t, 15.0, newVal, "Increment() on int should return correct value")

	// Increment with non-numeric value (should reset to delta)
	m.Set("ns3", "notnumber", "string")
	newVal, err = m.Increment("ns3", "notnumber", 100.0)
	assert.NoError(t, err, "Increment() on non-numeric should not error")
	assert.Equal(t, 100.0, newVal, "Increment() on non-numeric should return delta")

	// Negative delta
	m.Set("ns4", "balance", 50.0)
	newVal, err = m.Increment("ns4", "balance", -30.0)
	assert.NoError(t, err, "Increment() with negative delta should not error")
	assert.Equal(t, 20.0, newVal, "Increment() with negative delta should return correct value")
}

/*
Scenario: Deleting keys from state manager
Given a state manager with multiple keys across namespaces
When keys are deleted
Then they are removed, empty namespaces are cleaned up, and non‑existent deletions are no‑ops

Related spec scenarios: RS.MSC.25
*/
func TestManagerDelete(t *testing.T) {
	t.Parallel()

	m := NewManager()

	// Set up state
	m.Set("ns1", "key1", "val1")
	m.Set("ns1", "key2", "val2")
	m.Set("ns2", "key1", "val3")

	// Delete key
	m.Delete("ns1", "key1")

	// Verify deleted
	val, ok := m.Get("ns1", "key1")
	assert.False(t, ok, "Get() after Delete should return false")
	assert.Nil(t, val, "Get() after Delete should return nil")

	// Verify other keys still exist
	val, ok = m.Get("ns1", "key2")
	assert.True(t, ok, "Other key should still exist")
	assert.Equal(t, "val2", val, "Other key should have correct value")

	// Delete last key in namespace - namespace should be removed
	m.Delete("ns1", "key2")
	_, ok = m.Get("ns1", "key2")
	assert.False(t, ok, "Namespace should be empty after deleting last key")

	// Delete non-existent key - should be no-op
	m.Delete("ns3", "nonexistent")

	// Delete non-existent namespace - should be no-op
	m.Delete("ns3", "key1")
}

/*
Scenario: Clearing entire namespace from state manager
Given a state manager with multiple namespaces
When a namespace is cleared
Then all keys in that namespace are removed, other namespaces remain unchanged

Related spec scenarios: RS.MSC.25
*/
func TestManagerClearNamespace(t *testing.T) {
	t.Parallel()

	m := NewManager()

	// Set up state
	m.Set("ns1", "key1", "val1")
	m.Set("ns1", "key2", "val2")
	m.Set("ns2", "key1", "val3")

	// Clear namespace
	m.ClearNamespace("ns1")

	// Verify namespace cleared
	val, ok := m.Get("ns1", "key1") //nolint:ineffassign,staticcheck
	assert.False(t, ok, "Get() after ClearNamespace should return false")
	_, ok = m.Get("ns1", "key2")
	assert.False(t, ok, "Get() after ClearNamespace should return false")

	// Verify other namespace intact
	val, ok = m.Get("ns2", "key1")
	assert.True(t, ok, "Other namespace should still exist")
	assert.Equal(t, "val3", val, "Other namespace should have correct value")

	// Clear non-existent namespace - should be no-op
	m.ClearNamespace("ns3")
}

/*
Scenario: Retrieving a copy of a namespace from state manager
Given a state manager with populated namespaces
When a namespace copy is requested
Then a deep copy is returned and modifications to the copy do not affect the manager

Related spec scenarios: RS.MSC.20
*/
func TestManagerGetNamespace(t *testing.T) {
	t.Parallel()

	m := NewManager()

	// Empty namespace
	ns := m.GetNamespace("ns1")
	assert.Nil(t, ns, "GetNamespace() on empty namespace should return nil")

	// Populate namespace
	m.Set("ns1", "key1", "val1")
	m.Set("ns1", "key2", 42)
	m.Set("ns2", "key3", "other")

	// Get namespace copy
	ns = m.GetNamespace("ns1")
	require.NotNil(t, ns, "GetNamespace() should not return nil")
	assert.Len(t, ns, 2, "GetNamespace() should return map with 2 keys")
	assert.Equal(t, "val1", ns["key1"], "Namespace should contain correct value")
	assert.Equal(t, 42, ns["key2"], "Namespace should contain correct value")

	// Modification to copy should not affect manager
	ns["key1"] = "modified"
	val, ok := m.Get("ns1", "key1")
	assert.True(t, ok, "Get() should return true for existing key")
	assert.Equal(t, "val1", val, "Modifying copy should not affect manager")
}

/*
Scenario: Retrieving a copy of all state from manager
Given a state manager with multiple namespaces and keys
When a copy of all state is requested
Then a deep copy is returned and modifications to the copy do not affect the manager

Related spec scenarios: RS.MSC.20
*/
func TestManagerGetAll(t *testing.T) {
	t.Parallel()

	m := NewManager()

	// Empty
	all := m.GetAll()
	assert.Empty(t, all, "GetAll() on empty manager should be empty")

	// Add some state
	m.Set("ns1", "a", 1)
	m.Set("ns1", "b", 2)
	m.Set("ns2", "c", "three")

	all = m.GetAll()
	assert.Len(t, all, 2, "GetAll() should return 2 namespaces")
	assert.Len(t, all["ns1"], 2, "Namespace ns1 should have 2 keys")
	assert.Equal(t, 1, all["ns1"]["a"], "Namespace should contain correct value")
	assert.Equal(t, "three", all["ns2"]["c"], "Namespace should contain correct value")

	// Modification to copy should not affect manager
	all["ns1"]["a"] = 999
	val, ok := m.Get("ns1", "a")
	assert.True(t, ok, "Get() should return true for existing key")
	assert.Equal(t, 1, val, "Modifying GetAll() copy should not affect manager")
}

/*
Scenario: Concurrent access to state manager
Given a state manager accessed by multiple goroutines
When concurrent Set, Get, Increment, and GetAll operations occur
Then the manager remains internally consistent and final state matches expectations

Related spec scenarios: RS.MSC.20
*/
func TestManagerConcurrentAccess(t *testing.T) {
	t.Parallel()

	m := NewManager()
	done := make(chan bool)

	// Start goroutines that modify state
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				m.Set("ns1", string(rune('A'+id)), j)
				m.Get("ns1", string(rune('A'+id)))
				_, _ = m.Increment("ns2", "counter", 1.0)
				m.GetAll()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Final state should be consistent
	count, _ := m.Increment("ns2", "counter", 0.0)
	// Expect 1000 increments (10 goroutines × 100 each)
	assert.Equal(t, 1000.0, count, "Final counter should be 1000.0")
}

/*
Scenario: State isolation across prefixes
Given a state manager with two different prefixes (namespaces)
When values are set in each prefix
Then values are isolated and not visible across prefixes

Related spec scenarios: RS.MSC.26
*/
func TestManagerStateIsolationAcrossPrefixes(t *testing.T) {
	t.Parallel()

	m := NewManager()

	// Set values in different namespaces (representing different prefixes)
	m.Set("prefix1", "key", "value1")
	m.Set("prefix2", "key", "value2")

	// Verify isolation
	val1, ok1 := m.Get("prefix1", "key")
	assert.True(t, ok1, "key should exist in prefix1")
	assert.Equal(t, "value1", val1, "value should match in prefix1")

	val2, ok2 := m.Get("prefix2", "key")
	assert.True(t, ok2, "key should exist in prefix2")
	assert.Equal(t, "value2", val2, "value should match in prefix2")

	// Ensure no cross-contamination
	// Getting key from prefix1 should not return prefix2's value
	assert.NotEqual(t, val1, val2, "values should be isolated")
}
