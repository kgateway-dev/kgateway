package metricutils

import (
	"context"

	utils2 "github.com/solo-io/gloo/pkg/utils"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
)

type ConfigStatusMetricsOpts = gloov1.Settings_ObservabilityOptions_ConfigStatusMetricsOptions

type resourceKind int

const (
	virtualService resourceKind = iota
	gateway
)

var metricNames = map[resourceKind]string{
	virtualService: "validation.gateway.solo.io/virtual_service_config_status",
	// gateway:        "validation.gateway.solo.io/gateway_config_status",
}

var metricDescriptions = map[resourceKind]string{
	virtualService: "TODO",
}

// ConfigStatusMetrics is a collection of metrics for recording config status for various
// resource types
type ConfigStatusMetrics struct {
	vs *resourceMetric
}

// resourceMetric is functionally equivalent to a gauge. Additionally, it stores information
// regarding which labels, and how to obtain the values for those labels, should get applied.
type resourceMetric struct {
	gauge       *stats.Int64Measure
	labelToPath map[string]string
}

func NewConfigStatusMetrics(opts *ConfigStatusMetricsOpts) *ConfigStatusMetrics {
	return &ConfigStatusMetrics{
		vs: newResourceMetric(virtualService, opts.GetVirtualServiceLabels().GetLabelToPath()),
	}
}

func (m *ConfigStatusMetrics) SetResourceValid(ctx context.Context, resource resources.Resource) {
	switch typed := resource.(type) {
	case *gwv1.VirtualService:
		if m.vs != nil {
			contextutils.LoggerFrom(ctx).Debugf("Setting virtual service '%s' valid", typed.GetMetadata().Ref())
			mutators := getMutators(m.vs, typed)
			utils2.MeasureZero(ctx, m.vs.gauge, mutators...)
		}
	default:
		contextutils.LoggerFrom(ctx).Debugf("No resource metric handler configured for resource type: %T", resource)
	}
}

func (m *ConfigStatusMetrics) SetResourceInvalid(ctx context.Context, resource resources.Resource) {
	switch typed := resource.(type) {
	case *gwv1.VirtualService:
		if m.vs != nil {
			contextutils.LoggerFrom(ctx).Debugf("Setting virtual service '%s' invalid", typed.GetMetadata().Ref())
			mutators := getMutators(m.vs, typed)
			utils2.MeasureOne(ctx, m.vs.gauge, mutators...)
		}
	default:
		contextutils.LoggerFrom(ctx).Debugf("No resource metric handler configured for resource type: %T", resource)
	}
}

func getMutators(metric *resourceMetric, resource resources.Resource) []tag.Mutator {
	numLabels := len(metric.labelToPath)
	mutators := make([]tag.Mutator, numLabels)
	i := 0
	for k, v := range metric.labelToPath {
		// TODO(mitchaman): Don't use MustNewKey, handle the error
		key := tag.MustNewKey(k)
		value := extractValueFromResource(resource, v)
		mutators[i] = tag.Upsert(key, value)
		i++
	}
	return mutators
}

// Grab the value at the specified json path from the resource
func extractValueFromResource(resource resources.Resource, jsonPath string) string {
	// TODO(mitchaman): Actually use the jsonPath to look up the value, rather than assuming it's "metadata.name"
	return resource.GetMetadata().GetName()
}

// Returns a resourceMetric, or nil if labelToPath is nil or empty
func newResourceMetric(kind resourceKind, labelToPath map[string]string) *resourceMetric {
	numLabels := len(labelToPath)
	if numLabels > 0 {
		tagKeys := make([]tag.Key, numLabels)
		i := 0
		for k := range labelToPath {
			// TODO(mitchaman): Don't use MustNewKey, handle the error
			tagKeys[i] = tag.MustNewKey(k)
			i++
		}
		return &resourceMetric{
			gauge:       utils2.MakeGauge(metricNames[kind], metricDescriptions[kind], tagKeys...),
			labelToPath: labelToPath,
		}
	}
	return nil
}
