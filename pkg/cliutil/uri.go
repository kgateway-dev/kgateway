package cliutil

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rotisserie/eris"

	"github.com/hashicorp/go-multierror"

	"github.com/solo-io/go-utils/kubeutils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// Get the resource identified by the given URI.
// The URI can either be an http(s) address or a relative/absolute file path.
func GetResource(uri string) (io.ReadCloser, error) {
	var file io.ReadCloser
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		resp, err := http.Get(uri)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, eris.Errorf("http GET returned status %d for resource %s", resp.StatusCode, uri)
		}

		file = resp.Body
	} else {
		path, err := filepath.Abs(uri)
		if err != nil {
			return nil, eris.Wrapf(err, "getting absolute path for %v", uri)
		}

		f, err := os.Open(path)
		if err != nil {
			return nil, eris.Wrapf(err, "opening file %v", path)
		}
		file = f
	}

	// Write the body to file
	return file, nil
}

func GetIngressHost(proxyName, proxyNamespace, proxyPort string, localCluster bool, clusterName string) (string, error) {
	restCfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return "", eris.Wrapf(err, "getting kube rest config")
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return "", eris.Wrapf(err, "starting kube client")
	}
	svc, err := kube.CoreV1().Services(proxyNamespace).Get(proxyName, metav1.GetOptions{})
	if err != nil {
		return "", eris.Wrapf(err, "could not detect '%v' service in %v namespace. "+
			"Check that Gloo has been installed properly and is running with 'kubectl get pod -n gloo-system'",
			proxyName, proxyNamespace)
	}
	var svcPort *v1.ServicePort
	switch len(svc.Spec.Ports) {
	case 0:
		return "", eris.Errorf("service %v is missing ports", proxyName)
	case 1:
		svcPort = &svc.Spec.Ports[0]
	default:
		for _, p := range svc.Spec.Ports {
			if p.Name == proxyPort {
				svcPort = &p
				break
			}
		}
		if svcPort == nil {
			return "", eris.Errorf("named port %v not found on service %v", proxyPort, proxyName)
		}
	}

	var host, port string
	// gateway-proxy is an externally load-balanced service
	if len(svc.Status.LoadBalancer.Ingress) == 0 || localCluster {
		// assume nodeport on kubernetes
		// TODO: support more types of NodePort services
		host, err = getNodeIp(svc, kube, clusterName)
		if err != nil {
			return "", eris.Wrapf(err, "")
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

func getNodeIp(svc *v1.Service, kube kubernetes.Interface, clusterName string) (string, error) {
	// pick a node where one of our pods is running
	pods, err := kube.CoreV1().Pods(svc.Namespace).List(metav1.ListOptions{
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
		return "", eris.Errorf("no node found for %v's pods. ensure at least one pod has been deployed "+
			"for the %v service", svc.Name, svc.Name)
	}
	// special case for minikube
	// we run `minikube ip` which avoids an issue where
	// we get a NAT network IP when the minikube provider is virtualbox
	if nodeName == "minikube" {
		return minikubeIp(clusterName)
	}

	node, err := kube.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	for _, addr := range node.Status.Addresses {
		return addr.Address, nil
	}

	return "", eris.Errorf("no active addresses found for node %v", node.Name)
}

func minikubeIp(clusterName string) (string, error) {
	minikubeCmd := exec.Command("minikube", "ip", "-p", clusterName)

	hostname := &bytes.Buffer{}

	minikubeCmd.Stdout = hostname
	minikubeCmd.Stderr = os.Stderr
	err := minikubeCmd.Run()

	return strings.TrimSuffix(hostname.String(), "\n"), err
}

// Call kubectl port-forward. Callers are expected to clean up the returned portFwd *exec.cmd after the port-forward is no longer needed.
func PortForward(namespace string, resource string, localPort string, kubePort string, verbose bool) (*exec.Cmd, error) {

	/** port-forward command **/

	portFwd := exec.Command("kubectl", "port-forward", "-n", namespace,
		resource, fmt.Sprintf("%s:%s", localPort, kubePort))

	err := Initialize()
	if err != nil {
		return portFwd, err
	}
	logger := GetLogger()

	portFwd.Stderr = io.MultiWriter(logger, os.Stderr)
	if verbose {
		portFwd.Stdout = io.MultiWriter(logger, os.Stdout)
	} else {
		portFwd.Stdout = logger
	}

	if err := portFwd.Start(); err != nil {
		return portFwd, err
	}

	return portFwd, nil

}

// Call kubectl port-forward and make a GET request.
// Callers are expected to clean up the returned portFwd *exec.cmd after the port-forward is no longer needed.
func PortForwardGet(namespace string, resource string, localPort string, kubePort string, verbose bool, getPath string) (string, *exec.Cmd, error) {

	/** port-forward command **/

	portFwd, err := PortForward(namespace, resource, localPort, kubePort, verbose)
	if err != nil {
		return "", portFwd, err
	}

	// wait for port-forward to be ready
	result := make(chan string)
	errs := make(chan error)
	go func() {
		for {
			res, err := http.Get("http://localhost:" + localPort + getPath)
			if err != nil {
				errs <- err
				time.Sleep(time.Millisecond * 250)
				continue
			}
			if res.StatusCode != 200 {
				errs <- eris.Errorf("invalid status code: %v %v", res.StatusCode, res.Status)
				time.Sleep(time.Millisecond * 250)
				continue
			}
			b, err := ioutil.ReadAll(res.Body)
			if err != nil {
				errs <- err
				time.Sleep(time.Millisecond * 250)
				continue
			}
			res.Body.Close()
			result <- string(b)
			return
		}
	}()

	timer := time.Tick(time.Second * 5)

	var multiErr *multierror.Error
	for {
		select {
		case err := <-errs:
			multiErr = multierror.Append(multiErr, err)
		case res := <-result:
			return res, portFwd, nil
		case <-timer:
			return "", portFwd, eris.Errorf("timed out trying to connect to localhost during port-forward, errors: %v", multiErr)
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
		return 0, eris.Errorf("Error occured looking for an open tcp port")
	}
	return tcpAddr.Port, nil
}
