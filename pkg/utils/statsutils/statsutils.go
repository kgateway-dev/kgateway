package statsutils

import (
	"context"
	"log/slog"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

// MakeGauge returns a new gauge with the given name and description
func MakeGauge(name, description string, tagKeys ...tag.Key) *stats.Int64Measure {
	return MakeLastValueCounter(name, description, tagKeys...)
}

// MakeSumCounter returns a new counter with a Sum aggregation for the given name, description, and tag keys
func MakeSumCounter(name, description string, tagKeys ...tag.Key) *stats.Int64Measure {
	return MakeCounter(name, description, view.Sum(), tagKeys...)
}

// MakeLastValueCounter returns a counter with a LastValue aggregation for the given name, description, and tag keys
func MakeLastValueCounter(name, description string, tagKeys ...tag.Key) *stats.Int64Measure {
	return MakeCounter(name, description, view.LastValue(), tagKeys...)
}

// MakeCounter returns a new counter with the given name, description, aggregation, and tag keys
func MakeCounter(name, description string, aggregation *view.Aggregation, tagKeys ...tag.Key) *stats.Int64Measure {
	counter := Int64Measure(name, description)
	counterView := ViewForCounter(counter, aggregation, tagKeys...)

	_ = view.Register(counterView)

	return counter
}

// MeasureZero records a zero value to the given counter
func MeasureZero(ctx context.Context, counter *stats.Int64Measure, tags ...tag.Mutator) {
	Measure(ctx, counter, 0, tags...)
}

// MeasureOne records a one value to the given counter
func MeasureOne(ctx context.Context, counter *stats.Int64Measure, tags ...tag.Mutator) {
	Measure(ctx, counter, 1, tags...)
}

// Measure records the given value to the given counter
func Measure(ctx context.Context, counter *stats.Int64Measure, val int64, tags ...tag.Mutator) {
	if err := stats.RecordWithTags(
		ctx,
		tags,
		counter.M(val),
	); err != nil {
		slog.Error("error setting counter", "error", err, "counter", counter.Name())
	}
}

// Int64Measure returns a new Int64Measure with the given name and description
func Int64Measure(name, description string) *stats.Int64Measure {
	return stats.Int64(name, description, stats.UnitDimensionless)
}

// ViewForCounter returns a view for the given measure with the given aggregation and tag keys
func ViewForCounter(counter *stats.Int64Measure, aggregation *view.Aggregation, tagKeys ...tag.Key) *view.View {
	return &view.View{
		Name:        counter.Name(),
		Measure:     counter,
		Description: counter.Description(),
		Aggregation: aggregation,
		TagKeys:     tagKeys,
	}
}
