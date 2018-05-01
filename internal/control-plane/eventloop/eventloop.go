package eventloop

import (
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/mitchellh/hashstructure"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/internal/control-plane/bootstrap"
	"github.com/solo-io/gloo/internal/control-plane/configwatcher"
	"github.com/solo-io/gloo/internal/control-plane/filewatcher"
	"github.com/solo-io/gloo/internal/control-plane/reporter"
	"github.com/solo-io/gloo/internal/control-plane/translator"
	"github.com/solo-io/gloo/internal/control-plane/xds"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/bootstrap/artifactstorage"
	"github.com/solo-io/gloo/pkg/bootstrap/configstorage"
	secretwatchersetup "github.com/solo-io/gloo/pkg/bootstrap/secretwatcher"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/secretwatcher"
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

func Setup(opts bootstrap.Options, xdsPort int, stop <-chan struct{}) (*eventLoop, error) {
	store, err := configstorage.Bootstrap(opts.Options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create config store client")
	}

	cfgWatcher, err := configwatcher.NewConfigWatcher(store)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create config watcher")
	}

	secretWatcher, err := secretwatchersetup.Bootstrap(opts.Options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up secret watcher")
	}

	fileWatcher, err := setupFileWatcher(opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up file watcher")
	}

	// create a snapshot to give to misconfigured envoy instances
	badNodeSnapshot := xds.BadNodeSnapshot(opts.IngressOptions.BindAddress, opts.IngressOptions.Port)

	xdsConfig, _, err := xds.RunXDS(xdsPort, badNodeSnapshot)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start xds server")
	}

	plugs := plugins.RegisteredPlugins()

	trans := translator.NewTranslator(opts.IngressOptions, plugs)

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
		startfunc := func(edi plugins.EndpointDiscoveryInitFunc) func() error {
			return func() error {
				discovery, err := edi(opts.Options)
				if err != nil {
					log.Warnf("Starting endpoint discovery failed: %v, endpoints will not be discovered for this "+
						"upstream type", err)
					return nil
				}
				e.endpointDiscoveries = append(e.endpointDiscoveries, discovery)
				return nil
			}
		}(endpointDiscoveryInitializer)
		e.startFuncs = append(e.startFuncs, startfunc)
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

func setupFileWatcher(opts bootstrap.Options) (filewatcher.Interface, error) {
	store, err := artifactstorage.Bootstrap(opts.Options)
	if err != nil {
		return nil, errors.Wrap(err, "creating file storage client")
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
	go e.fileWatcher.Run(stop)
	go e.secretWatcher.Run(stop)

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
			// secrets for virtualservices
			for _, vService := range cfg.VirtualServices {
				if vService.SslConfig != nil && vService.SslConfig.SecretRef != "" {
					secretRefs = append(secretRefs, vService.SslConfig.SecretRef)
				}
			}
			go e.secretWatcher.TrackSecrets(secretRefs)
			go e.fileWatcher.TrackFiles(fileRefs)
			for _, discovery := range e.endpointDiscoveries {
				go func(epd endpointdiscovery.Interface) {
					epd.TrackUpstreams(cfg.Upstreams)
				}(discovery)
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

	log.Debugf("Setting xDS Snapshot for Role %v: %v", "ingress", snapshot)
	e.xdsConfig.SetSnapshot("ingress", *snapshot)
}

// fan out to cover all endpoint discovery services
func (e *eventLoop) endpointDiscovery() <-chan endpointTuple {
	aggregatedEndpointsChan := make(chan endpointTuple)
	for _, ed := range e.endpointDiscoveries {
		go func(endpointDisc endpointdiscovery.Interface) {
			for endpoints := range endpointDisc.Endpoints() {
				aggregatedEndpointsChan <- endpointTuple{
					endpoints:    endpoints,
					discoveredBy: endpointDisc,
				}
			}
		}(ed)
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
				aggregatedErrorsChan <-  errors.Wrap(err, "endpoint discovery encountered an error")
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
	h3, err := hashstructure.Hash(c.files, nil)
	if err != nil {
		panic(err)
	}
	h := h0 + h1 + h2 + h3
	return h
}

type endpointTuple struct {
	discoveredBy endpointdiscovery.Interface
	endpoints    endpointdiscovery.EndpointGroups
}
