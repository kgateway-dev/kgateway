package bootstrap

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

type multiSecretSourceResourceClientFactory struct {
	settings *v1.Settings_SecretOptions
}

func (m *multiSecretSourceResourceClientFactory) NewResourceClient(ctx context.Context, params factory.NewResourceClientParams) (clients.ResourceClient, error) {

	multiClient := &multiSecretSourceResourceClient{}
	var err error
	for k, v := range m.settings.GetSecretSourceMap() {
		switch source := v.GetSource().(type) {
		case *v1.Settings_SecretOptions_Source_Directory:
			{
				multiClient.directoryClientSettings = &directorySecretClientSettings{
					rootDir: source.Directory.GetDirectory(),
				}
				f := &factory.FileResourceClientFactory{RootDir: source.Directory.GetDirectory()}
				multiClient.clientMap[k], err = f.NewResourceClient(ctx, params)
				if err != nil {
					return nil, err
				}

			}
		}

	}
	return multiClient, nil
}

type (
	kubeSecretClientSettings struct {
	}
	directorySecretClientSettings struct {
		rootDir string
	}
)
type multiSecretSourceResourceClient struct {
	vaultClientSettings     *vaultSecretClientSettings
	kubeClientSettings      *kubeSecretClientSettings
	directoryClientSettings *directorySecretClientSettings

	resourceType resources.VersionedResource
	clientMap    map[string]clients.ResourceClient // do not use clients.ResourceClients here as that is not for this purpose
}

func (m *multiSecretSourceResourceClient) Kind() string {
	panic("not implemented") // TODO: Implement
}

func (m *multiSecretSourceResourceClient) NewResource() resources.Resource {
	panic("not implemented") // TODO: Implement
}

// Deprecated: implemented only by the kubernetes resource client. Will be removed from the interface.
func (m *multiSecretSourceResourceClient) Register() error {
	panic("not implemented") // TODO: Implement
}

func (m *multiSecretSourceResourceClient) Read(namespace string, name string, opts clients.ReadOpts) (resources.Resource, error) {
	panic("not implemented") // TODO: Implement
}

func (m *multiSecretSourceResourceClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	panic("not implemented") // TODO: Implement
}

func (m *multiSecretSourceResourceClient) Delete(namespace string, name string, opts clients.DeleteOpts) error {
	panic("not implemented") // TODO: Implement
}

func (m *multiSecretSourceResourceClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	panic("not implemented") // TODO: Implement
}

func (m *multiSecretSourceResourceClient) ApplyStatus(statusClient resources.StatusClient, inputResource resources.InputResource, opts clients.ApplyStatusOpts) (resources.Resource, error) {
	panic("not implemented") // TODO: Implement
}

func (m *multiSecretSourceResourceClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	panic("not implemented") // TODO: Implement
}
