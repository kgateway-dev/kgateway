package runner

import (
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/runner"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	discoveryRegistry "github.com/solo-io/gloo/projects/discovery/pkg/fds/discoveries/registry"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
)

type StartExtensions struct {
	DiscoveryFactoryFuncs []func() fds.FunctionDiscoveryFactory
}

func NewSetupFunc() bootstrap.SetupFunc {
	return runner.NewSetupFuncWithRunAndExtensions(StartFDS, nil)
}

// NewSetupFuncWithExtensions used as extension point for external repo
func NewSetupFuncWithExtensions(extensions StartExtensions) bootstrap.SetupFunc {
	runWithExtensions := func(opts runner.StartOpts) error {
		return StartFDSWithExtensions(opts, extensions)
	}
	return runner.NewSetupFuncWithRunAndExtensions(runWithExtensions, nil)
}

func GetFunctionDiscoveriesWithExtensions(opts runner.StartOpts, extensions StartExtensions) []fds.FunctionDiscoveryFactory {
	return GetFunctionDiscoveriesWithExtensionsAndRegistry(opts, discoveryRegistry.Plugins, extensions)
}

func GetFunctionDiscoveriesWithExtensionsAndRegistry(opts runner.StartOpts, registryDiscFacts func(opts runner.StartOpts) []fds.FunctionDiscoveryFactory, extensions StartExtensions) []fds.FunctionDiscoveryFactory {
	pluginfuncs := extensions.DiscoveryFactoryFuncs
	discFactories := registryDiscFacts(opts)
	for _, discoveryFactoryExtension := range pluginfuncs {
		pe := discoveryFactoryExtension()
		discFactories = append(discFactories, pe)
	}
	return discFactories
}

// FakeKubeNamespaceWatcher to eliminate the need for this fake client for non kube environments
// TODO: consider using regular solo-kit namespace client instead of KubeNamespace client
type FakeKubeNamespaceWatcher struct{}

func (f *FakeKubeNamespaceWatcher) Watch(opts clients.WatchOpts) (<-chan skkube.KubeNamespaceList, <-chan error, error) {
	return nil, nil, nil
}
func (f *FakeKubeNamespaceWatcher) BaseClient() clients.ResourceClient {
	return nil

}
func (f *FakeKubeNamespaceWatcher) Register() error {
	return nil
}
func (f *FakeKubeNamespaceWatcher) Read(name string, opts clients.ReadOpts) (*skkube.KubeNamespace, error) {
	return nil, nil
}
func (f *FakeKubeNamespaceWatcher) Write(resource *skkube.KubeNamespace, opts clients.WriteOpts) (*skkube.KubeNamespace, error) {
	return nil, nil
}
func (f *FakeKubeNamespaceWatcher) Delete(name string, opts clients.DeleteOpts) error {
	return nil
}
func (f *FakeKubeNamespaceWatcher) List(opts clients.ListOpts) (skkube.KubeNamespaceList, error) {
	return nil, nil
}
