# Gloo Edge Helm chart
This directory contains the resources used to generate the Gloo Edge Helm chart archive.

## generate.go
This go script takes the `*-template.yaml` files in this directory and performs value substitutions 
to generate the following files:

- `Chart.yaml`: contains information about the Gloo Edge chart
- `values.yaml`: default configuration values for the chart

Check the [Gloo Edge docs](https://gloo.solo.io/installation/quick_start/#2-choosing-a-deployment-option)
for a description of the different installation options.

## /crds
This directory contains the Gloo Edge `CustomResourceDefinitions`. This is the 
[required location](https://helm.sh/docs/topics/charts/#custom-resource-definitions-crds) for CRDs in Helm 3 charts.

## /templates
This directory contains the Helm templates used to generate the Gloo Edge manifests.