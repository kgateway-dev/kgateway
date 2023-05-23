package clients

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/hashicorp/go-multierror"
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

// SecretFactoryForSettings creates a resource client factory for provided config.
// Implemented as secrets.MultiResourceClient iff secretOptions API is configured.
func SecretFactoryForSettings(ctx context.Context,
	settings *v1.Settings,
	sharedCache memory.InMemoryResourceCache,
	cfg **rest.Config,
	clientset *kubernetes.Interface,
	kubeCoreCache *cache.KubeCoreCache,
	vaultClient *vaultapi.Client,
	pluralName string) (factory.ResourceClientFactory, error) {

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
		return NewMultiSecretResourceClientFactory(secretOpts.GetSources(),
			sharedCache,
			cfg,
			clientset,
			kubeCoreCache,
			vaultClient)
	}

	// Fallback on secretSource API if secretOptions not defined
	if deprecatedApiSource := settings.GetSecretSource(); deprecatedApiSource != nil {
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
		return NewVaultSecretClientFactory(vaultClient, pathPrefix, rootKey), nil
	case *v1.Settings_DirectorySecretSource:
		return &factory.FileResourceClientFactory{
			RootDir: filepath.Join(source.DirectorySecretSource.GetDirectory(), pluralName),
		}, nil
	}
	return nil, errors.Errorf("invalid config source type in secretSource")
}

type MultiSecretResourceClientFactory struct {
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
	ErrNilSourceSlice = errors.New("nil slice of secret sources")

	// ErrEmptySourceSlice indicates the factory held an empty slice of sources while
	// trying to create a new client, and we can therefore not initialize any sub-clients
	ErrEmptySourceSlice = errors.New("empty slice of secret sources")
)

// NewMultiSecretResourceClientFactory returns a resource client factory for a client
// supporting multiple sources
func NewMultiSecretResourceClientFactory(secretSources []*v1.Settings_SecretOptions_Source,
	sharedCache memory.InMemoryResourceCache,
	cfg **rest.Config,
	clientset *kubernetes.Interface,
	kubeCoreCache *cache.KubeCoreCache,
	vaultClient *vaultapi.Client) (*MultiSecretResourceClientFactory, error) {

	// Guard against nil source slice
	if secretSources == nil {
		return nil, ErrNilSourceSlice
	}

	return &MultiSecretResourceClientFactory{
		secretSources: secretSources,
		sharedCache:   sharedCache,
		cfg:           cfg,
		clientset:     clientset,
		kubeCoreCache: kubeCoreCache,
		vaultClient:   vaultClient,
	}, nil
}

type asyncClientInitParams struct {
	ctx             context.Context
	wg              *sync.WaitGroup
	multiClient     *MultiSecretResourceClient
	f               factory.ResourceClientFactory
	onInitialized   func(*MultiSecretResourceClient, clients.ResourceClient)
	newClientParams factory.NewResourceClientParams
	errChan         chan error
}

var initClientAsync = func(params *asyncClientInitParams) {
	c, err := params.f.NewResourceClient(params.ctx, params.newClientParams)
	if err != nil {
		params.errChan <- errors.Wrapf(err, "failed to initialize secret resource client from factory of type %T", params.f)
		return
	}

	params.onInitialized(params.multiClient, c)

	// Only report Done on success so we don't short-circuit the `wg.Wait()` with a failed init.
	// Guard against NPE if wg goes out of scope in NewResourceClient
	if params.wg != nil {
		params.wg.Done()
	}
}

