package install

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/kubeutils"
	"go.uber.org/zap"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/manifest"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/renderutil"
)

type GlooKubeInstallClient interface {
	KubectlApply(manifest []byte) error
	WaitForCrdsToBeRegistered(ctx context.Context, crds []string) error
}

type DefaultGlooKubeInstallClient struct{}

func (i *DefaultGlooKubeInstallClient) KubectlApply(manifest []byte) error {
	return install.KubectlApply(manifest)
}

func (i *DefaultGlooKubeInstallClient) WaitForCrdsToBeRegistered(ctx context.Context, crds []string) error {
	return waitForCrdsToBeRegistered(ctx, crds)
}

type NamespacedGlooKubeInstallClient struct {
	Namespace string
	Delegate  GlooKubeInstallClient
	Executor  func(stdin io.Reader, args ...string) error
}

func (i *NamespacedGlooKubeInstallClient) KubectlApply(manifest []byte) error {
	if i.Namespace == "" {
		return i.Delegate.KubectlApply(manifest)
	}
	return i.Executor(bytes.NewBuffer(manifest), "apply", "-n", i.Namespace, "-f", "-")
}

func (i *NamespacedGlooKubeInstallClient) WaitForCrdsToBeRegistered(ctx context.Context, crds []string) error {
	return i.Delegate.WaitForCrdsToBeRegistered(ctx, crds)
}

func waitForCrdsToBeRegistered(ctx context.Context, crds []string) error {
	apiExts := helpers.MustApiExtsClient()
	logger := contextutils.LoggerFrom(ctx)
	for _, crdName := range crds {
		logger.Debugw("waiting for crd to be registered", zap.String("crd", crdName))
		if err := kubeutils.WaitForCrdActive(apiExts, crdName); err != nil {
			return errors.Wrapf(err, "waiting for crd %v to become registered", crdName)
		}
	}

	return nil
}

type ManifestInstaller interface {
	InstallManifest(manifest []byte) error
	InstallCrds(ctx context.Context, crdNames []string, manifest []byte) error
}

type GlooKubeManifestInstaller struct {
	GlooKubeInstallClient GlooKubeInstallClient
}

func (i *GlooKubeManifestInstaller) InstallManifest(manifest []byte) error {
	if install.IsEmptyManifest(string(manifest)) {
		return nil
	}
	if err := i.GlooKubeInstallClient.KubectlApply(manifest); err != nil {
		return errors.Wrapf(err, "running kubectl apply on manifest")
	}
	return nil
}

func (i *GlooKubeManifestInstaller) InstallCrds(ctx context.Context, crdNames []string, manifest []byte) error {
	if err := i.InstallManifest(manifest); err != nil {
		return err
	}
	if err := i.GlooKubeInstallClient.WaitForCrdsToBeRegistered(ctx, crdNames); err != nil {
		return errors.Wrapf(err, "waiting for crds to be registered")
	}
	return nil
}

type DryRunManifestInstaller struct{}

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

func (i *DryRunManifestInstaller) InstallCrds(ctx context.Context, crdNames []string, manifest []byte) error {
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
}

type DefaultGlooStagedInstaller struct {
	chart             *chart.Chart
	values            *chart.Config
	renderOpts        renderutil.Options
	excludeResources  install.ResourceMatcherFunc
	manifestInstaller ManifestInstaller
	dryRun            bool
	ctx               context.Context
}

func NewGlooStagedInstaller(opts *options.Options, spec GlooInstallSpec, client GlooKubeInstallClient) (GlooStagedInstaller, error) {
	if path.Ext(spec.HelmArchiveUri) != ".tgz" && !strings.HasSuffix(spec.HelmArchiveUri, ".tar.gz") {
		return nil, errors.Errorf("unsupported file extension for Helm chart URI: [%s]. Extension must either be .tgz or .tar.gz", spec.HelmArchiveUri)
	}

	chart, err := install.GetHelmArchive(spec.HelmArchiveUri)
	if err != nil {
		return nil, errors.Wrapf(err, "retrieving gloo helm chart archive")
	}

	values, err := install.GetValuesFromFileIncludingExtra(chart, spec.ValueFileName, spec.UserValueFileName, spec.ExtraValues, spec.ValueCallbacks...)
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

	var manifestInstaller ManifestInstaller
	if opts.Install.DryRun {
		manifestInstaller = &DryRunManifestInstaller{}
	} else {
		manifestInstaller = &GlooKubeManifestInstaller{
			GlooKubeInstallClient: client,
		}
	}

	return &DefaultGlooStagedInstaller{
		chart:             chart,
		values:            values,
		renderOpts:        renderOpts,
		excludeResources:  spec.ExcludeResources,
		manifestInstaller: manifestInstaller,
		dryRun:            opts.Install.DryRun,
		ctx:               opts.Top.Ctx,
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
		excludeNonCrdsAndCollectCrdNames,
		install.ExcludeEmptyManifests)
	if err != nil {
		return errors.Wrapf(err, "rendering crd manifests")
	}

	if !i.dryRun {
		fmt.Printf("Installing CRDs...\n")
	}

	return i.manifestInstaller.InstallCrds(i.ctx, crdNames, crdManifestBytes)
}

func (i *DefaultGlooStagedInstaller) DoPreInstall() error {
	// Render and install Gloo manifest
	manifestBytes, err := install.RenderChart(i.chart, i.values, i.renderOpts,
		install.ExcludeNotes,
		install.IncludeOnlyPreInstall,
		install.ExcludeEmptyManifests,
		install.ExcludeMatchingResources(i.excludeResources))
	if err != nil {
		return err
	}
	if !i.dryRun {
		fmt.Printf("Preparing namespace and other pre-install tasks...\n")
	}
	return i.manifestInstaller.InstallManifest(manifestBytes)
}

func (i *DefaultGlooStagedInstaller) DoInstall() error {
	// Render and install Gloo manifest
	manifestBytes, err := install.RenderChart(i.chart, i.values, i.renderOpts,
		install.ExcludeNotes,
		install.ExcludePreInstall,
		install.ExcludeCrds,
		install.ExcludeEmptyManifests,
		install.ExcludeMatchingResources(i.excludeResources))
	if err != nil {
		return err
	}
	if !i.dryRun {
		fmt.Printf("Installing...\n")
	}
	return i.manifestInstaller.InstallManifest(manifestBytes)
}
