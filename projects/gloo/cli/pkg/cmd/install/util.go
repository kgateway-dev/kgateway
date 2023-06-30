package install

import (
	"fmt"
	"strings"
)

var (
	GlooNamespacedKinds    []string
	GlooClusterScopedKinds []string
	GlooCrdNames           []string
	GlooComponentLabels    map[string]string
)

func init() {

	GlooComponentLabels = map[string]string{
		"app": "(gloo,glooe-prometheus,gloo-fed)",
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
		"httpgateways.gateway.solo.io",
		"tcpgateways.gateway.solo.io",
		"proxies.gloo.solo.io",
		"settings.gloo.solo.io",
		"upstreams.gloo.solo.io",
		"upstreamgroups.gloo.solo.io",
		"virtualservices.gateway.solo.io",
		"routetables.gateway.solo.io",
		"authconfigs.enterprise.gloo.solo.io",
		"ratelimitconfigs.ratelimit.solo.io",
		"virtualhostoptions.gateway.solo.io",
		"routeoptions.gateway.solo.io",
		"graphqlapis.graphql.gloo.solo.io",
		// gloo-fed CRDs:
		"glooinstances.fed.solo.io",
		"failoverschemes.fed.solo.io",
		"federatedauthconfigs.fed.enterprise.gloo.solo.io",
		"federatedgateways.fed.gateway.solo.io",
		"federatedmatchablehttpgateways.fed.gateway.solo.io",
		"federatedmatchabletcpgateways.fed.gateway.solo.io",
		"federatedroutetables.fed.gateway.solo.io",
		"federatedsettings.fed.gloo.solo.io",
		"federatedupstreamgroups.fed.gloo.solo.io",
		"federatedupstreams.fed.gloo.solo.io",
		"federatedvirtualservices.fed.gateway.solo.io",
	}
}

func LabelsToFlagString(labelMap map[string]string) (labelString string) {
	for k, v := range labelMap {
		labelString += fmt.Sprintf("%s in %s,", k, v)
	}
	labelString = strings.TrimSuffix(labelString, ",")
	return
}
