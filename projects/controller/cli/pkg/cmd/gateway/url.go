package gateway

import (
	"fmt"

	"github.com/solo-io/gloo/pkg/cliutil"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func urlCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "url",
		Short: "print the http endpoint for a proxy",
		Long:  "Use this command to view the HTTP URL of a Proxy reachable from outside the cluster. You can connect to this address from a host on the same network (such as the host machine, in the case of minikube/minishift).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ingressHost, err := cliutil.GetIngressHost(opts.Top.Ctx, opts.Proxy.Name, opts.Metadata.GetNamespace(), opts.Proxy.Port,
				opts.Proxy.LocalCluster, opts.Proxy.LocalClusterName)
			if err != nil {
				return err
			}
			if opts.Proxy.Port == "http" || opts.Proxy.Port == "https" {
				fmt.Printf("%v://%v\n", opts.Proxy.Port, ingressHost)
			} else {
				fmt.Printf("%v\n", ingressHost)
			}
			return nil
		},
	}

	cmd.PersistentFlags().BoolVarP(&opts.Proxy.LocalCluster, "local-cluster", "l", false,
		"use when the target kubernetes cluster is running locally, e.g. in minikube or minishift. this will default "+
			"to true if LoadBalanced services are not assigned external IPs by your cluster")
	cmd.PersistentFlags().StringVarP(&opts.Proxy.LocalClusterName, "local-cluster-name", "p", "minikube",
		"name of the locally running minikube cluster.")
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func addressCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "address",
		Short: "print the socket address for a proxy",
		Long:  "Use this command to view the address (host:port) of a Proxy reachable from outside the cluster. You can connect to this address from a host on the same network (such as the host machine, in the case of minikube/minishift).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ingressHost, err := cliutil.GetIngressHost(opts.Top.Ctx, opts.Proxy.Name, opts.Metadata.GetNamespace(), opts.Proxy.Port,
				opts.Proxy.LocalCluster, opts.Proxy.LocalClusterName)
			if err != nil {
				return err
			}
			fmt.Printf("%v\n", ingressHost)
			return nil
		},
	}

	cmd.PersistentFlags().BoolVarP(&opts.Proxy.LocalCluster, "local-cluster", "l", false,
		"use when the target kubernetes cluster is running locally, e.g. in minikube or minishift. this will default "+
			"to true if LoadBalanced services are not assigned external IPs by your cluster")
	cmd.PersistentFlags().StringVarP(&opts.Proxy.LocalClusterName, "local-cluster-name", "p", "minikube",
		"name of the locally running minikube cluster.")
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
