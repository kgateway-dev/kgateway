package gateway

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"k8s.io/api/core/v1"

	"github.com/solo-io/go-utils/cliutils"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func urlCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "url",
		Short: "print the http endpoint for a proxy",
		Long:  "Use this command to view the HTTP URL of a Proxy reachable from outside the cluster. You can connect to this address from a host on the same network (such as the host machine, in the case of minikube/minishift).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ingressHost, err := getIngressHost(opts)
			if err != nil {
				return err
			}
			fmt.Printf("http://%v\n", ingressHost)
			return nil
		},
	}

	cmd.PersistentFlags().BoolVarP(&opts.Proxy.LocalCluster, "local-cluster", "l", false,
		"use when the target kubernetes cluster is running locally, e.g. in minikube or minishift. this will default "+
			"to true if LoadBalanced services are not assigned external IPs by your cluster")
	flagutils.AddNamespaceFlag(cmd.PersistentFlags(), &opts.Metadata.Namespace)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func getIngressHost(opts *options.Options) (string, error) {
	restCfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return "", errors.Wrapf(err, "getting kube rest config")
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return "", errors.Wrapf(err, "starting kube client")
	}
	svc, err := kube.CoreV1().Services(opts.Metadata.Namespace).Get(opts.Proxy.Name, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "could not detect '%v' service in %v namespace. "+
			"Check that Gloo has been installed properly and is running with 'kubectl get pod -n gloo-system'", opts.Proxy.Name)
	}
	var svcPort *v1.ServicePort
	switch len(svc.Spec.Ports) {
	case 0:
		return "", errors.Errorf("service %v is missing ports", opts.Proxy.Name)
	case 1:
		svcPort = &svc.Spec.Ports[0]
	default:
		for _, p := range svc.Spec.Ports {
			if p.Name == opts.Proxy.Port {
				svcPort = &p
				break
			}
		}
		if svcPort == nil {
			return "", errors.Errorf("named port %v not found on service %v", opts.Proxy.Port, opts.Proxy.Name)
		}
	}

	var host, port string
	// gateway-proxy is an externally load-balanced service
	if len(svc.Status.LoadBalancer.Ingress) == 0 || opts.Proxy.LocalCluster {
		// assume nodeport on kubernetes
		// TODO: support more types of NodePort services
		host, err = getNodeIp()
		if err != nil {
			return "", errors.Wrapf(err, "")
		}
		port = fmt.Sprintf("%v", svcPort.NodePort)
	} else {
		host = svc.Status.LoadBalancer.Ingress[0].Hostname
		if host == "" {
			host = svc.Status.LoadBalancer.Ingress[0].IP
		}
		port = fmt.Sprintf("%v", svcPort.Port)
	}
	return host + ":" + port, nil

}

func getNodeIp() (string, error) {
	kubectl := exec.Command("kubectl", "get", "node", "--output", "jsonpath={.items[0].status.addresses[0].address}")

	hostname := &bytes.Buffer{}

	kubectl.Stdout = hostname
	kubectl.Stderr = os.Stderr
	err := kubectl.Run()
	return strings.TrimSuffix(hostname.String(), "\n"), err
}
