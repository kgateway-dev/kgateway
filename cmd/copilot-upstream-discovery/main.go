package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"code.cloudfoundry.org/copilot"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/plugins/cloudfoundry"
	"github.com/spf13/cobra"

	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/bootstrap/configstorage"
	"github.com/solo-io/gloo/pkg/bootstrap/flags"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/signals"
	"github.com/solo-io/gloo/pkg/storage"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var opts bootstrap.Options

var rootCmd = &cobra.Command{
	Use:   "copilot-upstream-discovery",
	Short: "discovers cloud foundry services and publishes them as gloo upstreams",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := configstorage.Bootstrap(opts)
		if err != nil {
			return errors.Wrap(err, "failed to create config store client")
		}

		istioclient, err := cloudfoundry.GetClientFromOptions(opts.CoPilotOptions)

		if err != nil {
			return errors.Wrap(err, "failed to create copilot restclient config")
		}

		stop := signals.SetupSignalHandler()

		go runServiceDiscovery(istioclient, store, stop)

		<-stop
		log.Printf("shutting down")

		return nil
	},
}

func init() {
	// choose storage options (type, etc) for configs, secrets, and artifacts
	flags.AddConfigStorageOptionFlags(rootCmd, &opts)

	// storage backends
	flags.AddFileFlags(rootCmd, &opts)
	flags.AddKubernetesFlags(rootCmd, &opts)
	flags.AddConsulFlags(rootCmd, &opts)
	flags.AddVaultFlags(rootCmd, &opts)

	// copilot flags
	flags.AddCoPilotFlags(rootCmd, &opts)
}

func runServiceDiscovery(client copilot.IstioClient, store storage.Interface, stop <-chan struct{}) error {
	ctx := context.Background()
	serviceCtl := cloudfoundry.NewServiceController(ctx, store, client, 5*time.Second)

	go func(stop <-chan struct{}) {
		for {
			select {
			case err := <-serviceCtl.Error():
				log.Printf("copilot service discovery encountered error: %v", err)
			case <-stop:
				return
			}
		}
	}(stop)

	log.Printf("starting service discovery")
	serviceCtl.Run(stop)

	return nil
}
