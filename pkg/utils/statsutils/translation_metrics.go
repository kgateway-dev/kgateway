package statsutils

import (
	"log"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	translationTime      = stats.Float64("gloo_edge/translation_time_sec", "how long the translator takes in seconds", "s")
	translatorNameKey, _ = tag.NewKey("translator_name")
)

func init() {
	// Register views with OpenCensus
	if err := view.Register(
		&view.View{
			Name:        "gloo_edge/translation_time_sec",
			Measure:     translationTime,
			Description: "how long the translator takes in seconds",
			Aggregation: view.Distribution(0.01, 0.05, 0.1, 0.25, 0.5, 1, 5, 10, 60),
			TagKeys:     []tag.Key{translatorNameKey},
		},
	); err != nil {
		log.Fatalf("Failed to register views: %v", err)
	}
}

func NewTranslatorStopWatch(translatorName string) StopWatch {
	return NewStopWatch(translationTime, tag.Upsert(translatorNameKey, translatorName))
}
