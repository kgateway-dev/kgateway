package main

import (
	"io/ioutil"
	"os"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/solo-kit/pkg/utils/log"
)

//go:generate go run generate.go

var (
	glooValuesTemplate = "install/helm/gloo/values-template.yaml"
	glooValuesOutput = "install/helm/gloo/values.yaml"
	knativeValuesTemplate = "install/helm/gloo/values-knative-template.yaml"
	knativeValuesOutput = "install/helm/gloo/values-knative.yaml"
	chartTemplate = "install/helm/gloo/Chart-template.yaml"
	chartOutput = "install/helm/gloo/Chart.yaml"
	latestKnownVersion = "0.6.6"
)

func main() {
	var version string
	if len(os.Args) >= 2 {
		version = os.Args[1]
	} else {
		version = latestKnownVersion
	}
	log.Printf("Generating helm files.")
	if err := generateValues(version); err != nil {
		log.Fatalf("generate failed!: %v", err)
	}
}



func readConfig(path string) (*generate.Config, error) {
	var c generate.Config

	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading server config file: %s", path)
	}

	if err := yaml.Unmarshal(bytes, &c); err != nil {
		return nil, errors.Wrap(err, "failed parsing configuration file")
	}

	return &c, nil
}

func writeConfig(config *generate.Config, path string) error {
	bytes, err := yaml.Marshal(config)
	if err != nil {
		return errors.Wrapf(err, "failed marshaling config struct")
	}

	err = ioutil.WriteFile(path, bytes, os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "failing writing config file")
	}
	return nil
}

func generateValues(version string) error {
	config, err := readConfig(inputPath)
	if err != nil {
		return err
	}

	config.Gloo.Deployment.Image.Tag = version
	config.Discovery.Deployment.Image.Tag = version
	config.Gateway.Deployment.Image.Tag = version
	config.GatewayProxy.Deployment.Image.Tag = version
	config.Ingress.Deployment.Image.Tag = version
	config.IngressProxy.Deployment.Image.Tag = version

	return writeConfig(config, outputPath)
}