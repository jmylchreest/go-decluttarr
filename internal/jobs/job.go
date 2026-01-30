package jobs

import "context"

// Job represents a task that can be executed by the job manager
type Job interface {
	// Name returns the unique identifier for this job
	Name() string

	// Enabled returns whether this job should be executed
	Enabled() bool

	// Run executes the job with the given context
	Run(ctx context.Context) error
}

// JobStats holds statistics from a job run
type JobStats struct {
	Found   int // Number of items found matching criteria
	Removed int // Number of items actually removed
}

// StatsJob is an optional interface for jobs that track statistics
type StatsJob interface {
	Job
	// Stats returns the statistics from the last run
	Stats() JobStats
}
