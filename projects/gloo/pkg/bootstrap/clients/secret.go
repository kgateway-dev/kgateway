package clients

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"

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
	if params.VaultClientInitMap == nil {
		params.VaultClientInitMap = map[int]VaultClientInitFunc{}
	}

	if settings.GetSecretSource() == nil && settings.GetSecretOptions() == nil {
		if params.SharedCache == nil {
			return nil, errors.Errorf("internal error: shared cache cannot be nil")
		}
		return &factory.MemoryResourceClientFactory{
			Cache: params.SharedCache,
		}, nil
	}

	// Use secretOptions API if it is defined
	if secretOpts := settings.GetSecretOptions(); secretOpts != nil {
		return NewMultiSecretResourceClientFactory(MultiSecretFactoryParams{
			SecretSources:      secretOpts.GetSources(),
			SharedCache:        params.SharedCache,
			Cfg:                params.Cfg,
			Clientset:          params.Clientset,
			KubeCoreCache:      params.KubeCoreCache,
			VaultClientInitMap: params.VaultClientInitMap,
		})
	}

	// Fallback on secretSource API if secretOptions not defined
	if secretSource := settings.GetSecretSource(); secretSource != nil {
		return NewSecretResourceClientFactory(ctx, params)
	}

	return nil, errors.Errorf("invalid config source type")
}

