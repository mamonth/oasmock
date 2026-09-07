package eventbus

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
func TestScheduler_DeliversAtCadence(t *testing.T) {
	t.Parallel()

	sched := NewScheduler()
	defer sched.Shutdown()

	var count atomic.Int32
	job := sched.Add(&ScheduledJob{ID: "ex-1", Interval: 10 * time.Millisecond, Deliver: func() {
		count.Add(1)
	}})
	go sched.Run(job)

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
func TestScheduler_CancelStops(t *testing.T) {
	t.Parallel()

	sched := NewScheduler()
	defer sched.Shutdown()

	var count atomic.Int32
	job := sched.Add(&ScheduledJob{ID: "ex-1", Interval: 5 * time.Millisecond, Deliver: func() {
		count.Add(1)
	}})
	go sched.Run(job)

	deadline := time.Now().Add(100 * time.Millisecond)
	for count.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	before := count.Load()
	require.GreaterOrEqual(t, before, int32(2))

	removed, ok := sched.Cancel("ex-1")
	require.True(t, ok)
	require.NotNil(t, removed)

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
func TestScheduler_Shutdown(t *testing.T) {
	t.Parallel()

	sched := NewScheduler()
	var count atomic.Int32
	job := sched.Add(&ScheduledJob{ID: "ex-1", Interval: 5 * time.Millisecond, Deliver: func() {
		count.Add(1)
	}})
	go sched.Run(job)

	deadline := time.Now().Add(100 * time.Millisecond)
	for count.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	require.GreaterOrEqual(t, count.Load(), int32(2))

	sched.Shutdown()
	time.Sleep(30 * time.Millisecond)
	assert.True(t, sched.Stopped("ex-1"))
}

/*
Scenario: Cancelling an unknown job reports false
Given a scheduler without the named job
When cancel is called
Then it reports false and leaves no error

Related spec scenarios: RS.MAPI.31
*/
func TestScheduler_CancelUnknown(t *testing.T) {
	t.Parallel()

	sched := NewScheduler()
	defer sched.Shutdown()
	job, ok := sched.Cancel("unknown")
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
func TestScheduler_AddReplacesAndStopsPrevious(t *testing.T) {
	t.Parallel()

	sched := NewScheduler()
	defer sched.Shutdown()

	var oldCount atomic.Int32
	jobA := sched.Add(&ScheduledJob{ID: "ex-1", Interval: 5 * time.Millisecond, Deliver: func() {
		oldCount.Add(1)
	}})
	go sched.Run(jobA)

	deadline := time.Now().Add(100 * time.Millisecond)
	for oldCount.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	require.GreaterOrEqual(t, oldCount.Load(), int32(2))

	var newCount atomic.Int32
	jobB := sched.Add(&ScheduledJob{ID: "ex-1", Interval: 5 * time.Millisecond, Deliver: func() {
		newCount.Add(1)
	}})
	go sched.Run(jobB)

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
func TestScheduler_PanicInDeliverRemovesJob(t *testing.T) {
	t.Parallel()

	sched := NewScheduler()
	defer sched.Shutdown()

	var poisoned atomic.Bool
	poisoned.Store(true)
	job := sched.Add(&ScheduledJob{
		ID:       "boom",
		Interval: 5 * time.Millisecond,
		Deliver: func() {
			if poisoned.Load() {
				poisoned.Store(false)
				panic("deliver exploded")
			}
		},
	})
	go sched.Run(job)

	deadline := time.Now().Add(time.Second)
	for sched.Started("boom") && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	assert.False(t, sched.Started("boom"), "a panicking job must be removed from the scheduler")

	// The scheduler must remain usable for subsequently added jobs.
	var healthy atomic.Int32
	good := sched.Add(&ScheduledJob{ID: "healthy", Interval: 5 * time.Millisecond, Deliver: func() {
		healthy.Add(1)
	}})
	go sched.Run(good)

	deadline = time.Now().Add(500 * time.Millisecond)
	for healthy.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	assert.GreaterOrEqual(t, healthy.Load(), int32(2), "healthy jobs must keep delivering after a panic")
}
