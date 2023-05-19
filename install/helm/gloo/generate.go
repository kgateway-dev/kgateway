package main

import (
	"flag"
	"os"

	"github.com/ghodss/yaml"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/k8s-utils/installutils/helmchart"
)

var (
	valuesTemplate = "install/helm/gloo/values-template.yaml"
	valuesOutput   = "install/helm/gloo/values.yaml"
	docsOutput     = "docs/content/reference/values.txt"
	chartTemplate  = "install/helm/gloo/Chart-template.yaml"
	chartOutput    = "install/helm/gloo/Chart.yaml"
	// Helm docs are generated during builds. Since version changes each build, substitute with descriptive text.
	// Provide an example to clarify format (1.2.3, not v1.2.3).
	helmDocsVersionText = "<release_version, ex: 1.2.3>"

	flagOpts = defaultFlagOptions
)

type flagOptions struct {
	version string
	// If set, will generate helm docs. Note that some helm values are parameterized within this script to help testing.
	// When generating helm values for test purposes you should not set this flag, otherwise you will dirty the repo
	// with the test-specific helm values diff.
	generateHelmDocs   bool
	repoPrefixOverride string
	globalPullPolicy   string
}

const (
	versionFlag            = "version"
	generateHelmDocsFlag   = "generate-helm-docs"
	repoPrefixOverrideFlag = "repo-prefix-override"
	globalPullPolicyFlag   = "global-pull-policy-override"
)

var defaultFlagOptions = flagOptions{
	version:            "",
	generateHelmDocs:   false,
	repoPrefixOverride: "",
	globalPullPolicy:   "",
}

func ingestFlags() {
	flag.StringVar(&flagOpts.version, versionFlag, "", "required, version to use for generated helm files")
	flag.BoolVar(&flagOpts.generateHelmDocs, generateHelmDocsFlag, false, "(for release) if set, will generate docs for the helm values")
	flag.StringVar(&flagOpts.repoPrefixOverride, repoPrefixOverrideFlag, "", "(for tests) if set, will override container repo")
	flag.StringVar(&flagOpts.globalPullPolicy, globalPullPolicyFlag, "", "(for tests) if set, will override all image pull policies")
	flag.Parse()
}

func main() {
	ingestFlags()
	if flagOpts.version == "" {
		log.Fatalf("must pass a version with flag: %v", versionFlag)
	}

	log.Printf("Generating helm files.")
	if err := generateValuesYaml(flagOpts.version, flagOpts.repoPrefixOverride, flagOpts.globalPullPolicy); err != nil {
		log.Fatalf("generating values.yaml failed!: %v", err)
	}
	if flagOpts.generateHelmDocs {
		log.Printf("Generating helm value docs in file: %v", docsOutput)
		if err := generateValueDocs(helmDocsVersionText, flagOpts.repoPrefixOverride, flagOpts.globalPullPolicy); err != nil {
			log.Fatalf("generating values.yaml docs failed!: %v", err)
		}
	} else {
		log.Printf("NOT generating helm value docs, set %v to produce helm value docs", generateHelmDocsFlag)
	}
	if err := generateChartYaml(flagOpts.version); err != nil {
		log.Fatalf("generating Chart.yaml failed!: %v", err)
	}
}

func generateValuesYaml(version, repositoryPrefix, globalPullPolicy string) error {
	cfg, err := generateValuesConfig(version, repositoryPrefix, globalPullPolicy)
	if err != nil {
		return err
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
	bytes, err := os.ReadFile(path)
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

	err = os.WriteFile(path, bytes, os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "failing writing config file")
	}
	return nil
}

func writeDocs(docs helmchart.HelmValues, path string) error {
	err := os.WriteFile(path, []byte(docs.ToMarkdown()), os.ModePerm)
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

	cfg.Gloo.Deployment.Image.Tag = &version
	// this will be overwritten in solo-projects
	cfg.Gloo.Deployment.OssImageTag = &version
	cfg.Discovery.Deployment.Image.Tag = &version
	cfg.Gateway.CertGenJob.Image.Tag = &version
	cfg.Gateway.RolloutJob.Image.Tag = &version
	cfg.Gateway.CleanupJob.Image.Tag = &version

	cfg.AccessLogger.Image.Tag = &version

	cfg.Ingress.Deployment.Image.Tag = &version
	cfg.IngressProxy.Deployment.Image.Tag = &version
	cfg.Settings.Integrations.Knative.Proxy.Image.Tag = &version
	cfg.Global.GlooMtls.Sds.Image.Tag = &version
	cfg.Global.GlooMtls.EnvoySidecar.Image.Tag = &version

	for _, v := range cfg.GatewayProxies {
		v.PodTemplate.Image.Tag = &version
	}

	if repositoryPrefix != "" {
		cfg.Global.Image.Registry = &repositoryPrefix
	}

	if globalPullPolicy != "" {
		cfg.Global.Image.PullPolicy = &globalPullPolicy
	}

	return cfg, nil
}
