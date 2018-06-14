package eventloop

import (
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"

	"sort"

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

func rolesByName(roles []*v1.Role, virtualServices []*v1.VirtualService) map[string]*v1.Role {
	rolesByName := make(map[string]*v1.Role)
	for _, r := range roles {
		rolesByName[r.Name] = r
	}
	// create default if it doesn't exist
	if _, ok := rolesByName[defaultRole]; !ok {
		role := &v1.Role{
			Name: defaultRole,
			Metadata: &v1.Metadata{
				Annotations: map[string]string{"generated_by": "control-plane"},
			},
		}
		rolesByName[defaultRole] = role
	}
	// create any other roles that dont exist
	for _, vs := range virtualServices {
		for _, roleName := range vs.Roles {
			if _, ok := rolesByName[roleName]; !ok {
				role := &v1.Role{
					Name: roleName,
					Metadata: &v1.Metadata{
						Annotations: map[string]string{"generated_by": "control-plane"},
					},
				}
				rolesByName[roleName] = role
			}
		}
	}
	return rolesByName
}

func virtualServicesByRole(roles []*v1.Role, virtualServices []*v1.VirtualService) map[*v1.Role][]*v1.VirtualService {
	// clear prior virtual service ownership list
	for _, r := range roles {
		r.VirtualServices = nil
	}

	// get existing roles and create ones that don't exist yet
	rolesByName := rolesByName(roles, virtualServices)

	virtualServicesByRole := make(map[*v1.Role][]*v1.VirtualService)
	for _, vs := range virtualServices {
		if len(vs.Roles) == 0 {
			role := rolesByName[defaultRole]
			role.VirtualServices = append(role.VirtualServices, vs.Name)
			virtualServicesByRole[role] = append(virtualServicesByRole[role], vs)
		}
		for _, roleName := range vs.Roles {
			role := rolesByName[roleName]
			role.VirtualServices = append(role.VirtualServices, vs.Name)
			virtualServicesByRole[role] = append(virtualServicesByRole[role], vs)
		}
	}

	// sort the virtualservices on the roles; idempotency
	for role := range virtualServicesByRole {
		sort.SliceStable(role.VirtualServices, func(i, j int) bool {
			return role.VirtualServices[i] < role.VirtualServices[j]
		})
	}

	return virtualServicesByRole
}

func (e *eventLoop) updateXds(snap *snapshot.Cache) {
	if !snap.Ready() {
		log.Debugf("snapshot is not ready for translation yet")
		return
	}

	// map each virtual service to one or more roles
	// if no roles are defined, we fall back to the default Role, which is 'ingress'
	virtualServicesByRole := virtualServicesByRole(snap.Cfg.Roles, snap.Cfg.VirtualServices)

	// aggregate reports across all the roles
	allReports := make(map[string]reporter.ConfigObjectReport)

	// translate each set of resources (grouped by role) individually
	// and set the snapshot for that role
	for role, virtualServices := range virtualServicesByRole {
		if len(virtualServices) == 0 {
			log.Printf("nothing to do yet for role %v", role)
			continue
		}

		// get only the upstreams required for these virtual services
		upstreams := destinationUpstreams(snap.Cfg.Upstreams, virtualServices)
		endpoints := destinationEndpoints(upstreams, snap.Endpoints)
		roleSnapshot := &snapshot.Cache{
			Cfg: &v1.Config{
				Upstreams:       upstreams,
				VirtualServices: virtualServices,
			},
			Secrets:   snap.Secrets,
			Files:     snap.Files,
			Endpoints: endpoints,
		}

		log.Debugf("\nRole: %v\nGloo Snapshot (%v): %v", role, snap.Hash(), snap)

		// create new role object
		// this will be used to store the report for role-level errors
		var vsNames []string
		for _, vs := range virtualServices {
			vsNames = append(vsNames, vs.Name)
		}

		xdsSnapshot, reports, err := e.translator.Translate(role, roleSnapshot)
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

func destinationEndpoints(upstreams []*v1.Upstream, allEndpoints endpointdiscovery.EndpointGroups) endpointdiscovery.EndpointGroups {
	destinationEndpoints := make(endpointdiscovery.EndpointGroups)
	for _, us := range upstreams {
		eps, ok := allEndpoints[us.Name]
		if !ok {
			continue
		}
		destinationEndpoints[us.Name] = eps
	}
	return destinationEndpoints
}
