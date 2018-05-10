package eventloop

import (
	"time"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/pkg/errors"
	// "github.com/solo-io/gloo/pkg/endpointdiscovery"

	"github.com/solo-io/gloo/internal/control-plane/bootstrap"
	"github.com/solo-io/gloo/internal/control-plane/configwatcher"
	"github.com/solo-io/gloo/internal/control-plane/endpointswatcher"
	"github.com/solo-io/gloo/internal/control-plane/filewatcher"
	"github.com/solo-io/gloo/internal/control-plane/reporter"
	"github.com/solo-io/gloo/internal/control-plane/snapshot"
	"github.com/solo-io/gloo/internal/control-plane/translator"
	"github.com/solo-io/gloo/internal/control-plane/xds"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/bootstrap/artifactstorage"
	"github.com/solo-io/gloo/pkg/bootstrap/configstorage"
	secretwatchersetup "github.com/solo-io/gloo/pkg/bootstrap/secretwatcher"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
)

const defaultRole = "ingress"

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

	// map each virtual service to one or more roles
	// if no roles are defined, we fall back to the default Role, which is 'ingress'
	virtualServicesByRole := make(map[string][]*v1.VirtualService)
	for _, vs := range snap.Cfg.VirtualServices {
		if len(vs.Roles) == 0 {
			virtualServicesByRole[defaultRole] = append(virtualServicesByRole[defaultRole], vs)
		}
		for _, role := range vs.Roles {
			virtualServicesByRole[role] = append(virtualServicesByRole[role], vs)
		}
	}

	// translate each set of resources (grouped by role) individually
	// and set the snapshot for that role
	for role, virtualServices := range virtualServicesByRole {
		if len(virtualServices) == 0 {
			log.Printf("nothing to do yet for role %v", role)
			continue
		}

		// get only the upstreams required for these virtual services
		upstreams := destinationUpstreams(snap.Cfg.Upstreams, virtualServices)
		roleSnapshot := &snapshot.Cache{
			Cfg: &v1.Config{
				Upstreams:       upstreams,
				VirtualServices: virtualServices,
			},
			Secrets:   snap.Secrets,
			Files:     snap.Files,
			Endpoints: snap.Endpoints,
		}

		log.Debugf("\nRole: %v\nGloo Snapshot (%v): %v", role, snap.Hash(), snap)

		xdsSnapshot, reports, err := e.translator.Translate(roleSnapshot)
		if err != nil {
			// TODO: panic or handle these internal errors smartly
			log.Warnf("failed to translate for role %v: %v", role, err)
			return
		}

		var upstreamReports []reporter.ConfigObjectReport
		var virtualServiceReports []reporter.ConfigObjectReport

		var invalidConfig bool

		for _, rep := range reports {
			if rep.Err != nil {
				invalidConfig = true
				log.Warnf("user config error: %v: %v", rep.CfgObject.GetName(), rep.Err.Error())
			}
			switch rep.CfgObject.(type) {
			case *v1.Upstream:
				upstreamReports = append(upstreamReports, rep)
			case *v1.VirtualService:
				virtualServiceReports = append(virtualServiceReports, rep)
			}
		}

		if err := e.reporter.WriteRoleReports(role, virtualServiceReports); err != nil {
			log.Warnf("error writing reports: %v", err)
		}

		// TODO: ensure that upstream reports always come out the same
		// otherwise we may want to move this out to a single iteration after all
		// the translations finish
		// we can also optimize translation by eliminating errored upstreams from the
		// cache the first time they are errored
		if err := e.reporter.WriteGlobalReports(upstreamReports); err != nil {
			log.Warnf("error writing reports: %v", err)
		}

		if invalidConfig {
			log.Warnf("skipping xds update for snapshot version %v, check error reports", xdsSnapshot.GetVersion(envoycache.RouteType))
			continue
		}

		log.Debugf("Setting xDS Snapshot for Role %v: %v", role, xdsSnapshot)
		e.xdsConfig.SetSnapshot(role, *xdsSnapshot)
	}
	time.Sleep(time.Second)
}

// gets the subset of upstreams which are destinations for at least one route in at least one
// virtual service
func destinationUpstreams(allUpstreams []*v1.Upstream, virtualServices []*v1.VirtualService) []*v1.Upstream {
	destinationUpstreamNames := make(map[string]bool)
	for _, vs := range virtualServices {
		for _, route := range vs.Routes {
			dests := getAllDestinations(route)
			for _, dest := range dests {
				var upstreamName string
				switch typedDest := dest.DestinationType.(type) {
				case *v1.Destination_Upstream:
					upstreamName = typedDest.Upstream.Name
				case *v1.Destination_Function:
					upstreamName = typedDest.Function.UpstreamName
				default:
					panic("unknown destination type")
				}
				destinationUpstreamNames[upstreamName] = true
			}
		}
	}
	var destinationUpstreams []*v1.Upstream
	for _, us := range allUpstreams {
		if _, ok := destinationUpstreamNames[us.Name]; ok {
			destinationUpstreams = append(destinationUpstreams, us)
		}
	}
	return destinationUpstreams
}

func getAllDestinations(route *v1.Route) []*v1.Destination {
	var dests []*v1.Destination
	if route.SingleDestination != nil {
		dests = append(dests, route.SingleDestination)
	}
	for _, dest := range route.MultipleDestinations {
		dests = append(dests, dest.Destination)
	}
	return dests
}
