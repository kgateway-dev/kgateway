package metricutils

import (
	"context"
	"fmt"
	"strings"

	errors "github.com/rotisserie/eris"
	utils2 "github.com/solo-io/gloo/pkg/utils"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"k8s.io/client-go/util/jsonpath"
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

// NewConfigStatusMetrics creates and returns a ConfigStatusMetrics from the specified options.
// If the options are invalid, an error is returned.
func NewConfigStatusMetrics(opts *ConfigStatusMetricsOpts) (*ConfigStatusMetrics, error) {
	vs, err := newResourceMetric(virtualService, opts.GetVirtualServiceLabels().GetLabelToPath())
	if err != nil {
		return nil, err
	}
	return &ConfigStatusMetrics{
		vs: vs,
	}, nil
}

func (m *ConfigStatusMetrics) SetResourceValid(ctx context.Context, resource resources.Resource) {
	log := contextutils.LoggerFrom(ctx)
	switch typed := resource.(type) {
	case *gwv1.VirtualService:
		if m.vs != nil {
			log.Debugf("Setting virtual service '%s' valid", typed.GetMetadata().Ref())
			mutators, err := getMutators(m.vs, typed)
			if err != nil {
				log.Errorf("Error setting labels on %s: %s", metricNames[virtualService], err.Error())
			}
			utils2.MeasureZero(ctx, m.vs.gauge, mutators...)
		}
	default:
		log.Debugf("No resource metric handler configured for resource type: %T", resource)
	}
}

func (m *ConfigStatusMetrics) SetResourceInvalid(ctx context.Context, resource resources.Resource) {
	log := contextutils.LoggerFrom(ctx)
	switch typed := resource.(type) {
	case *gwv1.VirtualService:
		if m.vs != nil {
			log.Debugf("Setting virtual service '%s' invalid", typed.GetMetadata().Ref())
			mutators, err := getMutators(m.vs, typed)
			if err != nil {
				log.Errorf("Error setting labels on %s: %s", metricNames[virtualService], err.Error())
			}
			utils2.MeasureOne(ctx, m.vs.gauge, mutators...)
		}
	default:
		log.Debugf("No resource metric handler configured for resource type: %T", resource)
	}
}

func getMutators(metric *resourceMetric, resource resources.Resource) ([]tag.Mutator, error) {
	numLabels := len(metric.labelToPath)
	mutators := make([]tag.Mutator, numLabels)
	i := 0
	for k, v := range metric.labelToPath {
		key, err := tag.NewKey(k)
		if err != nil {
			return nil, err
		}
		value, err := extractValueFromResource(resource, v)
		if err != nil {
			return nil, err
		}
		mutators[i] = tag.Upsert(key, value)
		i++
	}
	return mutators, nil
}

// Grab the value at the specified json path from the resource
func extractValueFromResource(resource resources.Resource, jsonPath string) (string, error) {
	j := jsonpath.New("ConfigStatusMetrics")
	// Parse the template
	err := j.Parse(jsonPath)
	if err != nil {
		return "", err
	}
	// grab the result from the resource
	values, err := j.FindResults(resource)
	if err != nil {
		return "", nil
	}

	var valueStrings []string
	if len(values) == 0 || len(values[0]) == 0 {
		valueStrings = append(valueStrings, "<none>")
	}
	for i := range values {
		for j := range values[i] {
			valueStrings = append(valueStrings, fmt.Sprintf("%v", values[i][j].Interface()))
		}
	}
	output := strings.Join(valueStrings, ",")
	return output, nil
}

// Returns a resourceMetric, or nil if labelToPath is nil or empty. An error is returned if the
// labelToPath configuration is invalid (for example, specifies an invalid label key).
func newResourceMetric(kind resourceKind, labelToPath map[string]string) (*resourceMetric, error) {
	numLabels := len(labelToPath)
	if numLabels > 0 {
		tagKeys := make([]tag.Key, numLabels)
		i := 0
		for k := range labelToPath {
			var err error
			tagKeys[i], err = tag.NewKey(k)
			if err != nil {
				return nil, errors.Wrapf(err, "Error creating resourceMetric for %s", metricNames[virtualService])
			}
			i++
		}
		return &resourceMetric{
			gauge:       utils2.MakeGauge(metricNames[kind], metricDescriptions[kind], tagKeys...),
			labelToPath: labelToPath,
		}, nil
	}
	return nil, nil
}
