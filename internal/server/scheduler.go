package server

import (
	"sync"
	"time"
)

// recurringPush is a scheduled recurring push (RS.AMG.12-13).
type recurringPush struct {
	id       string
	channel  string
	interval time.Duration
	payload  []byte
	stop     chan struct{}
}

// pushScheduler runs recurring push jobs (RS.AMG.12-13). It is a pure
// fabrication decoupled from the HTTP surface; delivery is injected as a
// callback so the scheduler never reaches into Server.
type pushScheduler struct {
	mu   sync.Mutex
	jobs map[string]*recurringPush
	push func(channel string, payload []byte)
}

func newPushScheduler(push func(channel string, payload []byte)) *pushScheduler {
	return &pushScheduler{jobs: make(map[string]*recurringPush), push: push}
}

// add registers a job and returns it; run must be started in a goroutine.
func (s *pushScheduler) add(job *recurringPush) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.id] = job
}

// run delivers a scheduled push at its interval until stopped.
func (s *pushScheduler) run(id string) {
	s.mu.Lock()
	job := s.jobs[id]
	s.mu.Unlock()
	if job == nil {
		return
	}
	ticker := time.NewTicker(job.interval)
	defer ticker.Stop()
	for {
		select {
		case <-job.stop:
			return
		case <-ticker.C:
			s.push(job.channel, job.payload)
		}
	}
}

// stop unregisters a job and reports it. The caller closes its stop channel.
func (s *pushScheduler) stop(id string) (*recurringPush, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[id]
	delete(s.jobs, id)
	return job, job != nil
}

// shutdown stops all recurring push jobs. Each job's stop channel is closed
// exactly once by deleting it from the map first (RS.AMG.12, RS.MSC.49).
func (s *pushScheduler) shutdown() {
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
