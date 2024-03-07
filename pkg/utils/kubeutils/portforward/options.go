package portforward

import (
	"fmt"
	"io"
	"os"
)

type Option func(*properties)

type properties struct {
	kubeConfig        string
	kubeContext       string
	resourceType      string // deployment, service, pod
	resourceName      string
	resourceNamespace string
	localPort         int
	remotePort        int
	localAddress      string
	stdout            io.Writer
	stderr            io.Writer
}

func WithKindCluster(kindClusterName string) Option {
	return WithKubeContext(fmt.Sprintf("kind-%s", kindClusterName))
}

func WithKubeContext(kubeContext string) Option {
	return func(config *properties) {
		config.kubeContext = kubeContext
	}
}

func WithDeployment(name, namespace string) Option {
	return WithResource(name, namespace, "deployment")
}

func WithService(name, namespace string) Option {
	return WithResource(name, namespace, "service")
}

func WithResource(name, namespace, resourceType string) Option {
	return func(config *properties) {
		config.resourceName = name
		config.resourceNamespace = namespace
		config.resourceType = resourceType
	}
}

func WithRemotePort(remotePort int) Option {
	// 0 is special value for the local port, it will result in a port being chosen at random
	return WithPorts(0, remotePort)
}

func WithPorts(localPort, remotePort int) Option {
	return func(config *properties) {
		config.localPort = localPort
		config.remotePort = remotePort
	}
}

func WithWriters(out, err io.Writer) Option {
	return func(config *properties) {
		config.stdout = out
		config.stderr = err
	}
}

func buildPortForwardProperties(options ...Option) *properties {
	//default
	cfg := &properties{
		kubeConfig:        "",
		kubeContext:       "",
		resourceName:      "",
		resourceNamespace: "",
		localPort:         0,
		remotePort:        0,
		localAddress:      "localhost",
		stdout:            os.Stdout,
		stderr:            os.Stderr,
	}

	//apply opts
	for _, opt := range options {
		opt(cfg)
	}

	return cfg
}
