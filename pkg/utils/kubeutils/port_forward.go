package kubeutils

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"

	"github.com/avast/retry-go"

	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rotisserie/eris"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/skv2/pkg/multicluster/kubeconfig"
)

var _ PortForwarder = &forwarder{}

// Inspired by: https://github.com/istio/istio/blob/master/pkg/kube/portforwarder.go

// PortForwarder manages the forwarding of a single port.
type PortForwarder interface {
	// Start runs this forwarder.
	Start(ctx context.Context, options ...retry.Option) error

	// Address returns the local forwarded address. Only valid while the forwarder is running.
	Address() string

	// Close this forwarder and release any resources.
	Close()

	// ErrChan returns a channel that returns an error when one is encountered. While Start() may return an initial error,
	// the port-forward connection may be lost at anytime. The ErrChan can be read to determine if/when the port-forwarding terminates.
	// This can return nil if the port forwarding stops gracefully.
	ErrChan() <-chan error

	// WaitForStop blocks until connection closed (e.g. control-C interrupt)
	WaitForStop()
}

// NewPortForwarder returns an implementation of a PortForwarder
func NewPortForwarder(options ...PortForwardOption) PortForwarder {
	return &forwarder{
		stopCh:     make(chan struct{}, 1),
		properties: buildPortForwardProperties(options...),

		// The following are populated when Start is invoked
		errCh:      nil,
		restConfig: nil,
	}
}

type forwarder struct {
	stopCh chan struct{}
	errCh  chan error

	// properties represents the set of user-defined values to configure the forwarder
	properties *properties

	// restConfig is the set of attributes that are passed to a Kubernetes client
	// The value is derived from the properties
	restConfig *rest.Config
}

func (f *forwarder) Start(ctx context.Context, options ...retry.Option) error {
	return retry.Do(func() error {
		return f.attemptStart(ctx)
	}, options...)
}

func (f *forwarder) attemptStart(ctx context.Context) error {
	logger := contextutils.LoggerFrom(ctx)

	config, err := kubeconfig.GetRestConfigWithContext(f.properties.kubeConfig, f.properties.kubeContext, "")
	if err != nil {
		return err
	}
	f.restConfig = config

	podName, err := f.getPodName(ctx)
	if err != nil {
		return err
	}

	f.errCh = make(chan error, 1)
	readyCh := make(chan struct{}, 1)

	var fw *portforward.PortForwarder
	go func() {
		for {
			select {
			case <-f.stopCh:
				return
			default:
			}
			var err error
			// Build a new port forwarder.
			fw, err = f.portForwarderToPod(podName, readyCh)
			if err != nil {
				f.errCh <- fmt.Errorf("building port forwarder failed: %v", err)
				return
			}
			if err = fw.ForwardPorts(); err != nil {
				f.errCh <- fmt.Errorf("port forward: %v", err)
				return
			}
			f.errCh <- nil
			// At this point, either the stopCh has been closed, or port forwarder connection is broken.
			// the port forwarder should have already been ready before.
			// No need to notify the ready channel anymore when forwarding again.
			readyCh = nil
		}
	}()

	// We want to block Start() until we have either gotten an error or have started
	// We may later get an error, but that is handled async.
	select {
	case err := <-f.errCh:
		return fmt.Errorf("failure running port forward process: %v", err)
	case <-readyCh:
		p, err := fw.GetPorts()
		if err != nil {
			return fmt.Errorf("failed to get ports: %v", err)
		}
		if len(p) == 0 {
			return fmt.Errorf("got no ports")
		}
		// Set local port now, as it may have been 0 as input
		f.properties.localPort = int(p[0].Local)
		logger.Debugf("Port forward established %v -> %v.%v:%v", f.Address(), podName, podName, f.properties.remotePort)
		// The forwarder is now ready.
		return nil
	}
}

func (f *forwarder) Address() string {
	return net.JoinHostPort(f.properties.localAddress, strconv.Itoa(f.properties.localPort))
}

func (f *forwarder) Close() {
	close(f.stopCh)
	// Closing the stop channel should close anything
	// opened by f.forwarder.ForwardPorts()
}

func (f *forwarder) ErrChan() <-chan error {
	return f.errCh
}

func (f *forwarder) WaitForStop() {
	<-f.stopCh
}

func (f *forwarder) portForwarderToPod(podName string, readyCh chan struct{}) (*portforward.PortForwarder, error) {
	// the following code is based on this reference, https://github.com/kubernetes/client-go/issues/51
	roundTripper, upgrader, err := spdy.RoundTripperFor(f.restConfig)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", f.properties.resourceNamespace, podName)
	hostIP := strings.TrimLeft(f.restConfig.Host, "https:/")
	serverURL := url.URL{Scheme: "https", Path: path, Host: hostIP}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, &serverURL)

	return portforward.NewOnAddresses(dialer,
		[]string{f.properties.localAddress},
		[]string{fmt.Sprintf("%d:%d", f.properties.localPort, f.properties.remotePort)},
		f.stopCh,
		readyCh,
		f.properties.stdout,
		f.properties.stderr)
}

func (f *forwarder) getPodName(ctx context.Context) (string, error) {
	switch f.properties.resourceType {
	case "deployment":
		pods, err := GetPodsForDeployment(ctx, f.restConfig, f.properties.resourceName, f.properties.resourceNamespace)
		if err != nil {
			return "", err
		}

		if len(pods) == 0 {
			return "", eris.Errorf("No pods found for deployment %s: %s", f.properties.resourceNamespace, f.properties.resourceName)
		}
		return pods[0], nil

	case "service":
		pods, err := GetPodsForService(ctx, f.restConfig, f.properties.resourceName, f.properties.resourceNamespace)
		if err != nil {
			return "", err
		}

		if len(pods) == 0 {
			return "", eris.Errorf("No pods found for service %s: %s", f.properties.resourceNamespace, f.properties.resourceName)
		}
		return pods[0], nil
	}

	return f.properties.resourceName, nil
}
