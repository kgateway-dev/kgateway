package main

import (
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/go-utils/log"
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

	ifNotPresent = "IfNotPresent"
)

func main() {
	var version, repoPrefixOverride = "", ""
	if len(os.Args) < 2 {
		panic("Must provide version as argument")
	} else {
		version = os.Args[1]

		if len(os.Args) == 3 {
			repoPrefixOverride = os.Args[2]
		}

	}
	log.Printf("Generating helm files.")
	if err := generateGatewayValuesYaml(version, repoPrefixOverride); err != nil {
		log.Fatalf("generating values.yaml failed!: %v", err)
	}
	if err := generateKnativeValuesYaml(version, repoPrefixOverride); err != nil {
		log.Fatalf("generating values-knative.yaml failed!: %v", err)
	}
	if err := generateIngressValuesYaml(version, repoPrefixOverride); err != nil {
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
func generateGatewayValuesYaml(version, repositoryPrefix string) error {
	cfg, err := readGatewayConfig()
	if err != nil {
		return err
	}

	cfg.Gloo.Deployment.Image.Tag = version
	cfg.Discovery.Deployment.Image.Tag = version
	cfg.Gateway.Deployment.Image.Tag = version

	for _, v := range cfg.GatewayProxies {
		v.Deployment.Image.Tag = version
	}

	if version == "dev" {
		cfg.Gloo.Deployment.Image.PullPolicy = ifNotPresent
		cfg.Discovery.Deployment.Image.PullPolicy = ifNotPresent
		cfg.Gateway.Deployment.Image.PullPolicy = ifNotPresent
		for _, v := range cfg.GatewayProxies {
			v.Deployment.Image.PullPolicy= ifNotPresent
		}
	}

	if repositoryPrefix != "" {
		cfg.Gloo.Deployment.Image.Repository = replacePrefix(cfg.Gloo.Deployment.Image.Repository, repositoryPrefix)
		cfg.Discovery.Deployment.Image.Repository = replacePrefix(cfg.Discovery.Deployment.Image.Repository, repositoryPrefix)
		cfg.Gateway.Deployment.Image.Repository = replacePrefix(cfg.Gateway.Deployment.Image.Repository, repositoryPrefix)
		for _, v := range cfg.GatewayProxies {
			v.Deployment.Image.Repository= replacePrefix(v.Deployment.Image.Repository, repositoryPrefix)
		}

	}

	return writeYaml(cfg, valuesOutput)
}

// install with knative only
func generateKnativeValuesYaml(version, repositoryPrefix string) error {
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

	if repositoryPrefix != "" {
		cfg.Gloo.Deployment.Image.Repository = replacePrefix(cfg.Gloo.Deployment.Image.Repository, repositoryPrefix)
		cfg.Discovery.Deployment.Image.Repository = replacePrefix(cfg.Discovery.Deployment.Image.Repository, repositoryPrefix)
		cfg.Ingress.Deployment.Image.Repository = replacePrefix(cfg.Ingress.Deployment.Image.Repository, repositoryPrefix)
		cfg.Settings.Integrations.Knative.Proxy.Image.Repository = replacePrefix(cfg.Settings.Integrations.Knative.Proxy.Image.Repository, repositoryPrefix)

		// Also override for images that are not used in this option, so we don't have an inconsistent value file
		cfg.Gateway.Deployment.Image.Repository = replacePrefix(cfg.Gateway.Deployment.Image.Repository, repositoryPrefix)
		for _, v := range cfg.GatewayProxies {
			v.Deployment.Image.Repository= replacePrefix(v.Deployment.Image.Repository, repositoryPrefix)
		}
	}

	return writeYaml(&cfg, knativeValuesOutput)
}

// install with ingress only
func generateIngressValuesYaml(version, repositoryPrefix string) error {
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

	if repositoryPrefix != "" {
		cfg.Gloo.Deployment.Image.Repository = replacePrefix(cfg.Gloo.Deployment.Image.Repository, repositoryPrefix)
		cfg.Discovery.Deployment.Image.Repository = replacePrefix(cfg.Discovery.Deployment.Image.Repository, repositoryPrefix)
		cfg.Ingress.Deployment.Image.Repository = replacePrefix(cfg.Ingress.Deployment.Image.Repository, repositoryPrefix)
		cfg.IngressProxy.Deployment.Image.Repository = replacePrefix(cfg.IngressProxy.Deployment.Image.Repository, repositoryPrefix)

		// Also override for images that are not used in this option, so we don't have an inconsistent value file
		cfg.Gateway.Deployment.Image.Repository = replacePrefix(cfg.Gateway.Deployment.Image.Repository, repositoryPrefix)
		for _, v := range cfg.GatewayProxies {
			v.Deployment.Image.Repository= replacePrefix(v.Deployment.Image.Repository, repositoryPrefix)
		}
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

// We want to turn "quay.io/solo-io/gloo" into "<newPrefix>/gloo".
func replacePrefix(repository, newPrefix string) string {
	// Remove trailing slash, if present
	newPrefix = strings.TrimSuffix(newPrefix, "/")
	return strings.Join([]string{newPrefix, path.Base(repository)}, "/")
}
