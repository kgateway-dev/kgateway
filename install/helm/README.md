# Gloo Helm chart
This directory contains the resources used to generate the Gloo Helm chart archive.

## generate.go
This go script takes the `*-template.yaml` files in this directory and performs value substitutions 
to generate the following files:

- `Chart.yaml`: contains information about the Gloo chart
- `values.yaml`: default configuration values for the chart, used to generate manifest for the `gateway` deployment option
- `values-ingress.yaml`: values used to generate the manifest for the `ingress` deployment option
- `values-knative.yaml`: values used to generate the manifest for the `knative` deployment option

Check the [Gloo docs](https://gloo.solo.io/installation/quick_start/#2-choosing-a-deployment-option)
for a description of the different installation options.

## /templates
This directory contains the Helm templates used to generate the Gloo manifests.

### CRDs
Custom Resource Definitions in the `template` directory must be annotated as Helm `crd-install` hooks:

```yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: some-crd-name
  annotations:
    "helm.sh/hook": crd-install
...
```
This will avoid race conditions during the installation, i.e. situations where the CRDs are still pending when the 
manifest (which creates instances of these CRDs) is applied. How this is implemented depends on the installation method:
- When installing with Helm, the hooks will be processed by `Tiller` before applying the rest of the manifest
- When installing with `glooctl`, the behavior is the same, but implemented client-side: `glooctl` first renders and 
applies any CRD manifests, waits until they have been registered, and then applies the rest of the manifest.