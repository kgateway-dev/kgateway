package helmutils

import "fmt"

const (
	ChartName = "gloo"

	ChartRepositoryUrl     = "https://storage.googleapis.com/solo-public-helm"
	PrChartRepositoryUrl   = "https://storage.googleapis.com/solo-public-tagged-helm"
	RemoteChartUriTemplate = "https://storage.googleapis.com/solo-public-helm/charts/gloo-%s.tgz"
	RemoteChartName        = "gloo/gloo"
)

func GetRemoteChartUri(version string) string {
	return fmt.Sprintf(RemoteChartUriTemplate, version)
}
