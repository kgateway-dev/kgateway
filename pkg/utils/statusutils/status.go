package statusutils

import (
	"context"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	"github.com/solo-io/gloo/projects/gloo/api/external/solo/ratelimit"
	ratelimitpkg "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
)

func GetStatusReporterNamespaceOrDefault(defaultNamespace string) string {
	namespace, err := statusutils.GetStatusReporterNamespaceFromEnv()
	if err == nil {
		return namespace
	}

	return defaultNamespace
}

func GetStatusClientFromEnvOrDefault(defaultNamespace string, metricOpts map[string]*metrics.Labels) (resources.StatusClient, error) {
	statusReporterNamespace := GetStatusReporterNamespaceOrDefault(defaultNamespace)
	return GetStatusClientForNamespace(statusReporterNamespace, metricOpts)
}

func GetStatusClientForNamespace(namespace string, metricOpts map[string]*metrics.Labels) (resources.StatusClient, error) {
	statusMetrics, err := metrics.NewConfigStatusMetrics(metricOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create ConfigStatusMetrics")
	}
	return &HybridStatusClient{
		namespacedStatusClient: statusutils.NewNamespacedStatusesClient(namespace),
		statusMetrics:          statusMetrics,
	}, nil
}

var _ resources.StatusClient = &HybridStatusClient{}

// The HybridStatusClient is used while some resources support namespaced statuses
// and others (RateLimitConfig) do not
type HybridStatusClient struct {
	namespacedStatusClient *statusutils.NamespacedStatusesClient
	statusMetrics          metrics.ConfigStatusMetrics
}

func (h *HybridStatusClient) GetStatus(resource resources.InputResource) *core.Status {
	if h.shouldUseDeprecatedStatus(resource) {
		return resource.GetStatus()
	}

	return h.namespacedStatusClient.GetStatus(resource)
}

func (h *HybridStatusClient) SetStatus(resource resources.InputResource, status *core.Status) {
	if h.shouldUseDeprecatedStatus(resource) {
		resource.SetStatus(status)
	} else {
		h.namespacedStatusClient.SetStatus(resource, status)
	}
	h.setMetricForResource(resource, status)
}

func (h *HybridStatusClient) shouldUseDeprecatedStatus(resource resources.InputResource) bool {
	switch resource.(type) {
	case *ratelimit.RateLimitConfig:
		return true
	case *ratelimitpkg.RateLimitConfig:
		return true

	default:
		return false
	}
}

func (h *HybridStatusClient) setMetricForResource(resource resources.InputResource, status *core.Status) {
	// TODO(mitchaman): Pass a context through
	//   DO_NOT_SUBMIT
	ctx := context.TODO()
	if status.GetState() == core.Status_Warning || status.GetState() == core.Status_Rejected {
		h.statusMetrics.SetResourceInvalid(ctx, resource)
		return
	}
	h.statusMetrics.SetResourceValid(ctx, resource)
}
