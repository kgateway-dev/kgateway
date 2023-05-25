package clients

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/avast/retry-go"
	vaultapi "github.com/hashicorp/vault/api"
	errors "github.com/rotisserie/eris"
	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	_ clients.ResourceClient        = new(MultiSecretResourceClient)
	_ factory.ResourceClientFactory = new(MultiSecretResourceClientFactory)
)

// SecretSourceAPIVaultClientInitIndex is a dedicated index for use of the SecretSource API
const SecretSourceAPIVaultClientInitIndex = -1

type SecretFactoryParams struct {
	Settings           *v1.Settings
	SharedCache        memory.InMemoryResourceCache
	Cfg                **rest.Config
	Clientset          *kubernetes.Interface
	KubeCoreCache      *cache.KubeCoreCache
	VaultClientInitMap map[int]VaultClientInitFunc // map client init funcs to their index in the sources slice
	PluralName         string
}

// SecretFactoryForSettings creates a resource client factory for provided config.
// Implemented as secrets.MultiResourceClient iff secretOptions API is configured.
func SecretFactoryForSettings(ctx context.Context, params SecretFactoryParams) (factory.ResourceClientFactory, error) {
	settings := params.Settings
	sharedCache := params.SharedCache
	cfg := params.Cfg
	clientset := params.Clientset
	kubeCoreCache := params.KubeCoreCache
	pluralName := params.PluralName
	vaultClientInitMap := params.VaultClientInitMap
	if vaultClientInitMap == nil {
		vaultClientInitMap = map[int]VaultClientInitFunc{}
	}

	if settings.GetSecretSource() == nil && settings.GetSecretOptions() == nil {
		if sharedCache == nil {
			return nil, errors.Errorf("internal error: shared cache cannot be nil")
		}
		return &factory.MemoryResourceClientFactory{
			Cache: sharedCache,
		}, nil
	}

	// Use secretOptions API if it is defined
	if secretOpts := settings.GetSecretOptions(); secretOpts != nil {
		return NewMultiSecretResourceClientFactory(MultiSecretFactoryParams{
			SecretSources:      secretOpts.GetSources(),
			SharedCache:        sharedCache,
			Cfg:                cfg,
			Clientset:          clientset,
			KubeCoreCache:      kubeCoreCache,
			VaultClientInitMap: vaultClientInitMap,
		})
	}

	// Fallback on secretSource API if secretOptions not defined
	if deprecatedApiSource := settings.GetSecretSource(); deprecatedApiSource != nil {
		var vaultClient *vaultapi.Client
		if vaultClientFunc, ok := params.VaultClientInitMap[SecretSourceAPIVaultClientInitIndex]; ok {
			vaultClient = vaultClientFunc()
		}
		return NewSecretResourceClientFactory(ctx,
			settings,
			sharedCache,
			cfg,
			clientset,
			kubeCoreCache,
			vaultClient,
			pluralName)
	}

	return nil, errors.Errorf("invalid config source type")
}

func NewSecretResourceClientFactory(ctx context.Context,
	settings *v1.Settings,
	sharedCache memory.InMemoryResourceCache,
	cfg **rest.Config,
	clientset *kubernetes.Interface,
	kubeCoreCache *cache.KubeCoreCache,
	vaultClient *vaultapi.Client,
	pluralName string) (factory.ResourceClientFactory, error) {
	switch source := settings.GetSecretSource().(type) {
	case *v1.Settings_KubernetesSecretSource:
		if err := initializeForKube(ctx, cfg, clientset, kubeCoreCache, settings.GetRefreshRate(), settings.GetWatchNamespaces()); err != nil {
			return nil, errors.Wrapf(err, "initializing kube cfg clientset and core cache")
		}
		return &factory.KubeSecretClientFactory{
			Clientset:       *clientset,
			Cache:           *kubeCoreCache,
			SecretConverter: kubeconverters.GlooSecretConverterChain,
		}, nil
	case *v1.Settings_VaultSecretSource:
		rootKey := source.VaultSecretSource.GetRootKey()
		if rootKey == "" {
			rootKey = DefaultRootKey
		}
		pathPrefix := source.VaultSecretSource.GetPathPrefix()
		if pathPrefix == "" {
			pathPrefix = DefaultPathPrefix
		}
		return NewVaultSecretClientFactory(NoopVaultClientInitFunc(vaultClient), pathPrefix, rootKey), nil
	case *v1.Settings_DirectorySecretSource:
		return &factory.FileResourceClientFactory{
			RootDir: filepath.Join(source.DirectorySecretSource.GetDirectory(), pluralName),
		}, nil
	}
	return nil, errors.Errorf("invalid config source type in secretSource")
}

