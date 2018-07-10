package upstreamdiscovery

import (
	"context"
	"log"
	"time"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/plugins/cloudfoundry"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/upstream-discovery/bootstrap"
	"github.com/solo-io/gloo/pkg/upstream-discovery/consul"
	"github.com/solo-io/gloo/pkg/upstream-discovery/copilot"
	"github.com/solo-io/gloo/pkg/upstream-discovery/kube"
	kubeutils "github.com/solo-io/gloo/pkg/utils/kube"
)

func Start(opts bootstrap.Options, store storage.Interface, stop <-chan struct{}) error {
	if opts.UpstreamDiscoveryOptions.EnableDiscoveryForKubernetes {
		cfg, err := kubeutils.GetConfig(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
		if err != nil {
			return errors.Wrap(err, "failed to create kube restclient config")
		}

		kubeController, err := kube.NewUpstreamController(cfg, store, opts.ConfigStorageOptions.SyncFrequency)
		if err != nil {
			return errors.Wrap(err, "failed to create kubernetes upstream discovery service")
		}
		go runController("kubernetes", kubeController, stop)
	}

	if opts.UpstreamDiscoveryOptions.EnableDiscoveryForCopilot {
		istioclient, err := cloudfoundry.GetClientFromOptions(opts.CoPilotOptions)
		if err != nil {
			return errors.Wrap(err, "failed to create copilot client from config")
		}

		serviceCtl := copilot.NewUpstreamController(context.Background(), store, istioclient, 5*time.Second)
		go runController("copilot", serviceCtl, stop)
	}

	if opts.UpstreamDiscoveryOptions.EnableDiscoveryForConsul {
		cfg := opts.ConsulOptions.ToConsulConfig()

		// TODO: expose this as a separate flag (interval for restarting a blocking query)
		cfg.WaitTime = opts.ConfigStorageOptions.SyncFrequency

		serviceCtl, err := consul.NewUpstreamController(cfg, store)
		if err != nil {
			return errors.Wrap(err, "starting consul upstream discovery")
		}
		go runController("consul", serviceCtl, stop)
	}

	return nil
}

func runController(name string, controller Controller, stop <-chan struct{}) {
	go func(stop <-chan struct{}) {
		for {
			select {
			case err := <-controller.Error():
				log.Printf("%s service discovery encountered error: %v", name, err)
			case <-stop:
				return
			}
		}
	}(stop)

	log.Printf("starting %s service discovery", name)
	controller.Run(stop)
}
