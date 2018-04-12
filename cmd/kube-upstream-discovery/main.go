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
	"github.com/solo-io/gloo/pkg/bootstrap/flags"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/signals"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/consul"
	"github.com/solo-io/gloo/pkg/storage/crd"
	"github.com/solo-io/gloo/pkg/storage/file"
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
		store, err := createStorageClient(opts)
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

func createStorageClient(opts bootstrap.Options) (storage.Interface, error) {
	switch opts.ConfigStorageOptions.Type {
	case bootstrap.WatcherTypeFile:
		dir := opts.FileOptions.ConfigDir
		if dir == "" {
			return nil, errors.New("must provide directory for file config watcher")
		}
		client, err := file.NewStorage(dir, opts.ConfigStorageOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start file config watcher for directory %v", dir)
		}
		return client, nil
	case bootstrap.WatcherTypeKube:
		cfg, err := clientcmd.BuildConfigFromFlags(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
		if err != nil {
			return nil, errors.Wrap(err, "building kube restclient")
		}
		cfgWatcher, err := crd.NewStorage(cfg, opts.KubeOptions.Namespace, opts.ConfigStorageOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start kube config watcher with config %#v", opts.KubeOptions)
		}
		return cfgWatcher, nil
	case bootstrap.WatcherTypeConsul:
		cfg := opts.ConsulOptions.ToConsulConfig()
		cfgWatcher, err := consul.NewStorage(cfg, opts.ConsulOptions.RootPath, opts.ConfigStorageOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start consul config watcher with config %#v", opts.ConsulOptions)
		}
		return cfgWatcher, nil
	}
	return nil, errors.Errorf("unknown or unspecified config watcher type: %v", opts.ConfigStorageOptions.Type)
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
