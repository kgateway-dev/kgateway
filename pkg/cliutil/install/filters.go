package install

import (
	"k8s.io/helm/pkg/manifest"
	"regexp"
	"strings"
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

var commentRegex = regexp.MustCompile("#.*")

func isEmptyManifest(manifest string) bool {
	removeComments := commentRegex.ReplaceAllString(manifest, "")
	removeNewlines := strings.Replace(removeComments, "\n", "", -1)
	removeDashes := strings.Replace(removeNewlines, "---", "", -1)
	return removeDashes == ""
}
