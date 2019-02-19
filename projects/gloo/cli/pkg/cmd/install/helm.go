package install

import (
	"fmt"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/go-utils/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/manifest"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/renderutil"
	"k8s.io/helm/pkg/tiller"
	"regexp"
	"strings"
)

const (
	glooHelmRepo = "https://storage.googleapis.com/solo-public-helm/charts/gloo-%s.tgz"
)

var defaultKubeVersion = fmt.Sprintf("%s.%s", chartutil.DefaultKubeVersion.Major, chartutil.DefaultKubeVersion.Minor)

func getHelmManifestBytes(opts *options.Options, overrideUri string) ([]byte, error) {
	manifests, err := getManifests(opts, overrideUri)
	if err != nil {
		return nil, err
	}
	return convertManifestToBytes(manifests)
}

func getManifests(opts *options.Options, overrideUri string) ([]manifest.Manifest, error) {
	if overrideUri == "" {
		overrideUri = fmt.Sprintf(glooHelmRepo, version.Version)
	}
	file, err := readFile(overrideUri)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	renderOpts := renderutil.Options{
		ReleaseOptions: chartutil.ReleaseOptions{
			Namespace: opts.Install.Namespace,
			Name:      "gloo",
		},
		KubeVersion: defaultKubeVersion,
	}

	// Check chart requirements to make sure all dependencies are present in /charts
	c, err := chartutil.LoadArchive(file)
	if err != nil {
		return nil, errors.Wrapf(err, "loading chart")
	}

	// config.Values is never used by helm
	config := &chart.Config{Raw: "{}"}
	renderedTemplates, err := renderutil.Render(c, config, renderOpts)
	if err != nil {
		return nil, err
	}

	manifests := manifest.SplitManifests(renderedTemplates)
	return tiller.SortByKind(manifests), nil
}

func convertManifestToBytes(manifests []manifest.Manifest) ([]byte, error) {
	validManifests := make([]string, len(manifests))
	for _, manifest := range manifests {
		if !isEmptyManifest(manifest.Content) {
			validManifests = append(validManifests, manifest.Content)
		}
	}
	finalManifest := strings.Join(validManifests, "\n---\n")
	return []byte(finalManifest), nil
}

var commentRegex = regexp.MustCompile("#.*")

func isEmptyManifest(manifest string) bool {
	removeComments := commentRegex.ReplaceAllString(manifest, "")
	removeNewlines := strings.Replace(removeComments, "\n", "", -1)
	removeDashes := strings.Replace(removeNewlines, "---", "", -1)
	return removeDashes == ""
}
