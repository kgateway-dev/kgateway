package metrics

import (
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	translatorSubsystem = "translator"
	translatorNameLabel = "translator"
	nameLabel           = "name"
	namespaceLabel      = "namespace"
	resultLabel         = "result"
)

var (
	translationHistogramBuckets = []float64{0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}
	translationsTotal           = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: translatorSubsystem,
			Name:      "translations_total",
			Help:      "Total number of translations",
		},
		[]string{nameLabel, namespaceLabel, translatorNameLabel, resultLabel},
	)
	translationDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       translatorSubsystem,
			Name:                            "translation_duration_seconds",
			Help:                            "Translation duration",
			Buckets:                         translationHistogramBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{nameLabel, namespaceLabel, translatorNameLabel},
	)
	translationsRunning = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: translatorSubsystem,
			Name:      "translations_running",
			Help:      "Current number of translations running",
		},
		[]string{nameLabel, namespaceLabel, translatorNameLabel},
	)
)

type TranslatorMetricLabels struct {
	Name       string
	Namespace  string
	Translator string
}

func (t TranslatorMetricLabels) toMetricsLabels() []metrics.Label {
	return []metrics.Label{
		{Name: nameLabel, Value: t.Name},
		{Name: namespaceLabel, Value: t.Namespace},
		{Name: translatorNameLabel, Value: t.Translator},
	}
}

// CollectTranslationMetrics is called at the start of a translation function to
// begin metrics collection and returns a function called at the end to complete
// metrics recording.
func CollectTranslationMetrics(labels TranslatorMetricLabels) func(error) {
	if !metrics.Active() {
		return func(err error) {}
	}

	start := time.Now()

	translationsRunning.Add(1, labels.toMetricsLabels()...)

	return func(err error) {
		duration := time.Since(start)

		translationDuration.Observe(duration.Seconds(), labels.toMetricsLabels()...)

		result := "success"
		if err != nil {
			result = "error"
		}

		translationsTotal.Inc(append(labels.toMetricsLabels(),
			metrics.Label{Name: resultLabel, Value: result},
		)...)

		translationsRunning.Sub(1, labels.toMetricsLabels()...)
	}
}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	translationsTotal.Reset()
	translationDuration.Reset()
	translationsRunning.Reset()
}
