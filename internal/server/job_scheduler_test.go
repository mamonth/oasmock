package server

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: An interval job delivers at its cadence
Given a per-example interval job registered in the scheduler
When the job runs
Then the delivery callback fires repeatedly at the configured interval

Related spec scenarios: RS.EXT.22, RS.MAPI.25
*/
func TestJobScheduler_DeliversAtCadence(t *testing.T) {
	t.Parallel()

	sched := newJobScheduler()
	defer sched.shutdown()

	var count atomic.Int32
	job := sched.add(&scheduledJob{id: "ex-1", interval: 10 * time.Millisecond, deliver: func() {
		count.Add(1)
	}})
	go sched.run(job)

	deadline := time.Now().Add(200 * time.Millisecond)
	for count.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	assert.GreaterOrEqual(t, count.Load(), int32(2))
}

/*
Scenario: Cancelling an interval job stops further deliveries
Given a running interval job
When the job is cancelled by id
Then no further deliveries occur after cancellation

Related spec scenarios: RS.EXT.22, RS.MAPI.30
*/
func TestJobScheduler_CancelStops(t *testing.T) {
	t.Parallel()

	sched := newJobScheduler()
	defer sched.shutdown()

	var count atomic.Int32
	job := sched.add(&scheduledJob{id: "ex-1", interval: 5 * time.Millisecond, deliver: func() {
		count.Add(1)
	}})
	go sched.run(job)

	deadline := time.Now().Add(100 * time.Millisecond)
	for count.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	before := count.Load()
	require.GreaterOrEqual(t, before, int32(2))

	job, ok := sched.cancel("ex-1")
	require.True(t, ok)
	require.NotNil(t, job)

	time.Sleep(40 * time.Millisecond)
	// At most the one tick already in flight at the moment of cancellation may
	// land; any sustained cadence (5ms here would add ~8) means cancel failed.
	assert.LessOrEqual(t, count.Load()-before, int32(1), "no further deliveries may occur after cancellation")
}

/*
Scenario: Shutting down the scheduler stops all interval jobs
Given running interval jobs
When the scheduler shuts down
Then the jobs are cancelled and registered entries removed

Related spec scenarios: RS.MAPI.25, RS.MSC.49
*/
func TestJobScheduler_Shutdown(t *testing.T) {
	t.Parallel()

	sched := newJobScheduler()
	var count atomic.Int32
	job := sched.add(&scheduledJob{id: "ex-1", interval: 5 * time.Millisecond, deliver: func() {
		count.Add(1)
	}})
	go sched.run(job)

	deadline := time.Now().Add(100 * time.Millisecond)
	for count.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	require.GreaterOrEqual(t, count.Load(), int32(2))

	sched.shutdown()
	time.Sleep(30 * time.Millisecond)
	assert.True(t, sched.stopped("ex-1"))
}

/*
Scenario: Cancelling an unknown job reports false
Given a scheduler without the named job
When cancel is called
Then it reports false and leaves no error

Related spec scenarios: RS.MAPI.31
*/
func TestJobScheduler_CancelUnknown(t *testing.T) {
	t.Parallel()

	sched := newJobScheduler()
	defer sched.shutdown()
	job, ok := sched.cancel("unknown")
	assert.False(t, ok)
	assert.Nil(t, job)
}

/*
Scenario: Re-adding a job id stops the previous job
Given a running job and a new job registered under the same id
When the second job is added
Then the previous job's deliveries stop and only the new job delivers onward

Related spec scenarios: RS.EXT.22, RS.MAPI.25
*/
func TestJobScheduler_AddReplacesAndStopsPrevious(t *testing.T) {
	t.Parallel()

	sched := newJobScheduler()
	defer sched.shutdown()

	var oldCount atomic.Int32
	jobA := sched.add(&scheduledJob{id: "ex-1", interval: 5 * time.Millisecond, deliver: func() {
		oldCount.Add(1)
	}})
	go sched.run(jobA)

	deadline := time.Now().Add(100 * time.Millisecond)
	for oldCount.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	require.GreaterOrEqual(t, oldCount.Load(), int32(2))

	var newCount atomic.Int32
	jobB := sched.add(&scheduledJob{id: "ex-1", interval: 5 * time.Millisecond, deliver: func() {
		newCount.Add(1)
	}})
	go sched.run(jobB)

	deadline = time.Now().Add(100 * time.Millisecond)
	for newCount.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	require.GreaterOrEqual(t, newCount.Load(), int32(2))

	frozen := oldCount.Load()
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, frozen, oldCount.Load(), "the replaced job must be stopped")
}

/*
Scenario: A panicking interval job is contained and removed
Given an interval job whose delivery callback panics
When the job runs
Then the panic is recovered, the job is unregistered, and the scheduler keeps
serving other jobs instead of silently losing the cadence

Related spec scenarios: RS.EXT.22, RS.MAPI.25
*/
func TestJobScheduler_PanicInDeliverRemovesJob(t *testing.T) {
	t.Parallel()

	sched := newJobScheduler()
	defer sched.shutdown()

	var poisoned atomic.Bool
	poisoned.Store(true)
	job := sched.add(&scheduledJob{
		id:       "boom",
		interval: 5 * time.Millisecond,
		deliver: func() {
			if poisoned.Load() {
				poisoned.Store(false)
				panic("deliver exploded")
			}
		},
	})
	go sched.run(job)

	deadline := time.Now().Add(time.Second)
	for sched.started("boom") && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	assert.False(t, sched.started("boom"), "a panicking job must be removed from the scheduler")

	// The scheduler must remain usable for subsequently added jobs.
	var healthy atomic.Int32
	good := sched.add(&scheduledJob{id: "healthy", interval: 5 * time.Millisecond, deliver: func() {
		healthy.Add(1)
	}})
	go sched.run(good)

	deadline = time.Now().Add(500 * time.Millisecond)
	for healthy.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	assert.GreaterOrEqual(t, healthy.Load(), int32(2), "healthy jobs must keep delivering after a panic")
}
