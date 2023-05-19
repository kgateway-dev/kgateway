package install

import (
	"io"
	"os"

	"github.com/solo-io/gloo/pkg/cliutil"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
)

const (
	tempChartFilePermissions = 0644
	helmNamespaceEnvVar      = "HELM_NAMESPACE"
	helmKubecontextEnvVar    = "HELM_KUBECONTEXT"
)

var verbose bool

func setVerbose(b bool) {
	verbose = b
}

//go:generate mockgen -destination mocks/mock_helm_client.go -package mocks github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install HelmClient

// This interface implements the Helm CLI actions. The implementation relies on the Helm 3 libraries.
type HelmClient interface {
	// Prepare an installation object that can then be .Run() with a chart object
	NewInstall(namespace, releaseName string, dryRun bool, context string) (HelmInstallation, *cli.EnvSettings, error)

	// Prepare an un-installation object that can then be .Run() with a release name
	NewUninstall(namespace string) (HelmUninstallation, error)

	// List the already-existing releases in the given namespace
	ReleaseList(namespace string) (HelmReleaseListRunner, error)

	// Returns the Helm chart archive located at the given URI (can be either an http(s) address or a file path)
	DownloadChart(chartArchiveUri string) (*chart.Chart, error)

	// Returns true if the release with the given name exists in the given namespace
	ReleaseExists(namespace, releaseName string) (releaseExists bool, err error)
}

// an interface around Helm's action.Install struct
//
//go:generate mockgen -destination mocks/mock_helm_installation.go -package mocks github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install HelmInstallation
type HelmInstallation interface {
	Run(chrt *chart.Chart, vals map[string]interface{}) (*release.Release, error)
}

// an interface around Helm's action.Uninstall struct
//
//go:generate mockgen -destination mocks/mock_helm_uninstallation.go -package mocks github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install HelmUninstallation
type HelmUninstallation interface {
	Run(name string) (*release.UninstallReleaseResponse, error)
}

var _ HelmInstallation = &action.Install{}
var _ HelmUninstallation = &action.Uninstall{}

// an interface around Helm's action.List struct
//
//go:generate mockgen -destination mocks/mock_helm_release_list.go -package mocks github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install HelmReleaseListRunner
type HelmReleaseListRunner interface {
	Run() ([]*release.Release, error)
	SetFilter(filter string)
}

// a HelmClient that talks to the kube api server and creates resources
func DefaultHelmClient() HelmClient {
	return &defaultHelmClient{}
}

type defaultHelmClient struct {
}

func (d *defaultHelmClient) NewInstall(namespace, releaseName string, dryRun bool, context string) (HelmInstallation, *cli.EnvSettings, error) {
	actionConfig, settings, err := newActionConfig(namespace, context)
	if err != nil {
		return nil, nil, err
	}
	settings.Debug = verbose

	client := action.NewInstall(actionConfig)
	client.ReleaseName = releaseName
	client.Namespace = namespace
	client.DryRun = dryRun

	// If this is a dry run, we don't want to query the API server.
	// In the future we can make this configurable to emulate the `helm template --validate` behavior.
	client.ClientOnly = dryRun

	return client, settings, nil
}

func (d *defaultHelmClient) NewUninstall(namespace string) (HelmUninstallation, error) {
	actionConfig, _, err := newActionConfig(namespace, "")
	if err != nil {
		return nil, err
	}
	return action.NewUninstall(actionConfig), nil
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
	actionConfig, _, err := newActionConfig(namespace, "")
	if err != nil {
		return nil, err
	}
	return &helmReleaseListRunner{
		list: action.NewList(actionConfig),
	}, nil
}

func (d *defaultHelmClient) DownloadChart(chartArchiveUri string) (*chart.Chart, error) {

	// 1. Get a reader to the chart file (remote URL or local file path)
	chartFileReader, err := cliutil.GetResource(chartArchiveUri)
	if err != nil {
		return nil, err
	}
	defer func() { _ = chartFileReader.Close() }()

	// 2. Write chart to a temporary file
	chartBytes, err := io.ReadAll(chartFileReader)
	if err != nil {
		return nil, err
	}

	chartFile, err := os.CreateTemp("", "gloo-helm-chart")
	if err != nil {
		return nil, err
	}
	charFilePath := chartFile.Name()
	defer func() { _ = os.RemoveAll(charFilePath) }()

	if err := os.WriteFile(charFilePath, chartBytes, tempChartFilePermissions); err != nil {
		return nil, err
	}

	// 3. Load the chart file
	chartObj, err := loader.Load(charFilePath)
	if err != nil {
		return nil, err
	}

	return chartObj, nil
}

func (d *defaultHelmClient) ReleaseExists(namespace, releaseName string) (releaseExists bool, err error) {
	list, err := d.ReleaseList(namespace)
	if err != nil {
		return false, err
	}
	list.SetFilter(releaseName)

	releases, err := list.Run()
	if err != nil {
		return false, err
	}

	for _, r := range releases {
		releaseExists = releaseExists || r.Name == releaseName
	}

	return releaseExists, nil
}

// Build a Helm EnvSettings struct
// basically, abstracted cli.New() into our own function call because of the weirdness described in the big comment below
func NewCLISettings(namespace, context string) *cli.EnvSettings {
	// The installation namespace is expressed as a "config override" in the Helm internals
	// It's normally set by the --namespace flag when invoking the Helm binary, which ends up
	// setting a non-exported field in the Helm settings struct (https://github.com/helm/helm/blob/v3.0.1/pkg/cli/environment.go#L77)
	// However, we are not invoking the Helm binary, so that field doesn't get set. It is left as "", which means
	// that any resources that are non-namespaced (at the time of writing, some of Prometheus's resources do not
	// have a namespace attached to them but they probably should) wind up in the default namespace from YOUR
	// kube config. To get around this, we temporarily set an env var before the Helm settings are initialized
	// so that the proper namespace override is piped through. (https://github.com/helm/helm/blob/v3.0.1/pkg/cli/environment.go#L64)
	if os.Getenv(helmNamespaceEnvVar) == "" {
		os.Setenv(helmNamespaceEnvVar, namespace)
		defer os.Setenv(helmNamespaceEnvVar, "")
	}
	if os.Getenv(helmKubecontextEnvVar) == "" && context != "" {
		os.Setenv(helmKubecontextEnvVar, context)
		defer os.Setenv(helmKubecontextEnvVar, "")
	}

	return cli.New()
}

func noOpDebugLog(_ string, _ ...interface{}) {}

// Returns an action configuration that can be used to create Helm actions and the Helm env settings.
// We currently get the Helm storage driver from the standard HELM_DRIVER env (defaults to 'secret').
func newActionConfig(namespace, context string) (*action.Configuration, *cli.EnvSettings, error) {
	settings := NewCLISettings(namespace, context)
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), noOpDebugLog); err != nil {
		return nil, nil, err
	}
	return actionConfig, settings, nil
}
