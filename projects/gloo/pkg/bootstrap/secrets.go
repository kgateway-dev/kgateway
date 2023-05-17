package bootstrap

import (
	"context"
	"strings"

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

const (
	// DO_NOT_SUBMIT: should this be configurable?
	// If so, then we need to make sure it is validated against filenames if we leave the prefix (i.e. we do not modify the secret api to include the source client)
	prefixIdentifier = "glooclient="
)

var (
	errInvalidClientPrefix = func(prefix string) error {
		return errors.Wrapf(ErrInvalidClientPrefix, "invalid source client prefix %s", prefix)
	}
	// errInvalidClientPrefix is used to indicate to caller that a client prefix was found, but is not
	// in our set of valid clients. This should be wrapped in errInvalidClientPrefix to display the
	// provided invalid prefix to the user
	ErrInvalidClientPrefix = errors.New("valid prefixes are (case-insensitive) glooclient=vault, glooclient=directory, glooclient=kubernetes")

	// ErrMissingClientPrefix is used to indicate to caller that parsing of the secret name found no
	// valid client prefix. Client prefixes must be formatted glooclient=%s
	ErrMissingClientPrefix = errors.New("no client prefix found")

	errClientNotFound = func(prefix string) error {
		return errors.Wrapf(ErrClientNotFound, "no client configured for prefix %s", prefix)
	}
	// ErrClientNotFound indicates that a client prefix was found and is valid, but no client
	// is currently configured to handle it. This should be wrapped in errClientNotFound to display
	// the provided prefix for which we do not have a client to the user
	ErrClientNotFound = errors.New("secret client not found")
)

func (m *multiSecretSourceResourceClient) getDefaultClient() (clients.ResourceClient, string, error) {
	defaultClientName := m.defaultSource.String()
	if client, ok := m.clientMap[defaultClientName]; ok {
		return client, defaultClientName, nil
	}
	// this should never happen
	return nil, "", errors.New("unable to find default client")
}

func (m *multiSecretSourceResourceClient) getClientForSecret(name string) (clients.ResourceClient, error) {

	// Check for existance of any underscore-delimited prefix
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return nil, ErrMissingClientPrefix
	}
	// Check the prefix conforms to the format we expect
	_, clientName, ok := strings.Cut(prefix, prefixIdentifier)
	if !ok {
		return nil, ErrMissingClientPrefix
	}

	// Check that the prefix specifies a supported client
	if _, ok := v1.Settings_SecretOptions_DefaultSource_value[strings.ToUpper(clientName)]; !ok {
		return nil, errInvalidClientPrefix(prefix)
	}

	// Check that we have the requested client configured
	if client, ok := m.clientMap[clientName]; ok {
		return client, nil
	}

	return nil, errClientNotFound(clientName)
}

func (m *multiSecretSourceResourceClient) getClientForSecretOrDefault(ctx context.Context, name string) (clients.ResourceClient, error) {
	client, getClientErr := m.getClientForSecret(name)
	if getClientErr == nil {
		return client, nil
	}

	defaultClient, defaultClientName, err := m.getDefaultClient()
	if err != nil {
		return nil, err
	}

	// DO_NOT_SUBMIT: how should each of these error cases be handled?
	logger := contextutils.LoggerFrom(ctx)
	if errors.Is(getClientErr, ErrClientNotFound) {
		logger.Warnf("Client not found for secret with name %s; using default client %s", name, defaultClientName)
		return defaultClient, nil
	}
	if errors.Is(getClientErr, ErrInvalidClientPrefix) {
		logger.Warnf("Client prefix is not valid for secret with name %s; using default client %s", name, defaultClientName)
		return defaultClient, nil
	}
	if errors.Is(getClientErr, ErrMissingClientPrefix) {
		logger.Warnf("Client prefix is not found for secret with name %s; using default client %s", name, defaultClientName)
		return defaultClient, nil
	}

	// This should never happen
	return nil, getClientErr

}

// For all relevant/implemented CRUD operations on the client,
// Check resource metadata name for a valid source prefix e.g. "glooclient=vault_mysecret"
// - Use specified client if prefix exists and client exists
// - Use default client if no prefix exists
// - If prefix exists but we don't have a matching client should we
//   - Attempt default client (this could lead to confusing placement of secrets. we could strip the prefix which might help some)
//   - Return an error (this might be overly restrictive/bad UX)

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
