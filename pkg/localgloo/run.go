package localgloo

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/bootstrap/configstorage"
	controlplanebootstrap "github.com/solo-io/gloo/pkg/control-plane/bootstrap"
	"github.com/solo-io/gloo/pkg/control-plane/eventloop"
	functiondiscovery "github.com/solo-io/gloo/pkg/function-discovery/eventloop"
	functiondiscoveryopts "github.com/solo-io/gloo/pkg/function-discovery/options"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/upstream-discovery"
	upstreamdiscbootstrap "github.com/solo-io/gloo/pkg/upstream-discovery/bootstrap"
	//register plugins
	_ "github.com/solo-io/gloo/pkg/control-plane/install"
)

func Run(stop <-chan struct{}, xdsPort int, baseOpts bootstrap.Options, controlPlaneOpts controlplanebootstrap.Options, upstreamDiscoveryOpts upstreamdiscbootstrap.Options, functionDiscoveryOpts functiondiscoveryopts.DiscoveryOptions) {
	go runControlPlane(stop, controlPlaneOpts, xdsPort)
	go runFunctionDiscovery(stop, baseOpts, functionDiscoveryOpts)
	go runUpstreamDiscovery(stop, upstreamDiscoveryOpts)
	<-stop
}

func runControlPlane(stop <-chan struct{}, controlPlaneOpts controlplanebootstrap.Options, xdsPort int) error {
	eventLoop, err := eventloop.Setup(controlPlaneOpts, xdsPort)
	if err != nil {
		log.Fatalf("%v", errors.Wrap(err, "setting up event loop"))
	}
	eventLoop.Run(stop)
	return nil
}

func runFunctionDiscovery(stop <-chan struct{}, baseOpts bootstrap.Options, functionDiscoveryOpts functiondiscoveryopts.DiscoveryOptions) error {
	errs := make(chan error)

	finished := make(chan error)
	go func() { finished <- functiondiscovery.Run(baseOpts, functionDiscoveryOpts, stop, errs) }()
	go func() {
		for {
			select {
			case err := <-errs:
				log.Warnf("function discovery error: %v", err)
			}
		}
	}()
	return <-finished
}

func runUpstreamDiscovery(stop <-chan struct{}, upstreamDiscoveryOpts upstreamdiscbootstrap.Options) error {
	store, err := configstorage.Bootstrap(upstreamDiscoveryOpts.Options)
	if err != nil {
		return errors.Wrap(err, "failed to create config store client")
	}

	// enable kubernetes service discovery by default if no discovery option has been enabled
	if !upstreamDiscoveryOpts.UpstreamDiscoveryOptions.DiscoveryEnabled() {
		upstreamDiscoveryOpts.UpstreamDiscoveryOptions.EnableDiscoveryForKubernetes = true
	}

	if err := upstreamdiscovery.Start(upstreamDiscoveryOpts, store, stop); err != nil {
		log.Warnf("%v", errors.Wrap(err, "initializing upstream discovery failed"))
	}

	<-stop
	log.Printf("shutting down upstream discovery")

	return nil
}
