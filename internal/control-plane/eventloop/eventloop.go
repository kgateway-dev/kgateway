package eventloop

import (
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/mitchellh/hashstructure"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/storage/consul"
	consulfiles "github.com/solo-io/gloo/pkg/storage/dependencies/consul"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/crd"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	filestore "github.com/solo-io/gloo/pkg/storage/dependencies/file"
	kubestore "github.com/solo-io/gloo/pkg/storage/dependencies/kube"
	"github.com/solo-io/gloo/pkg/storage/file"
	"github.com/solo-io/gloo/internal/control-plane/configwatcher"
	"github.com/solo-io/gloo/internal/control-plane/reporter"
	"github.com/solo-io/gloo/internal/control-plane/translator"
	"github.com/solo-io/gloo/internal/control-plane/xds"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/internal/control-plane/filewatcher"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/secretwatcher"
	filesecrets "github.com/solo-io/gloo/pkg/secretwatcher/file"
	kubesecrets "github.com/solo-io/gloo/pkg/secretwatcher/kube"
	"github.com/solo-io/gloo/pkg/secretwatcher/vault"
)

type eventLoop struct {
	configWatcher       configwatcher.Interface
	secretWatcher       secretwatcher.Interface
	fileWatcher         filewatcher.Interface
	endpointDiscoveries []endpointdiscovery.Interface
	reporter            reporter.Interface
	translator          *translator.Translator
	xdsConfig           envoycache.SnapshotCache
	getDependencies     func(cfg *v1.Config) []*plugins.Dependencies

	startFuncs []func() error
}

func Setup(opts bootstrap.Options, stop <-chan struct{}) (*eventLoop, error) {
	store, err := createStorageClient(opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create config store client")
	}

	cfgWatcher, err := configwatcher.NewConfigWatcher(store)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create config watcher")
	}

	secretWatcher, err := setupSecretWatcher(opts, stop)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up secret watcher")
	}

	fileWatcher, err := setupFileWatcher(opts, stop)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up file watcher")
	}

	xdsConfig, _, err := xds.RunXDS(opts.XdsOptions.Port)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start xds server")
	}

	plugs := plugins.RegisteredPlugins()

	trans := translator.NewTranslator(plugs)

	e := &eventLoop{
		configWatcher:   cfgWatcher,
		secretWatcher:   secretWatcher,
		fileWatcher:     fileWatcher,
		translator:      trans,
		xdsConfig:       xdsConfig,
		getDependencies: getDependenciesFor(plugs),
		reporter:        reporter.NewReporter(store),
	}

	for _, endpointDiscoveryInitializer := range plugins.EndpointDiscoveryInitializers() {
		e.startFuncs = append(e.startFuncs, func() error {
			discovery, err := endpointDiscoveryInitializer(opts)
			if err != nil {
				log.Warnf("Starting endpoint discovery failed: %v, endpoints will not be discovered for this "+
					"upstream type", err)
				return nil
			}
			e.endpointDiscoveries = append(e.endpointDiscoveries, discovery)
			return nil
		})
	}
	return e, nil
}

func getDependenciesFor(translatorPlugins []plugins.TranslatorPlugin) func(cfg *v1.Config) []*plugins.Dependencies {
	return func(cfg *v1.Config) []*plugins.Dependencies {
		var dependencies []*plugins.Dependencies
		// secrets plugins need
		for _, plug := range translatorPlugins {
			dep := plug.GetDependencies(cfg)
			if dep != nil {
				dependencies = append(dependencies, dep)
			}
		}
		return dependencies
	}
}

