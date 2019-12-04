package install

import (
	"github.com/solo-io/gloo/pkg/cliutil/helm"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
)

//go:generate mockgen -destination mocks/mock_helm_client.go -package mocks github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install HelmClient
type HelmClient interface {
	// prepare an installation object that can then be .Run() with a chart object
	NewInstall(namespace, releaseName string, dryRun bool) (HelmInstallation, *cli.EnvSettings, error)
	NewUninstall(namespace string) (HelmUninstallation, error)

	// list the already-existing releases in the given namespace
	ReleaseList(namespace string) (HelmReleaseListRunner, error)
	DownloadChart(chartArchiveUri string) (*chart.Chart, error)
}

// an interface around Helm's action.Install struct
//go:generate mockgen -destination mocks/mock_helm_installation.go -package mocks github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install HelmInstallation
type HelmInstallation interface {
	Run(chrt *chart.Chart, vals map[string]interface{}) (*release.Release, error)
}

// an interface around Helm's action.Uninstall struct
//go:generate mockgen -destination mocks/mock_helm_uninstallation.go -package mocks github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install HelmUninstallation
type HelmUninstallation interface {
	Run(name string) (*release.UninstallReleaseResponse, error)
}

var _ HelmInstallation = &action.Install{}
var _ HelmUninstallation = &action.Uninstall{}

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

type defaultHelmClient struct {
}

func (d *defaultHelmClient) NewInstall(namespace, releaseName string, dryRun bool) (HelmInstallation, *cli.EnvSettings, error) {
	return helm.NewInstall(namespace, releaseName, dryRun)
}

func (d *defaultHelmClient) NewUninstall(namespace string) (HelmUninstallation, error) {
	return helm.NewUninstall(namespace)
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

func ReleaseExists(helmClient HelmClient, namespace, releaseName string) (releaseExists bool, err error) {
	list, err := helmClient.ReleaseList(namespace)
	if err != nil {
		return false, err
	}
	list.SetFilter(constants.GlooReleaseName)

	releases, err := list.Run()
	if err != nil {
		return false, err
	}

	for _, r := range releases {
		releaseExists = releaseExists || r.Name == releaseName
	}

	return releaseExists, nil
}
