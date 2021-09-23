package statusutils

import (
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

func GetStatusClientFromEnvOrDefault(defaultNamespace string) resources.StatusClient {
	statusReporterNamespace := GetStatusReporterNamespaceOrDefault(defaultNamespace)
	return GetStatusClientForNamespace(statusReporterNamespace)
}

func GetStatusClientForNamespace(namespace string) resources.StatusClient {
	return &HybridStatusClient{
		namespacedStatusClient: statusutils.NewNamespacedStatusesClient(namespace),
	}
}

var _ resources.StatusClient = &HybridStatusClient{}

// The HybridStatusClient is used while some resources support namespaced statuses
// and others (RateLimitConfig) do not
type HybridStatusClient struct {
	namespacedStatusClient *statusutils.NamespacedStatusesClient
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
		return
	}

	h.namespacedStatusClient.SetStatus(resource, status)
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
