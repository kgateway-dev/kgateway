# Gloo Helm chart
This directory contains the resources used to generate the Gloo Helm chart archive.

## generate.go
This go script takes the `*-template.yaml` files in this directory and performs value substitutions 
to generate the following files:

- `Chart.yaml`: contains information about the Gloo chart
- `values.yaml`: default configuration values for the chart

Check the [Gloo docs](https://gloo.solo.io/installation/quick_start/#2-choosing-a-deployment-option)
for a description of the different installation options.

## /crds
This directory contains the Gloo `CustomResourceDefinitions`.

## /templates
This directory contains the Helm templates used to generate the Gloo manifests.