type MultiSecretResourceClientFactory struct {
	secretSources      []*v1.Settings_SecretOptions_Source
	sharedCache        memory.InMemoryResourceCache
	cfg                **rest.Config
	clientset          *kubernetes.Interface
	kubeCoreCache      *cache.KubeCoreCache
	vaultClientInitMap map[int]VaultClientInitFunc

	refreshRate     *durationpb.Duration
	watchNamespaces []string
}

var (
	// ErrNilSourceSlice indicates a nil slice of sources was passed to the factory,
	// and we can therefore not initialize any sub-clients
	ErrNilSourceSlice = errors.New("nil slice of secret sources")

	// ErrEmptySourceSlice indicates the factory held an empty slice of sources while
	// trying to create a new client, and we can therefore not initialize any sub-clients
	ErrEmptySourceSlice = errors.New("empty slice of secret sources")
)

type MultiSecretFactoryParams struct {
	SecretSources      []*v1.Settings_SecretOptions_Source
	SharedCache        memory.InMemoryResourceCache
	Cfg                **rest.Config
	Clientset          *kubernetes.Interface
	KubeCoreCache      *cache.KubeCoreCache
	VaultClientInitMap map[int]VaultClientInitFunc
}

// NewMultiSecretResourceClientFactory returns a resource client factory for a client
// supporting multiple sources
func NewMultiSecretResourceClientFactory(params MultiSecretFactoryParams) (factory.ResourceClientFactory, error) {

	// Guard against nil source slice
	if params.SecretSources == nil {
		return nil, ErrNilSourceSlice
	}

	return &MultiSecretResourceClientFactory{
		secretSources:      params.SecretSources,
		sharedCache:        params.SharedCache,
		cfg:                params.Cfg,
		clientset:          params.Clientset,
		kubeCoreCache:      params.KubeCoreCache,
		vaultClientInitMap: params.VaultClientInitMap,
	}, nil
}

type asyncClientInitParams struct {
	multiClient     *MultiSecretResourceClient
	rcFactoryFunc   func() (factory.ResourceClientFactory, error)
	onInitialized   func(*MultiSecretResourceClient, clients.ResourceClient)
	newClientParams factory.NewResourceClientParams
	errChan         chan error
	doneChan        chan struct{}
}

var initClientAsync = func(ctx context.Context, params *asyncClientInitParams) {
	var c clients.ResourceClient
	var err error

	// run the func that produces our factory, including necessary setup for then
	// factory, in a way that allows us to catch factory creation errors.
	// it is possible that this should be in the retry?
	rcFactory, err := params.rcFactoryFunc()
	if err != nil {
		params.errChan <- err
		return
	}

	err = retry.Do(func() error {
		var tryErr error
		c, tryErr = rcFactory.NewResourceClient(ctx, params.newClientParams)
		return tryErr
	})
	if err != nil {
		params.errChan <- errors.Wrapf(err, "failed to initialize secret resource client from factory of type %T", rcFactory)
		return
	}

	params.onInitialized(params.multiClient, c)

	params.doneChan <- struct{}{}
}

