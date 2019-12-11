package install

var (
	// These will get cleaned up by uninstall always
	GlooSystemKinds []string
	// These will get cleaned up only if uninstall all is chosen
	GlooRbacKinds []string
	// These will get cleaned up by uninstall if delete-crds or all is chosen
	GlooCrdNames []string

	// Set up during pre-install (for OS gloo, namespace only)
	GlooPreInstallKinds     []string
	GlooInstallKinds        []string
	GlooGatewayUpgradeKinds []string
	ExpectedLabels          map[string]string

	KnativeCrdNames []string
)

func init() {
	GlooPreInstallKinds = []string{"Namespace"}

	GlooSystemKinds = []string{
		"Deployment",
		"Service",
		"ServiceAccount",
		"ConfigMap",
		"Job",
	}

	GlooRbacKinds = []string{
		"ClusterRole",
		"ClusterRoleBinding",
	}
	GlooPreInstallKinds = append(GlooPreInstallKinds,
		"ServiceAccount",
		"Gateway",
		"Job",
		"Settings",
		"ValidatingWebhookConfiguration",
	)
	GlooPreInstallKinds = append(GlooPreInstallKinds, GlooRbacKinds...)
	GlooInstallKinds = GlooSystemKinds

	GlooGatewayUpgradeKinds = append(GlooInstallKinds, "Job")

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

	ExpectedLabels = map[string]string{
		"app": "gloo",
	}
}
