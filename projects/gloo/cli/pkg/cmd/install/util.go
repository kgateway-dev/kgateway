package install

import (
	"fmt"
	"strings"
)

var (
	GlooNamespacedKinds    []string
	GlooClusterScopedKinds []string
	GlooCrdNames           []string

	GlooComponentLabels map[string]string

	KnativeCrdNames []string
)

func init() {

	GlooComponentLabels = map[string]string{
		"app": "(gloo,glooe-prometheus,dev-portal)",
	}

	GlooNamespacedKinds = []string{
		"Deployment",
		"DaemonSet",
		"Service",
		"ConfigMap",
		"ServiceAccount",
		"Role",
		"RoleBinding",
		"Job",
	}

	GlooClusterScopedKinds = []string{
		"ClusterRole",
		"ClusterRoleBinding",
		"ValidatingWebhookConfiguration",
	}

	GlooCrdNames = []string{
		"gateways.gateway.solo.io",
		"proxies.gloo.solo.io",
		"settings.gloo.solo.io",
		"upstreams.gloo.solo.io",
		"upstreamgroups.gloo.solo.io",
		"virtualservices.gateway.solo.io",
		"routetables.gateway.solo.io",
		"authconfigs.enterprise.gloo.solo.io",
	}

	KnativeCrdNames = []string{
		"virtualservices.networking.istio.io",
		"certificates.networking.internal.knative.dev",
		"clusteringresses.networking.internal.knative.dev",
		"configurations.serving.knative.dev",
		"images.caching.internal.knative.dev",
		"podautoscalers.autoscaling.internal.knative.dev",
		"revisions.serving.knative.dev",
		"routes.serving.knative.dev",
		"services.serving.knative.dev",
		"serverlessservices.networking.internal.knative.dev",
	}

}

func LabelsToFlagString(labelMap map[string]string) (labelString string) {
	for k, v := range labelMap {
		labelString += fmt.Sprintf("%s in %s,", k, v)
	}
	labelString = strings.TrimSuffix(labelString, ",")
	return
}
