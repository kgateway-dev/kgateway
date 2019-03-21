package install

import (
	"regexp"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/helm/helm/pkg/hooks"
	"github.com/solo-io/go-utils/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/helm/pkg/manifest"
)

// This type represents a function that can be used to filter and transform a list of manifests.
// It returns three values:
//   - skip: if true, the input manifest will be excluded from the output
//   - content: if skip is false, this value will be included in the output manifest
//   - err: if != nil, the whole manifest retrieval operation will fail
type ManifestFilterFunc func(input []manifest.Manifest) (output []manifest.Manifest, err error)

// We need to define this ourselves, because if we unmarshal into `apiextensions.CustomResourceDefinition`
// we don't get the ObjectMeta (in the yaml they are nested under `metadata`, but the k8s struct has
// them as top level fields...)
type resourceType struct {
	Metadata v1.ObjectMeta
	v1.TypeMeta
}

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

type resourceMatcherFunc func(resource resourceType) (bool, error)

var preInstallMatcher resourceMatcherFunc = func(resource resourceType) (bool, error) {
	helmPreInstallHook, ok := resource.Metadata.Annotations[hooks.HookAnno]
	if !ok || helmPreInstallHook != hooks.PreInstall {
		return false, nil
	}
	return true, nil
}

var crdInstallMatcher resourceMatcherFunc = func(resource resourceType) (bool, error) {
	crdKind := resource.TypeMeta.Kind == CrdKindName
	if crdKind {
		// Check whether the CRD is a Helm "crd-install" hook.
		// If not, throw an error, because this will cause race conditions when installing with Helm (which is
		// not the case here, but we want to validate the manifests whenever we have the chance)
		helmCrdInstallHookAnnotation, ok := resource.Metadata.Annotations[hooks.HookAnno]
		if !ok || helmCrdInstallHookAnnotation != hooks.CRDInstall {
			return crdKind, errors.Errorf("CRD [%s] must be annotated as a Helm '%s' hook", resource.Metadata.Name, hooks.CRDInstall)
		}
	}
	return crdKind, nil
}

var nonCrdInstallMatcher resourceMatcherFunc = func(resource resourceType) (bool, error) {
	isCrdInstall, err := crdInstallMatcher(resource)
	return !isCrdInstall, err
}

var nonPreInstallMatcher resourceMatcherFunc = func(resource resourceType) (bool, error) {
	isPreInstall, err := preInstallMatcher(resource)
	return !isPreInstall, err
}

var excludeByMatcher = func(input []manifest.Manifest, matches resourceMatcherFunc) (output []manifest.Manifest, resourceNames []string, err error) {
	resourceNames = make([]string, 0)
	for _, man := range input {
		// Split manifest into individual YAML docs
		nonMatching := make([]string, 0)
		for _, doc := range strings.Split(man.Content, "---") {

			var resource resourceType
			if err := yaml.Unmarshal([]byte(doc), &resource); err != nil {
				return nil, nil, errors.Wrapf(err, "parsing resource: %s", doc)
			}

			isMatch, err := matches(resource)
			if err != nil {
				return nil, nil, err
			}
			if !isMatch {
				resourceNames = append(resourceNames, resource.Metadata.Name)
				nonMatching = append(nonMatching, doc)
			}
		}

		output = append(output, manifest.Manifest{
			Name:    man.Name,
			Head:    man.Head,
			Content: strings.Join(nonMatching, YamlDocumentSeparator),
		})
	}
	return
}

// Filters out any pre-install from each manifest
var ExcludePreInstall ManifestFilterFunc = func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
	manifest, _, err := excludeByMatcher(input, preInstallMatcher)
	return manifest, err
}

// Filters out anything but pre-install
var IncludeOnlyPreInstall ManifestFilterFunc = func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
	manifest, _, err := excludeByMatcher(input, nonPreInstallMatcher)
	return manifest, err
}

// Filters out any CRD from each manifest
var ExcludeCrds ManifestFilterFunc = func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
	manifest, _, err := excludeByMatcher(input, crdInstallMatcher)
	return manifest, err
}

var ExcludeNonCrds = func(input []manifest.Manifest) (output []manifest.Manifest, names []string, err error) {
	return excludeByMatcher(input, nonCrdInstallMatcher)
}

// Filters out NOTES.txt files
var ExcludeNotes ManifestFilterFunc = func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
	for _, man := range input {
		if strings.HasSuffix(man.Name, NotesFileName) {
			continue
		}
		output = append(output, man)
	}
	return
}

// If this is a knative deployment, we have to check whether knative itself is already installed in the cluster.
// If knative is already installed and we don't own it, don't install/upgrade/uninstall it (It's okay to update the installation if we own it).
func SkipKnativeInstall() (bool, error) {
	installed, ours, err := CheckKnativeInstallation()
	if err != nil {
		return true, errors.Wrapf(err, "checking for knative installation")
	}
	skipKnativeInstall := installed && !ours
	return skipKnativeInstall, nil
}

func KnativeResourceFilterFunction(skipKnativeInstall bool) ManifestFilterFunc {
	return func(input []manifest.Manifest) ([]manifest.Manifest, error) {
		var output []manifest.Manifest
		for _, man := range input {
			if strings.Contains(man.Name, "knative") && skipKnativeInstall {
				continue
			}
			output = append(output, man)
		}
		return output, nil
	}
}

var ExcludeNonKnative ManifestFilterFunc = func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
	for _, man := range input {
		if !strings.Contains(man.Name, "knative") {
			continue
		}
		output = append(output, man)
	}
	return output, nil
}

var commentRegex = regexp.MustCompile("#.*")

func isEmptyManifest(manifest string) bool {
	removeComments := commentRegex.ReplaceAllString(manifest, "")
	removeNewlines := strings.Replace(removeComments, "\n", "", -1)
	removeDashes := strings.Replace(removeNewlines, "---", "", -1)
	return removeDashes == ""
}

func getKinds(manifest string) ([]string, error) {
	var kinds []string
	for _, doc := range strings.Split(manifest, "---") {
		var resource resourceType
		if err := yaml.Unmarshal([]byte(doc), &resource); err != nil {
			return nil, errors.Wrapf(err, "parsing resource: %s", doc)
		}
		kinds = append(kinds, resource.Kind)
	}
	return kinds, nil
}

func validateResourceLabels(manifest string, labels map[string]string) error {
	if labels == nil {
		return nil
	}
	for _, doc := range strings.Split(manifest, "---") {
		var resource resourceType
		if err := yaml.Unmarshal([]byte(doc), &resource); err != nil {
			return errors.Wrapf(err, "parsing resource: %s", doc)
		}
		actualLabels := resource.Metadata.Labels
		for k, v := range labels {
			val, ok := actualLabels[k]
			if !ok || v != val {
				return errors.Errorf("validating labels: expected %s=%s", k, v)
			}
		}
	}
	return nil
}