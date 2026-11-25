// Package janitor runs the periodic housekeeping jobs: revoking expired dynamic credentials
// and announcing secrets that have just expired.
package janitor

import (
	"context"
	"time"
)

type Job interface{ Run(ctx context.Context) }

type JobFunc func(ctx context.Context)

func (f JobFunc) Run(ctx context.Context) { f(ctx) }

type Janitor struct {
	interval time.Duration
	jobs     []Job
}

func New(interval time.Duration, jobs ...Job) *Janitor {
	return &Janitor{interval: interval, jobs: jobs}
}

// Run executes every job immediately and then on each tick until ctx is cancelled.
func (j *Janitor) Run(ctx context.Context) {
	t := time.NewTicker(j.interval)
	defer t.Stop()
	for {
		for _, job := range j.jobs {
			job.Run(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
