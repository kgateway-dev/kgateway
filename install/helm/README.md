# Gloo Edge Helm chart
This directory contains the resources used to generate the Gloo Edge Helm chart archive.

> All make targets are currently defined in the [Makefile](https://github.com/solo-io/gloo/blob/master/Makefile) and should be executed from the root of the repository.

## Directory Structure
### generate.go
This go script takes the `*-template.yaml` files in this directory and performs value substitutions 
to generate the following files:

- `Chart.yaml`: contains information about the Gloo Edge chart
- `values.yaml`: default configuration values for the chart

Check the [Gloo Edge docs](https://docs.solo.io/gloo-edge/latest/installation/)
for a description of the different installation options.

### /crds
This directory contains the Gloo Edge `CustomResourceDefinitions`. This is the 
[required location](https://helm.sh/docs/topics/charts/#custom-resource-definitions-crds) for CRDs in Helm 3 charts.

### /templates
This directory contains the Helm templates used to generate the Gloo Edge manifests.

## Build
> For each of the commands below, you can explicitly set `{VERSION}` environment variable, or not define on, and a version will be automatically provided.

### Generate Files
To generate the `Chart.yaml` and `values.yaml` files:
```make
VERSION=<VERSION> make generate-helm-files
```

### Package Chart
To package a Gloo Edge helm chart:
```make
VERSION=<VERSION> make package-chart
```

This [packaged Chart archive](https://helm.sh/docs/helm/helm_package/) is written to the `_output/charts` directory.

### Package Chart for Tests
We use Gloo Edge charts locally for tests. These tests use the `_test` directory to pull the archive. To package charts for tests:
```make
VERSION=<VERSION> make build-test-chart
```

## Install
To install Gloo Edge using Helm:
```shell
helm install gloo gloo/gloo
```

Useful links:
- [Helm documentation](https://helm.sh/docs/helm/helm_install/)
- [Solo documentation](https://docs.solo.io/gloo-edge/latest/installation/gateway/kubernetes/#installing-on-kubernetes-with-helm)

## Release
During a Gloo Edge release, the `gloo` chart is published to the [Google Cloud Storage](https://storage.googleapis.com/solo-public-helm).

## Testing
To run all tests in this project:
```make
TEST_PKG=install/test make test
```
