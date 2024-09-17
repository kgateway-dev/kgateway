package cliutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"

	"github.com/avast/retry-go/v4"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"

	"github.com/hashicorp/go-multierror"
	errors "github.com/rotisserie/eris"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultTimeout = 60 * time.Second
)

var (
	defaultRetryOptions = []retry.Option{
		retry.LastErrorOnly(true),
		retry.Delay(100 * time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.Attempts(5),
	}
)

// GetResource identified by the given URI.
// The URI can either be a http(s) address or a relative/absolute file path.
func GetResource(uri string) (io.ReadCloser, error) {
	var file io.ReadCloser
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		resp, err := http.Get(uri)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, errors.Errorf("http GET returned status %d for resource %s", resp.StatusCode, uri)
		}

		file = resp.Body
	} else {
		path, err := filepath.Abs(uri)
		if err != nil {
			return nil, errors.Wrapf(err, "getting absolute path for %v", uri)
		}

		f, err := os.Open(path)
		if err != nil {
			return nil, errors.Wrapf(err, "opening file %v", path)
		}
		file = f
	}

	// Write the body to file
	return file, nil
}

// GetIngressHost returns the host address of the ingress
func GetIngressHost(ctx context.Context, proxyName, proxyNamespace, proxyPort string, localCluster bool, clusterName string) (string, error) {
	restCfg, err := kubeutils.GetRestConfigWithKubeContext("")
	if err != nil {
		return "", errors.Wrapf(err, "getting kube rest config")
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return "", errors.Wrapf(err, "starting kube client")
	}
	svc, err := kube.CoreV1().Services(proxyNamespace).Get(ctx, proxyName, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "could not detect '%v' service in %v namespace. "+
			"Check that Gloo has been installed properly and is running with 'kubectl get pod -n gloo-system'",
			proxyName, proxyNamespace)
	}
	var svcPort *corev1.ServicePort
	switch len(svc.Spec.Ports) {
	case 0:
		return "", errors.Errorf("service %v is missing ports", proxyName)
	case 1:
		svcPort = &svc.Spec.Ports[0]
	default:
		for _, p := range svc.Spec.Ports {
			if p.Name == proxyPort {
				pDurable := p
				svcPort = &pDurable
				break
			}
		}
		if svcPort == nil {
			return "", errors.Errorf("named port %v not found on service %v", proxyPort, proxyName)
		}
	}

	var host, port string
	serviceType := svc.Spec.Type
	if localCluster {
		serviceType = corev1.ServiceTypeNodePort
	}
	switch serviceType {
	case corev1.ServiceTypeClusterIP:
		// There are a few edge cases where glooctl could be run in an environment where this is not a fatal error
		// However the service type ClusterIP does not accept incoming traffic which doesnt work as a ingress
		logger := GetLogger()
		logger.Write([]byte("Warning: Potentially invalid proxy configuration, proxy may not accepting incoming connections"))
		host = svc.Spec.ClusterIP
		port = fmt.Sprintf("%v", svcPort.Port)
	case corev1.ServiceTypeNodePort:
		// TODO: support more types of NodePort services
		host, err = getNodeIp(ctx, svc, kube, clusterName)
		if err != nil {
			return "", errors.Wrapf(err, "")
		}
		port = fmt.Sprintf("%v", svcPort.NodePort)
	case corev1.ServiceTypeLoadBalancer:
		if len(svc.Status.LoadBalancer.Ingress) == 0 {
			return "", errors.Errorf("load balancer ingress not found on service %v", proxyName)
		}
		host = svc.Status.LoadBalancer.Ingress[0].Hostname
		if host == "" {
			host = svc.Status.LoadBalancer.Ingress[0].IP
		}
		port = fmt.Sprintf("%v", svcPort.Port)
	}
	return host + ":" + port, nil
}

