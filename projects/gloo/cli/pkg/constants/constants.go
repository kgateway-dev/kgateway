package constants

const (
	GlooHelmRepoTemplate = "https://storage.googleapis.com/solo-public-helm/charts/gloo-%s.tgz"
	// TODO --- verify url
	GlooWithUiHelmRepoTemplate = "https://storage.googleapis.com/gloo-os-ui-helm/charts/gloo-%s.tgz"
	IngressValuesFileName      = "values-ingress.yaml"
	GatewayValuesFileName      = "" // empty because the chart will have the 'gateway' values by default
	KnativeValuesFileName      = "values-knative.yaml"
	KnativeServingNamespace    = "knative-serving"
)
