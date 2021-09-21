package statusutils

import (
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
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
	return statusutils.NewNamespacedStatusesClient(statusReporterNamespace)
}