func getNodeIp(ctx context.Context, svc *corev1.Service, kube kubernetes.Interface, clusterName string) (string, error) {
	// pick a node where one of our pods is running
	pods, err := kube.CoreV1().Pods(svc.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(svc.Spec.Selector).String(),
	})
	if err != nil {
		return "", err
	}
	var nodeName string
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" {
			nodeName = pod.Spec.NodeName
			break
		}
	}
	if nodeName == "" {
		return "", errors.Errorf("no node found for %v's pods. ensure at least one pod has been deployed "+
			"for the %v service", svc.Name, svc.Name)
	}
	// special case for minikube
	// we run `minikube ip` which avoids an issue where
	// we get a NAT network IP when the minikube provider is virtualbox
	if nodeName == "minikube" {
		return minikubeIp(clusterName)
	}

	node, err := kube.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	for _, addr := range node.Status.Addresses {
		return addr.Address, nil
	}

	return "", errors.Errorf("no active addresses found for node %v", node.Name)
}

func minikubeIp(clusterName string) (string, error) {
	minikubeCmd := exec.Command("minikube", "ip", "-p", clusterName)

	hostname := &bytes.Buffer{}

	minikubeCmd.Stdout = hostname
	minikubeCmd.Stderr = os.Stderr
	err := minikubeCmd.Run()

	return strings.TrimSuffix(hostname.String(), "\n"), err
}

// PortForward call kubectl port-forward. Callers are expected to clean up the returned portFwd *exec.cmd after the port-forward is no longer needed.
// Deprecated: Prefer portforward.NewPortForwarder
func PortForward(ctx context.Context, namespace string, resource string, localPort string, kubePort string, verbose bool) (portforward.PortForwarder, error) {
	err := Initialize()
	if err != nil {
		return nil, err
	}
	logger := GetLogger()

	outWriter := logger
	errWriter := io.MultiWriter(logger, os.Stderr)
	if verbose {
		outWriter = io.MultiWriter(logger, os.Stdout)
	}

	resourceTypeName := strings.Split(resource, "/") // ie. deployment/gloo
	localPortInt, err := strconv.Atoi(localPort)
	if err != nil {
		return nil, err
	}
	remotePortInt, err := strconv.Atoi(kubePort)
	if err != nil {
		return nil, err
	}

	portForwarder := portforward.NewPortForwarder(
		portforward.WithResource(resourceTypeName[1], namespace, resourceTypeName[0]),
		portforward.WithPorts(localPortInt, remotePortInt),
		portforward.WithWriters(outWriter, errWriter),
	)

	err = portForwarder.Start(ctx, defaultRetryOptions...)
	if err != nil {
		return nil, err
	}

	return portForwarder, nil
}

// PortForwardGet call kubectl port-forward and make a GET request.
// Callers are expected to clean up the returned portforward.PortForwarder after the port-forward is no longer needed.
// Deprecated: Prefer portforward.NewPortForwarder
func PortForwardGet(ctx context.Context, namespace string, resource string, localPort string, kubePort string, verbose bool, getPath string) (string, portforward.PortForwarder, error) {

	/** port-forward command **/
	portForwarder, err := PortForward(ctx, namespace, resource, localPort, kubePort, verbose)
	if err != nil {
		return "", nil, err
	}

	localCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	// wait for port-forward to be ready
	retryInterval := time.Millisecond * 250
	result := make(chan string)
	errs := make(chan error)
	go func() {
		for {
			select {
			case <-localCtx.Done():
				return
			default:
			}
			res, err := http.Get(fmt.Sprintf("http://%s/%s", portForwarder.Address(), strings.TrimPrefix(getPath, "/")))
			if err != nil {
				errs <- err
				time.Sleep(retryInterval)
				continue
			}
			if res.StatusCode != http.StatusOK {
				errs <- errors.Errorf("invalid status code: %v %v", res.StatusCode, res.Status)
				time.Sleep(retryInterval)
				continue
			}
			b, err := io.ReadAll(res.Body)
			if err != nil {
				errs <- err
				time.Sleep(retryInterval)
				continue
			}
			res.Body.Close()
			result <- string(b)
			return
		}
	}()

	var multiErr *multierror.Error
	for {
		select {
		case err := <-errs:
			multiErr = multierror.Append(multiErr, err)
		case res := <-result:
			return res, portForwarder, nil
		case <-localCtx.Done():
			return "", portForwarder, errors.Errorf("timed out trying to connect to localhost during port-forward, errors: %v", multiErr)
		}
	}

}
func GetFreePort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	tcpAddr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.Errorf("Error occurred looking for an open tcp port")
	}
	return tcpAddr.Port, nil
}
