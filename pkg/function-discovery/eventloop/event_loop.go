package eventloop

import (
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"

	"github.com/hashicorp/consul/api"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/bootstrap/artifactstorage"
	"github.com/solo-io/gloo/pkg/bootstrap/configstorage"
	"github.com/solo-io/gloo/pkg/bootstrap/secretstorage"
	secretwatchersetup "github.com/solo-io/gloo/pkg/bootstrap/secretwatcher"
	"github.com/solo-io/gloo/pkg/function-discovery/detector"
	"github.com/solo-io/gloo/pkg/function-discovery/fission"
	"github.com/solo-io/gloo/pkg/function-discovery/grpc"
	"github.com/solo-io/gloo/pkg/function-discovery/nats-streaming"
	"github.com/solo-io/gloo/pkg/function-discovery/openfaas"
	"github.com/solo-io/gloo/pkg/function-discovery/options"
	"github.com/solo-io/gloo/pkg/function-discovery/projectfn"
	"github.com/solo-io/gloo/pkg/function-discovery/resolver"
	"github.com/solo-io/gloo/pkg/function-discovery/swagger"
	"github.com/solo-io/gloo/pkg/function-discovery/updater"
	"github.com/solo-io/gloo/pkg/function-discovery/upstreamwatcher"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/secretwatcher"
	kubeutils "github.com/solo-io/gloo/pkg/utils/kube"
)

const (
	maxThreadsPerUpstream = 25
)

type workItem struct {
	upstream *v1.Upstream
	secrets  secretwatcher.SecretMap
}

func Run(opts bootstrap.Options, discoveryOpts options.DiscoveryOptions, stop <-chan struct{}, errs chan error) error {
	store, err := configstorage.Bootstrap(opts)
	if err != nil {
		return errors.Wrap(err, "failed to create config store client")
	}

	upstreams, err := upstreamwatcher.WatchUpstreams(store, stop, errs)
	if err != nil {
		return errors.Wrap(err, "failed to start monitoring upstreams")
	}

	secretWatcher, err := secretwatchersetup.Bootstrap(opts)
	if err != nil {
		return errors.Wrap(err, "failed to set up secret watcher")
	}

	secretStore, err := secretstorage.Bootstrap(opts)
	if err != nil {
		return errors.Wrap(err, "failed to set up secret storage client")
	}

	files, err := artifactstorage.Bootstrap(opts)
	if err != nil {
		return errors.Wrap(err, "creating file storage client")
	}

	go secretWatcher.Run(stop)

	resolve := createResolver(opts)

	var detectors []detector.Interface
	if discoveryOpts.AutoDiscoverNATS {
		//TODO: support cluster ids
		detectors = append(detectors, nats.NewNatsDetector(""))
	}

	if discoveryOpts.AutoDiscoverFaaS {
		detectors = append(detectors, openfaas.NewFaaSDetector())
	}
	if discoveryOpts.AutoDiscoverProjectFn {
		detectors = append(detectors, projectfn.NewProjectFnDetector())
	}
	if discoveryOpts.AutoDiscoverFission {
		detectors = append(detectors, fission.NewFissionDetector())
	}

	if discoveryOpts.AutoDiscoverSwagger {
		detectors = append(detectors, swagger.NewSwaggerDetector(discoveryOpts.SwaggerUrisToTry))
	}
	if discoveryOpts.AutoDiscoverGRPC {
		detectors = append(detectors, grpc.NewGRPCDetector(files))
	}

	marker := detector.NewMarker(detectors, resolve)

	var cache struct {
		secrets   secretwatcher.SecretMap
		upstreams []*v1.Upstream
	}

	workQueues := make(map[string]chan *workItem)

	updateUpstream := func(us *v1.Upstream, secrets secretwatcher.SecretMap) {
		log.Debugf("attempting update for %v", us.Name)
		if err := updater.UpdateServiceInfo(store, us.Name, marker); err != nil {
			go func() {
				errs <- errors.Wrapf(err, "updating upstream %v", us.Name)
			}()
		}
		if err := updater.UpdateFunctions(resolve, store, secretStore, files, us.Name, secrets); err != nil {
			go func() {
				errs <- errors.Wrapf(err, "updating upstream %v", us.Name)
			}()
		}
	}

	update := func() {
		var usNames []string
		for _, us := range cache.upstreams {
			usNames = append(usNames, us.Name)
		}
		log.Debugf("beginning update for %v upstreams: %v", usNames, len(cache.upstreams))

		// clean queues for upstreams that have been deleted
		for usName := range workQueues {
			var upstreamFound bool
			for _, us := range cache.upstreams {
				if usName == us.Name {
					upstreamFound = true
					break
				}
			}
			if !upstreamFound {
				close(workQueues[usName])
				delete(workQueues, usName)
			}
		}

		// updating secret refs can happen async
		// if new secrets come in, it will trigger a new update
		go func(upstreams []*v1.Upstream) {
			// update secret refs on secret watcher
			refs := updater.GetSecretRefsToWatch(upstreams)
			secretWatcher.TrackSecrets(refs)
		}(cache.upstreams)

		for _, us := range cache.upstreams {
			_, ok := workQueues[us.Name]
			if !ok {
				workQueues[us.Name] = make(chan *workItem, maxThreadsPerUpstream)
				// start worker thread for this upstream
				go func(workQueues map[string]chan *workItem, usName string) {
					log.Debugf("starting goroutine for %s", usName)
					// allow upstream time to start up
					time.Sleep(time.Second * 2)
					for work := range workQueues[usName] {
						updateUpstream(work.upstream, work.secrets)
					}
					log.Debugf("exiting goroutine for %s", usName)
				}(workQueues, us.Name)
			}
			workQueues[us.Name] <- &workItem{upstream: us, secrets: cache.secrets}
		}
	}

	ticker := time.NewTicker(opts.ConfigStorageOptions.SyncFrequency)
	defer ticker.Stop()
	for {
		select {
		case cache.secrets = <-secretWatcher.Secrets():
			update()
		case cache.upstreams = <-upstreams:
			update()
		case <-ticker.C:
			update()
		case err := <-secretWatcher.Error():
			errs <- err
		case <-stop:
			return nil
		}
	}
}

func createResolver(opts bootstrap.Options) resolver.Resolver {
	kube, err := func() (kubernetes.Interface, error) {
		cfg, err := kubeutils.GetConfig(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
		if err != nil {
			return nil, err
		}
		kube, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return nil, err
		}
		return kube, nil
	}()
	if err != nil {
		log.Warnf("create kube client failed: %v. functonal services running in kubernetes will not be discovered " +
			"by function discovery")
	}
	consul, err := func() (*api.Client, error) {
		cfg := opts.ConsulOptions.ToConsulConfig()
		return api.NewClient(cfg)
	}()
	if err != nil {
		log.Warnf("create consul client failed: %v. functional services running in consul will " +
			"not be discovered by function discovery")
	}
	return resolver.NewResolver(kube, consul)
}
