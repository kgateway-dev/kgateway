package eventloop

import (
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/internal/control-plane/bootstrap"
	"github.com/solo-io/gloo/internal/control-plane/configwatcher"
	"github.com/solo-io/gloo/internal/control-plane/endpointswatcher"
	"github.com/solo-io/gloo/internal/control-plane/filewatcher"
	"github.com/solo-io/gloo/internal/control-plane/reporter"
	"github.com/solo-io/gloo/internal/control-plane/snapshot"
	"github.com/solo-io/gloo/internal/control-plane/translator"
	"github.com/solo-io/gloo/internal/control-plane/xds"
	defaultv1 "github.com/solo-io/gloo/pkg/api/defaults/v1"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/bootstrap/artifactstorage"
	"github.com/solo-io/gloo/pkg/bootstrap/configstorage"
	secretwatchersetup "github.com/solo-io/gloo/pkg/bootstrap/secretwatcher"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
)

type eventLoop struct {
	snapshotEmitter *snapshot.Emitter
	reporter        reporter.Interface
	translator      *translator.Translator
	xdsConfig       envoycache.SnapshotCache
	opts            bootstrap.IngressOptions
}

func Setup(opts bootstrap.Options, xdsPort int) (*eventLoop, error) {
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

	trans := translator.NewTranslator(plugs)

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
		opts:            opts.IngressOptions,
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

func (e *eventLoop) Run(stop <-chan struct{}) {
	go e.snapshotEmitter.Run(stop)

	// cache the most recent read for any of these
	var oldHash uint64
	for {
		select {
		case <-stop:
			log.Printf("event loop shutting down")
			return
		case snap := <-e.snapshotEmitter.Snapshot():
			newHash := snap.Hash()
			log.Printf("\nold hash: %v\nnew hash: %v", oldHash, newHash)
			if newHash == oldHash {
				continue
			}
			log.Debugf("new snapshot received")
			oldHash = newHash
			e.updateXds(snap)
		case err := <-e.snapshotEmitter.Error():
			log.Warnf("error in control plane event loop: %v", err)
		}
	}
}

func (e *eventLoop) updateXds(snap *snapshot.Cache) {
	if !snap.Ready() {
		log.Debugf("snapshot is not ready for translation yet")
		return
	}

	// aggregate reports across all the roles
	allReports := make(map[string]reporter.ConfigObjectReport)

	roles := snap.Cfg.Roles

	var gatewayRole *v1.Role
	for i, role := range roles {
		if role.Name == defaultv1.GatewayRoleName {
			gatewayRole = role
			if len(gatewayRole.Listeners) != 2 {
				gatewayRole = defaultv1.GatewayRole(e.opts.BindAddress, e.opts.Port, e.opts.SecurePort)
				roles[i] = gatewayRole
			}
			break
		}
	}

	// if the gateway role is not predefined, create it
	if gatewayRole == nil {
		gatewayRole = defaultv1.GatewayRole(e.opts.BindAddress, e.opts.Port, e.opts.SecurePort)
		roles = append(roles, gatewayRole)
	}

	// ensure the gateway role contains the latest
	defaultv1.AssignGatewayVirtualServices(gatewayRole.Listeners[0], gatewayRole.Listeners[1], snap.Cfg.VirtualServices)

	// translate each set of resources (grouped by role) individually
	// and set the snapshot for that role
	for _, role := range roles {
		//// get only the upstreams required for these virtual services
		//upstreams := destinationUpstreams(snap.Cfg.Upstreams, virtualServices)
		//endpoints := destinationEndpoints(upstreams, snap.Endpoints)
		//roleSnapshot := &snapshot.Cache{
		//	Cfg: &v1.Config{
		//		Upstreams:       upstreams,
		//		VirtualServices: virtualServices,
		//	},
		//	Secrets:   snap.Secrets,
		//	Files:     snap.Files,
		//	Endpoints: endpoints,
		//}

		log.Debugf("\nRole: %v\nGloo Snapshot (%v): %v", role.Name, snap.Hash(), snap)

		xdsSnapshot, reports, err := e.translator.Translate(role, snap)
		if err != nil {
			// TODO: panic or handle these internal errors smartly
			log.Warnf("INTERNAL ERROR: failed to run translator for role %v: %v", role, err)
			continue
		}

		var roleRejected bool
		// merge reports them together
		for _, rep := range reports {
			allReports[rep.CfgObject.GetName()] = rep

			if rep.Err != nil {
				log.Warnf("user config in role %v failed with err %v", role, rep.Err.Error())
				roleRejected = true
			}
		}

		if roleRejected {
			log.Warnf("role %v rejected", role)
			continue
		}

		log.Debugf("Setting xDS Snapshot for Role %v: %v", role, xdsSnapshot)
		e.xdsConfig.SetSnapshot(role.Name, *xdsSnapshot)
	}

	var reports []reporter.ConfigObjectReport
	for _, rep := range allReports {
		reports = append(reports, rep)
	}

	if err := e.reporter.WriteReports(reports); err != nil {
		log.Warnf("error writing reports: %v", err)
	}
}

