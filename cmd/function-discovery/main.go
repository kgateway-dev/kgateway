package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/solo-io/gloo/internal/function-discovery/eventloop"
	"github.com/solo-io/gloo/internal/function-discovery/options"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/bootstrap/flags"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/signals"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var (
	opts          bootstrap.Options
	discoveryOpts options.DiscoveryOptions
)

var rootCmd = &cobra.Command{
	Use:   "gloo-function-discovery",
	Short: "discovers functions for swagger, google functions, and lambda upstreams",

	RunE: func(cmd *cobra.Command, args []string) error {
		stop := signals.SetupSignalHandler()
		errs := make(chan error)

		finished := make(chan error)
		go func() { finished <- eventloop.Run(opts, discoveryOpts, stop, errs) }()
		go func() {
			for {
				select {
				case err := <-errs:
					log.Warnf("discovery error: %v", err)
				}
			}
		}()
		return <-finished
	},
}

func init() {
	// choose storage options (type, etc) for configs, secrets, and artifacts
	flags.AddConfigStorageOptionFlags(rootCmd, &opts)
	flags.AddSecretStorageOptionFlags(rootCmd, &opts)
	flags.AddFileStorageOptionFlags(rootCmd, &opts)

	// storage backends
	flags.AddFileFlags(rootCmd, &opts)
	flags.AddKubernetesFlags(rootCmd, &opts)
	flags.AddConsulFlags(rootCmd, &opts)
	flags.AddVaultFlags(rootCmd, &opts)

	// function discovery: upstream service type detection
	rootCmd.PersistentFlags().BoolVar(&discoveryOpts.AutoDiscoverSwagger, "detect-swagger-upstreams", true, "enable automatic discovery of upstreams that implement Swagger by querying for common Swagger Doc endpoints.")
	rootCmd.PersistentFlags().BoolVar(&discoveryOpts.AutoDiscoverNATS, "detect-nats-upstreams", true, "enable automatic discovery of upstreams that are running NATS by connecting to the default cluster id.")
	rootCmd.PersistentFlags().BoolVar(&discoveryOpts.AutoDiscoverGRPC, "detect-grpc-upstreams", true, "enable automatic discovery of upstreams that are running gRPC Services and haeve reflection enabled.")
	rootCmd.PersistentFlags().BoolVar(&discoveryOpts.AutoDiscoverFaaS, "detect-openfaas", true, "enable automatic discovery for OpenFaaS functions.")
	rootCmd.PersistentFlags().BoolVar(&discoveryOpts.AutoDiscoverFission, "detect-fission", true, "enable automatic discovery for Fission functions.")
	rootCmd.PersistentFlags().BoolVar(&discoveryOpts.AutoDiscoverProjectFn, "detect-project-fn", true, "enable automatic discovery project fn upstreams.")
	rootCmd.PersistentFlags().StringSliceVar(&discoveryOpts.SwaggerUrisToTry, "swagger-uris", []string{}, "paths function discovery should try to use to discover swagger services. function discovery will query http://<upstream>/<uri> for the swagger.json document. "+
		"if found, REST functions will be discovered for this upstream.")
}
