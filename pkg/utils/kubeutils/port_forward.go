package kubeutils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/rotisserie/eris"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/skv2/pkg/multicluster/kubeconfig"
)

// Inspired by: https://github.com/solo-io/gloo-mesh-enterprise/blob/main/pkg/utils/kubeutils/port_forward.go

type properties struct {
	kubeConfig        string
	kubeContext       string
	resourceName      string
	resourceNamespace string
	localPort         string
	remotePort        string
	stdout            io.Writer
	stderr            io.Writer
}

type PortForwardOption func(*properties)

func WithKindCluster(kindClusterName string) PortForwardOption {
	return func(config *properties) {
		config.kubeContext = fmt.Sprintf("kind-%s", kindClusterName)
	}
}

func WithResource(name, namespace string) PortForwardOption {
	return func(config *properties) {
		config.resourceName = name
		config.resourceNamespace = namespace
	}
}

func WithPorts(localPort, remotePort int) PortForwardOption {
	return func(config *properties) {
		if localPort == 0 {
			config.localPort = ""
		} else {
			config.localPort = strconv.Itoa(localPort)
		}
		config.remotePort = strconv.Itoa(remotePort)
	}
}

func WithWriters(out, err io.Writer) PortForwardOption {
	return func(config *properties) {
		config.stdout = out
		config.stderr = err
	}
}

func buildPortForwardProperties(options ...PortForwardOption) *properties {
	//default
	cfg := &properties{
		kubeConfig:        "",
		kubeContext:       "",
		resourceName:      "",
		resourceNamespace: "",
		localPort:         "",
		remotePort:        "",
		stdout:            os.Stdout,
		stderr:            os.Stderr,
	}

	//apply opts
	for _, opt := range options {
		opt(cfg)
	}

	return cfg
}

// PortForwardFromDeployment opens a port-forward against the specified deployment. Returns the local port.
// If localPort is unspecified, a free port will be chosen at random.
// Canceling the context will stop the port-forward.
func PortForwardFromDeployment(ctx context.Context, options ...PortForwardOption) (string, error) {
	config := buildPortForwardProperties(options...)

	pods, err := GetPodsForDeployment(ctx, config.kubeConfig, config.kubeContext, config.resourceName, config.resourceNamespace)
	if err != nil {
		return "", err
	}

	if len(pods) == 0 {
		return "", eris.Errorf("No pods found for deployment %s: %s", config.resourceNamespace, config.resourceName)
	}

	config.resourceName = pods[0]
	return portForwardFromPod(ctx, config)
}

// PortForwardFromSvc opens a port-forward against the specified service. Returns the local port.
// If localPort is unspecified, a free port will be chosen at random.
// Canceling the context will stop the port-forward.
func PortForwardFromSvc(ctx context.Context, options ...PortForwardOption) (string, error) {
	config := buildPortForwardProperties(options...)

	pods, err := GetPodsForService(ctx, config.kubeConfig, config.kubeContext, config.resourceName, config.resourceNamespace)
	if err != nil {
		return "", err
	}

	if len(pods) == 0 {
		return "", eris.Errorf("No pods found for service %s: %s", config.resourceNamespace, config.resourceName)
	}

	config.resourceName = pods[0]
	return portForwardFromPod(ctx, config)
}

func portForwardFromPod(ctx context.Context, options *properties) (string, error) {
	logger := contextutils.LoggerFrom(ctx)
	// select random open local port if unspecified
	if options.localPort == "" {
		freePort, err := cliutils.GetFreePort()
		if err != nil {
			return "", err
		}
		options.localPort = strconv.Itoa(freePort)
	}
	logger.Debugf("forwarding port %s of pod %s in namespace %s to local port %s",
		options.remotePort, options.resourceName, options.resourceNamespace, options.localPort)

	config, err := kubeconfig.GetRestConfigWithContext(options.kubeConfig, options.kubeContext, "")
	if err != nil {
		return "", err
	}

	// the following code is based on this reference, https://github.com/kubernetes/client-go/issues/51
	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return "", err
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", options.resourceNamespace, options.resourceName)
	hostIP := strings.TrimLeft(config.Host, "https:/")
	serverURL := url.URL{Scheme: "https", Path: path, Host: hostIP}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, &serverURL)

	stopChan, readyChan := make(chan struct{}, 1), make(chan struct{}, 1)

	forwarder, err := portforward.New(
		dialer,
		[]string{fmt.Sprintf("%s:%s", options.localPort, options.remotePort)},
		stopChan,
		readyChan,
		options.stdout,
		options.stderr)
	if err != nil {
		return "", err
	}
	errChan := make(chan error, 1)
	go func() {
		if err = forwarder.ForwardPorts(); err != nil { // Locks until stopChan is closed.
			logger.Errorf("%v", err)
			errChan <- err
		}
	}()
	go func() {
		<-ctx.Done() // wait until done, then..
		close(stopChan)
	}()

	// pause until port-forward is ready, or we receive an error
	select {
	case err := <-errChan:
		return "", err
	case <-readyChan:
	}

	return options.localPort, nil
}
