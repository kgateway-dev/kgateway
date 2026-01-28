package stopwatch

import (
	"context"
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

var (
	translationTime = metrics.NewHistogram(metrics.HistogramOpts{
		Name:    "translation_time_seconds",
		Help:    "how long the translator takes in seconds",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 5, 10, 60},
	}, []string{"translator_name"})
)

func NewTranslatorStopWatch(translatorName string) StopWatch {
	return NewStopWatch(translationTime, metrics.Label{Name: "translator_name", Value: translatorName})
}

// StopWatch is a stopwatch that records the duration of an operation and records a prometheus metric for the time between Start and Stop
type StopWatch interface {
	Start()
	Stop(ctx context.Context) time.Duration
}

type stopwatch struct {
	startTime time.Time
	histogram metrics.Histogram
	label     metrics.Label
}

// NewStopWatch creates a new StopWatch that records the duration of an operation and records a prometheus metric for the time between Start and Stop
// The metric is recorded with the provided histogram and label
func NewStopWatch(histogram metrics.Histogram, label metrics.Label) StopWatch {
	return &stopwatch{
		histogram: histogram,
		label:     label,
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
	s.histogram.Observe(duration.Seconds(), s.label)
	return duration
}
