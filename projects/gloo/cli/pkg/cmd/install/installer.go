package install

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/manifest"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/renderutil"
	"path"
	"strings"
	"time"
)

type KubeInstallClient interface {
	KubectlApply(manifest []byte) error
	WaitForCrdsToBeRegistered(crds []string, timeout, interval time.Duration) error
	CheckKnativeInstallation() (bool, bool, error) // isInstalled, isOurs, error
}

type DefaultKubeInstallClient struct {}

func (i *DefaultKubeInstallClient) KubectlApply(manifest []byte) error {
	return install.KubectlApply(manifest)
}

func (i *DefaultKubeInstallClient) WaitForCrdsToBeRegistered(crds []string, timeout, interval time.Duration) error {
	return install.WaitForCrdsToBeRegistered(crds, timeout, interval)
}

func (i *DefaultKubeInstallClient) CheckKnativeInstallation() (bool, bool, error) {
	return install.CheckKnativeInstallation()
}

type ManifestInstaller interface {
	InstallManifest(manifest []byte) error
	InstallCrds(crdNames []string, manifest []byte) error
}

type KubeManifestInstaller struct {
	KubeInstallClient KubeInstallClient
}

func (i *KubeManifestInstaller) InstallManifest(manifest []byte) error {
	if install.IsEmptyManifest(string(manifest)) {
		return nil
	}
	if err := i.KubeInstallClient.KubectlApply(manifest); err != nil {
		return errors.Wrapf(err, "running kubectl apply on manifest")
	}
	return nil
}

func (i *KubeManifestInstaller) InstallCrds(crdNames []string, manifest []byte) error {
	if err := i.InstallManifest(manifest); err != nil {
		return err
	}
	if err := i.KubeInstallClient.WaitForCrdsToBeRegistered(crdNames, time.Second*5, time.Millisecond*500); err != nil {
		return errors.Wrapf(err, "waiting for crds to be registered")
	}
	return nil
}

type DryRunManifestInstaller struct {}

func (i *DryRunManifestInstaller) InstallManifest(manifest []byte) error {
	manifestString := string(manifest)
	if install.IsEmptyManifest(manifestString) {
		return nil
	}
	fmt.Printf("%s", manifestString)
	// For safety, print a YAML separator so multiple invocations of this function will produce valid output
	fmt.Println("\n---")
	return nil
}

func (i *DryRunManifestInstaller) InstallCrds(crdNames []string, manifest []byte) error {
	return i.InstallManifest(manifest)
}

type KnativeInstallStatus struct {
	isInstalled bool
	isOurs      bool
}

type GlooStagedInstaller interface {
	DoCrdInstall() error
	DoPreInstall() error
	DoInstall() error
	DoKnativeInstall() error
}

type DefaultGlooStagedInstaller struct {
	chart *chart.Chart
	values *chart.Config
	renderOpts renderutil.Options
	knativeInstallStatus KnativeInstallStatus
	excludeResources install.ResourceMatcherFunc
	manifestInstaller ManifestInstaller
}

func NewGlooStagedInstaller(opts *options.Options, spec GlooInstallSpec, client KubeInstallClient) (GlooStagedInstaller, error) {
	if path.Ext(spec.HelmArchiveUri) != ".tgz" && !strings.HasSuffix(spec.HelmArchiveUri, ".tar.gz") {
		return nil, errors.Errorf("unsupported file extension for Helm chart URI: [%s]. Extension must either be .tgz or .tar.gz", spec.HelmArchiveUri)
	}

	chart, err := install.GetHelmArchive(spec.HelmArchiveUri)
	if err != nil {
		return nil, errors.Wrapf(err, "retrieving gloo helm chart archive")
	}

	values, err := install.GetValuesFromFileIncludingExtra(chart, spec.ValueFileName, spec.ExtraValues)
	if err != nil {
		return nil, errors.Wrapf(err, "retrieving value file: %s", spec.ValueFileName)
	}

	// These are the .Release.* variables used during rendering
	renderOpts := renderutil.Options{
		ReleaseOptions: chartutil.ReleaseOptions{
			Namespace: opts.Install.Namespace,
			Name:      spec.ProductName,
		},
	}

	isInstalled, isOurs, err := client.CheckKnativeInstallation()
	if err != nil {
		return nil, err
	}
	knativeInstallStatus := KnativeInstallStatus{
		isInstalled: isInstalled,
		isOurs: isOurs,
	}

	var manifestInstaller ManifestInstaller
	if opts.Install.DryRun {
		manifestInstaller = &DryRunManifestInstaller{}
	} else {
		manifestInstaller = &KubeManifestInstaller{
			KubeInstallClient: client,
		}
	}

	return &DefaultGlooStagedInstaller{
		chart: chart,
		values: values,
		renderOpts: renderOpts,
		knativeInstallStatus: knativeInstallStatus,
		excludeResources: nil,
		manifestInstaller: manifestInstaller,
	}, nil
}

func (i *DefaultGlooStagedInstaller) DoCrdInstall() error {

	// Keep only CRDs and collect the names
	var crdNames []string
	excludeNonCrdsAndCollectCrdNames := func(input []manifest.Manifest) ([]manifest.Manifest, error) {
		manifests, resourceNames, err := install.ExcludeNonCrds(input)
		crdNames = resourceNames
		return manifests, err
	}

	// Render and install CRD manifests
	crdManifestBytes, err := install.RenderChart(i.chart, i.values, i.renderOpts,
		install.ExcludeNotes,
		install.KnativeResourceFilterFunction(i.knativeInstallStatus.isInstalled),
		excludeNonCrdsAndCollectCrdNames,
		install.ExcludeEmptyManifests)
	if err != nil {
		return errors.Wrapf(err, "rendering crd manifests")
	}

	return i.manifestInstaller.InstallCrds(crdNames, crdManifestBytes)
}

func (i *DefaultGlooStagedInstaller) DoPreInstall() error {
	// Render and install Gloo manifest
	manifestBytes, err := install.RenderChart(i.chart, i.values, i.renderOpts,
		install.ExcludeNotes,
		install.ExcludeKnative,
		install.IncludeOnlyPreInstall,
		install.ExcludeEmptyManifests,
		install.ExcludeMatchingResources(i.excludeResources))
	if err != nil {
		return err
	}
	return i.manifestInstaller.InstallManifest(manifestBytes)
}

func (i *DefaultGlooStagedInstaller) DoInstall() error {
	// Render and install Gloo manifest
	manifestBytes, err := install.RenderChart(i.chart, i.values, i.renderOpts,
		install.ExcludeNotes,
		install.ExcludeKnative,
		install.ExcludePreInstall,
		install.ExcludeCrds,
		install.ExcludeEmptyManifests,
		install.ExcludeMatchingResources(i.excludeResources))
	if err != nil {
		return err
	}
	return i.manifestInstaller.InstallManifest(manifestBytes)
}

// This is a bit tricky. The manifest is already filtered based on the values file. If the values file includes
// knative stuff, then we may want to do a knative install -- if there isn't an install already, or if there is
// an install and it's ours (i.e. an upgrade)
func (i *DefaultGlooStagedInstaller) DoKnativeInstall() error {
	// Exclude everything but knative non-crds
	manifestBytes, err := install.RenderChart(i.chart, i.values, i.renderOpts,
		install.ExcludeNonKnative,
		install.KnativeResourceFilterFunction(i.knativeInstallStatus.isInstalled && !i.knativeInstallStatus.isOurs),
		install.ExcludeCrds)
	if err != nil {
		return err
	}
	return i.manifestInstaller.InstallManifest(manifestBytes)
}

