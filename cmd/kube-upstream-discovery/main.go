package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/solo-io/gloo/internal/kube-upstream-discovery/controller"
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
	Use:   "kube-upstream-discovery",
	Short: "discovers kubernetes services and publishes them as gloo upstreams",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := configstorage.Bootstrap(opts)
		if err != nil {
			return errors.Wrap(err, "failed to create config store client")
		}
		cfg, err := clientcmd.BuildConfigFromFlags(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
		if err != nil {
			return errors.Wrap(err, "failed to create kube restclient config")
		}

		stop := signals.SetupSignalHandler()

		go runServiceDiscovery(cfg, store, stop)

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
}

func runServiceDiscovery(cfg *rest.Config, store storage.Interface, stop <-chan struct{}) error {
	serviceCtl, err := controller.NewServiceController(cfg, store, opts.ConfigStorageOptions.SyncFrequency)
	if err != nil {
		return errors.Wrap(err, "failed to create service discovery service")
	}

	go func(stop <-chan struct{}) {
		for {
			select {
			case err := <-serviceCtl.Error():
				log.Printf("service discovery encountered error: %v", err)
			case <-stop:
				return
			}
		}
	}(stop)

	log.Printf("starting service discovery")
	serviceCtl.Run(stop)

	return nil
}
