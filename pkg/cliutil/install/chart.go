package install

import (
	"io/ioutil"
	"log"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/solo-io/gloo/install/helm/gloo/generate"

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
	CrdKindName           = "CustomResourceDefinition"
	NotesFileName         = "NOTES.txt"
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

// use to overwrite / modify values file before passing to helm
type ValuesCallback func(config *generate.HelmConfig)

// Searches for the value file with the given name in the chart and returns its raw content.
// NOTE: this also sets the namespace.create attribute to 'true'.
func GetValuesFromFileIncludingExtra(helmChart *chart.Chart, fileName string, userValuesFileNames []string, extraValues chartutil.Values, valueOptions ...ValuesCallback) (*chart.Config, error) {
	rawAdditionalValues := "{}"
	if fileName != "" {
		var found bool
		for _, valueFile := range helmChart.Files {
			if valueFile.TypeUrl == fileName {
				rawAdditionalValues = string(valueFile.Value)
				found = true
				break
			}
		}
		if !found {
			return nil, errors.Errorf("could not find value file [%s] in Helm chart archive", fileName)
		}
	}

	// Convert value file content to struct
	valueStruct := &generate.HelmConfig{}
	if err := yaml.Unmarshal([]byte(rawAdditionalValues), valueStruct); err != nil {
		return nil, errors.Errorf("invalid format for value file [%s] in Helm chart archive", fileName)
	}

	// Namespace creation is disabled by default, otherwise install with helm will fail
	// (`helm install --namespace=<namespace_name>` creates the given namespace)
	valueStruct.Namespace = &generate.Namespace{Create: true}

	for _, opt := range valueOptions {
		opt(valueStruct)
	}

	valueBytes, err := yaml.Marshal(valueStruct)
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshaling value file struct")
	}

	// unmarshal to helm values so we can merge
	values, err := chartutil.ReadValues(valueBytes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading values")
	}

	if extraValues != nil {
		values.MergeInto(extraValues)
	}

	for _, userValuesFileName := range userValuesFileNames {
		if userValuesFileName == "" {
			continue
		}
		uservalues, err := ioutil.ReadFile(userValuesFileName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed reading user values "+userValuesFileName)
		}
		userValues, err := chartutil.ReadValues(uservalues)
		if err != nil {
			return nil, errors.Wrapf(err, "failed parsing user values")
		}
		values.MergeInto(userValues)
	}

	valuesString, err := values.YAML()
	if err != nil {
		return nil, errors.Wrapf(err, "failed values struct")
	}

	// NOTE: config.Values is never used by helm
	return &chart.Config{Raw: valuesString}, nil
}

func GetValuesFromFile(helmChart *chart.Chart, fileName string) (*chart.Config, error) {
	return GetValuesFromFileIncludingExtra(helmChart, fileName, []string{""}, nil)
}

// Renders the content of the given Helm chart archive:
//   - helmChart: the Gloo helm chart archive
//   - overrideValues: value to override the chart defaults. NOTE: passing `nil` means "ignore the chart's default values"!
//   - renderOptions: options to be used in the render
//   - filterFunctions: a collection of functions that can be used to filter and transform the contents of the manifest. Will be applied in the given order.
func RenderChart(helmChart *chart.Chart, overrideValues *chart.Config, renderOptions renderutil.Options, filterFunctions ...ManifestFilterFunc) ([]byte, error) {
	// Helm uses the standard go log package. Redirect its output to the debug.log file  so that we don't
	// expose useless warnings to the user.
	log.SetOutput(cliutil.GetLogger())

	// - Rendering the helm chart locally in Go side-effects the provided helm chart, removing any dependencies
	//   that were not needed but neglects to remove those same deps from the requirements.lock file.
	//   Thus, if we try to render the same chart twice in a row (while disabling some subcharts), the second
	//   requirements.lock check (CheckDependencies() in renderutil.Render()) will fail because it expects all
	//   subcharts to be there, and the provided chart has already had its dependencies pruned
	// - To avoid this, we make a copy of the dependencies before render, and restore them to undo the side-effect
	var origDeps []*chart.Chart
	for _, dep := range helmChart.Dependencies {
		origDeps = append(origDeps, dep)
	}
	renderedTemplates, err := renderutil.Render(helmChart, overrideValues, renderOptions)
	if err != nil {
		return nil, err
	}
	helmChart.Dependencies = origDeps

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
