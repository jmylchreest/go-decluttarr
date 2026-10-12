package jobs

import (
	"context"
	"maps"
	"sync"
)

// A recorder belongs to one job invocation; no counts survive into the next
// cycle, and concurrent policy checks can safely contribute to the same job.
type skipRecorder struct {
	mu     sync.Mutex
	counts map[string]int
}
type skipRecorderKey struct{}

// RecordSkip counts a decision in the current job's cycle summary. Reasons must
// be fixed labels, never titles, hashes, credentials or raw API error messages.
// Calls outside Manager.RunAll have no summary to update and are no-ops.
func RecordSkip(ctx context.Context, reason string) {
	recorder, ok := ctx.Value(skipRecorderKey{}).(*skipRecorder)
	if !ok {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.counts == nil {
		recorder.counts = make(map[string]int)
	}
	recorder.counts[reason]++
}
func (r *skipRecorder) snapshot() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maps.Clone(r.counts)
}
func skipRemoval(ctx context.Context, reason string) string {
	RecordSkip(ctx, reason)
	return "skip"
}