func createStorageClient(opts bootstrap.Options) (storage.Interface, error) {
	switch opts.ConfigWatcherOptions.Type {
	case bootstrap.WatcherTypeFile:
		dir := opts.FileOptions.ConfigDir
		if dir == "" {
			return nil, errors.New("must provide directory for file config watcher")
		}
		client, err := file.NewStorage(dir, opts.ConfigWatcherOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start file config watcher for directory %v", dir)
		}
		return client, nil
	case bootstrap.WatcherTypeKube:
		cfg, err := clientcmd.BuildConfigFromFlags(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
		if err != nil {
			return nil, errors.Wrap(err, "building kube restclient")
		}
		cfgWatcher, err := crd.NewStorage(cfg, opts.KubeOptions.Namespace, opts.ConfigWatcherOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start kube config watcher with config %#v", opts.KubeOptions)
		}
		return cfgWatcher, nil
	case bootstrap.WatcherTypeConsul:
		cfg := opts.ConsulOptions.ToConsulConfig()
		cfgWatcher, err := consul.NewStorage(cfg, opts.ConsulOptions.RootPath, opts.ConfigWatcherOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start consul config watcher with config %#v", opts.ConsulOptions)
		}
		return cfgWatcher, nil
	}
	return nil, errors.Errorf("unknown or unspecified config watcher type: %v", opts.ConfigWatcherOptions.Type)
}

func setupSecretWatcher(opts bootstrap.Options, stop <-chan struct{}) (secretwatcher.Interface, error) {
	switch opts.SecretWatcherOptions.Type {
	case bootstrap.WatcherTypeFile:
		secretWatcher, err := filesecrets.NewSecretWatcher(opts.FileOptions.SecretDir, opts.SecretWatcherOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start file secret watcher with config %#v", opts.KubeOptions)
		}
		return secretWatcher, nil
	case bootstrap.WatcherTypeKube:
		secretWatcher, err := kubesecrets.NewSecretWatcher(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig, opts.SecretWatcherOptions.SyncFrequency, stop)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start kube secret watcher with config %#v", opts.KubeOptions)
		}
		return secretWatcher, nil
	case bootstrap.WatcherTypeVault:
		secretWatcher, err := vault.NewVaultSecretWatcher(opts.SecretWatcherOptions.SyncFrequency, opts.VaultOptions.Retries, opts.VaultOptions.VaultAddr, opts.VaultOptions.AuthToken, stop)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start vault secret watcher with config %#v", opts.VaultOptions)
		}
		return secretWatcher, nil
	}
	return nil, errors.Errorf("unknown or unspecified secret watcher type: %v", opts.SecretWatcherOptions.Type)
}

