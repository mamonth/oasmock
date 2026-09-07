package eventbus

import (
	"log/slog"
	"sync"
	"time"
)

// ScheduledJob is a single per-example recurring delivery job. Deliver runs the
// full delivery pipeline (render + recipient partition + push) for the owning
// example on every tick.
type ScheduledJob struct {
	ID       string
	Interval time.Duration
	// ExampleID is the client-facing example identity (the POST /_mock/examples
	// id for runtime examples, the spec example name otherwise), used for
	// schedule lifecycle envelopes (RS.AMG.27).
	ExampleID string
	Channel   string
	stop      chan struct{}
	Deliver   func()
}

// Scheduler runs per-example interval jobs. It is a pure fabrication decoupled
// from both the HTTP surface and the event broker: delivery is injected per job
// so the scheduler never reaches into the caller.
type Scheduler struct {
	mu   sync.Mutex
	jobs map[string]*ScheduledJob
}

// NewScheduler creates an empty scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{jobs: make(map[string]*ScheduledJob)}
}

// Add registers a job and returns it; Run must be started in a goroutine. A job
// already registered under the same id is replaced: its stop channel is closed
// so its ticker loop ends and no further deliveries occur.
func (s *Scheduler) Add(job *ScheduledJob) *ScheduledJob {
	if job.stop == nil {
		job.stop = make(chan struct{})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.jobs[job.ID]; ok {
		delete(s.jobs, job.ID)
		close(existing.stop)
	}
	s.jobs[job.ID] = job
	return job
}

// Run delivers a job at its interval until stopped or shut down. The stop
// channel is checked before each tick so a cancelled job does not run further
// deliveries even when a tick is already due. A panic inside a delivery is
// contained: the job is unregistered so the cadence is not silently lost, the
// panic is logged, and the scheduler keeps serving other jobs.
func (s *Scheduler) Run(job *ScheduledJob) {
	if job == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("interval job delivery panicked; job removed", "id", job.ID, "panic", r)
			s.Cancel(job.ID)
		}
	}()
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-job.stop:
			return
		default:
		}
		select {
		case <-job.stop:
			return
		case <-ticker.C:
			job.Deliver()
		}
	}
}

// Started reports whether a job is currently registered (running or pending).
func (s *Scheduler) Started(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.jobs[id]
	return ok
}

// Stopped reports whether a job has been fully removed.
func (s *Scheduler) Stopped(id string) bool {
	return !s.Started(id)
}

// Cancel unregisters a job by id and reports it, returning the removed job so
// the caller can emit lifecycle metadata. The caller closes its stop channel
// to end any in-flight ticker loop.
func (s *Scheduler) Cancel(id string) (*ScheduledJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	delete(s.jobs, id)
	close(job.stop)
	return job, true
}

// Shutdown stops all scheduled jobs. Each job's stop channel is closed exactly
// once by deleting it from the map first.
func (s *Scheduler) Shutdown() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, job := range s.jobs {
		delete(s.jobs, id)
		close(job.stop)
	}
}
