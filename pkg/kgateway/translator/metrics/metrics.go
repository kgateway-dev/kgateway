package metrics

import (
	"sync"
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

	// seriesCache holds the resolved series for each label set. HTTPRoute
	// translation records metrics for every route on every gateway translation,
	// so resolving the series once avoids label validation and vec lookups on
	// that hot path. ResetMetrics clears it; anything else that removes series
	// from these vecs must clear it too.
	seriesCache     = map[TranslatorMetricLabels]*translationSeries{}
	seriesCacheLock sync.RWMutex
)

type translationSeries struct {
	running  metrics.GaugeSeries
	duration metrics.HistogramSeries
	success  metrics.CounterSeries
}

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

// getSeries returns the cached series for the labels, resolving them on first use.
// The success series is resolved up front, so it is exported at zero even for a
// label set whose translations have only failed.
func getSeries(labels TranslatorMetricLabels) *translationSeries {
	seriesCacheLock.RLock()
	s, ok := seriesCache[labels]
	seriesCacheLock.RUnlock()
	if ok {
		return s
	}

	l := labels.toMetricsLabels()
	s = &translationSeries{
		running:  translationsRunning.With(l...),
		duration: translationDuration.With(l...),
		success:  translationsTotal.With(append(l, metrics.Label{Name: resultLabel, Value: "success"})...),
	}

	seriesCacheLock.Lock()
	defer seriesCacheLock.Unlock()
	if cached, ok := seriesCache[labels]; ok {
		return cached
	}
	seriesCache[labels] = s

	return s
}

// CollectTranslationMetrics is called at the start of a translation function to
// begin metrics collection and returns a function called at the end to complete
// metrics recording.
func CollectTranslationMetrics(labels TranslatorMetricLabels) func(error) {
	if !metrics.Active() {
		return func(err error) {}
	}

	s := getSeries(labels)
	start := time.Now()

	s.running.Add(1)

	return func(err error) {
		s.duration.Observe(time.Since(start).Seconds())

		if err != nil {
			translationsTotal.Inc(append(labels.toMetricsLabels(),
				metrics.Label{Name: resultLabel, Value: "error"},
			)...)
		} else {
			s.success.Inc()
		}

		s.running.Sub(1)
	}
}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	seriesCacheLock.Lock()
	defer seriesCacheLock.Unlock()
	clear(seriesCache)

	translationsTotal.Reset()
	translationDuration.Reset()
	translationsRunning.Reset()
}
