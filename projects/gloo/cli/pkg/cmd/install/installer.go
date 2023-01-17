package install

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/pkg/cliutil/helm"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/yaml"
)

var (
	// ChartAndReleaseFlagErr is thrown when both options are set.
	ChartAndReleaseFlagErr = func(chartOverride, versionOverride string) error {
		return eris.Errorf("you may not specify both a chart with -f and a release version with --version. Received: %s and %s", chartOverride, versionOverride)
	}
	// UnreleasedWithoutOverrideErr details that the user is running an unreleased or invalid verison of gloo.
	UnreleasedWithoutOverrideErr = eris.Errorf("you must provide a Gloo Helm chart URI via the 'file' option " +
		"when running an unreleased version of glooctl")
)

type Installer interface {
	Install(installerConfig *InstallerConfig) error
}

type InstallerConfig struct {
	InstallCliArgs *options.Install
	ExtraValues    map[string]interface{}
	Mode           Mode
	Verbose        bool
	Ctx            context.Context
}

type Mode int

const (
	Gloo Mode = iota
	GlooWithUI
	Enterprise
	Federation
)

// NewInstaller consumes a helm client and sets up an installer for usage in glooctl.
func NewInstaller(opts *options.Options, helmClient HelmClient) Installer {
	client := helpers.MustKubeClientWithKubecontext(opts.Top.KubeContext)
	return NewInstallerWithWriter(helmClient, client.CoreV1().Namespaces(), os.Stdout)
}

// NewInstallerWithWriter creates a new Installer for a given helm client,
// namespace and output writer. The extra variables are exposed to allow for ease of testing.
func NewInstallerWithWriter(helmClient HelmClient, kubeNsClient v1.NamespaceInterface, outputWriter io.Writer) Installer {
	return &installer{
		helmClient:         helmClient,
		kubeNsClient:       kubeNsClient,
		dryRunOutputWriter: outputWriter,
	}
}

// Install gloo given the provided config.
func (i *installer) Install(installerConfig *InstallerConfig) error {
	return i.installFromConfig(installerConfig)
}

func (i *installer) installFromConfig(installerConfig *InstallerConfig) error {
	helmInstallConfig := installerConfig.InstallCliArgs.Gloo
	namespace := helmInstallConfig.Namespace
	releaseName := helmInstallConfig.HelmReleaseName
	if !installerConfig.InstallCliArgs.DryRun {
		if releaseExists, err := i.helmClient.ReleaseExists(namespace, releaseName); err != nil {
			return err
		} else if releaseExists {
			return GlooAlreadyInstalled(namespace)
		}
		if helmInstallConfig.CreateNamespace {
			// Create the namespace if it doesn't exist. Helm3 no longer does this.
			i.createNamespace(installerConfig.Ctx, namespace)
		}
	}

	preInstallMessage(installerConfig.InstallCliArgs, installerConfig.Mode)

	helmInstall, helmEnv, err := i.helmClient.NewInstall(namespace, releaseName, installerConfig.InstallCliArgs.DryRun, "")
	if err != nil {
		return err
	}

	chartUri, err := getChartUri(helmInstallConfig.HelmChartOverride,
		strings.TrimPrefix(installerConfig.InstallCliArgs.Version, "v"),
		installerConfig.Mode)
	if err != nil {
		return err
	}
	if installerConfig.Verbose {
		fmt.Printf("Looking for chart at %s\n", chartUri)
	}

	chartObj, err := i.helmClient.DownloadChart(chartUri)
	if err != nil {
		return err
	}

	// determine if it's an enterprise chart by checking if has gloo as a dependency
	// if so, overwrite the installation mode to Enterprise
	installerConfig.Mode = Gloo
	for _, dependency := range chartObj.Dependencies() {
		if dependency.Metadata.Name == constants.GlooReleaseName {
			installerConfig.Mode = Enterprise
			break
		}
	}

	err = setExtraValues(installerConfig)
	if err != nil {
		return err
	}

	// Merge values provided via the '--values' flag
	valueOpts := &values.Options{
		ValueFiles: helmInstallConfig.HelmChartValueFileNames,
	}
	cliValues, err := valueOpts.MergeValues(getter.All(helmEnv))
	if err != nil {
		return err
	}

	// Merge the CLI flag values into the extra values, giving the latter higher precedence.
	// (The first argument to CoalesceTables has higher priority)
	completeValues := chartutil.CoalesceTables(installerConfig.ExtraValues, cliValues)
	if installerConfig.Verbose {
		b, err := json.Marshal(completeValues)
		if err != nil {
			fmt.Printf("error: %v\n", err)
		}
		y, err := yaml.JSONToYAML(b)
		if err != nil {
			fmt.Printf("error: %v\n", err)
		}
		fmt.Printf("Installing the %s chart with the following value overrides:\n%s\n", chartObj.Metadata.Name, string(y))
	}

	rel, err := helmInstall.Run(chartObj, completeValues)
	if err != nil {
		// TODO: verify whether we actually log something there after these changes
		_, _ = fmt.Fprintf(os.Stderr, "\nGloo failed to install! Detailed logs available at %s.\n", cliutil.GetLogsPath())
		return err
	}
	if installerConfig.Verbose {
		fmt.Printf("Successfully ran helm install with release %s\n", releaseName)
	}

	if installerConfig.InstallCliArgs.DryRun {
		if err := i.printReleaseManifest(rel); err != nil {
			return err
		}
	}

	postInstallMessage(installerConfig.InstallCliArgs, installerConfig.Mode)

	return nil
}

