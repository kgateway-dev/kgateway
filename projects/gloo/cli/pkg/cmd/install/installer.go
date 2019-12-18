package install

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/gloo/pkg/cliutil/install"

	"github.com/solo-io/gloo/pkg/cliutil/helm"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/go-utils/errors"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/yaml"
)

type Installer interface {
	Install(installerConfig *InstallerConfig) error
}

type InstallerConfig struct {
	InstallCliArgs *options.Install
	ExtraValues    map[string]interface{}
	Enterprise     bool
	Verbose        bool
}

func NewInstaller(helmClient HelmClient) Installer {
	return NewInstallerWithWriter(helmClient, &install.CmdKubectl{}, os.Stdout)
}

// visible for testing
func NewInstallerWithWriter(helmClient HelmClient, kubeCli install.KubeCli, outputWriter io.Writer) Installer {
	return &installer{
		helmClient:         helmClient,
		kubeCli:            kubeCli,
		dryRunOutputWriter: outputWriter,
	}
}

func (i *installer) Install(installerConfig *InstallerConfig) error {
	namespace := installerConfig.InstallCliArgs.Namespace
	releaseName := installerConfig.InstallCliArgs.HelmReleaseName
	if !installerConfig.InstallCliArgs.DryRun {
		if releaseExists, err := i.helmClient.ReleaseExists(namespace, releaseName); err != nil {
			return err
		} else if releaseExists {
			return GlooAlreadyInstalled(namespace)
		}
		if installerConfig.InstallCliArgs.CreateNamespace {
			// Create the namespace if it doesn't exist. Helm3 no longer does this.
			i.createNamespace(namespace)
		}
	}

	preInstallMessage(installerConfig.InstallCliArgs, installerConfig.Enterprise)

	helmInstall, helmEnv, err := i.helmClient.NewInstall(namespace, releaseName, installerConfig.InstallCliArgs.DryRun)
	if err != nil {
		return err
	}

	chartUri, err := getChartUri(installerConfig.InstallCliArgs.HelmChartOverride, version.StripV(installerConfig.InstallCliArgs.ReleaseVersion),
		installerConfig.InstallCliArgs.WithUi, installerConfig.Enterprise)
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

	// Merge values provided via the '--values' flag
	valueOpts := &values.Options{
		ValueFiles: installerConfig.InstallCliArgs.HelmChartValueFileNames,
	}
	cliValues, err := valueOpts.MergeValues(getter.All(helmEnv))
	if err != nil {
		return err
	}

	// We need this to avoid rendering the CRDs we include in the /templates directory
	// for backwards-compatibility with Helm 2.
	setCrdCreateToFalse(installerConfig)

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

	postInstallMessage(installerConfig.InstallCliArgs, installerConfig.Enterprise)

	return nil
}

func (i *installer) createNamespace(namespace string) {
	fmt.Printf("Creating namespace %s... ", namespace)
	if err := i.kubeCli.Kubectl(nil, "create", "namespace", namespace); err != nil {
		fmt.Printf("\nUnable to create namespace %s. Continuing...\n", namespace)
	} else {
		fmt.Printf("Done.\n")
	}
}

func setCrdCreateToFalse(config *InstallerConfig) {
	if config.ExtraValues == nil {
		config.ExtraValues = map[string]interface{}{}
	}

	mapWithCrdValueToOverride := config.ExtraValues

	// If this is an enterprise install, `crds.create` is nested under the `gloo` field
	if config.Enterprise {
		if _, ok := config.ExtraValues["gloo"]; !ok {
			config.ExtraValues["gloo"] = map[string]interface{}{}
		}
		mapWithCrdValueToOverride = config.ExtraValues["gloo"].(map[string]interface{})
	}

	mapWithCrdValueToOverride["crds"] = map[string]interface{}{
		"create": false,
	}
}

func (i *installer) printReleaseManifest(release *release.Release) error {
	// Print CRDs
	for _, crdFile := range release.Chart.CRDs() {
		_, _ = fmt.Fprintln(i.dryRunOutputWriter, string(crdFile.Data))
		_, _ = fmt.Fprintln(i.dryRunOutputWriter, "---")
	}

	// Print hook resources
	nonCleanupHooks, err := helm.GetNonCleanupHooks(release.Hooks)
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
func getChartUri(chartOverride, versionOverride string, withUi, enterprise bool) (string, error) {

	if chartOverride != "" && versionOverride != "" {
		return "", errors.Errorf("you may not specify both a chart with -f and a release version with --version. Received: %s and %s", chartOverride, versionOverride)
	}

	var helmChartArchiveUri string
	if chartOverride != "" {
		helmChartArchiveUri = chartOverride
	} else {
		enterpriseVersion, err := version.GetLatestEnterpriseVersion(true)
		if err != nil {
			return "", err
		}
		if enterprise {
			helmChartArchiveUri = chartUriWithVersion(GlooEHelmRepoTemplate, versionOverride, enterpriseVersion)
		} else if withUi {
			helmChartArchiveUri = chartUriWithVersion(constants.GlooWithUiHelmRepoTemplate, versionOverride, enterpriseVersion)
		} else {
			if versionOverride != "" {
				helmChartArchiveUri = fmt.Sprintf(constants.GlooHelmRepoTemplate, versionOverride)
			} else {
				glooOsVersion, err := getGlooVersionToInstall(chartOverride)
				if err != nil {
					return "", err
				}
				helmChartArchiveUri = fmt.Sprintf(constants.GlooHelmRepoTemplate, glooOsVersion)
			}
		}
	}

	if path.Ext(helmChartArchiveUri) != ".tgz" && !strings.HasSuffix(helmChartArchiveUri, ".tar.gz") {
		return "", errors.Errorf("unsupported file extension for Helm chart URI: [%s]. Extension must either be .tgz or .tar.gz", helmChartArchiveUri)
	}
	return helmChartArchiveUri, nil
}

func getGlooVersionToInstall(chartOverride string) (string, error) {
	if !version.IsReleaseVersion() && chartOverride == "" {
		return "", errors.Errorf("you must provide a Gloo Helm chart URI via the 'file' option " +
			"when running an unreleased version of glooctl")
	}
	return version.Version, nil
}

func chartUriWithVersion(template, releaseVersionOverride, latestVersion string) string {
	if releaseVersionOverride != "" {
		return fmt.Sprintf(template, releaseVersionOverride)
	} else {
		return fmt.Sprintf(template, latestVersion)
	}
}

func preInstallMessage(installOpts *options.Install, enterprise bool) {
	if installOpts.DryRun {
		return
	}
	if enterprise {
		fmt.Println("Starting Gloo Enterprise installation...")
	} else {
		fmt.Println("Starting Gloo installation...")
	}
}
func postInstallMessage(installOpts *options.Install, enterprise bool) {
	if installOpts.DryRun {
		return
	}
	if enterprise {
		fmt.Println("Gloo Enterprise was successfully installed!")
	} else {
		fmt.Println("Gloo was successfully installed!")
	}

}

type installer struct {
	helmClient         HelmClient
	kubeCli            install.KubeCli
	dryRunOutputWriter io.Writer
}
