package statsutils

import (
	"context"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
)

type StopWatch interface {
	Start()
	Stop(ctx context.Context)
}

type stopwatch struct {
	startTime time.Time
	measure   *stats.Float64Measure
	labels    []tag.Mutator
}

func NewStopWatch(measure *stats.Float64Measure, labels ...tag.Mutator) StopWatch {
	return &stopwatch{
		measure: measure,
		labels:  labels,
	}
}

func (s *stopwatch) Start() {
	s.startTime = time.Now()
}

func (s *stopwatch) Stop(ctx context.Context) {
	duration := time.Since(s.startTime).Seconds()
	tagCtx, _ := tag.New(ctx, s.labels...)
	stats.Record(tagCtx, s.measure.M(duration))
}
