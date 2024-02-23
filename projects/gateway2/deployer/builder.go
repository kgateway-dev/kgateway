package deployer

import (
	"io/fs"

	"github.com/solo-io/gloo/pkg/version"
	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/apimachinery/pkg/runtime"
)

type properties struct {
	chart  *chart.Chart
	scheme *runtime.Scheme

	// A collection of values which will be injected into the Helm chart
	// We should aggregate these using a Go struct that can be re-used to generate the
	// Helm API for this chart

	dev            bool
	controllerName string
	port           int

	// An internal error that occurred during construction of the deployer
	constructionErr error
}

type Option func(*properties)

func WithChartFs(fs fs.FS) Option {
	return func(p *properties) {
		helmChart, err := loadFs(fs)
		if err != nil {
			p.constructionErr = err
		} else {
			p.chart = helmChart

			// (sam-heilbron): Is this necessary?
			// simulate what `helm package` in the Makefile does
			if version.Version != version.UndefinedVersion {
				p.chart.Metadata.AppVersion = version.Version
				p.chart.Metadata.Version = version.Version
			}
		}
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
	cfg := &properties{}

	//apply opts
	for _, opt := range options {
		opt(cfg)
	}

	return cfg
}

func BuildDeployer(options ...Option) (*Deployer, error) {
	config := buildDeployerProperties(options...)

	if config.constructionErr != nil {
		return nil, config.constructionErr
	}

	return &Deployer{
		chart:  config.chart,
		scheme: config.scheme,

		dev:            config.dev,
		controllerName: config.controllerName,
		port:           config.port,
	}, nil
}