func setupFileWatcher(opts bootstrap.Options, stop <-chan struct{}) (filewatcher.Interface, error) {
	var store dependencies.FileStorage
	switch opts.SecretWatcherOptions.Type {
	case bootstrap.WatcherTypeFile:
		s, err := filestore.NewFileStorage(opts.FileOptions.FilesDir, opts.FileWatcherOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start filesystem-based file watcher with config %#v", opts.FileOptions)
		}
		store = s
	case bootstrap.WatcherTypeKube:
		cfg, err := clientcmd.BuildConfigFromFlags(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
		if err != nil {
			return nil, errors.Wrap(err, "building kube restclient")
		}
		s, err := kubestore.NewFileStorage(cfg, opts.KubeOptions.Namespace, opts.FileWatcherOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start kube configmap-based file watcher with config %#v", opts.KubeOptions)
		}
		store = s
	case bootstrap.WatcherTypeConsul:
		cfg := opts.ConsulOptions.ToConsulConfig()
		s, err := consulfiles.NewFileStorage(cfg, opts.ConsulOptions.RootPath, opts.ConfigWatcherOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start consul KV-based file watcher with config %#v", opts.ConsulOptions)
		}
		store = s
	default:
		return nil, errors.Errorf("unknown or unspecified file watcher type: %v", opts.FileWatcherOptions.Type)
	}
	return filewatcher.NewFileWatcher(store)
}

func (e *eventLoop) Run(stop <-chan struct{}) error {
	for _, fn := range e.startFuncs {
		if err := fn(); err != nil {
			return err
		}
	}
	for _, eds := range e.endpointDiscoveries {
		go eds.Run(stop)
	}
	go e.configWatcher.Run(stop)

	endpointDiscovery := e.endpointDiscovery()
	workerErrors := e.errors()

	// cache the most recent read for any of these
	var hash uint64
	current := newCache()
	sync := func(current *cache) {
		newHash := current.hash()
		if hash == newHash {
			return
		}
		hash = newHash
		e.updateXds(current)
	}
	for {
		select {
		case cfg := <-e.configWatcher.Config():
			log.Debugf("change triggered by config")
			current.cfg = cfg
			dependencies := e.getDependencies(cfg)
			var secretRefs, fileRefs []string
			for _, dep := range dependencies {
				secretRefs = append(secretRefs, dep.SecretRefs...)
				fileRefs = append(fileRefs, dep.FileRefs...)
			}
			// secrets for virtualhosts
			for _, vhost := range cfg.VirtualHosts {
				if vhost.SslConfig != nil && vhost.SslConfig.SecretRef != "" {
					secretRefs = append(secretRefs, vhost.SslConfig.SecretRef)
				}
			}
			go e.secretWatcher.TrackSecrets(secretRefs)
			go e.fileWatcher.TrackFiles(fileRefs)
			for _, discovery := range e.endpointDiscoveries {
				go func() {
					discovery.TrackUpstreams(cfg.Upstreams)
				}()
			}
			sync(current)
		case secrets := <-e.secretWatcher.Secrets():
			log.Debugf("change triggered by secrets")
			current.secrets = secrets
			sync(current)
		case files := <-e.fileWatcher.Files():
			log.Debugf("change triggered by files")
			current.files = files
			sync(current)
		case endpointTuple := <-endpointDiscovery:
			log.Debugf("change triggered by endpoints")
			current.endpoints[endpointTuple.discoveredBy] = endpointTuple.endpoints
			sync(current)
		case err := <-workerErrors:
			log.Warnf("error in control plane event loop: %v", err)
		}
	}
}

func (e *eventLoop) updateXds(cache *cache) {
	if !cache.ready() {
		log.Debugf("cache is not fully constructed to produce a first snapshot yet")
		return
	}

	aggregatedEndpoints := make(endpointdiscovery.EndpointGroups)
	for _, endpointGroups := range cache.endpoints {
		for upstreamName, endpointSet := range endpointGroups {
			aggregatedEndpoints[upstreamName] = endpointSet
		}
	}
	snapshot, reports, err := e.translator.Translate(translator.Inputs{
		Cfg:       cache.cfg,
		Secrets:   cache.secrets,
		Files:     cache.files,
		Endpoints: aggregatedEndpoints,
	})
	if err != nil {
		// TODO: panic or handle these internal errors smartly
		log.Warnf("failed to translate based on the latest config: %v", err)
		return
	}

	if err := e.reporter.WriteReports(reports); err != nil {
		log.Warnf("error writing reports: %v", err)
	}

	for _, st := range reports {
		if st.Err != nil {
			log.Warnf("user config error: %v: %v", st.CfgObject.GetName(), st.Err)
		}
	}

	log.Debugf("FINAL: XDS Snapshot: %v", snapshot)
	e.xdsConfig.SetSnapshot(xds.NodeKey, *snapshot)
}

// fan out to cover all endpoint discovery services
func (e *eventLoop) endpointDiscovery() <-chan endpointTuple {
	aggregatedEndpointsChan := make(chan endpointTuple)
	for _, ed := range e.endpointDiscoveries {
		go func() {
			for endpoints := range ed.Endpoints() {
				aggregatedEndpointsChan <- endpointTuple{
					endpoints:    endpoints,
					discoveredBy: ed,
				}
			}
		}()
	}
	return aggregatedEndpointsChan
}

// fan out to cover all channels that return errors
func (e *eventLoop) errors() <-chan error {
	aggregatedErrorsChan := make(chan error)
	go func() {
		for err := range e.configWatcher.Error() {
			aggregatedErrorsChan <- errors.Wrap(err, "config watcher encountered an error")
		}
	}()
	go func() {
		for err := range e.secretWatcher.Error() {
			aggregatedErrorsChan <- errors.Wrap(err, "secret watcher encountered an error")
		}
	}()
	go func() {
		for err := range e.fileWatcher.Error() {
			aggregatedErrorsChan <- errors.Wrap(err, "file watcher encountered an error")
		}
	}()
	for _, ed := range e.endpointDiscoveries {
		go func() {
			for err := range ed.Error() {
				aggregatedErrorsChan <- err
			}
		}()
	}
	return aggregatedErrorsChan
}

// cache contains the latest "gloo snapshot"
type cache struct {
	cfg     *v1.Config
	secrets secretwatcher.SecretMap
	files   filewatcher.Files
	// need to separate endpoints by the service who discovered them
	endpoints map[endpointdiscovery.Interface]endpointdiscovery.EndpointGroups
}

func newCache() *cache {
	return &cache{
		endpoints: make(map[endpointdiscovery.Interface]endpointdiscovery.EndpointGroups),
	}
}

// ready doesn't necessarily tell us whetehr endpoints have been discovered yet
// but that's okay. envoy won't mind
func (c *cache) ready() bool {
	return c.cfg != nil
}

func (c *cache) hash() uint64 {
	h0, err := hashstructure.Hash(*c.cfg, nil)
	if err != nil {
		panic(err)
	}
	h1, err := hashstructure.Hash(c.secrets, nil)
	if err != nil {
		panic(err)
	}
	h2, err := hashstructure.Hash(c.endpoints, nil)
	if err != nil {
		panic(err)
	}
	h := h0 + h1 + h2
	return h
}

type endpointTuple struct {
	discoveredBy endpointdiscovery.Interface
	endpoints    endpointdiscovery.EndpointGroups
}
