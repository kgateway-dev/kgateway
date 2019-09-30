package main

import (
	"io/ioutil"
	"os"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/go-utils/installutils/helmchart"
	"github.com/solo-io/go-utils/log"
)

var (
	valuesTemplate        = "install/helm/gloo/values-gateway-template.yaml"
	valuesOutput          = "install/helm/gloo/values.yaml"
	docsOutput            = "docs/helm-values.md"
	knativeValuesTemplate = "install/helm/gloo/values-knative-template.yaml"
	knativeValuesOutput   = "install/helm/gloo/values-knative.yaml"
	ingressValuesTemplate = "install/helm/gloo/values-ingress-template.yaml"
	ingressValuesOutput   = "install/helm/gloo/values-ingress.yaml"
	chartTemplate         = "install/helm/gloo/Chart-template.yaml"
	chartOutput           = "install/helm/gloo/Chart.yaml"

	always = "Always"
)

func main() {
	var version, repoPrefixOverride, globalPullPolicy string
	if len(os.Args) < 2 {
		panic("Must provide version as argument")
	} else {
		version = os.Args[1]

		if len(os.Args) >= 3 {
			repoPrefixOverride = os.Args[2]
		}
		if len(os.Args) >= 4 {
			globalPullPolicy = os.Args[3]
		}
	}

	log.Printf("Generating helm files.")
	if err := generateGatewayValuesYaml(version, repoPrefixOverride, globalPullPolicy); err != nil {
		log.Fatalf("generating values.yaml failed!: %v", err)
	}
	if err := generateKnativeValuesYaml(version, repoPrefixOverride, globalPullPolicy); err != nil {
		log.Fatalf("generating values-knative.yaml failed!: %v", err)
	}
	if err := generateIngressValuesYaml(version, repoPrefixOverride, globalPullPolicy); err != nil {
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

func writeDocs(docs helmchart.HelmValues, path string) error {
	err := ioutil.WriteFile(path, []byte(docs.ToMarkdown()), os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "failing writing helm values file")
	}
	return nil
}

func readGatewayConfig() (*generate.HelmConfig, error) {
	var config generate.HelmConfig
	if err := readYaml(valuesTemplate, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// install with gateway only
func generateGatewayValuesYaml(version, repositoryPrefix, globalPullPolicy string) error {
	cfg, err := readGatewayConfig()
	if err != nil {
		return err
	}

	cfg.Gloo.Deployment.Image.Tag = version
	cfg.Discovery.Deployment.Image.Tag = version
	cfg.Gateway.Deployment.Image.Tag = version
	if cfg.Gateway.CertGenJob != nil {
		cfg.Gateway.CertGenJob.Image.Tag = version
	}
	cfg.Gateway.ConversionJob.Image.Tag = version
	cfg.AccessLogger.Image.Tag = version

	for _, v := range cfg.GatewayProxies {
		v.PodTemplate.Image.Tag = version
	}

	if version == "dev" {
		cfg.Gloo.Deployment.Image.PullPolicy = always
		cfg.Discovery.Deployment.Image.PullPolicy = always
		cfg.Gateway.Deployment.Image.PullPolicy = always
		cfg.Gateway.ConversionJob.Image.PullPolicy = always
		cfg.AccessLogger.Image.PullPolicy = always
		for _, v := range cfg.GatewayProxies {
			v.PodTemplate.Image.PullPolicy = always
		}
		if cfg.Gateway.CertGenJob != nil {
			cfg.Gateway.CertGenJob.Image.PullPolicy = always
		}
	}

	if repositoryPrefix != "" {
		cfg.Global.Image.Registry = repositoryPrefix
	}

	if globalPullPolicy != "" {
		cfg.Global.Image.PullPolicy = globalPullPolicy
	}

	if err := writeDocs(helmchart.Doc(cfg), docsOutput); err != nil {
		return err
	}

	return writeYaml(cfg, valuesOutput)
}

// install with knative only
func generateKnativeValuesYaml(version, repositoryPrefix, globalPullPolicy string) error {
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
		cfg.Gloo.Deployment.Image.PullPolicy = always
		cfg.Discovery.Deployment.Image.PullPolicy = always
		cfg.Ingress.Deployment.Image.PullPolicy = always
		cfg.Settings.Integrations.Knative.Proxy.Image.PullPolicy = always
	}

	if repositoryPrefix != "" {
		cfg.Global.Image.Registry = repositoryPrefix
	}

	if globalPullPolicy != "" {
		cfg.Global.Image.PullPolicy = globalPullPolicy
	}

	return writeYaml(&cfg, knativeValuesOutput)
}

// install with ingress only
func generateIngressValuesYaml(version, repositoryPrefix, globalPullPolicy string) error {
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
		cfg.Gloo.Deployment.Image.PullPolicy = always
		cfg.Discovery.Deployment.Image.PullPolicy = always
		cfg.Ingress.Deployment.Image.PullPolicy = always
		cfg.IngressProxy.Deployment.Image.PullPolicy = always
	}

	if repositoryPrefix != "" {
		cfg.Global.Image.Registry = repositoryPrefix
	}

	if globalPullPolicy != "" {
		cfg.Global.Image.PullPolicy = globalPullPolicy
	}

	return writeYaml(&cfg, ingressValuesOutput)
}

func generateChartYaml(version string) error {
	var chart generate.Chart
	if err := readYaml(chartTemplate, &chart); err != nil {
		return err
	}

	chart.Version = version

	return writeYaml(&chart, chartOutput)
}
