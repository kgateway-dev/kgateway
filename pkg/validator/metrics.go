package validator

import (
	"context"
	"errors"
	"time"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

type ValidationCaller string

const (
	CallerUnknown             ValidationCaller = "unknown"
	CallerRouteMatcher        ValidationCaller = "route_matcher"
	CallerRouteFull           ValidationCaller = "route_full"
	CallerBackend             ValidationCaller = "backend"
	CallerTrafficPolicy       ValidationCaller = "traffic_policy"
	CallerBackendConfigPolicy ValidationCaller = "backend_config_policy"

	validationSubsystem             = "validation"
	validationCallerLabel           = "caller"
	validationModeLabel             = "mode"
	validationResultLabel           = "result"
	validationResultValid           = "valid"
	validationResultInvalidXDS      = "invalid_xds"
	validationResultInvocationError = "invocation_error"
	validatorModeLabel              = "validator_mode"
)

type validationCallerContextKey struct{}

var (
	validationCalls = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: validationSubsystem,
			Name:      "calls_total",
			Help:      "Total number of Envoy validation requests.",
		},
		[]string{validationCallerLabel},
	)
	validationCacheHits = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: validationSubsystem,
			Name:      "cache_hits_total",
			Help:      "Total number of Envoy validation cache hits.",
		},
		[]string{validationCallerLabel},
	)
	validationCacheMisses = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: validationSubsystem,
			Name:      "cache_misses_total",
			Help:      "Total number of Envoy validation cache misses.",
		},
		[]string{validationCallerLabel},
	)
	validationValid = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: validationSubsystem,
			Name:      "valid_total",
			Help:      "Total number of successful Envoy validation results.",
		},
		[]string{validationCallerLabel},
	)
	validationInvalidXDS = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: validationSubsystem,
			Name:      "invalid_xds_total",
			Help:      "Total number of Envoy invalid-xDS validation results.",
		},
		[]string{validationCallerLabel},
	)
	validationInvocationErrors = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: validationSubsystem,
			Name:      "invocation_errors_total",
			Help:      "Total number of Envoy validation invocation errors.",
		},
		[]string{validationCallerLabel},
	)
	validationDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem: validationSubsystem,
			Name:      "duration_seconds",
			Help:      "Duration of Envoy validation requests.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{validationCallerLabel, validationResultLabel},
	)
	validationModeInfo = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: validationSubsystem,
			Name:      "mode",
			Help:      "Configured validation mode. The active mode series has value 1.",
		},
		[]string{validationModeLabel, validatorModeLabel},
	)
)

// RecordValidationMode publishes the controller's configured validation modes.
func RecordValidationMode(mode apisettings.ValidationMode, validatorMode apisettings.ValidatorMode) {
	if !metrics.Active() {
		return
	}
	validationModeInfo.Reset()
	validationModeInfo.Set(1, []metrics.Label{
		{Name: validationModeLabel, Value: string(mode)},
		{Name: validatorModeLabel, Value: string(validatorMode)},
	}...)
}

func WithValidationCaller(ctx context.Context, caller ValidationCaller) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if caller == "" {
		caller = CallerUnknown
	}
	return context.WithValue(ctx, validationCallerContextKey{}, caller)
}

func validationCaller(ctx context.Context) string {
	if ctx == nil {
		return string(CallerUnknown)
	}
	caller, ok := ctx.Value(validationCallerContextKey{}).(ValidationCaller)
	if !ok || caller == "" {
		return string(CallerUnknown)
	}
	return string(caller)
}

func validationResultFromError(err error) string {
	switch {
	case err == nil:
		return validationResultValid
	case errors.Is(err, ErrInvalidXDS):
		return validationResultInvalidXDS
	default:
		return validationResultInvocationError
	}
}

func validationLabels(caller string) []metrics.Label {
	return []metrics.Label{{Name: validationCallerLabel, Value: caller}}
}

func validationResultLabels(caller, result string) []metrics.Label {
	return []metrics.Label{
		{Name: validationCallerLabel, Value: caller},
		{Name: validationResultLabel, Value: result},
	}
}

func recordValidationCall(caller string) {
	if !metrics.Active() {
		return
	}
	validationCalls.Inc(validationLabels(caller)...)
}

func recordValidationCacheHit(caller string) {
	if !metrics.Active() {
		return
	}
	validationCacheHits.Inc(validationLabels(caller)...)
}

func recordValidationCacheMiss(caller string) {
	if !metrics.Active() {
		return
	}
	validationCacheMisses.Inc(validationLabels(caller)...)
}

func recordValidationResult(caller, result string, start time.Time) {
	if !metrics.Active() {
		return
	}
	switch result {
	case validationResultValid:
		validationValid.Inc(validationLabels(caller)...)
	case validationResultInvalidXDS:
		validationInvalidXDS.Inc(validationLabels(caller)...)
	default:
		validationInvocationErrors.Inc(validationLabels(caller)...)
	}
	validationDuration.Observe(time.Since(start).Seconds(), validationResultLabels(caller, result)...)
}
