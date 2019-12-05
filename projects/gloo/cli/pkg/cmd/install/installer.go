package install

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/go-utils/errors"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	return NewInstallerWithWriter(helmClient, os.Stdout)
}

// visible for testing
func NewInstallerWithWriter(helmClient HelmClient, outputWriter io.Writer) Installer {
	return &installer{
		helmClient:         helmClient,
		dryRunOutputWriter: outputWriter,
	}
}

func (i *installer) Install(installerConfig *InstallerConfig) error {
	namespace := installerConfig.InstallCliArgs.Namespace
	if !installerConfig.InstallCliArgs.DryRun {
		if releaseExists, err := ReleaseExists(i.helmClient, namespace, installerConfig.InstallCliArgs.HelmReleaseName); err != nil {
			return err
		} else if releaseExists {
			return GlooAlreadyInstalled(namespace)
		}
	}

	preInstallMessage(installerConfig.InstallCliArgs, installerConfig.Enterprise)

	helmInstall, helmEnv, err := i.helmClient.NewInstall(namespace, installerConfig.InstallCliArgs.HelmReleaseName, installerConfig.InstallCliArgs.DryRun)
	if err != nil {
		return err
	}

	chartUri, err := getChartUri(installerConfig.InstallCliArgs.HelmChartOverride, installerConfig.InstallCliArgs.WithUi, installerConfig.Enterprise)
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
		fmt.Printf("Installing the %s chart with the following value overrides:\n%v\n", chartObj.Metadata.Name, y)
	}

	rel, err := helmInstall.Run(chartObj, completeValues)
	if err != nil {
		// TODO: verify whether we actually log something there after these changes
		_, _ = fmt.Fprintf(os.Stderr, "\nGloo failed to install! Detailed logs available at %s.\n", cliutil.GetLogsPath())
		return err
	}
	if installerConfig.Verbose {
		fmt.Printf("Successfully ran helm install with release %s\n", installerConfig.InstallCliArgs.HelmReleaseName)
	}

	if installerConfig.InstallCliArgs.DryRun {
		if err := i.printReleaseManifest(rel); err != nil {
			return err
		}
	}

	postInstallMessage(installerConfig.InstallCliArgs, installerConfig.Enterprise)

	return nil
}

func (i *installer) printReleaseManifest(release *release.Release) error {
	// Print CRDs
	for _, crdFile := range release.Chart.CRDs() {
		fmt.Fprintf(i.dryRunOutputWriter, "%s", string(crdFile.Data))
		fmt.Fprintln(i.dryRunOutputWriter, "---")
	}

	// Print hook resources
	nonCleanupHooks, err := GetNonCleanupHooks(release.Hooks)
	if err != nil {
		return err
	}
	for _, hook := range nonCleanupHooks {
		fmt.Fprintln(i.dryRunOutputWriter, hook.Manifest)
		fmt.Fprintln(i.dryRunOutputWriter, "---")
	}

	// Print the actual release resources
	fmt.Fprintf(i.dryRunOutputWriter, "%s", release.Manifest)

	// For safety, print a YAML separator so multiple invocations of this function will produce valid output
	fmt.Fprintln(i.dryRunOutputWriter, "---")
	return nil
}

// some resources are duplicated because of weirdness with Helm hooks.
// a job needs a service account/rbac resources, and we would like those to be cleaned up after the job is complete
// this isn't really expressible cleanly through Helm hooks.
func GetNonCleanupHooks(hooks []*release.Hook) (results []*release.Hook, err error) {
	for _, hook := range hooks {
		// Parse the resource in order to access the annotations
		var resource struct{ Metadata v1.ObjectMeta }
		if err := yaml.Unmarshal([]byte(hook.Manifest), &resource); err != nil {
			return nil, errors.Wrapf(err, "parsing resource: %s", hook.Manifest)
		}

		// Skip hook cleanup resources
		if annotations := resource.Metadata.Annotations; len(annotations) > 0 {
			if _, ok := annotations[constants.HookCleanupResourceAnnotation]; ok {
				continue
			}
		}

		results = append(results, hook)
	}

	return results, nil
}

// The resulting URI can be either a URL or a local file path.
func getChartUri(chartOverride string, withUi bool, enterprise bool) (string, error) {
	var helmChartArchiveUri string

	if enterprise {
		helmChartArchiveUri = fmt.Sprintf(GlooEHelmRepoTemplate, version.EnterpriseTag)
	} else if withUi {
		helmChartArchiveUri = fmt.Sprintf(constants.GlooWithUiHelmRepoTemplate, version.EnterpriseTag)
	} else {
		glooOsVersion, err := getGlooVersion(chartOverride)
		if err != nil {
			return "", err
		}
		helmChartArchiveUri = fmt.Sprintf(constants.GlooHelmRepoTemplate, glooOsVersion)
	}

	if chartOverride != "" {
		helmChartArchiveUri = chartOverride
	}

	if path.Ext(helmChartArchiveUri) != ".tgz" && !strings.HasSuffix(helmChartArchiveUri, ".tar.gz") {
		return "", errors.Errorf("unsupported file extension for Helm chart URI: [%s]. Extension must either be .tgz or .tar.gz", helmChartArchiveUri)
	}
	return helmChartArchiveUri, nil
}

func getGlooVersion(chartOverride string) (string, error) {
	if !version.IsReleaseVersion() && chartOverride == "" {
		return "", errors.Errorf("you must provide a Gloo Helm chart URI via the 'file' option " +
			"when running an unreleased version of glooctl")
	}
	return version.Version, nil
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
	dryRunOutputWriter io.Writer
}
