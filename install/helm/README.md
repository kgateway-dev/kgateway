# Overview

This directory contains the resources to deploy the project via [Helm](https://helm.sh/docs/helm/helm_install/).

## Directory Structure

- `gloo`: contains the legacy Gloo chart that will be replaced by the kgateway chart
- `kgateway`: contains the WIP kgateway chart

### /kgateway

The kgateway chart contains the new Kubernetes Gateway API implementation. The RBAC configurations in `templates/rbac.yaml` are generated from the API definitions in `projects/gateway2/api/v1alpha1` using kubebuilder's controller-gen tool.

### /gloo

#### generate.go

This go script takes the `*-template.yaml` files in this directory and performs value substitutions
to generate the following files:

- `Chart.yaml`: contains information about the K8s Gateway chart
- `values.yaml`: default configuration values for the chart

Check the [K8s Gateway docs](https://docs.solo.io/k8s-gateway/latest/installation/)
for a description of the different installation options.

#### /crds
This directory contains the K8s Gateway `CustomResourceDefinitions`. This is the
[required location](https://helm.sh/docs/topics/charts/#custom-resource-definitions-crds) for CRDs in Helm 3 charts.

#### /templates
This directory contains the Helm templates used to generate the K8s Gateway manifests.

## Helm-centric commands for the legacy Gloo chart

Relevant commands to helm, meant to be run from the **root of this repository**

```bash
# VERSION is an optional environment variable.  If not specified, a default will be computed
VERSION=$VERSION make generate-helm-files    # generate `Chart.yaml` and `values.yaml` files
VERSION=$VERSION make package-chart          # package a helm chart to `_output/charts` directory (used for releasing)
VERSION=$VERSION make build-test-chart       # package a helm chart to `_test` directory (used for testing)

helm install gloo gloo/gloo                  # install Gloo Edge using Helm
TEST_PKG=install/test make test              # run all tests in this project
```
