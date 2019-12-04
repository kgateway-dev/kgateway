package install

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/pkg/cliutil/helm"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/go-utils/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/yaml"
)

type Installer interface {
	Install(installOpts *options.Install, extraValues map[string]interface{}, enterprise bool) error
}

//go:generate mockgen -destination mocks/mock_helm_client.go -package mocks github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install HelmClient
type HelmClient interface {
	// prepare an installation object that can then be .Run() with a chart object
	NewInstall(namespace, releaseName string, dryRun bool) (HelmInstallation, *cli.EnvSettings, error)

	// list the already-existing releases in the given namespace
	ReleaseList(namespace string) (HelmReleaseListRunner, error)

	DownloadChart(chartArchiveUri string) (*chart.Chart, error)
}

// an interface around Helm's action.Install struct
//go:generate mockgen -destination mocks/mock_helm_installation.go -package mocks github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install HelmInstallation
type HelmInstallation interface {
	Run(chrt *chart.Chart, vals map[string]interface{}) (*release.Release, error)
}

var _ HelmInstallation = &action.Install{}

// an interface around Helm's action.List struct
//go:generate mockgen -destination mocks/mock_helm_release_list.go -package mocks github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install HelmReleaseListRunner
type HelmReleaseListRunner interface {
	Run() ([]*release.Release, error)
	SetFilter(filter string)
}

// a HelmClient that talks to the kube api server and creates resources
func DefaultHelmClient() HelmClient {
	return &defaultHelmClient{}
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

func (i *installer) Install(installOpts *options.Install, extraValues map[string]interface{}, enterprise bool) error {
	namespace := installOpts.Namespace
	if !installOpts.DryRun {
		if releaseExists, err := ReleaseExists(i.helmClient, namespace); err != nil {
			return err
		} else if releaseExists {
			return GlooAlreadyInstalled(namespace)
		}
	}

	preInstallMessage(installOpts, enterprise)

	helmInstall, helmEnv, err := i.helmClient.NewInstall(namespace, constants.GlooReleaseName, installOpts.DryRun)
	if err != nil {
		return err
	}

	chartUri, err := getChartUri(installOpts.HelmChartOverride, installOpts.WithUi, enterprise)
	if err != nil {
		return err
	}
	if verbose {
		fmt.Printf("Looking for chart at %s\n", chartUri)
	}

	chartObj, err := i.helmClient.DownloadChart(chartUri)
	if err != nil {
		return err
	}

	// Merge values provided via the '--values' flag
	valueOpts := &values.Options{
		ValueFiles: installOpts.HelmChartValueFileNames,
	}
	cliValues, err := valueOpts.MergeValues(getter.All(helmEnv))
	if err != nil {
		return err
	}

	// Merge the CLI flag values into the extra values, giving the latter higher precedence.
	// (The first argument to CoalesceTables has higher priority)
	completeValues := chartutil.CoalesceTables(extraValues, cliValues)
	if verbose {
		fmt.Printf("Merged CLI values into default values: %v\n", completeValues)
	}

	rel, err := helmInstall.Run(chartObj, completeValues)
	if err != nil {
		// TODO: verify whether we actually log something there after these changes
		_, _ = fmt.Fprintf(os.Stderr, "\nGloo failed to install! Detailed logs available at %s.\n", cliutil.GetLogsPath())
		return err
	}
	if verbose {
		fmt.Printf("Successfully ran helm install with release %s\n", constants.GlooReleaseName)
	}

	if installOpts.DryRun {
		if err := i.printReleaseManifest(rel); err != nil {
			return err
		}
	}

	postInstallMessage(installOpts, enterprise)

	return nil
}

func ReleaseExists(helmClient HelmClient, namespace string) (bool, error) {
	list, err := helmClient.ReleaseList(namespace)
	if err != nil {
		return false, err
	}
	list.SetFilter(constants.GlooReleaseName)

	releases, err := list.Run()
	if err != nil {
		return false, err
	}

	return len(releases) > 0, nil
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

type defaultHelmClient struct {
}

func (d *defaultHelmClient) NewInstall(namespace, releaseName string, dryRun bool) (HelmInstallation, *cli.EnvSettings, error) {
	return helm.NewInstall(namespace, releaseName, dryRun)
}

type helmReleaseListRunner struct {
	list *action.List
}

func (h *helmReleaseListRunner) Run() ([]*release.Release, error) {
	return h.list.Run()
}

func (h *helmReleaseListRunner) SetFilter(filter string) {
	h.list.Filter = filter
}

func (d *defaultHelmClient) ReleaseList(namespace string) (HelmReleaseListRunner, error) {
	list, err := helm.NewList(namespace)
	if err != nil {
		return nil, err
	}
	return &helmReleaseListRunner{list: list}, nil
}

func (d *defaultHelmClient) DownloadChart(chartArchiveUri string) (*chart.Chart, error) {
	return helm.DownloadChart(chartArchiveUri)
}
