package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/solo-io/gloo/internal/upstream-discovery"
	"github.com/solo-io/gloo/internal/upstream-discovery/bootstrap"
	internalflags "github.com/solo-io/gloo/internal/upstream-discovery/bootstrap/flags"
	"github.com/solo-io/gloo/pkg/bootstrap/configstorage"
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

var opts bootstrap.Options

var rootCmd = &cobra.Command{
	Use:   "upstream-discovery",
	Short: "discovers services on various platforms and publishes them as Gloo upstreams",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := configstorage.Bootstrap(opts.Options)
		if err != nil {
			return errors.Wrap(err, "failed to create config store client")
		}
		stop := signals.SetupSignalHandler()

		// enable kubernetes service discovery by default if no discovery option has been enabled
		if !opts.UpstreamDiscoveryOptions.DiscoveryEnabled() {
			opts.UpstreamDiscoveryOptions.EnableDiscoveryForKubernetes = true
		}

		if err := upstreamdiscovery.Start(opts, store, stop); err != nil {
			return errors.Wrap(err, "initializing upstream discovery failed")
		}

		<-stop
		log.Printf("shutting down")

		return nil
	},
}

func init() {
	// choose storage options (type, etc) for configs, secrets, and artifacts
	baseOpts := &opts.Options
	flags.AddConfigStorageOptionFlags(rootCmd, baseOpts)

	// storage backends
	flags.AddFileFlags(rootCmd, baseOpts)
	flags.AddConsulFlags(rootCmd, baseOpts)

	// kubernetes flags
	// used for both storage and ud
	flags.AddKubernetesFlags(rootCmd, baseOpts)

	// copilot flags
	// used for ud
	flags.AddCoPilotFlags(rootCmd, baseOpts)

	// upstream discovery options
	internalflags.AddUpstreamDiscoveryFlags(rootCmd, &opts)
}
