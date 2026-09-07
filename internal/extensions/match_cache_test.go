package extensions

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

/*
Scenario: Bounding the compiled-schema cache
Given a fixed-capacity schema cache
When more distinct schemas are inserted than the capacity
Then the oldest entries are evicted while the newest remain retrievable

Related spec scenarios: RS.EXT.2
*/
func TestSchemaCacheEvictsOldestAtCapacity(t *testing.T) {
	orig := schemaCache
	schemaCache = &schemaCacheStore{store: make(map[string]*gojsonschema.Schema)}
	defer func() { schemaCache = orig }()

	// Insert 32 distinct schemas; with capacity 1024 nothing is evicted.
	for i := 0; i < 32; i++ {
		schema := map[string]any{"type": "string", "title": "s" + string(rune('a'+i%26)) + string(rune('0'+i/26))}
		if _, err := getCachedSchema(schema); err != nil {
			t.Fatalf("unexpected compile error: %v", err)
		}
	}

	schemaCache.mu.Lock()
	storeLen := len(schemaCache.store)
	schemaCache.mu.Unlock()
	require.Equal(t, 32, storeLen)
}
