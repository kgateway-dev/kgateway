package helmutils

import "fmt"

const (
	ChartName = "gloo"

	ChartRepositoryUrl     = "https://storage.googleapis.com/solo-public-helm"
	RemoteChartUriTemplate = "https://storage.googleapis.com/solo-public-helm/charts/%s-%s.tgz"
	RemoteChartName        = "gloo/gloo"
)

func GetRemoteChartUri(version string) string {
	return fmt.Sprintf(RemoteChartUriTemplate, ChartName, version)
}
