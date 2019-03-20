package install

import (
	"fmt"
	"path"
	"strings"
	"time"

	"k8s.io/helm/pkg/proto/hapi/chart"

	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/go-utils/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/manifest"
	"k8s.io/helm/pkg/renderutil"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
)


var namespacedKinds []string
var clusterWideKinds []string
var otherAllowedKinds []string

func init() {
	namespacedKinds = []string {
		"Deployment",
		"Service",
		"ConfigMap",
		"ReplicaSet",
	}

	clusterWideKinds = []string {
		"CustomResourceDefinition",
		"ClusterRole",
		"ClusterRoleBinding",
	}

	otherAllowedKinds = []string {
		"Settings",
		"Namespace",
	}
}

// Entry point for all three GLoo installation commands
func installGloo(opts *options.Options, valueFileName string) error {

	// Get Gloo release version
	glooVersion, err := getGlooVersion(opts)
	if err != nil {
		return err
	}

	// Get location of Gloo helm chart
	helmChartArchiveUri := fmt.Sprintf(constants.GlooHelmRepoTemplate, glooVersion)
	if helmChartOverride := opts.Install.HelmChartOverride; helmChartOverride != "" {
		helmChartArchiveUri = helmChartOverride
	}

	if err := installFromUri(helmChartArchiveUri, opts, valueFileName); err != nil {
		return errors.Wrapf(err, "installing Gloo from helm chart")
	}
	return nil
}

func getGlooVersion(opts *options.Options) (string, error) {
	if !version.IsReleaseVersion() && opts.Install.HelmChartOverride == "" {
		return "", errors.Errorf("you must provide a Gloo Helm chart URI via the 'file' option " +
			"when running an unreleased version of glooctl")
	}
	return version.Version, nil

}

func installFromUri(helmArchiveUri string, opts *options.Options, valuesFileName string) error {

	if path.Ext(helmArchiveUri) != ".tgz" && !strings.HasSuffix(helmArchiveUri, ".tar.gz") {
		return errors.Errorf("unsupported file extension for Helm chart URI: [%s]. Extension must either be .tgz or .tar.gz", helmArchiveUri)
	}

	chart, err := install.GetHelmArchive(helmArchiveUri)
	if err != nil {
		return errors.Wrapf(err, "retrieving gloo helm chart archive")
	}

	values, err := install.GetValuesFromFile(chart, valuesFileName)
	if err != nil {
		return errors.Wrapf(err, "retrieving value file: %s", valuesFileName)
	}

	// These are the .Release.* variables used during rendering
	renderOpts := renderutil.Options{
		ReleaseOptions: chartutil.ReleaseOptions{
			Namespace: opts.Install.Namespace,
			Name:      "gloo",
		},
	}

	skipKnativeInstall, err := install.SkipKnativeInstall()
	if err != nil {
		return err
	}

	knativeFunc := install.KnativeResourceFilterFunction(skipKnativeInstall)

	if err := doCrdInstall(opts, chart, values, renderOpts, knativeFunc); err != nil {
		return err
	}

	if err := doPreInstall(opts, chart, values, renderOpts); err != nil {
		return err
	}

	if !skipKnativeInstall {
		if err := doKnativeInstall(opts, chart, values, renderOpts); err != nil {
			return err
		}
	}

	return doInstall(opts, chart, values, renderOpts)
}

func doCrdInstall(
	opts *options.Options,
	chart *chart.Chart,
	values *chart.Config,
	renderOpts renderutil.Options,
	knativeFilterFunction install.ManifestFilterFunc) error {

	// Keep only CRDs and collect the names
	var crdNames []string
	excludeNonCrdsAndCollectCrdNames := func(input []manifest.Manifest) ([]manifest.Manifest, error) {
		manifests, resourceNames, err := install.ExcludeNonCrds(input)
		crdNames = resourceNames
		return manifests, err
	}

	// Render and install CRD manifests
	crdManifestBytes, err := install.RenderChart(chart, values, renderOpts,
		install.ExcludeNotes,
		knativeFilterFunction,
		excludeNonCrdsAndCollectCrdNames,
		install.ExcludeEmptyManifests)
	if err != nil {
		return errors.Wrapf(err, "rendering crd manifests")
	}
	if err := install.InstallManifest(crdManifestBytes, opts.Install.DryRun); err != nil {
		return errors.Wrapf(err, "installing crd manifests")
	}

	// Only run if this is not a dry run
	if !opts.Install.DryRun {
		if err := install.WaitForCrdsToBeRegistered(crdNames, time.Second*5, time.Millisecond*500); err != nil {
			return errors.Wrapf(err, "waiting for crds to be registered")
		}
	}

	return nil
}

func doPreInstall(
	opts *options.Options,
	chart *chart.Chart,
	values *chart.Config,
	renderOpts renderutil.Options) error {
	// Render and install Gloo manifest
	manifestBytes, err := install.RenderChart(chart, values, renderOpts,
		install.ExcludeNotes,
		install.KnativeResourceFilterFunction(true),
		install.IncludeOnlyPreInstall,
		install.ExcludeEmptyManifests)
	if err != nil {
		return err
	}
	return install.InstallManifest(manifestBytes, opts.Install.DryRun)
}

func doInstall(
	opts *options.Options,
	chart *chart.Chart,
	values *chart.Config,
	renderOpts renderutil.Options) error {
	// Render and install Gloo manifest
	manifestBytes, err := install.RenderChart(chart, values, renderOpts,
		install.ExcludeNotes,
		install.KnativeResourceFilterFunction(true),
		install.ExcludePreInstall,
		install.ExcludeCrds,
		install.ExcludeEmptyManifests)
	if err != nil {
		return err
	}
	return install.InstallManifest(manifestBytes, opts.Install.DryRun)
}

func doKnativeInstall(
	opts *options.Options,
	chart *chart.Chart,
	values *chart.Config,
	renderOpts renderutil.Options) error {
	// Exclude everything but knative non-crds
	manifestBytes, err := install.RenderChart(chart, values, renderOpts,
		install.ExcludeNonKnative,
		install.ExcludeCrds)
	if err != nil {
		return err
	}
	return install.InstallManifest(manifestBytes, opts.Install.DryRun)
}

