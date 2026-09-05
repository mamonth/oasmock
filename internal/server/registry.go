package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/runtime"
)

// dynamicExample is a management-injected mock response with optional
// conditions, one-time use and TTL (RS.MAPI.2-9, RS.MSC.25-32).
type dynamicExample struct {
	onceID     string
	addedAt    time.Time
	ttl        int
	once       bool
	conditions map[string]any
	response   struct {
		code    int
		headers map[string]string
		body    any
	}
}

// isExpired reports whether the example's TTL has elapsed.
// Examples without a TTL (ttl <= 0) never expire.
func isExpired(ex dynamicExample) bool {
	if ex.ttl <= 0 {
		return false
	}
	return !ex.addedAt.Add(time.Duration(ex.ttl) * time.Second).After(time.Now())
}

const ttlSweepInterval = time.Second

// exampleRegistry owns the x-mock-once marker set, the management-injected
// dynamic example store and the TTL sweep goroutine (RS.MSC.48-49).
type exampleRegistry struct {
	onceMu          sync.RWMutex
	onceExamples    map[string]bool
	dyMu            sync.RWMutex
	dynamicExamples map[string][]dynamicExample
	sweepCtx        context.Context
	sweepCancel     context.CancelFunc
	verbose         bool
}

func newExampleRegistry(verbose bool) *exampleRegistry {
	return &exampleRegistry{
		onceExamples:    make(map[string]bool),
		dynamicExamples: make(map[string][]dynamicExample),
		verbose:         verbose,
	}
}

// markOnceUsed marks an example as used (for x-mock-once).
func (r *exampleRegistry) markOnceUsed(id string) {
	r.onceMu.Lock()
	defer r.onceMu.Unlock()
	r.onceExamples[id] = true
}

// isOnceUsed checks if an example has been used.
func (r *exampleRegistry) isOnceUsed(id string) bool {
	r.onceMu.RLock()
	defer r.onceMu.RUnlock()
	return r.onceExamples[id]
}

// addDynamic stores a management-injected dynamic example under a route key.
func (r *exampleRegistry) addDynamic(key string, ex dynamicExample) {
	r.dyMu.Lock()
	defer r.dyMu.Unlock()
	r.dynamicExamples[key] = append(r.dynamicExamples[key], ex)
}

// removeDynamic removes every dynamic example with the given id (onceID) from
// all route keys and its once marker, reporting whether any was removed.
func (r *exampleRegistry) removeDynamic(id string) bool {
	r.dyMu.Lock()
	defer r.dyMu.Unlock()
	removed := false
	for key, examples := range r.dynamicExamples {
		kept := examples[:0]
		for _, ex := range examples {
			if ex.onceID == id {
				removed = true
				r.onceMu.Lock()
				delete(r.onceExamples, id)
				r.onceMu.Unlock()
				continue
			}
			kept = append(kept, ex)
		}
		if len(kept) == 0 {
			delete(r.dynamicExamples, key)
		} else {
			r.dynamicExamples[key] = kept
		}
	}
	return removed
}

// selectDynamic returns the first dynamic example matching a route key that is
// not once-used, not expired and whose conditions evaluate, along with its
// index key. It returns nil when none matches.
func (r *exampleRegistry) selectDynamic(key string, eval runtime.Evaluator) (*dynamicExample, string) {
	if r.verbose {
		slog.Debug("selectDynamicExample", "key", key, "numExamples", len(r.dynamicExamples[key]))
	}
	r.dyMu.RLock()
	examples := r.dynamicExamples[key]
	r.dyMu.RUnlock()
	for idx, ex := range examples {
		if !r.exampleEligible(ex, eval) {
			continue
		}
		// Matched
		if ex.once {
			r.markOnceUsed(ex.onceID)
		}
		if r.verbose {
			slog.Debug("selectDynamicExample: returning matched example", "idx", idx)
		}
		return &ex, fmt.Sprintf("dynamic:%d", idx)
	}
	if r.verbose {
		slog.Debug("selectDynamicExample: no matching examples found", "key", key)
	}
	return nil, ""
}

// exampleEligible reports whether a dynamic example is selectable: it is not
// once-used or expired, and its conditions evaluate true (or it has none).
func (r *exampleRegistry) exampleEligible(ex dynamicExample, eval runtime.Evaluator) bool {
	if ex.once && r.isOnceUsed(ex.onceID) {
		return false
	}
	if isExpired(ex) {
		return false
	}
	if len(ex.conditions) == 0 {
		return true
	}
	matched, err := extensions.EvaluateParamsMatch(extensions.ParamsMatch(ex.conditions), eval)
	return err == nil && matched
}

// sweepExpired removes expired dynamic examples and their once markers.
func (r *exampleRegistry) sweepExpired() {
	r.dyMu.Lock()
	defer r.dyMu.Unlock()

	for key, examples := range r.dynamicExamples {
		kept := make([]dynamicExample, 0, len(examples))
		for idx, ex := range examples {
			if !isExpired(ex) {
				kept = append(kept, ex)
				continue
			}
			r.onceMu.Lock()
			delete(r.onceExamples, ex.onceID)
			r.onceMu.Unlock()
			if r.verbose {
				slog.Debug("Removed expired dynamic example", "key", key, "idx", idx, "ttl", ex.ttl)
			}
		}
		if len(kept) == 0 {
			delete(r.dynamicExamples, key)
		} else {
			r.dynamicExamples[key] = kept
		}
	}
}

// startSweep launches the background goroutine that periodically removes
// expired dynamic examples from memory.
func (r *exampleRegistry) startSweep() {
	ctx, cancel := context.WithCancel(context.Background())
	r.sweepCtx = ctx
	r.sweepCancel = cancel
	go func() {
		ticker := time.NewTicker(ttlSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.sweepExpired()
			}
		}
	}()
}

// stopSweep cancels the TTL sweep goroutine.
func (r *exampleRegistry) stopSweep() {
	if r.sweepCancel != nil {
		r.sweepCancel()
	}
}
