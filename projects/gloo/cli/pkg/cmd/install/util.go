package install

import (
	"fmt"
	"os"

	"github.com/solo-io/gloo/install/helm/gloo/generate"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/go-utils/errors"
)

var (
	// These will get cleaned up by uninstall always
	GlooSystemKinds []string
	// These will get cleaned up only if uninstall all is chosen
	GlooRbacKinds []string
	// These will get cleaned up by uninstall if delete-crds or all is chosen
	GlooCrdNames []string

	// Set up during pre-install (for OS gloo, namespace only)
	GlooPreInstallKinds     []string
	GlooInstallKinds        []string
	GlooGatewayUpgradeKinds []string
	ExpectedLabels          map[string]string

	KnativeCrdNames []string
)

func init() {
	GlooPreInstallKinds = []string{"Namespace"}

	GlooSystemKinds = []string{
		"Deployment",
		"Service",
		"ConfigMap",
	}

	GlooRbacKinds = []string{
		"ClusterRole",
		"ClusterRoleBinding",
	}

	GlooInstallKinds = append(GlooSystemKinds, GlooRbacKinds...)

	GlooGatewayUpgradeKinds = append(GlooInstallKinds, "Job")

	GlooCrdNames = []string{
		"gateways.gateway.solo.io.v2",
		"proxies.gloo.solo.io",
		"settings.gloo.solo.io",
		"upstreams.gloo.solo.io",
		"upstreamgroups.gloo.solo.io",
		"virtualservices.gateway.solo.io",
	}

	KnativeCrdNames = []string{
		"virtualservices.networking.istio.io",
		"certificates.networking.internal.knative.dev",
		"clusteringresses.networking.internal.knative.dev",
		"configurations.serving.knative.dev",
		"images.caching.internal.knative.dev",
		"podautoscalers.autoscaling.internal.knative.dev",
		"revisions.serving.knative.dev",
		"routes.serving.knative.dev",
		"services.serving.knative.dev",
		"serverlessservices.networking.internal.knative.dev",
	}

	ExpectedLabels = map[string]string{
		"app": "gloo",
	}
}

type GlooInstallSpec struct {
	ProductName       string // gloo or glooe
	HelmArchiveUri    string
	ValueFileName     string
	UserValueFileName string
	ExtraValues       map[string]string
	ValueCallbacks    []install.ValuesCallback
	ExcludeResources  install.ResourceMatcherFunc
}

// Entry point for all three GLoo installation commands
func installGloo(opts *options.Options, valueFileName string) error {
	if !opts.Install.DryRun {
		fmt.Printf("Starting Gloo installation...\n")
	}
	spec, err := GetInstallSpec(opts, valueFileName)
	if err != nil {
		return err
	}
	kubeInstallClient := DefaultGlooKubeInstallClient{}
	if err := InstallGloo(opts, *spec, &kubeInstallClient); err != nil {
		fmt.Fprintf(os.Stderr, "\nGloo failed to install! Detailed logs available at %s.\n", cliutil.GetLogsPath())
		return err
	}
	if !opts.Install.DryRun {
		fmt.Printf("\nGloo was successfully installed!\n")
	}
	return nil
}

func GetInstallSpec(opts *options.Options, valueFileName string) (*GlooInstallSpec, error) {
	// Get Gloo release version
	glooVersion, err := getGlooVersion(opts)
	if err != nil {
		return nil, err
	}

	// Get location of Gloo helm chart
	helmChartArchiveUri := fmt.Sprintf(constants.GlooHelmRepoTemplate, glooVersion)
	if helmChartOverride := opts.Install.HelmChartOverride; helmChartOverride != "" {
		helmChartArchiveUri = helmChartOverride
	}

	var extraValues map[string]string
	if opts.Install.Upgrade {
		extraValues = map[string]string{"gateway": "{upgrade: true}"}
	}
	var valueCallbacks []install.ValuesCallback
	if opts.Install.Knative.InstallKnativeVersion != "" {
		valueCallbacks = append(valueCallbacks, func(config *generate.HelmConfig) {
			if config.Settings != nil &&
				config.Settings.Integrations != nil &&
				config.Settings.Integrations.Knative != nil &&
				config.Settings.Integrations.Knative.Enabled != nil &&
				*config.Settings.Integrations.Knative.Enabled {

				config.Settings.Integrations.Knative.Version = &opts.Install.Knative.InstallKnativeVersion

			}
		})
	}

	return &GlooInstallSpec{
		HelmArchiveUri:    helmChartArchiveUri,
		ValueFileName:     valueFileName,
		UserValueFileName: opts.Install.HelmChartValues,
		ProductName:       "gloo",
		ExtraValues:       extraValues,
		ValueCallbacks:    valueCallbacks,
		ExcludeResources:  nil,
	}, nil
}

func getGlooVersion(opts *options.Options) (string, error) {
	if !version.IsReleaseVersion() && opts.Install.HelmChartOverride == "" {
		return "", errors.Errorf("you must provide a Gloo Helm chart URI via the 'file' option " +
			"when running an unreleased version of glooctl")
	}
	return version.Version, nil
}

func InstallGloo(opts *options.Options, spec GlooInstallSpec, client GlooKubeInstallClient) error {
	installer, err := NewGlooStagedInstaller(opts, spec, client)
	if err != nil {
		return err
	}

	if err := installer.DoCrdInstall(); err != nil {
		return err
	}

	if err := installer.DoPreInstall(); err != nil {
		return err
	}

	return installer.DoInstall()
}