func (i *installer) createNamespace(ctx context.Context, namespace string) {
	_, err := i.kubeNsClient.Get(ctx, namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		fmt.Printf("Creating namespace %s... ", namespace)
		if _, err := i.kubeNsClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}, metav1.CreateOptions{
			TypeMeta:     metav1.TypeMeta{},
			DryRun:       nil,
			FieldManager: "",
		}); err != nil {
			fmt.Printf("\nUnable to create namespace %s. Continuing...\n", namespace)
		} else {
			fmt.Printf("Done.\n")
		}
	} else if apierrors.IsAlreadyExists(err) {
		fmt.Printf("\nNamespace %s already exists. Continuing...\n", namespace)
	} else {
		fmt.Printf("\nUnable to check if namespace %s exists. Continuing...\n", namespace)
	}

}

// if enterprise, nest any gloo helm values under "gloo" heading
func setExtraValues(config *InstallerConfig) error {
	if config.ExtraValues == nil || config.Mode != Enterprise {
		return nil
	}

	newExtraValues := map[string]interface{}{}

	var glooHelmConfigEmpty generate.HelmConfig
	for k, v := range config.ExtraValues {

		var glooHelmConfigValue generate.HelmConfig

		// use json as a middleman between map and struct
		valueBytes, err := json.Marshal(map[string]interface{}{k: v})
		if err != nil {
			return err
		}
		err = json.Unmarshal(valueBytes, &glooHelmConfigValue)
		if err != nil {
			return err
		}

		// if the chart with the value isn't the same as the empty one, value is gloo value that needs to be nested
		if !reflect.DeepEqual(glooHelmConfigValue, glooHelmConfigEmpty) {
			if _, ok := newExtraValues[constants.GlooReleaseName]; !ok {
				newExtraValues[constants.GlooReleaseName] = map[string]interface{}{}
			}
			newExtraValues[constants.GlooReleaseName].(map[string]interface{})[k] = v
		} else {
			newExtraValues[k] = v
		}
	}

	config.ExtraValues = newExtraValues
	return nil
}

func (i *installer) printReleaseManifest(release *release.Release) error {
	// Print CRDs
	for _, crdFile := range release.Chart.CRDs() {
		_, _ = fmt.Fprintln(i.dryRunOutputWriter, string(crdFile.Data))
		_, _ = fmt.Fprintln(i.dryRunOutputWriter, "---")
	}

	// Print hook resources
	nonCleanupHooks, err := helm.GetHooks(release.Hooks)
	if err != nil {
		return err
	}
	for _, hook := range nonCleanupHooks {
		_, _ = fmt.Fprintln(i.dryRunOutputWriter, hook.Manifest)
		_, _ = fmt.Fprintln(i.dryRunOutputWriter, "---")
	}

	// Print the actual release resources
	_, _ = fmt.Fprintf(i.dryRunOutputWriter, "%s", release.Manifest)

	// For safety, print a YAML separator so multiple invocations of this function will produce valid output
	_, _ = fmt.Fprintln(i.dryRunOutputWriter, "---")
	return nil
}

