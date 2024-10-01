package namespaces

import (
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

// NoOpKubeNamespaceWatcher to eliminate the need for this fake client for non kube environments
// Currently the generated code (event loop, snapshots) expects the KubeNamespaceClient to not be nil
// and the KubeNamespaceClient does not have checks to ensure it is handled properly if nil
// so this is the workaround for now until we rewrite the generated code
type NoOpKubeNamespaceWatcher struct{}

func (f *NoOpKubeNamespaceWatcher) Watch(opts clients.WatchOpts) (<-chan skkube.KubeNamespaceList, <-chan error, error) {
	return nil, nil, nil
}

func (f *NoOpKubeNamespaceWatcher) BaseClient() clients.ResourceClient {
	return nil
}

func (f *NoOpKubeNamespaceWatcher) Register() error {
	return nil
}

func (f *NoOpKubeNamespaceWatcher) Read(name string, opts clients.ReadOpts) (*skkube.KubeNamespace, error) {
	return nil, nil
}

func (f *NoOpKubeNamespaceWatcher) Write(resource *skkube.KubeNamespace, opts clients.WriteOpts) (*skkube.KubeNamespace, error) {
	return nil, nil
}

func (f *NoOpKubeNamespaceWatcher) Delete(name string, opts clients.DeleteOpts) error {
	return nil
}

func (f *NoOpKubeNamespaceWatcher) List(opts clients.ListOpts) (skkube.KubeNamespaceList, error) {
	return nil, nil
}