// NewResourceClient implements ResourceClientFactory by creating a new client with each sub-client initialized
func (m *MultiSecretResourceClientFactory) NewResourceClient(ctx context.Context, params factory.NewResourceClientParams) (clients.ResourceClient, error) {
	if len(m.secretSources) == 0 {
		return nil, ErrEmptySourceSlice
	}

	multiClient := &MultiSecretResourceClient{}

	// Create a WaitGroup to wait for at least one client to be initialized. Because
	// some clients may rely on others to be initialized before themselves successfully
	// finish initializing, we do not want to block on this race. We trust they will
	// eventually become healthy or we will log loudly and emit metrics if they fail.
	wg := &sync.WaitGroup{}
	wg.Add(1)

	errChan := make(chan error)

	clientCallback := func(multiClient *MultiSecretResourceClient, boolFieldToSet *bool) func(multiClient *MultiSecretResourceClient, client clients.ResourceClient) {
		return func(multiClient *MultiSecretResourceClient, client clients.ResourceClient) {
			multiClient.Lock()
			defer multiClient.Unlock()
			multiClient.clientList = append(multiClient.clientList, client)
			*boolFieldToSet = true
		}
	}

	for _, v := range m.secretSources {
		initParams := &asyncClientInitParams{
			ctx:         ctx,
			wg:          wg,
			multiClient: multiClient,
			errChan:     errChan,
		}

		switch source := v.GetSource().(type) {
		case *v1.Settings_SecretOptions_Source_Directory:
			{
				directory := source.Directory.GetDirectory()
				if directory == "" {
					return nil, errors.New("directory cannot be empty string")
				}
				initParams.f = &factory.FileResourceClientFactory{
					RootDir: directory,
				}
				initParams.onInitialized = clientCallback(multiClient, &multiClient.hasDirectory)
			}
		case *v1.Settings_SecretOptions_Source_Kubernetes:
			{
				if err := initializeForKube(ctx, m.cfg, m.clientset, m.kubeCoreCache, m.refreshRate, m.watchNamespaces); err != nil {
					return nil, errors.Wrapf(err, "initializing kube cfg clientset and core cache")
				}
				initParams.f = &factory.KubeSecretClientFactory{
					Clientset:       *m.clientset,
					Cache:           *m.kubeCoreCache,
					SecretConverter: kubeconverters.GlooSecretConverterChain,
				}
				initParams.onInitialized = clientCallback(multiClient, &multiClient.hasKubernetes)
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
				initParams.f = NewVaultSecretClientFactory(m.vaultClient, pathPrefix, rootKey)
				initParams.onInitialized = clientCallback(multiClient, &multiClient.hasVault)
			}
		}

		go initClientAsync(initParams)
	}

	// Construct a chan so we can use the `wg.Wait()` in a select
	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	// We wait for at least one client to be initialized, then return our multiClient.
	// Alternately, if we receive a number of errors equivalent to the number of clients
	// we are trying to configure, then we know that all clients have failed to initialize.
	multiErr := multierror.Error{}
	for i := 0; i < len(m.secretSources); i++ {
		select {
		case <-waitCh:
			return multiClient, nil
		case recvErr := <-errChan:
			contextutils.LoggerFrom(ctx).Error(recvErr)
			// DO_NOT_SUBMIT: emit metric for failed resource client initialization
			multiErr.Errors = append(multiErr.Errors, recvErr)
		}
	}

	// We shouldn't ever receive nil on the errChan, but if we do this would be
	// a terribly obscure bug. Check for nil and return a canned err if nil.
	var err error
	if err = multiErr.ErrorOrNil(); err == nil {
		err = errors.New("secrets client(s) failed to initialize")
	}
	return nil, err
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
	return len(m.clientList)
}

func (m *MultiSecretResourceClient) HasKubernetes() bool {
	return m.hasKubernetes
}

func (m *MultiSecretResourceClient) HasDirectory() bool {
	return m.hasDirectory
}

func (m *MultiSecretResourceClient) HasVault() bool {
	return m.hasVault
}

func (m *MultiSecretResourceClient) Kind() string {
	if len(m.clientList) == 0 {
		return ""
	}

	// Any of the clients should be able to handle this identically
	return m.clientList[0].Kind()
}

func (m *MultiSecretResourceClient) NewResource() resources.Resource {
	if len(m.clientList) == 0 {
		return nil
	}

	// Any of the clients should be able to handle this identically
	return m.clientList[0].NewResource()
}

// Deprecated: implemented only by the kubernetes resource client. Will be removed from the interface.
func (m *MultiSecretResourceClient) Register() error {
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

func (m *MultiSecretResourceClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
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
