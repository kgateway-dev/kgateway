package install

import (
	"strings"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/go-utils/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/manifest"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/renderutil"
	"k8s.io/helm/pkg/tiller"
)

const (
	YamlDocumentSeparator = "\n---\n"
	CrdFilePrefix         = "crds/"
)

// Returns the Helm chart archive located at the given URI (can be either an http(s) address or a file path)
func GetHelmArchive(chartArchiveUri string) (*chart.Chart, error) {

	// Download chart archive
	chartFile, err := cliutil.GetResource(chartArchiveUri)
	if err != nil {
		return nil, err
	}
	//noinspection GoUnhandledErrorResult
	defer chartFile.Close()

	// Check chart requirements to make sure all dependencies are present in /charts
	helmChart, err := chartutil.LoadArchive(chartFile)
	if err != nil {
		return nil, errors.Wrapf(err, "loading chart archive")
	}
	return helmChart, err
}

// Extracts the sub-chart that contains templates for the CRDs that have to be applied before the main chart.
func GetCrdChart(helmChart *chart.Chart) (*chart.Chart, error) {
	var crdFiles []*chartutil.BufferedFile
	for _, file := range helmChart.Files {
		if strings.HasPrefix(file.TypeUrl, CrdFilePrefix) {
			crdFiles = append(crdFiles, &chartutil.BufferedFile{Name: strings.TrimPrefix(file.TypeUrl, CrdFilePrefix), Data: file.Value})
		}
	}
	crdChart, err := chartutil.LoadFiles(crdFiles)
	if err != nil {
		return nil, err
	}
	return crdChart, nil
}

// Searches for the value file with the given name in the chart and returns its raw content.
func GetValueFile(helmChart *chart.Chart, fileName string) (*chart.Config, error) {
	rawAdditionalValues := "{}"
	if fileName != "" {
		var found bool
		for _, valueFile := range helmChart.Files {
			if valueFile.TypeUrl == fileName {
				rawAdditionalValues = string(valueFile.Value)
			}
			found = true
		}
		if !found {
			return nil, errors.Errorf("could not find value file [%s] in Helm chart archive", fileName)
		}
	}

	// NOTE: config.Values is never used by helm
	return &chart.Config{Raw: rawAdditionalValues}, nil
}

// Renders the content of the given Helm chart archive:
//   - helmChart: the Gloo helm chart archive
//   - overrideValues: value to override the chart defaults. NOTE: passing `nil` means "ignore the chart's default values"!
//   - renderOptions: options to be used in the render
//   - filterFunctions: a collection of functions that can be used to filter and transform the contents of the manifest. Will be applied in the given order.
func RenderChart(helmChart *chart.Chart, overrideValues *chart.Config, renderOptions renderutil.Options, filterFunctions ...ManifestFilterFunc) ([]byte, error) {
	renderedTemplates, err := renderutil.Render(helmChart, overrideValues, renderOptions)
	if err != nil {
		return nil, err
	}

	manifests := tiller.SortByKind(manifest.SplitManifests(renderedTemplates))

	// Apply filter functions to manifests
	for _, filterFunc := range filterFunctions {
		manifests, err = filterFunc(manifests)
		if err != nil {
			return nil, errors.Wrapf(err, "applying filter function")
		}
	}

	// Collect manifests
	var manifestsContent []string
	for _, m := range manifests {
		manifestsContent = append(manifestsContent, m.Content)
	}

	return []byte(strings.Join(manifestsContent, YamlDocumentSeparator)), nil
}
