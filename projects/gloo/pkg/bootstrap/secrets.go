package bootstrap

import (
	"context"

	vaultapi "github.com/hashicorp/vault/api"
	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/errors"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type multiSecretSourceResourceClientFactory struct {
	defaultSource   *v1.Settings_SecretOptions_DefaultSource
	secretSourceMap map[string]*v1.Settings_SecretOptions_Source
	sharedCache     memory.InMemoryResourceCache
	cfg             **rest.Config
	clientset       *kubernetes.Interface
	kubeCoreCache   *cache.KubeCoreCache
	vaultClient     *vaultapi.Client

	refreshRate     *durationpb.Duration
	watchNamespaces []string
}

func newMultiSecretSourceResourceClientFactory(defaultSource *v1.Settings_SecretOptions_DefaultSource,
	secretSourceMap map[string]*v1.Settings_SecretOptions_Source,
	sharedCache memory.InMemoryResourceCache,
	cfg **rest.Config,
	clientset *kubernetes.Interface,
	kubeCoreCache *cache.KubeCoreCache,
	vaultClient *vaultapi.Client) (*multiSecretSourceResourceClientFactory, error) {

	// Default to Kubernetes secret source
	if defaultSource == nil {
		defaultSource = new(v1.Settings_SecretOptions_DefaultSource)
	}
	// Guard against nil source map
	if secretSourceMap == nil {
		secretSourceMap = make(map[string]*v1.Settings_SecretOptions_Source)
	}

	return &multiSecretSourceResourceClientFactory{
		defaultSource:   defaultSource,
		secretSourceMap: secretSourceMap,
		sharedCache:     sharedCache,
		cfg:             cfg,
		clientset:       clientset,
		kubeCoreCache:   kubeCoreCache,
		vaultClient:     vaultClient,
	}, nil
}

func (m *multiSecretSourceResourceClientFactory) NewResourceClient(ctx context.Context, params factory.NewResourceClientParams) (clients.ResourceClient, error) {

	multiClient := &multiSecretSourceResourceClient{
		defaultSource: m.defaultSource,
	}
	var err error
	var f factory.ResourceClientFactory
	for k, v := range m.secretSourceMap {
		if _, ok := v1.Settings_SecretOptions_DefaultSource_value[k]; !ok {
			return nil, errors.Errorf("unrecognized secret source %s", k)
		}
		switch source := v.GetSource().(type) {
		case *v1.Settings_SecretOptions_Source_Directory:
			{
				f = &factory.FileResourceClientFactory{
					RootDir: source.Directory.GetDirectory(),
				}
			}
		case *v1.Settings_SecretOptions_Source_Kubernetes:
			{
				if err = initializeForKube(ctx, m.cfg, m.clientset, m.kubeCoreCache, m.refreshRate, m.watchNamespaces); err != nil {
					return nil, errors.Wrapf(err, "initializing kube cfg clientset and core cache")
				}
				f = &factory.KubeSecretClientFactory{
					Clientset:       *m.clientset,
					Cache:           *m.kubeCoreCache,
					SecretConverter: kubeconverters.GlooSecretConverterChain,
				}
			}
		case *v1.Settings_SecretOptions_Source_Vault:
			{
				rootKey := source.Vault.GetRootKey()
				if rootKey == "" {
					rootKey = DefaultRootKey
				}
				pathPrefix := source.Vault.GetPathPrefix()
				if pathPrefix == "" {
					pathPrefix = DefaultPathPrefix
				}
				f = NewVaultSecretClientFactory(m.vaultClient, pathPrefix, rootKey)
			}
		}
		multiClient.clientMap[k], err = f.NewResourceClient(ctx, params)
		if err != nil {
			return nil, err
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

// we should not need a RWMutex here because we are only ever writing to the map during
// its instantion in NewResourceClient
type multiSecretSourceResourceClient struct {
	clientMap     map[string]clients.ResourceClient // do not use clients.ResourceClients here as that is not for this purpose
	defaultSource *v1.Settings_SecretOptions_DefaultSource
}

// For all relevant/implemented CRUD operations on the client,
// Check resource metadata name for a valid source prefix e.g. "Vault_mysecret"
// (perhaps this should be case-insensitive)
// - Use specified client if prefix exists and client exists
// - Use default client if no prefix exists
// - If prefix exists but we don't have a matching client should we
//   - Attempt default client
//   - Return an error

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
