package check

import (
	"fmt"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/spf13/cobra"
	v13 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"time"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.CHECK_COMMAND.Use,
		Short: constants.CHECK_COMMAND.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			ok, err := checkResources(opts)
			if err != nil {
				// Not returning error here because this shouldn't propagate as a standard CLI error, which prints usage.
				fmt.Printf("Error!\n")
				fmt.Printf("%s\n", err.Error())
				os.Exit(1)
			} else if !ok {
				fmt.Printf("Problems detected!\n")
				os.Exit(1)
			} else {
				fmt.Printf("No problems detected.\n")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddNamespaceFlag(pflags, &opts.Metadata.Namespace)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func checkResources(opts *options.Options) (bool, error) {
	err := checkConnection()
	if err != nil {
		return false, err
	}
	ok, err := checkPods(opts)
	if !ok || err != nil {
		return ok, err
	}
	settings, err := getSettings(opts)
	if err != nil {
		return false, err
	}

	namespaces, err := getNamespaces(settings)
	if err != nil {
		return false, err
	}

	knownUpstreams, ok, err := checkUpstreams(namespaces)
	if !ok || err != nil {
		return ok, err
	}

	ok, err = checkUpstreamGroups(namespaces)
	if !ok || err != nil {
		return ok, err
	}

	ok, err = checkSecrets(namespaces)
	if !ok || err != nil {
		return ok, err
	}

	ok, err = checkVirtualServices(namespaces, knownUpstreams)
	if !ok || err != nil {
		return ok, err
	}

	ok, err = checkGateways(namespaces)
	if !ok || err != nil {
		return ok, err
	}

	ok, err = checkProxies(namespaces)
	if !ok || err != nil {
		return ok, err
	}

	return true, nil
}

func checkPods(opts *options.Options) (bool, error) {
	fmt.Printf("Checking pods... ")
	client := helpers.MustKubeClient()
	pods, err := client.CoreV1().Pods(opts.Metadata.Namespace).List(v12.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, pod := range pods.Items {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == v13.PodReady && condition.Status != v13.ConditionTrue {
				fmt.Printf("Pod %s in namespace %s is not ready!\n", pod.Name, pod.Namespace)
				return false, err
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func getSettings(opts *options.Options) (*v1.Settings, error) {
	client := helpers.MustSettingsClient()
	return client.Read(opts.Metadata.Namespace, defaults.SettingsName, clients.ReadOpts{})
}

func getNamespaces(settings *v1.Settings) ([]string, error) {
	if settings.WatchNamespaces != nil {
		return settings.WatchNamespaces, nil
	}

	return helpers.GetNamespaces()
}

func checkUpstreams(namespaces []string) ([]string, bool, error) {
	fmt.Printf("Checking upstreams... ")
	client := helpers.MustUpstreamClient()
	var knownUpstreams []string
	for _, ns := range namespaces {
		upstreams, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return nil, false, err
		}
		for _, upstream := range upstreams {
			if upstream.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected upstream: %s\n", renderMetadata(upstream.GetMetadata()))
				fmt.Printf("Reason: %s", upstream.Status.Reason)
				return nil, false, nil
			}
			knownUpstreams = append(knownUpstreams, renderMetadata(upstream.GetMetadata()))
		}
	}
	fmt.Printf("OK\n")
	return knownUpstreams, true, nil
}

func checkUpstreamGroups(namespaces []string) (bool, error) {
	fmt.Printf("Checking upstream groups... ")
	client := helpers.MustUpstreamGroupClient()
	for _, ns := range namespaces {
		upstreamGroups, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, upstreamGroup := range upstreamGroups {
			if upstreamGroup.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected upstream group: %s\n", renderMetadata(upstreamGroup.GetMetadata()))
				fmt.Printf("Reason: %s", upstreamGroup.Status.Reason)
				return false, nil
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func checkVirtualServices(namespaces, knownUpstreams []string) (bool, error) {
	fmt.Printf("Checking virtual services... ")
	client := helpers.MustVirtualServiceClient()
	for _, ns := range namespaces {
		virtualServices, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, virtualService := range virtualServices {
			if virtualService.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected virtual service: %s\n", renderMetadata(virtualService.GetMetadata()))
				fmt.Printf("Reason: %s", virtualService.Status.GetReason())
				return false, nil
			}
			for _, route := range virtualService.GetVirtualHost().GetRoutes() {
				if route.GetRouteAction() != nil {
					if route.GetRouteAction().GetSingle() != nil {
						us := route.GetRouteAction().GetSingle()
						if us.GetUpstream() != nil {
							if !cliutils.Contains(knownUpstreams, renderRef(us.GetUpstream())) {
								fmt.Printf("Virtual service references unknown upstream:\n")
								fmt.Printf("  Virtual service: %s\n", renderMetadata(virtualService.GetMetadata()))
								fmt.Printf("  Upstream: %s\n", renderRef(us.GetUpstream()))
								return false, nil
							}
						}
					}
				}
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func checkGateways(namespaces []string) (bool, error) {
	fmt.Printf("Checking gateways... ")
	client := helpers.MustGatewayClient()
	for _, ns := range namespaces {
		gateways, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, gateway := range gateways {
			if gateway.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected gateway: %s\n", renderMetadata(gateway.GetMetadata()))
				fmt.Printf("Reason: %s", gateway.Status.Reason)
				return false, nil
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func checkProxies(namespaces []string) (bool, error) {
	fmt.Printf("Checking proxies... ")
	client := helpers.MustProxyClient()
	for _, ns := range namespaces {
		proxies, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, proxy := range proxies {
			if proxy.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected proxy: %s\n", renderMetadata(proxy.GetMetadata()))
				fmt.Printf("Reason: %s", proxy.Status.Reason)
				return false, nil
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func checkSecrets(namespaces []string) (bool, error) {
	fmt.Printf("Checking secrets... ")
	client := helpers.MustSecretClient()
	for _, ns := range namespaces {
		_, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		// currently this would only find syntax errors
	}
	fmt.Printf("OK\n")
	return true, nil
}

func renderMetadata(metadata core.Metadata) string {
	return renderNamespaceName(metadata.Namespace, metadata.Name)
}

func renderRef(ref *core.ResourceRef) string {
	return renderNamespaceName(ref.Namespace, ref.Name)
}

func renderNamespaceName(namespace, name string) string {
	return fmt.Sprintf("%s %s", namespace, name)
}

// Checks whether the cluster that the kubeconfig points at is available
// The timeout for the kubernetes client is set to a low value to notify the user of the failure
func checkConnection() error {
	client, err := helpers.GetKubernetesClientWithTimeout(time.Second)
	if err != nil {
		return errors.Wrapf(err, "Could not get kubernetes client")
	}
	_, err = client.CoreV1().Namespaces().List(v12.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "Could not communicate with kubernetes cluster")
	}
	return nil
}