func NewSecretResourceClientFactory(ctx context.Context, params SecretFactoryParams) (factory.ResourceClientFactory, error) {
	switch source := params.Settings.GetSecretSource().(type) {
	case *v1.Settings_KubernetesSecretSource:
		if err := initializeForKube(ctx, params.Cfg, params.Clientset, params.KubeCoreCache, params.Settings.GetRefreshRate(), params.Settings.GetWatchNamespaces()); err != nil {
			return nil, errors.Wrapf(err, "initializing kube cfg clientset and core cache")
		}
		return &factory.KubeSecretClientFactory{
			Clientset:       *params.Clientset,
			Cache:           *params.KubeCoreCache,
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
		vaultClientFunc := params.VaultClientInitMap[SecretSourceAPIVaultClientInitIndex]
		// We do not error upon creating a vault ResourceClientFactory, but we can
		// check for a nil API client here before returning it and error appropriately.
		f := NewVaultSecretClientFactory(vaultClientFunc, pathPrefix, rootKey)
		if vaultClientFactory, ok := f.(*factory.VaultSecretClientFactory); ok && vaultClientFactory.Vault == nil {
			return nil, errors.New("resource client creation failed due to nil vault API client")
		}
		return f, nil
	case *v1.Settings_DirectorySecretSource:
		return &factory.FileResourceClientFactory{
			RootDir: filepath.Join(source.DirectorySecretSource.GetDirectory(), params.PluralName),
		}, nil
	}
	return nil, errors.Errorf("invalid config source type in secretSource")
}

type MultiSecretResourceClientFactory struct {
	secretSources      SourceList
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

type SourceList []*v1.Settings_SecretOptions_Source

func (s SourceList) Len() int {
	return len(s)
}

func (s SourceList) Less(i int, j int) bool {
	// kube > directory > vault
	switch iConc := s[i].GetSource().(type) {
	case *v1.Settings_SecretOptions_Source_Kubernetes:
		// always put kube first. we should never have 2 kube sources defined, but
		// if we do they are pulling from the same source so their order shouldn't matter
		return true
	case *v1.Settings_SecretOptions_Source_Directory:
		switch jConc := s[j].GetSource().(type) {
		case *v1.Settings_SecretOptions_Source_Kubernetes:
			return false
		case *v1.Settings_SecretOptions_Source_Vault:
			return true
		case *v1.Settings_SecretOptions_Source_Directory:
			return iConc.Directory.GetDirectory() < jConc.Directory.GetDirectory()
		}
	case *v1.Settings_SecretOptions_Source_Vault:
		switch jConc := s[j].GetSource().(type) {
		case *v1.Settings_SecretOptions_Source_Vault:
			return iConc.Vault.String() < jConc.Vault.String()
		default:
			return false
		}
	}
	return i < j
}

func (s SourceList) Swap(i int, j int) {
	tmp := s[i]
	s[i] = s[j]
	s[j] = tmp
}

func (s SourceList) sort() SourceList {
	sort.Stable(s)
	return s
}

type MultiSecretFactoryParams struct {
	SecretSources      SourceList
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
		secretSources:      params.SecretSources, // the source list is sorted in Setup before being passed in here
		sharedCache:        params.SharedCache,
		cfg:                params.Cfg,
		clientset:          params.Clientset,
		kubeCoreCache:      params.KubeCoreCache,
		vaultClientInitMap: params.VaultClientInitMap,
	}, nil
}

func (m *MultiSecretResourceClientFactory) getFactoryForSource(ctx context.Context,
	source *v1.Settings_SecretOptions_Source,
	vaultInitFunc VaultClientInitFunc) (factory.ResourceClientFactory, error) {

	switch source := source.GetSource().(type) {
	case *v1.Settings_SecretOptions_Source_Directory:
		{
			directory := source.Directory.GetDirectory()
			if directory == "" {
				return nil, errors.New("directory cannot be empty string")
			}
			return &factory.FileResourceClientFactory{
				RootDir: directory,
			}, nil
		}
	case *v1.Settings_SecretOptions_Source_Kubernetes:
		{
			if err := initializeForKube(ctx, m.cfg, m.clientset, m.kubeCoreCache, m.refreshRate, m.watchNamespaces); err != nil {
				return nil, errors.Wrapf(err, "initializing kube cfg clientset and core cache")
			}
			return &factory.KubeSecretClientFactory{
				Clientset:       *m.clientset,
				Cache:           *m.kubeCoreCache,
				SecretConverter: kubeconverters.GlooSecretConverterChain,
			}, nil
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
			// We do not error upon creating a ResourceClientFactory, but we can
			// check for a nil API client when attempting to use the factory to
			// create a ResourceClient and error here.
			f := NewVaultSecretClientFactory(vaultInitFunc, pathPrefix, rootKey)
			if vaultClientFactory, ok := f.(*factory.VaultSecretClientFactory); ok && vaultClientFactory.Vault == nil {
				return nil, errors.New("resource client creation failed due to nil vault API client")
			}
			return f, nil
		}
	}
	return nil, errors.Errorf("invalid config source type in secretSource")
}

// NewResourceClient implements ResourceClientFactory by creating a new client with each sub-client initialized
func (m *MultiSecretResourceClientFactory) NewResourceClient(ctx context.Context, params factory.NewResourceClientParams) (clients.ResourceClient, error) {
	if len(m.secretSources) == 0 {
		return nil, ErrEmptySourceSlice
	}

	// If we have only a single source, use the factory and client for that source
	if len(m.secretSources) == 1 {
		f, err := m.getFactoryForSource(ctx, m.secretSources[0], m.vaultClientInitMap[0])
		if err != nil {
			return nil, err
		}
		return f.NewResourceClient(ctx, params)
	}

	multiClient := &MultiSecretResourceClient{RWMutex: &sync.RWMutex{}}

	for i := range m.secretSources {
		f, err := m.getFactoryForSource(ctx, m.secretSources[i], m.vaultClientInitMap[i])
		if err != nil {
			return nil, err
		}

		c, err := f.NewResourceClient(ctx, params)
		if err != nil {
			return nil, err
		}

		multiClient.clientList = append(multiClient.clientList, c)
	}

	return multiClient, nil
}

// MultiSecretResourceClient represents a client that is minimally implemented to facilitate Gloo operations.
// Specifically, only List and Watch are properly implemented.
//
// Direct access to clientList is deliberately omitted to prevent changing clients
// with an open Watch leading to inconsistent and undefined behavior
type MultiSecretResourceClient struct {
	// guard against concurrent slice access
	*sync.RWMutex

	// do not use clients.ResourceClients here as that is not for this purpose.
	// clientList is guaranteed to be stable sorted due to sorting the sources
	// in the Setup function
	clientList []clients.ResourceClient
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
	m.Lock()
	defer m.Unlock()
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
