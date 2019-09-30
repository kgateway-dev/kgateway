package utils

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

func MakeCounter(name, description string, tags ...tag.Mutator) *stats.Int64Measure {
	counter := stats.Int64(name, description, "1")

	_ = view.Register(&view.View{
		Name:        counter.Name(),
		Measure:     counter,
		Description: counter.Description(),
		Aggregation: view.LastValue(),
	})

	return counter
}

func Increment(ctx context.Context, counter *stats.Int64Measure, tags ...tag.Mutator) {
	if err := stats.RecordWithTags(
		ctx,
		tags,
		counter.M(1),
	); err != nil {
		contextutils.LoggerFrom(ctx).Errorf("incrementing counter %v: %v", counter.Name(), err)
	}
}
