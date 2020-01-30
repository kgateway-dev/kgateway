package main

import (
	"io/ioutil"
	"os"

	"github.com/ghodss/yaml"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/go-utils/installutils/helmchart"
	"github.com/solo-io/go-utils/log"
)

var (
	valuesTemplate = "install/helm/gloo/values-template.yaml"
	valuesOutput   = "install/helm/gloo/values.yaml"
	docsOutput     = "docs/content/installation/gateway/kubernetes/values.txt"
	chartTemplate  = "install/helm/gloo/Chart-template.yaml"
	chartOutput    = "install/helm/gloo/Chart.yaml"
	// For non-release builds, the string "dev" is used as the version
	devVersionTag = "dev"
	// Helm docs are generated during builds. Since version changes each build, substitute with descriptive text.
	// Provide an example to clarify format (1.2.3, not v1.2.3).
	helmDocsVersionText = "<release_version, ex: 1.2.3>"

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
	if err := generateValuesYaml(version, repoPrefixOverride, globalPullPolicy); err != nil {
		log.Fatalf("generating values.yaml failed!: %v", err)
	}
	if err := generateValueDocs(helmDocsVersionText, repoPrefixOverride, globalPullPolicy); err != nil {
		log.Fatalf("generating values.yaml docs failed!: %v", err)
	}
	if err := generateChartYaml(version); err != nil {
		log.Fatalf("generating Chart.yaml failed!: %v", err)
	}
}

func generateValuesYaml(version, repositoryPrefix, globalPullPolicy string) error {
	cfg, err := generateValuesConfig(version, repositoryPrefix, globalPullPolicy)
	if err != nil {
		return err
	}

	// customize config as needed for dev builds
	if version == devVersionTag {
		cfg.Gloo.Deployment.Image.PullPolicy = always
		cfg.Discovery.Deployment.Image.PullPolicy = always
		cfg.Gateway.Deployment.Image.PullPolicy = always
		cfg.Gateway.CertGenJob.Image.PullPolicy = always

		cfg.AccessLogger.Image.PullPolicy = always

		cfg.Ingress.Deployment.Image.PullPolicy = always
		cfg.IngressProxy.Deployment.Image.PullPolicy = always
		cfg.Settings.Integrations.Knative.Proxy.Image.PullPolicy = always

		for _, v := range cfg.GatewayProxies {
			v.PodTemplate.Image.PullPolicy = always
		}
	}

	return writeYaml(cfg, valuesOutput)
}

func generateValueDocs(version, repositoryPrefix, globalPullPolicy string) error {
	cfg, err := generateValuesConfig(version, repositoryPrefix, globalPullPolicy)
	if err != nil {
		return err
	}

	// customize config as needed for docs
	// (currently only the version text differs, and this is passed as a function argument)

	return writeDocs(helmchart.Doc(cfg), docsOutput)
}

func generateChartYaml(version string) error {
	var chart generate.Chart
	if err := readYaml(chartTemplate, &chart); err != nil {
		return err
	}

	chart.Version = version

	return writeYaml(&chart, chartOutput)
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

func readValuesTemplate() (*generate.HelmConfig, error) {
	var config generate.HelmConfig
	if err := readYaml(valuesTemplate, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func generateValuesConfig(version, repositoryPrefix, globalPullPolicy string) (*generate.HelmConfig, error) {
	cfg, err := readValuesTemplate()
	if err != nil {
		return nil, err
	}

	cfg.Gloo.Deployment.Image.Tag = version
	cfg.Discovery.Deployment.Image.Tag = version
	cfg.Gateway.Deployment.Image.Tag = version
	cfg.Gateway.CertGenJob.Image.Tag = version

	cfg.AccessLogger.Image.Tag = version

	cfg.Ingress.Deployment.Image.Tag = version
	cfg.IngressProxy.Deployment.Image.Tag = version
	cfg.Settings.Integrations.Knative.Proxy.Image.Tag = version

	for _, v := range cfg.GatewayProxies {
		v.PodTemplate.Image.Tag = version
	}

	if repositoryPrefix != "" {
		cfg.Global.Image.Registry = repositoryPrefix
	}

	if globalPullPolicy != "" {
		cfg.Global.Image.PullPolicy = globalPullPolicy
	}

	return cfg, nil
}
