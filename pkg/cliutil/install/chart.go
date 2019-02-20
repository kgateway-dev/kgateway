package install

import (
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/go-utils/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/manifest"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/renderutil"
	"k8s.io/helm/pkg/tiller"
	"strings"
)

const YamlDocumentSeparator = "\n---\n"

// Renders the content of a the Helm chart archive located at the given URI.
//   - chartArchiveUri: location of the chart, this can be either an http(s) address or a file path
//   - valueFileName: if provided, the function will look for a value file with the given name in the archive and use it to override chart defaults
//   - renderOptions: options to be used in the render
//   - manifestFilter: a collection of functions that can be used to filter and transform the contents of the manifest. Will be applied in the given order.
func GetHelmManifest(chartArchiveUri, valueFileName string, opts renderutil.Options, filterFunctions ...ManifestFilterFunc) ([]byte, error) {

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

	additionalValues, err := getAdditionalValues(helmChart, valueFileName)
	if err != nil {
		return nil, errors.Wrapf(err, "reading value file")
	}

	renderedTemplates, err := renderutil.Render(helmChart, additionalValues, opts)
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

// Searches for the value file with the given name in the chart and returns its raw content.
func getAdditionalValues(helmChart *chart.Chart, fileName string) (*chart.Config, error) {
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
