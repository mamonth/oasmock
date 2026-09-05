package server

import (
	"log/slog"
	"sync"
	"time"
)

// scheduledJob is a single per-example recurring delivery job (design D4).
// deliver runs the full delivery pipeline (render + recipient partition +
// push) for the owning example on every tick.
type scheduledJob struct {
	id       string
	interval time.Duration
	// exampleID is the client-facing example identity (the POST /_mock/examples
	// id for runtime examples, the spec example name otherwise), used for
	// schedule lifecycle envelopes (RS.AMG.27).
	exampleID string
	channel   string
	stop      chan struct{}
	deliver   func()
}

// jobScheduler runs per-example interval jobs. It is a pure fabrication
// decoupled from both the HTTP surface and the event broker: delivery is
// injected per job so the scheduler never reaches into Server.
type jobScheduler struct {
	mu   sync.Mutex
	jobs map[string]*scheduledJob
}

func newJobScheduler() *jobScheduler {
	return &jobScheduler{jobs: make(map[string]*scheduledJob)}
}

// add registers a job and returns it; run must be started in a goroutine. A
// job already registered under the same id is replaced: its stop channel is
// closed so its ticker loop ends and no further deliveries occur.
func (s *jobScheduler) add(job *scheduledJob) *scheduledJob {
	if job.stop == nil {
		job.stop = make(chan struct{})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.jobs[job.id]; ok {
		delete(s.jobs, job.id)
		close(existing.stop)
	}
	s.jobs[job.id] = job
	return job
}

// run delivers a job at its interval until stopped or shut down. The stop
// channel is checked before each tick so a cancelled job does not run further
// deliveries even when a tick is already due. A panic inside a delivery is
// contained: the job is unregistered so the cadence is not silently lost, the
// panic is logged, and the scheduler keeps serving other jobs.
func (s *jobScheduler) run(job *scheduledJob) {
	if job == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("interval job delivery panicked; job removed", "id", job.id, "panic", r)
			s.cancel(job.id)
		}
	}()
	ticker := time.NewTicker(job.interval)
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
			job.deliver()
		}
	}
}

// started reports whether a job is currently registered (running or pending).
func (s *jobScheduler) started(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.jobs[id]
	return ok
}

// stopped reports whether a job has been fully removed.
func (s *jobScheduler) stopped(id string) bool {
	return !s.started(id)
}

// cancel unregisters a job by id and reports it, returning the removed job so
// the caller can emit lifecycle metadata. The caller closes its stop channel
// to end any in-flight ticker loop.
func (s *jobScheduler) cancel(id string) (*scheduledJob, bool) {
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

// shutdown stops all scheduled jobs. Each job's stop channel is closed exactly
// once by deleting it from the map first.
func (s *jobScheduler) shutdown() {
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
