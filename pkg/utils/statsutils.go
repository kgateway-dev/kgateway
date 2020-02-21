package utils

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

func MakeSumCounter(name, description string, tagKeys ...tag.Key) *stats.Int64Measure {
	return MakeCounter(name, description, view.Sum(), tagKeys...)
}

func MakeLastValueCounter(name, description string, tagKeys ...tag.Key) *stats.Int64Measure {
	return MakeCounter(name, description, view.LastValue(), tagKeys...)
}

func MakeCounter(name, description string, aggregation *view.Aggregation, tagKeys ...tag.Key) *stats.Int64Measure {
	counter := stats.Int64(name, description, "1")

	_ = view.Register(&view.View{
		Name:        counter.Name(),
		Measure:     counter,
		Description: counter.Description(),
		Aggregation: aggregation,
		TagKeys:     tagKeys,
	})

	return counter
}

func MeasureOne(ctx context.Context, counter *stats.Int64Measure, tags ...tag.Mutator) {
	Measure(ctx, counter, 1, tags...)
}

func Measure(ctx context.Context, counter *stats.Int64Measure, val int64, tags ...tag.Mutator) {
	if err := stats.RecordWithTags(
		ctx,
		tags,
		counter.M(val),
	); err != nil {
		contextutils.LoggerFrom(ctx).Errorf("setting counter %v: %v", counter.Name(), err)
	}
}
