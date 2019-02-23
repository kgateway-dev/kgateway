# Gloo Helm chart
This directory contains the resources used to generate the Gloo Helm chart archive.

## generate.go
This go script takes the `*-template.yaml` files in this directory and performs value substitutions 
to generate the following files:

- `Chart.yaml`: contains information about the Gloo chart
- `values.yaml`: default configuration values for the chart, used to generate manifest for the `gateway` deployment option
- `values-ingress.yaml`: values used to generate the manifest for the `ingress` deployment option
- `values-knative.yaml`: values used to generate the manifest for the `knative` deployment option
- `crds/Chart.yaml`: contains information about the CRD subchart chart (see next paragraph)

Check the [Gloo docs](https://gloo.solo.io/installation/quick_start/#2-choosing-a-deployment-option)
for a description of the different installation options.

## /crds
This directory contains a chart to build manifests for any Custom Resource Definition that should be part 
of the Gloo installation. When `glooctl` installs Gloo, it renders and applies the templates in this 
directory first, using the same value files used by the regular Gloo chart for variable substitution. 
This is done in order to avoid race conditions during the installation, i.e. situations where the CRDs 
are still pending when the manifest (which creates instances of these CRDs) is applied.

## /templates
This directory contains the Helm templates used to generate the Gloo manifests.
