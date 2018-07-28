package eventloop

import (
	"net"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/pkg/errors"
	defaultv1 "github.com/solo-io/gloo/pkg/api/defaults/v1"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/bootstrap/artifactstorage"
	"github.com/solo-io/gloo/pkg/bootstrap/configstorage"
	"github.com/solo-io/gloo/pkg/bootstrap/secretstorage"
	"github.com/solo-io/gloo/pkg/control-plane/bootstrap"
	"github.com/solo-io/gloo/pkg/control-plane/configwatcher"
	"github.com/solo-io/gloo/pkg/control-plane/endpointswatcher"
	"github.com/solo-io/gloo/pkg/control-plane/filewatcher"
	"github.com/solo-io/gloo/pkg/control-plane/reporter"
	"github.com/solo-io/gloo/pkg/control-plane/snapshot"
	"github.com/solo-io/gloo/pkg/control-plane/translator"
	"github.com/solo-io/gloo/pkg/control-plane/xds"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/secretwatcher"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

// Config for the event loop. The event loop receives events from various sources, and triggers regeneration
// of and xDS snapshot via the *translator.Translator.
type Config struct {
	// Configuration options for endpoint discovery and xDS
	Options bootstrap.Options
	// Storage to read upstreams and vhosts, and to write reports
	Store storage.Interface
	// Secrets to watch for changes
	Secrets dependencies.SecretStorage
	// Config files to watch for changes
	Files dependencies.FileStorage
	// The address the xDS server should bind to
	XdsBindAddress net.Addr
}

type EventLoop interface {
	Run(stop <-chan struct{})
}

type eventLoop struct {
	snapshotEmitter *snapshot.Emitter
	reporter        reporter.Interface
	translator      *translator.Translator
	xdsConfig       envoycache.SnapshotCache
	opts            bootstrap.IngressOptions
}

func Setup(opts bootstrap.Options, xdsPort int) (EventLoop, error) {
	store, err := configstorage.Bootstrap(opts.Options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create config store client")
	}

	secrets, err := secretstorage.Bootstrap(opts.Options)
	if err != nil {
		return nil, errors.Wrap(err, "creating secret storage client")
	}

	files, err := artifactstorage.Bootstrap(opts.Options)
	if err != nil {
		return nil, errors.Wrap(err, "creating file storage client")
	}
	cfg := Config{
		Options:        opts,
		Store:          store,
		Secrets:        secrets,
		Files:          files,
		XdsBindAddress: &net.TCPAddr{Port: xdsPort},
	}

	return SetupWithConfig(cfg)
}

func SetupWithConfig(cfg Config) (EventLoop, error) {
	cfgWatcher, err := configwatcher.NewConfigWatcher(cfg.Store)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create config watcher")
	}

	secretWatcher, err := secretwatcher.NewSecretWatcher(cfg.Secrets)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create config watcher")
	}

	fileWatcher, err := filewatcher.NewFileWatcher(cfg.Files)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up file watcher")
	}

	plugs := plugins.RegisteredPlugins()

	var edPlugins []plugins.EndpointDiscoveryPlugin
	for _, plug := range plugs {

		// initialize the plugin
		if err := plug.Init(cfg.Options.Options); err == nil {
			if edp, ok := plug.(plugins.EndpointDiscoveryPlugin); ok {
				edPlugins = append(edPlugins, edp)
			}
		} else {
			log.Warnf("Error initializing plugin: %v", err)
		}
	}

	endpointsWatcher := endpointswatcher.NewEndpointsWatcher(edPlugins...)

	snapshotEmitter := snapshot.NewEmitter(
		cfgWatcher,
		secretWatcher,
		fileWatcher,
		endpointsWatcher,
		getDependenciesFor(plugs),
	)

	trans := translator.NewTranslator(plugs)

	// create a snapshot to give to misconfigured envoy instances
	badNodeSnapshot := xds.BadNodeSnapshot(cfg.Options.IngressOptions.BindAddress, cfg.Options.IngressOptions.Port)

	var callbacks plugins.XdsCallbacks
	for _, plug := range plugs {
		if xdsPlug, ok := plug.(plugins.XdsPlugin); ok {
			callbacks = append(callbacks, xdsPlug.Callbacks())
		}
	}

	xdsConfig, _, err := xds.RunXDS(cfg.XdsBindAddress, badNodeSnapshot, callbacks)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start xds server")
	}

	e := &eventLoop{
		snapshotEmitter: snapshotEmitter,
		translator:      trans,
		xdsConfig:       xdsConfig,
		reporter:        reporter.NewReporter(cfg.Store),
		opts:            cfg.Options.IngressOptions,
	}

	return e, nil
}

func getDependenciesFor(translatorPlugins []plugins.TranslatorPlugin) func(cfg *v1.Config) []*plugins.Dependencies {
	return func(cfg *v1.Config) []*plugins.Dependencies {
		var dependencies []*plugins.Dependencies
		// secrets plugins need
		for _, plug := range translatorPlugins {
			if dependentPlugin, ok := plug.(plugins.PluginWithDependencies); ok {
				dep := dependentPlugin.GetDependencies(cfg)
				if dep != nil {
					dependencies = append(dependencies, dep)
				}
			}
		}
		return dependencies
	}
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
		log.Debugf("\nRole: %v\nGloo Snapshot (%v): %v", role.Name, snap.Hash(), snap)

		xdsSnapshot, reports := e.translator.Translate(role, snap)

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
