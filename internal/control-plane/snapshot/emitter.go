package snapshot

import (
	"github.com/solo-io/gloo/internal/control-plane/configwatcher"
	"github.com/solo-io/gloo/pkg/secretwatcher"
	"github.com/solo-io/gloo/internal/control-plane/filewatcher"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/pkg/errors"
)

// the snapshot emitter wraps various config sources
// and emits a new snapshot when any element in the config updates
type Emitter struct {
	configWatcher    configwatcher.Interface
	secretWatcher    secretwatcher.Interface
	fileWatcher      filewatcher.Interface
	endpointsWatcher endpointdiscovery.Interface

	getDependencies func(cfg *v1.Config) []*plugins.Dependencies
	snapshots       chan *Cache
	errors          chan error
}

func NewEmitter(configWatcher configwatcher.Interface,
	secretWatcher secretwatcher.Interface,
	fileWatcher filewatcher.Interface,
	endpointsWatcher endpointdiscovery.Interface,
	getDependencies func(cfg *v1.Config) []*plugins.Dependencies) *Emitter {

	return &Emitter{
		configWatcher:    configWatcher,
		secretWatcher:    secretWatcher,
		fileWatcher:      fileWatcher,
		endpointsWatcher: endpointsWatcher,

		getDependencies: getDependencies,
		snapshots:       make(chan *Cache),
	}
}

func (e *Emitter) Snapshot() <-chan *Cache {
	return e.snapshots
}

func (e *Emitter) Run(stop <-chan struct{}) {
	go e.configWatcher.Run(stop)
	go e.fileWatcher.Run(stop)
	go e.secretWatcher.Run(stop)
	go e.endpointsWatcher.Run(stop)
	e.errors = e.aggregateErrors()

	latest := newCache()
	for {
		select {
		case <-stop:
			return
		case cfg := <-e.configWatcher.Config():
			log.Printf("change triggered by config")
			latest.Cfg = cfg

			// update everything we're tracking
			dependencies := e.getDependencies(cfg)
			var secretRefs, fileRefs []string
			for _, dep := range dependencies {
				secretRefs = append(secretRefs, dep.SecretRefs...)
				fileRefs = append(fileRefs, dep.FileRefs...)
			}
			// secrets for virtual services
			for _, vService := range cfg.VirtualServices {
				if vService.SslConfig != nil && vService.SslConfig.SslSecrets != nil {
					secretRef, ok := vService.SslConfig.SslSecrets.(*v1.SSLConfig_SecretRef)
					if !ok {
						panic("ssl files are not currenty supported for vservices")
					}
					secretRefs = append(secretRefs, secretRef.SecretRef)
				}
			}
			go e.secretWatcher.TrackSecrets(secretRefs)
			go e.fileWatcher.TrackFiles(fileRefs)
			go e.endpointsWatcher.TrackUpstreams(cfg.Upstreams)

			e.snapshots <- latest
		case secrets := <-e.secretWatcher.Secrets():
			log.Debugf("change triggered by secrets")
			latest.Secrets = secrets
			e.snapshots <- latest
		case files := <-e.fileWatcher.Files():
			log.Debugf("change triggered by files")
			latest.Files = files
			e.snapshots <- latest
		case endpoints := <-e.endpointsWatcher.Endpoints():
			log.Debugf("change triggered by endpoints")
			latest.Endpoints = endpoints
			e.snapshots <- latest
		}
	}
}

// fan out to cover all channels that return errors
func (e *Emitter) Error() <-chan error {
	return e.errors
}

func (e *Emitter) aggregateErrors() chan error {
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
	go func() {
		for err := range e.endpointsWatcher.Error() {
			aggregatedErrorsChan <- errors.Wrap(err, "endpoints watcher encountered an error")
		}
	}()
	return aggregatedErrorsChan
}
