package install

import (
	"github.com/ghodss/yaml"
	"github.com/solo-io/go-utils/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"regexp"
	"strings"

	"k8s.io/helm/pkg/manifest"
)

// This type represents a function that can be used to filter and transform a list of manifests.
// It returns three values:
//   - skip: if true, the input manifest will be excluded from the output
//   - content: if skip is false, this value will be included in the output manifest
//   - err: if != nil, the whole manifest retrieval operation will fail
type ManifestFilterFunc func(input []manifest.Manifest) (output []manifest.Manifest, err error)

// Returns only non-empty manifests
var ExcludeEmptyManifests ManifestFilterFunc = func(input []manifest.Manifest) ([]manifest.Manifest, error) {
	var output []manifest.Manifest
	for _, manifest := range input {
		if !isEmptyManifest(manifest.Content) {
			output = append(output, manifest)
		}

	}
	return output, nil
}

// Filters out any CRD from each manifest
var ExcludeCrds ManifestFilterFunc = func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
	for _, man := range input {

		// Split manifest into individual YAML docs
		nonCrdDocs := make([]string, 0)
		for _, doc := range strings.Split(man.Content, "---") {

			// We need to define this ourselves, because if we unmarshal into `apiextensions.CustomResourceDefinition`
			// we don't get the ObjectMeta (in the yaml they are nested under `metadata`, but the k8s struct has
			// them as top level fields...)
			var resource struct {
				Metadata v1.ObjectMeta
				v1.TypeMeta
			}
			if err := yaml.Unmarshal([]byte(doc), &resource); err != nil {
				return nil, errors.Wrapf(err, "parsing resource: %s", doc)
			}

			// Keep only non-CRD resources
			if resource.TypeMeta.Kind != CrdKindName {
				nonCrdDocs = append(nonCrdDocs, doc)
			}
		}

		output = append(output, manifest.Manifest{
			Name:    man.Name,
			Head:    man.Head,
			Content: strings.Join(nonCrdDocs, YamlDocumentSeparator),
		})
	}
	return
}

// If this is a knative deployment, we have to check whether knative itself is already installed in the cluster.
// If knative is already installed and we don't own it, don't install/upgrade it (It's okay to update the installation if we own it).

// If this is not a knative deployment, skipKnativeInstall might still evaluate to true, but in that case Helm will
// filter out all the knative resources during template rendering.
func GetKnativeResourceFilterFunction() (ManifestFilterFunc, error) {
	installed, ours, err := CheckKnativeInstallation()
	if err != nil {
		return nil, errors.Wrapf(err, "checking for knative installation")
	}
	skipKnativeInstall := installed && !ours
	return func(input []manifest.Manifest) ([]manifest.Manifest, error) {
		var output []manifest.Manifest
		for _, man := range input {
			if strings.Contains(man.Name, "knative") && skipKnativeInstall {
				continue
			}
			output = append(output, man)
		}
		return output, nil
	}, nil
}

var commentRegex = regexp.MustCompile("#.*")

func isEmptyManifest(manifest string) bool {
	removeComments := commentRegex.ReplaceAllString(manifest, "")
	removeNewlines := strings.Replace(removeComments, "\n", "", -1)
	removeDashes := strings.Replace(removeNewlines, "---", "", -1)
	return removeDashes == ""
}
