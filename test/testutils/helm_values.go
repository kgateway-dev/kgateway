package testutils

import (
	"encoding/json"
	"path"

	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/gloo/pkg/utils/helmutils"
	"helm.sh/helm/v3/pkg/strvals"
	"knative.dev/pkg/test/helpers"
	k8syamlutil "sigs.k8s.io/yaml"
)

// HelmValues is a struct that holds the values that will be passed to a Helm chart
type HelmValues struct {
	ValuesFile string
	ValuesArgs []string // each entry should look like `path.to.helm.field=value`
}

// ValidateHelmValues ensures that the unstructured helm values that are provided
// to a chart match the Go type used to generate the Helm documentation
// Returns nil if all the provided values are all included in the Go struct
// Returns an error if a provided value is not included in the Go struct.
//
// Example:
//
//	Failed to render manifest
//	    Unexpected error:
//	        <*errors.errorString | 0xc000fedf40>: {
//	            s: "error unmarshaling JSON: while decoding JSON: json: unknown field \"useTlsTagging\"",
//	        }
//	        error unmarshaling JSON: while decoding JSON: json: unknown field "useTlsTagging"
//	    occurred
//
// This means that the unstructured values provided to the Helm chart contain a field `useTlsTagging`
// but the Go struct does not contain that field.
func ValidateHelmValues(unstructuredHelmValues map[string]interface{}) error {
	// This Go type is the source of truth for the Helm docs
	var structuredHelmValues generate.HelmConfig

	unstructuredHelmValueBytes, err := json.Marshal(unstructuredHelmValues)
	if err != nil {
		return err
	}

	// This ensures that an error will be raised if there is an unstructured helm value
	// defined but there is not the equivalent type defined in our Go struct
	//
	// When an error occurs, this means the Go type needs to be amended
	// to include the new field (which is the source of truth for our docs)
	return k8syamlutil.UnmarshalStrict(unstructuredHelmValueBytes, &structuredHelmValues)
}

// BuildHelmValues reads the base values.yaml file from a Helm chart and merges it with the provided values
// each entry in valuesArgs should look like `path.to.helm.field=value`
func BuildHelmValues(values HelmValues) (map[string]interface{}, error) {
	// read the chart's base values file first
	rootDir, err := helpers.GetRootDir()
	if err != nil {
		return nil, err
	}
	chartPath := path.Join(rootDir, "install", "helm", "gloo")

	return BuildHelmValuesForChart(chartPath, values)
}

// BuildHelmValuesForChart reads the base values.yaml file from a Helm chart and merges it with the provided values
// each entry in valuesArgs should look like `path.to.helm.field=value`
func BuildHelmValuesForChart(chartPath string, values HelmValues) (map[string]interface{}, error) {
	// read the chart's base values file first
	finalValues, err := helmutils.UnmarshalValuesFile(path.Join(chartPath, "values.yaml"))
	if err != nil {
		return nil, err
	}

	for _, v := range values.ValuesArgs {
		err := strvals.ParseInto(v, finalValues)
		if err != nil {
			return nil, err
		}
	}

	if values.ValuesFile != "" {
		// these lines ripped out of Helm internals
		// https://github.com/helm/helm/blob/release-3.0/pkg/cli/values/options.go
		mapFromFile, err := helmutils.UnmarshalValuesFile(values.ValuesFile)
		if err != nil {
			return nil, err
		}

		// Merge with the previous map
		finalValues = helmutils.MergeMaps(finalValues, mapFromFile)
	}

	return finalValues, nil
}