// NewResourceClient implements ResourceClientFactory by creating a new client with each sub-client initialized
func (m *MultiSecretResourceClientFactory) NewResourceClient(ctx context.Context, params factory.NewResourceClientParams) (clients.ResourceClient, error) {
	if len(m.secretSources) == 0 {
		return nil, ErrEmptySourceSlice
	}

	multiClient := &MultiSecretResourceClient{RWMutex: &sync.RWMutex{}}

	var f factory.ResourceClientFactory

	for i, v := range m.secretSources {
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
				multiClient.hasDirectory = true
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
				multiClient.hasKubernetes = true
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
				vaultInit, ok := m.vaultClientInitMap[i]
				if !ok {
					return nil, errors.Errorf("unable to find vault client init for vault source at location %d", i)
				}
				// Use a Factory which will attempt to retry client connection
				f = NewVaultSecretClientFactory(vaultInit, pathPrefix, rootKey)
				multiClient.hasVault = true
			}
		}

		c, err := f.NewResourceClient(ctx, params)
		if err != nil {
			return nil, err
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

// MultiSecretResourceClient represents a client that is minimally implemented to facilitate Gloo operations.
// Specifically, only List and Watch are properly implemented.
//
// Direct access to clientList is deliberately omitted to prevent changing clients
// with an open Watch leading to inconsistent and undefined behavior
type MultiSecretResourceClient struct {
	*sync.RWMutex                          // because we are initializing clients asynchronously in parallel
	clientList    []clients.ResourceClient // do not use clients.ResourceClients here as that is not for this purpose
	hasKubernetes bool
	hasDirectory  bool
	hasVault      bool
}

// NumClients returns the number of clients configured in the clientList. Clients
// are only added to the list if/when they succeed initialization.
func (m *MultiSecretResourceClient) NumClients() int {
	m.Lock()
	defer m.Unlock()
	return len(m.clientList)
}

func (m *MultiSecretResourceClient) HasKubernetes() bool {
	m.Lock()
	defer m.Unlock()
	return m.hasKubernetes
}

func (m *MultiSecretResourceClient) HasDirectory() bool {
	m.Lock()
	defer m.Unlock()
	return m.hasDirectory
}

func (m *MultiSecretResourceClient) HasVault() bool {
	m.Lock()
	defer m.Unlock()
	return m.hasVault
}

func (m *MultiSecretResourceClient) Kind() string {
	// we know we have >0 clients due to the check in NewMultiSecretResourceClientFactory.NewResourceClient
	return m.clientList[0].Kind()
}

func (m *MultiSecretResourceClient) NewResource() resources.Resource {
	m.Lock()
	defer m.Unlock()
	if len(m.clientList) == 0 {
		return nil
	}

	// Any of the clients should be able to handle this identically
	return m.clientList[0].NewResource()
}

// Deprecated: implemented only by the kubernetes resource client. Will be removed from the interface.
func (m *MultiSecretResourceClient) Register() error {
	// return no error since the EC2 plugin calls the Register() function
	return nil
}

func (m *MultiSecretResourceClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
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

type resourceListAggregator map[int]resources.ResourceList

func (r *resourceListAggregator) aggregate() resources.ResourceList {
	m := *r
	rl := make(resources.ResourceList, 0, len(m))
	for _, v := range m {
		rl = append(rl, v...)
	}
	return rl
}

func (r *resourceListAggregator) set(k int, v resources.ResourceList) {
	m := *r
	m[k] = v
}

// newResourceListAggregator initializes by calling List on each client in the clientList
// and returning a populated *resourceListAggregator. An error here will cause the
// calling Watch to return an error
func newResourceListAggregator(mc *MultiSecretResourceClient, namespace string, opts clients.WatchOpts) (*resourceListAggregator, error) {
	r := &resourceListAggregator{}
	listOpts := clients.ListOpts{
		Ctx:                opts.Ctx,
		Cluster:            opts.Cluster,
		Selector:           opts.Selector,
		ExpressionSelector: opts.ExpressionSelector,
	}
	for i := range mc.clientList {
		l, err := mc.clientList[i].List(namespace, listOpts)
		if err != nil {
			return nil, err
		}
		r.set(i, l)
	}
	return r, nil
}

func (m *MultiSecretResourceClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	listChan := make(chan resources.ResourceList)
	errChan := make(chan error)

	// create a new aggregator so we can keep the last known state of individual clients.
	// this allows us to send a single ResourceList to the api snapshot emitter, which
	// expects values received on the returned channel to be atomically complete
	resourceListPerClient, err := newResourceListAggregator(m, namespace, opts)
	if err != nil {
		return nil, nil, err
	}

	for i := range m.clientList {
		idx := i
		clientListChan, clientErrChan, err := m.clientList[i].Watch(namespace, opts)
		if err != nil {
			return nil, nil, err
		}
		// set a goroutine for each client to call its Watch, then aggregate and send
		// on each receive.
		go func() {
			for {
				select {
				case <-opts.Ctx.Done():
					return
				case clientList := <-clientListChan:
					resourceListPerClient.set(idx, clientList)
					listChan <- resourceListPerClient.aggregate()
				case clientErr := <-clientErrChan:
					errChan <- clientErr
				}
			}
		}()

	}

	return listChan, errChan, nil
}

var (
	errNotImplMultiSecretClient = func(ctx context.Context, method string) error {
		err := errors.Wrap(ErrNotImplemented, fmt.Sprintf("%s in MultiSecretResourceClient", method))
		contextutils.LoggerFrom(ctx).DPanic(err.Error())

		return err
	}
)

func (m *MultiSecretResourceClient) Read(namespace string, name string, opts clients.ReadOpts) (resources.Resource, error) {
	return nil, errNotImplMultiSecretClient(opts.Ctx, "Read")
}

func (m *MultiSecretResourceClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	return nil, errNotImplMultiSecretClient(opts.Ctx, "Write")
}

func (m *MultiSecretResourceClient) Delete(namespace string, name string, opts clients.DeleteOpts) error {
	return errNotImplMultiSecretClient(opts.Ctx, "Delete")
}

func (m *MultiSecretResourceClient) ApplyStatus(statusClient resources.StatusClient, inputResource resources.InputResource, opts clients.ApplyStatusOpts) (resources.Resource, error) {
	return nil, errNotImplMultiSecretClient(opts.Ctx, "ApplyStatus")
}
