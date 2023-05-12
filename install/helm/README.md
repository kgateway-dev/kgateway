# Gloo Edge Helm chart
This directory contains the resources used to generate the Gloo Edge Helm chart archive.

📝 [make targets](https://opensource.com/article/18/8/what-how-makefile]) are currently defined in the [Makefile](https://github.com/solo-io/gloo/blob/main/Makefile) and should be executed from the root of the repository 📝

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

## Helm-centric commands
Relevant commands to helm, meant to be run from the **root of this repository**

```bash
# VERSION is an optional environment variable.  If not specified, a default will be computed
VERSION=$VERSION make generate-helm-files    # generate `Chart.yaml` and `values.yaml` files
VERSION=$VERSION make package-chart          # package a helm chart to `_output/charts` directory (used for releasing)
VERSION=$VERSION make build-test-chart       # package a helm chart to `_test` directory (used for testing)

helm install gloo gloo/gloo                  # install Gloo Edge using Helm
TEST_PKG=install/test make test              # run all tests in this project
```

Further reading:
- What is [helm](https://helm.sh/docs/helm/helm_install/)?
- How do I [install on kubernetes with helm](https://docs.solo.io/gloo-edge/latest/installation/gateway/kubernetes/#installing-on-kubernetes-with-helm)?
- What is a [packaged Chart archive](https://helm.sh/docs/helm/helm_package/)?
- Where are our [gloo charts published](https://storage.googleapis.com/solo-public-helm) to?

