package deployer

import (
	"io/fs"

	"github.com/solo-io/gloo/projects/gateway2/controller"
	"github.com/solo-io/gloo/projects/gateway2/helm"

	"github.com/solo-io/gloo/pkg/version"
	"k8s.io/apimachinery/pkg/runtime"
)

type properties struct {
	chartFs fs.FS

	scheme         *runtime.Scheme
	controllerName string

	// A collection of values which will be injected into the Helm chart
	// We should aggregate these using a Go struct that can be re-used to generate the
	// Helm API for this chart
	dev  bool
	port int
}

type Option func(*properties)

func WithChartFs(fs fs.FS) Option {
	return func(p *properties) {
		p.chartFs = fs
	}
}

func WithScheme(scheme *runtime.Scheme) Option {
	return func(p *properties) {
		p.scheme = scheme
	}
}

func WithXdsServer(port int) Option {
	return func(p *properties) {
		p.port = port
	}
}

func WithControllerName(controllerName string) Option {
	return func(p *properties) {
		p.controllerName = controllerName
	}
}

func WithDevMode(devMode bool) Option {
	return func(p *properties) {
		p.dev = devMode
	}
}

func buildDeployerProperties(options ...Option) *properties {
	//default
	cfg := &properties{
		chartFs:        helm.GlooGatewayHelmChart,
		scheme:         nil,
		controllerName: controller.GatewayControllerName,

		port: 0,
		dev:  false,
	}

	//apply opts
	for _, opt := range options {
		opt(cfg)
	}

	return cfg
}

// NewDeployer builds a Deployer or returns an error if one could not be constructed
func NewDeployer(options ...Option) (*Deployer, error) {
	config := buildDeployerProperties(options...)

	helmChart, err := loadFs(config.chartFs)
	if err != nil {
		return nil, err
	} else {
		// (sam-heilbron): Is this necessary?
		// simulate what `helm package` in the Makefile does
		if version.Version != version.UndefinedVersion {
			helmChart.Metadata.AppVersion = version.Version
			helmChart.Metadata.Version = version.Version
		}
	}

	return &Deployer{
		chart:  helmChart,
		scheme: config.scheme,

		dev:            config.dev,
		controllerName: config.controllerName,
		port:           config.port,
	}, nil
}
