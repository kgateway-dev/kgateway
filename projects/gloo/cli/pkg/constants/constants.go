package constants

const (
	GlooHelmRepoTemplate    = "https://storage.googleapis.com/solo-public-helm/charts/gloo-%s.tgz"
	IngressValuesFileName   = "values-ingress.yaml"
	KnativeValuesFileName   = "values-knative.yaml"
	KnativeCrdsUrlTemplate  = "https://github.com/solo-io/gloo/releases/download/v%s/knative-crds-0.3.0.yaml"
	KnativeUrlTemplate      = "https://github.com/solo-io/gloo/releases/download/v%s/knative-no-istio-0.3.0.yaml"
	KnativeServingNamespace = "knative-serving"
)
