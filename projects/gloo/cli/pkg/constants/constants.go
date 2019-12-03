package constants

const (
	GlooHelmRepoTemplate       = "https://storage.googleapis.com/solo-public-helm/charts/gloo-%s.tgz"
	GlooWithUiHelmRepoTemplate = "https://storage.googleapis.com/gloo-os-ui-helm/charts/gloo-os-with-ui-%s.tgz"
	GlooReleaseName            = "gloo"
	IngressValuesFileName      = "values-ingress.yaml"
	GatewayValuesFileName      = "" // empty because the chart will have the 'gateway' values by default
	KnativeValuesFileName      = "values-knative.yaml"
	GlooReleaseName            = "gloo"
	KnativeServingNamespace    = "knative-serving"
	// This annotation is present on resources that are included in the chart only to clean up hooks.
	// We use it to filter out those resources wherever that it necessary.
	HookCleanupResourceAnnotation = "solo.io/hook-cleanup"
)
