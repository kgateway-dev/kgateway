package main

import (
	"io/ioutil"
	"os"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/solo-kit/pkg/utils/log"
)

var (
	valuesTemplate        = "install/helm/gloo/values-gateway-template.yaml"
	valuesOutput          = "install/helm/gloo/values.yaml"
	knativeValuesTemplate = "install/helm/gloo/values-knative-template.yaml"
	knativeValuesOutput   = "install/helm/gloo/values-knative.yaml"
	ingressValuesTemplate = "install/helm/gloo/values-ingress-template.yaml"
	ingressValuesOutput   = "install/helm/gloo/values-ingress.yaml"
	chartTemplate         = "install/helm/gloo/Chart-template.yaml"
	chartOutput           = "install/helm/gloo/Chart.yaml"
	crdChartTemplate      = "install/helm/gloo/crds/Chart-template.yaml"
	crdChartOutput        = "install/helm/gloo/crds/Chart.yaml"

	ifNotPresent = "IfNotPresent"
)

func main() {
	var version string
	if len(os.Args) < 2 {
		panic("Must provide version as argument")
	} else {
		version = os.Args[1]
	}
	log.Printf("Generating helm files.")
	if err := generateGatewayValuesYaml(version); err != nil {
		log.Fatalf("generating values.yaml failed!: %v", err)
	}
	if err := generateKnativeValuesYaml(version); err != nil {
		log.Fatalf("generating values-knative.yaml failed!: %v", err)
	}
	if err := generateIngressValuesYaml(version); err != nil {
		log.Fatalf("generating values-ingress.yaml failed!: %v", err)
	}
	if err := generateChartYaml(version); err != nil {
		log.Fatalf("generating Chart.yaml failed!: %v", err)
	}
}

func readYaml(path string, obj interface{}) error {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "failed reading server config file: %s", path)
	}

	if err := yaml.Unmarshal(bytes, obj); err != nil {
		return errors.Wrap(err, "failed parsing configuration file")
	}

	return nil
}

func writeYaml(obj interface{}, path string) error {
	bytes, err := yaml.Marshal(obj)
	if err != nil {
		return errors.Wrapf(err, "failed marshaling config struct")
	}

	err = ioutil.WriteFile(path, bytes, os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "failing writing config file")
	}
	return nil
}

func readGatewayConfig() (*generate.Config, error) {
	var config generate.Config
	if err := readYaml(valuesTemplate, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// install with gateway only
func generateGatewayValuesYaml(version string) error {
	cfg, err := readGatewayConfig()
	if err != nil {
		return err
	}

	cfg.Gloo.Deployment.Image.Tag = version
	cfg.Discovery.Deployment.Image.Tag = version
	cfg.Gateway.Deployment.Image.Tag = version
	cfg.GatewayProxy.Deployment.Image.Tag = version

	if version == "dev" {
		cfg.Gloo.Deployment.Image.PullPolicy = ifNotPresent
		cfg.Discovery.Deployment.Image.PullPolicy = ifNotPresent
		cfg.Gateway.Deployment.Image.PullPolicy = ifNotPresent
		cfg.GatewayProxy.Deployment.Image.PullPolicy = ifNotPresent
	}

	return writeYaml(cfg, valuesOutput)
}

// install with knative only
func generateKnativeValuesYaml(version string) error {
	cfg, err := readGatewayConfig()
	if err != nil {
		return err
	}
	// overwrite any non-zero values
	if err := readYaml(knativeValuesTemplate, &cfg); err != nil {
		return err
	}

	cfg.Gloo.Deployment.Image.Tag = version
	cfg.Discovery.Deployment.Image.Tag = version
	cfg.Ingress.Deployment.Image.Tag = version
	cfg.Settings.Integrations.Knative.Proxy.Image.Tag = version

	if version == "dev" {
		cfg.Gloo.Deployment.Image.PullPolicy = ifNotPresent
		cfg.Discovery.Deployment.Image.PullPolicy = ifNotPresent
		cfg.Ingress.Deployment.Image.PullPolicy = ifNotPresent
		cfg.Settings.Integrations.Knative.Proxy.Image.PullPolicy = ifNotPresent
	}

	return writeYaml(&cfg, knativeValuesOutput)
}

// install with ingress only
func generateIngressValuesYaml(version string) error {
	cfg, err := readGatewayConfig()
	if err != nil {
		return err
	}
	// overwrite any non-zero values
	if err := readYaml(ingressValuesTemplate, &cfg); err != nil {
		return err
	}

	cfg.Gloo.Deployment.Image.Tag = version
	cfg.Discovery.Deployment.Image.Tag = version
	cfg.Ingress.Deployment.Image.Tag = version
	cfg.IngressProxy.Deployment.Image.Tag = version

	if version == "dev" {
		cfg.Gloo.Deployment.Image.PullPolicy = ifNotPresent
		cfg.Discovery.Deployment.Image.PullPolicy = ifNotPresent
		cfg.Ingress.Deployment.Image.PullPolicy = ifNotPresent
		cfg.IngressProxy.Deployment.Image.PullPolicy = ifNotPresent
	}

	return writeYaml(&cfg, ingressValuesOutput)
}

func generateChartYaml(version string) error {
	var chart, crdChart generate.Chart
	if err := readYaml(chartTemplate, &chart); err != nil {
		return err
	}

	chart.Version = version

	if err := writeYaml(&chart, chartOutput); err != nil {
		return err
	}

	if err := readYaml(crdChartTemplate, &crdChart); err != nil {
		return err
	}

	crdChart.Version = version

	return writeYaml(&crdChart, crdChartOutput)

}
