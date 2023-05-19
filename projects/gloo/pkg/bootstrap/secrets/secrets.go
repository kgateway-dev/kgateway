package secrets

import (
	"context"

	vaultapi "github.com/hashicorp/vault/api"
	errors "github.com/rotisserie/eris"
	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/file"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/vault"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type MultiResourceClientFactory struct {
	secretSources []*v1.Settings_SecretOptions_Source
	sharedCache   memory.InMemoryResourceCache
	cfg           **rest.Config
	clientset     *kubernetes.Interface
	kubeCoreCache *cache.KubeCoreCache
	vaultClient   *vaultapi.Client

	refreshRate     *durationpb.Duration
	watchNamespaces []string
}

var (
	// ErrNilSourceSlice indicates a nil slice of sources was passed to the factory,
	// and we can therefore not initialize any sub-clients
	ErrNilSourceSlice = errors.New("nil slice of secretSources")
)

// NewMultiResourceClientFactory returns a resource client factory for a client
// supporting multiple sources
func NewMultiResourceClientFactory(secretSources []*v1.Settings_SecretOptions_Source,
	sharedCache memory.InMemoryResourceCache,
	cfg **rest.Config,
	clientset *kubernetes.Interface,
	kubeCoreCache *cache.KubeCoreCache,
	vaultClient *vaultapi.Client) (*MultiResourceClientFactory, error) {

	// Guard against nil source slice
	if secretSources == nil {
		return nil, ErrNilSourceSlice
	}

	return &MultiResourceClientFactory{
		secretSources: secretSources,
		sharedCache:   sharedCache,
		cfg:           cfg,
		clientset:     clientset,
		kubeCoreCache: kubeCoreCache,
		vaultClient:   vaultClient,
	}, nil
}

// NewResourceClient implements ResourceClientFactory by creating a new client with each sub-client initialized
func (m *MultiResourceClientFactory) NewResourceClient(ctx context.Context, params factory.NewResourceClientParams) (clients.ResourceClient, error) {

	multiClient := &MultiResourceClient{}
	var f factory.ResourceClientFactory
	for _, v := range m.secretSources {
		switch source := v.GetSource().(type) {
		case *v1.Settings_SecretOptions_Source_Directory:
			{
				directory := source.Directory.GetDirectory()
				if directory == "" {
					return nil, errors.New("directory cannot be empty string")
				}
				f = &factory.FileResourceClientFactory{
					RootDir: directory,
				}
			}
		case *v1.Settings_SecretOptions_Source_Kubernetes:
			{
				if err := initializeForKube(ctx, m.cfg, m.clientset, m.kubeCoreCache, m.refreshRate, m.watchNamespaces); err != nil {
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
		c, err := f.NewResourceClient(ctx, params)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to initialize secret resource client from factory of type %T", f)
		}

		multiClient.clientList = append(multiClient.clientList, c)
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
type MultiResourceClient struct {
	clientList []clients.ResourceClient // do not use clients.ResourceClients here as that is not for this purpose
}

// Clients provides access to the sub-clients used by this multi-client. This is primarily
// exposed for testing; care should be taken when mutating this client at runtime
// as the Watch method may have spawned goroutines with clients that will not
// be updated by the mutation.
func (m *MultiResourceClient) Clients() []clients.ResourceClient {
	return m.clientList
}

func (m *MultiResourceClient) Kind() string {
	if len(m.clientList) == 0 {
		return ""
	}

	// Any of the clients should be able to handle this identically
	return m.clientList[0].Kind()
}

func (m *MultiResourceClient) NewResource() resources.Resource {
	if len(m.clientList) == 0 {
		return nil
	}

	// Any of the clients should be able to handle this identically
	return m.clientList[0].NewResource()
}

// Deprecated: implemented only by the kubernetes resource client. Will be removed from the interface.
func (m *MultiResourceClient) Register() error {
	for _, v := range m.clientList {
		switch concreteClient := v.(type) {
		case *vault.ResourceClient:
			continue
		case *file.ResourceClient:
			continue
		default:
			return concreteClient.Register()
		}
	}
	return errors.New("Register method only implemented on Kubernetes resource client")
}

func (m *MultiResourceClient) Read(namespace string, name string, opts clients.ReadOpts) (resources.Resource, error) {
	errMsg := "Read not implemented on multiSecretSourceResourceClient"
	contextutils.LoggerFrom(opts.Ctx).DPanic(errMsg)
	return nil, errors.New(errMsg)
}

func (m *MultiResourceClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	errMsg := "Write not implemented on multiSecretSourceResourceClient"
	contextutils.LoggerFrom(opts.Ctx).DPanic(errMsg)
	return nil, errors.New(errMsg)
}

func (m *MultiResourceClient) Delete(namespace string, name string, opts clients.DeleteOpts) error {
	errMsg := "Delete not implemented on multiSecretSourceResourceClient"
	contextutils.LoggerFrom(opts.Ctx).DPanic(errMsg)
	return errors.New(errMsg)
}

func (m *MultiResourceClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	list := resources.ResourceList{}
	for i := range m.clientList {
		clientList, err := m.clientList[i].List(namespace, opts)
		if err != nil {
			return nil, err
		}
		list = append(list, clientList...)
	}

	return list, nil
}

func (m *MultiResourceClient) ApplyStatus(statusClient resources.StatusClient, inputResource resources.InputResource, opts clients.ApplyStatusOpts) (resources.Resource, error) {
	errMsg := "ApplyStatus not implemented on multiSecretSourceResourceClient"
	contextutils.LoggerFrom(opts.Ctx).DPanic(errMsg)
	return nil, errors.New(errMsg)
}

func (m *MultiResourceClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	listChan := make(chan resources.ResourceList)
	errChan := make(chan error)
	for i := range m.clientList {
		clientListChan, clientErrChan, err := m.clientList[i].Watch(namespace, opts)
		if err != nil {
			return nil, nil, err
		}
		go func() {
			for {
				select {
				case <-opts.Ctx.Done():
					return
				case clientList := <-clientListChan:
					listChan <- clientList
				case clientErr := <-clientErrChan:
					errChan <- clientErr
				}
			}
		}()

	}

	return listChan, errChan, nil
}
