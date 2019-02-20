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

// This type represents a function that can be used to filter and transform the contents of a manifest.
// It returns three values:
//   - skip: if true, the input manifest will be excluded from the output
//   - content: if skip is false, this value will be included in the output manifest
//   - err: if != nil, the whole manifest retrieval operation will fail
type ManifestFilterFunc func(manifest manifest.Manifest) (skip bool, content string, err error)

// Renders the content of a the Helm chart archive located at the given URI.
//   - chartArchiveUri: location of the chart, this can be either an http(s) address or a file path
//   - valueFileName: if provided, the function will look for a value file with the given name in the archive and use it to override chart defaults
//   - renderOptions: options to be used in the render
//   - manifestFilter: used to filter and transform the contents of the manifest
func GetHelmManifest(chartArchiveUri, valueFileName string, opts renderutil.Options, filterFunc ManifestFilterFunc) ([]byte, error) {

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

	// Collect rendered manifests
	var validManifests []string
	for _, manifestFile := range tiller.SortByKind(manifest.SplitManifests(renderedTemplates)) {
		manifestContent := manifestFile.Content

		// Apply filter function, if provided
		if filterFunc != nil {
			skip, content, err := filterFunc(manifestFile)
			if err != nil {
				return nil, errors.Wrapf(err, "reading value file")
			}
			if skip {
				continue
			}
			manifestContent = content
		}

		validManifests = append(validManifests, manifestContent)
	}

	return []byte(strings.Join(validManifests, YamlDocumentSeparator)), nil
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
