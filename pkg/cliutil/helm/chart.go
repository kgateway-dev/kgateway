package helm

import (
	"io/ioutil"
	"os"

	"github.com/solo-io/gloo/pkg/cliutil"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

const tempChartFilePermissions = 0644

// Returns the Helm chart archive located at the given URI (can be either an http(s) address or a file path)
func DownloadChart(chartArchiveUri string) (*chart.Chart, error) {

	// 1. Get a reader to the chart file (remote URL or local file path)
	chartFileReader, err := cliutil.GetResource(chartArchiveUri)
	if err != nil {
		return nil, err
	}
	defer func() { _ = chartFileReader.Close() }()

	// 2. Write chart to a temporary file
	chartBytes, err := ioutil.ReadAll(chartFileReader)
	if err != nil {
		return nil, err
	}

	chartFile, err := ioutil.TempFile("", "gloo-helm-chart")
	if err != nil {
		return nil, err
	}
	charFilePath := chartFile.Name()
	defer func() { _ = os.RemoveAll(charFilePath) }()

	if err := ioutil.WriteFile(charFilePath, chartBytes, tempChartFilePermissions); err != nil {
		return nil, err
	}

	// 3. Load the chart file
	chartObj, err := loader.Load(charFilePath)
	if err != nil {
		return nil, err
	}

	return chartObj, nil
}