// The resulting URI can be either a URL or a local file path.
func getChartUri(chartOverride, versionOverride string, mode Mode) (string, error) {

	if chartOverride != "" && versionOverride != "" {
		return "", ChartAndReleaseFlagErr(chartOverride, versionOverride)
	}

	var helmChartRepoTemplate, helmChartVersion string
	switch mode {
	case Enterprise:
		helmChartRepoTemplate = GlooEHelmRepoTemplate
	case Gloo:
		helmChartRepoTemplate = constants.GlooHelmRepoTemplate
	default:
		helmChartRepoTemplate = constants.GlooHelmRepoTemplate
	}

	if versionOverride != "" {
		helmChartVersion = versionOverride
	} else {
		switch mode {
		case Enterprise:
			enterpriseVersion, err := version.GetLatestEnterpriseVersion(true)
			if err != nil {
				return "", eris.Wrap(UnreleasedWithoutOverrideErr, err.Error())
			}
			helmChartVersion = enterpriseVersion
		case Gloo:
			glooOsVersion, err := getDefaultGlooInstallVersion(chartOverride)
			if err != nil {
				return "", err
			}
			helmChartVersion = glooOsVersion
		}
	}

	helmChartArchiveUri := fmt.Sprintf(helmChartRepoTemplate, helmChartVersion)

	if chartOverride != "" {
		helmChartArchiveUri = chartOverride
	}

	if path.Ext(helmChartArchiveUri) != ".tgz" && !strings.HasSuffix(helmChartArchiveUri, ".tar.gz") {
		return "", eris.Errorf("unsupported file extension for Helm chart URI: [%s]. Extension must either be .tgz or .tar.gz", helmChartArchiveUri)
	}
	return helmChartArchiveUri, nil
}

func getDefaultGlooInstallVersion(chartOverride string) (string, error) {
	if !version.IsReleaseVersion() && chartOverride == "" {
		return "", UnreleasedWithoutOverrideErr
	}
	return version.Version, nil
}

func preInstallMessage(installOpts *options.Install, mode Mode) {
	if installOpts.DryRun {
		return
	}
	switch mode {
	case Enterprise:
		fmt.Println("Starting Gloo Edge Enterprise installation...")
	default:
		fmt.Println("Starting Gloo Edge installation...")
	}
}
func postInstallMessage(installOpts *options.Install, mode Mode) {
	if installOpts.DryRun {
		return
	}
	switch mode {
	case Enterprise:
		fmt.Println("\nGloo Edge Enterprise was successfully installed!")
		if installOpts.WithGlooFed {
			fmt.Println("\nGloo Edge Federation was successfully installed!")
			fmt.Println("\nYou can now register your cluster with:")
			fmt.Println("\nFor GKE clusters:")
			fmt.Println(" glooctl cluster register --cluster-name [ex. gloo-fed-remote] --remote-context [gke-context-name] --federation-namespace [default: gloo-fed]")
			fmt.Println("\nFor kind clusters:")
			fmt.Println(" glooctl cluster register --cluster-name [ex. kind-local] --remote-context [ex. kind-local] --local-cluster-domain-override [ex. host.docker.internal] --federation-namespace [default: gloo-fed]")
			fmt.Println("\nSee the cluster registration guide for more information: https://docs.solo.io/gloo-edge/latest/guides/gloo_federation/cluster_registration/")
		}
	default:
		fmt.Println("\nGloo Edge was successfully installed!")
	}
}

type installer struct {
	helmClient         HelmClient
	kubeNsClient       v1.NamespaceInterface
	dryRunOutputWriter io.Writer
}
