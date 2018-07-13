package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/bootstrap/configstorage"
	"github.com/solo-io/gloo/pkg/bootstrap/flags"
	controlplanebootstrap "github.com/solo-io/gloo/pkg/control-plane/bootstrap"
	internalflags "github.com/solo-io/gloo/pkg/control-plane/bootstrap/flags"
	"github.com/solo-io/gloo/pkg/control-plane/eventloop"
	functiondiscovery "github.com/solo-io/gloo/pkg/function-discovery/eventloop"
	functiondiscoveryopts "github.com/solo-io/gloo/pkg/function-discovery/options"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/signals"
	"github.com/solo-io/gloo/pkg/upstream-discovery"
	upstreamdiscbootstrap "github.com/solo-io/gloo/pkg/upstream-discovery/bootstrap"
	upstreamdiscoveryflags "github.com/solo-io/gloo/pkg/upstream-discovery/bootstrap/flags"
	"github.com/spf13/cobra"

	//register plugins
	_ "github.com/solo-io/gloo/pkg/control-plane/install"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var (
	baseOpts              bootstrap.Options
	controlPlaneOpts      controlplanebootstrap.Options
	upstreamDiscoveryOpts upstreamdiscbootstrap.Options
	functionDiscoveryOpts functiondiscoveryopts.DiscoveryOptions
	xdsPort               int
)

func lethal(f func() error) {
	if err := f(); err != nil {
		log.Fatalf("error: %v", err)
	}
}

var rootCmd = &cobra.Command{
	Use:   "localgloo",
	Short: "runs the gloo control plane, upstream discovery, and function discovery in a single binary",
	RunE: func(cmd *cobra.Command, args []string) error {
		stop := signals.SetupSignalHandler()
		go lethal(func() error { return runControlPlane(stop) })
		go runUpstreamDiscovery(stop)
		go lethal(func() error { return runFunctionDiscovery(stop) })
		<-stop
		return nil
	},
}

func init() {
	// choose storage options (type, etc) for configs, secrets, and artifacts
	flags.AddConfigStorageOptionFlags(rootCmd, &baseOpts)
	flags.AddSecretStorageOptionFlags(rootCmd, &baseOpts)
	flags.AddFileStorageOptionFlags(rootCmd, &baseOpts)

	// storage and service discovery backends
	flags.AddFileFlags(rootCmd, &baseOpts)
	flags.AddKubernetesFlags(rootCmd, &baseOpts)
	flags.AddConsulFlags(rootCmd, &baseOpts)
	flags.AddCoPilotFlags(rootCmd, &baseOpts)
	flags.AddVaultFlags(rootCmd, &baseOpts)

	initControlPlane()
	initUpstreamDiscovery()
	initFunctionDiscovery()

}

func initControlPlane() {

	// xds port
	rootCmd.PersistentFlags().IntVar(&xdsPort, "xds.port", 8081, "port to serve envoy xDS services. this port should be specified in your envoy's static config")

	// Ingress flags
	internalflags.AddIngressFlags(rootCmd, &controlPlaneOpts)
}

func initUpstreamDiscovery() {
	// upstream discovery options
	upstreamdiscoveryflags.AddUpstreamDiscoveryFlags(rootCmd, &upstreamDiscoveryOpts)
}

func initFunctionDiscovery() {
	// function discovery: upstream service type detection
	rootCmd.PersistentFlags().BoolVar(&functionDiscoveryOpts.AutoDiscoverSwagger, "detect-swagger-upstreams", true, "enable automatic discovery of upstreams that implement Swagger by querying for common Swagger Doc endpoints.")
	rootCmd.PersistentFlags().BoolVar(&functionDiscoveryOpts.AutoDiscoverNATS, "detect-nats-upstreams", true, "enable automatic discovery of upstreams that are running NATS by connecting to the default cluster id.")
	rootCmd.PersistentFlags().BoolVar(&functionDiscoveryOpts.AutoDiscoverGRPC, "detect-grpc-upstreams", true, "enable automatic discovery of upstreams that are running gRPC Services and haeve reflection enabled.")
	rootCmd.PersistentFlags().BoolVar(&functionDiscoveryOpts.AutoDiscoverFaaS, "detect-openfaas", true, "enable automatic discovery for OpenFaaS functions.")
	rootCmd.PersistentFlags().BoolVar(&functionDiscoveryOpts.AutoDiscoverFission, "detect-fission", true, "enable automatic discovery for Fission functions.")
	rootCmd.PersistentFlags().BoolVar(&functionDiscoveryOpts.AutoDiscoverProjectFn, "detect-project-fn", true, "enable automatic discovery project fn upstreams.")
	rootCmd.PersistentFlags().StringSliceVar(&functionDiscoveryOpts.SwaggerUrisToTry, "swagger-uris", []string{}, "paths function discovery should try to use to discover swagger services. function discovery will query http://<upstream>/<uri> for the swagger.json document. "+
		"if found, REST functions will be discovered for this upstream.")
}

func runControlPlane(stop <-chan struct{}) error {
	controlPlaneOpts.Options = baseOpts

	eventLoop, err := eventloop.Setup(controlPlaneOpts, xdsPort)
	if err != nil {
		return errors.Wrap(err, "setting up event loop")
	}
	eventLoop.Run(stop)
	return nil
}

func runFunctionDiscovery(stop <-chan struct{}) error {
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
func runUpstreamDiscovery(stop <-chan struct{}) error {
	upstreamDiscoveryOpts.Options = baseOpts

	store, err := configstorage.Bootstrap(upstreamDiscoveryOpts.Options)
	if err != nil {
		return errors.Wrap(err, "failed to create config store client")
	}

	// enable kubernetes service discovery by default if no discovery option has been enabled
	if !upstreamDiscoveryOpts.UpstreamDiscoveryOptions.DiscoveryEnabled() {
		upstreamDiscoveryOpts.UpstreamDiscoveryOptions.EnableDiscoveryForKubernetes = true
	}

	if err := upstreamdiscovery.Start(upstreamDiscoveryOpts, store, stop); err != nil {
		return errors.Wrap(err, "initializing upstream discovery failed")
	}

	<-stop
	log.Printf("shutting down upstream discovery")

	return nil
}
