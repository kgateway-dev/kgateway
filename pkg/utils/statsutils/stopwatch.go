package statsutils

import (
	"context"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
)

// StopWatch is a stopwatch that records the duration of an operation and records an opencensus metric for the time between Start and Stop
type StopWatch interface {
	Start()
	Stop(ctx context.Context) time.Duration
}

type stopwatch struct {
	startTime time.Time
	measure   *stats.Float64Measure
	labels    []tag.Mutator
}

// NewStopWatch creates a new StopWatch that records the duration of an operation and records an opencensus metric for the time between Start and Stop
// The metric is recorded with the provided measurement and labels as a tag
func NewStopWatch(measure *stats.Float64Measure, labels ...tag.Mutator) StopWatch {
	return &stopwatch{
		measure: measure,
		labels:  labels,
	}
}

// Start starts the stopwatch
func (s *stopwatch) Start() {
	s.startTime = time.Now()
}

// Stop stops the stopwatch and records the duration of the operation
// Note: Stop() should be called only once per Start() call, otherwise this could lead to double-counting in any
// metrics that rely on this stopwatch and redundant logging.
func (s *stopwatch) Stop(ctx context.Context) time.Duration {
	duration := time.Since(s.startTime)
	tagCtx, _ := tag.New(ctx, s.labels...)
	stats.Record(tagCtx, s.measure.M(duration.Seconds()))
	return duration
}
