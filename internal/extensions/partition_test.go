package extensions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Partitioning match conditions by connection reference
Given an x-mock-match whose conditions reference {$connection.*} on either side
When the match is partitioned
Then conditions referencing {$connection.*} land in the connection bucket and
all other conditions land in the common bucket

Related spec scenarios: RS.EXT.24, RS.EXT.25
*/
func TestPartitionConnectionConditions(t *testing.T) {
	t.Parallel()

	pm := ParamsMatch{
		"{$event.name}":         "orderCreated",
		"{$connection.id}":      "{$event.connectionId}",
		"{$request.query.role}": "admin",
		"{$connection.channel}": "/alerts",
	}

	common, conn := PartitionConnectionConditions(pm)
	assert.Equal(t, ParamsMatch{
		"{$event.name}":         "orderCreated",
		"{$request.query.role}": "admin",
	}, common)
	assert.Equal(t, ParamsMatch{
		"{$connection.id}":      "{$event.connectionId}",
		"{$connection.channel}": "/alerts",
	}, conn)
}

/*
Scenario: Partitioning without connection conditions yields an empty bucket
Given an x-mock-match with no {$connection.*} references
When the match is partitioned
Then the connection bucket is empty, enabling the broadcast fast path

Related spec scenarios: RS.EXT.25
*/
func TestPartitionConnectionConditions_EmptyConnectionBucket(t *testing.T) {
	t.Parallel()

	pm := ParamsMatch{
		"{$event.name}": "orderCreated",
	}

	common, conn := PartitionConnectionConditions(pm)
	assert.Equal(t, pm, common)
	assert.Empty(t, conn)
}

/*
Scenario: A condition value referencing connection context partitions that side
Given a condition whose value references {$connection.*} while the key does not
When the match is partitioned
Then the condition lands in the connection bucket because a side references it

Related spec scenarios: RS.EXT.24, RS.EXT.27
*/
func TestPartitionConnectionConditions_ValueSideReference(t *testing.T) {
	t.Parallel()

	pm := ParamsMatch{
		"{$event.name}":   "orderCreated",
		"{$event.target}": "{$connection.id}",
	}

	common, conn := PartitionConnectionConditions(pm)
	assert.Equal(t, ParamsMatch{"{$event.name}": "orderCreated"}, common)
	assert.Equal(t, ParamsMatch{"{$event.target}": "{$connection.id}"}, conn)
}

/*
Scenario: Partition helper is deterministic for empty input
Given an empty x-mock-match
When the match is partitioned
Then both buckets are empty

Related spec scenarios: RS.EXT.25
*/
func TestPartitionConnectionConditions_Empty(t *testing.T) {
	t.Parallel()

	common, conn := PartitionConnectionConditions(ParamsMatch{})
	assert.Empty(t, common)
	assert.Empty(t, conn)

	require.NotPanics(t, func() {
		PartitionConnectionConditions(nil)
	})
}
