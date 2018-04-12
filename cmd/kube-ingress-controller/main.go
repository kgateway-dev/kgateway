package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/solo-io/gloo/internal/kube-ingress-controller/ingress"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/bootstrap/flags"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/signals"
	"github.com/solo-io/gloo/pkg/storage"
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

var globalIngress bool
var ingressServiceName string

var rootCmd = &cobra.Command{
	Use:   "kube-ingress-controller",
	Short: "Enables gloo to function as a kubernetes ingress controller",
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

		go runIngressController(cfg, store, stop)

		<-stop
		log.Printf("shutting down")
		return nil
	},
}

func init() {
	// choose storage options for configs
	flags.AddConfigStorageOptionFlags(rootCmd, &opts)

	// config storage backends
	flags.AddFileFlags(rootCmd, &opts)
	flags.AddKubernetesFlags(rootCmd, &opts)
	flags.AddConsulFlags(rootCmd, &opts)

	// ingress-specific
	rootCmd.PersistentFlags().BoolVar(&globalIngress, "global", true, "use gloo as the cluster-wide kubernetes ingress")
	rootCmd.PersistentFlags().StringVar(&ingressServiceName, "service", "", "The name of the proxy service (envoy) if running in-cluster. If --service is set, the ingress controller will update ingress objects with the load balancer endpoints")
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
	}
	return nil, errors.Errorf("unknown or unspecified config watcher type: %v", opts.ConfigStorageOptions.Type)
}

func runIngressController(cfg *rest.Config, store storage.Interface, stop <-chan struct{}) error {
	ingressCtl, err := ingress.NewIngressController(cfg, store, opts.ConfigStorageOptions.SyncFrequency, globalIngress)
	if err != nil {
		return errors.Wrap(err, "failed to create ingress controller")
	}

	if ingressServiceName != "" {
		ingressSync, err := ingress.NewIngressSyncer(cfg, opts.ConfigStorageOptions.SyncFrequency, stop, globalIngress, ingressServiceName)
		if err != nil {
			return errors.Wrap(err, "failed to start load balancer status syncer")
		}
		go func(stop <-chan struct{}) {
			log.Printf("starting ingress status sync")
			for {
				select {
				case err := <-ingressSync.Error():
					log.Printf("ingress sync encountered error: %v", err)
				case <-stop:
					return
				}
			}
		}(stop)
	}

	go func(stop <-chan struct{}) {
		log.Printf("starting ingress sync")
		for {
			select {
			case err := <-ingressCtl.Error():
				log.Printf("ingress controller encountered error: %v", err)
			case <-stop:
				return
			}
		}
	}(stop)

	log.Printf("starting ingress controller")
	ingressCtl.Run(stop)

	return nil
}
