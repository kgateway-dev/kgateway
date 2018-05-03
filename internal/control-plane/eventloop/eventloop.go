package eventloop

import (
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
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
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/internal/control-plane/endpointswatcher"
	"github.com/solo-io/gloo/internal/control-plane/snapshot"
)

type eventLoop struct {
	snapshotEmitter *snapshot.Emitter
	reporter        reporter.Interface
	translator      *translator.Translator
	xdsConfig       envoycache.SnapshotCache
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

	plugs := plugins.RegisteredPlugins()

	var edPlugins []plugins.EndpointDiscoveryPlugin
	for _, plug := range plugs {
		if edp, ok := plug.(plugins.EndpointDiscoveryPlugin); ok {
			edPlugins = append(edPlugins, edp)
		}
	}

	endpointsWatcher := endpointswatcher.NewEndpointsWatcher(opts.Options, edPlugins...)

	snapshotEmitter := snapshot.NewEmitter(cfgWatcher, secretWatcher,
		fileWatcher, endpointsWatcher, getDependenciesFor(plugs))

	trans := translator.NewTranslator(opts.IngressOptions, plugs)

	// create a snapshot to give to misconfigured envoy instances
	badNodeSnapshot := xds.BadNodeSnapshot(opts.IngressOptions.BindAddress, opts.IngressOptions.Port)

	xdsConfig, _, err := xds.RunXDS(xdsPort, badNodeSnapshot)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start xds server")
	}

	e := &eventLoop{
		snapshotEmitter: snapshotEmitter,
		translator:      trans,
		xdsConfig:       xdsConfig,
		reporter:        reporter.NewReporter(store),
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
			go e.endpointsWatcher.TrackUpstreams(cfg.Upstreams)

			sync(current)
		case secrets := <-e.secretWatcher.Secrets():
			log.Debugf("change triggered by secrets")
			current.secrets = secrets
			sync(current)
		case files := <-e.fileWatcher.Files():
			log.Debugf("change triggered by files")
			current.files = files
			sync(current)
		case endpoints := <-e.endpointsWatcher.Endpoints():
			log.Debugf("change triggered by endpoints")
			current.endpoints = endpoints
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

	snapshot, reports, err := e.translator.Translate(translator.Inputs{
		Cfg:       cache.cfg,
		Secrets:   cache.secrets,
		Files:     cache.files,
		Endpoints: cache.endpoints,
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
