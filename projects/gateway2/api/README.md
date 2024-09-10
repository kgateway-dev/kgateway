# APIs for Gloo Gateway - Kubernetes Gateway API Integration

This directory contains Go types for custom resources that Gloo Gateway uses with its Kubernetes Gateway API integration.

## Adding a new API / CRD

These are the steps required to add a new CRD to be used in the Kubernetes Gateway integration:

1. If creating a new API version (e.g. `v1`, `v2alpha1`), create a new directory for the version and create a `doc.go` file with the `// +kubebuilder:object:generate=true` annotation, so that Go types in that directory will be converted into CRDs when codegen is run.
    - The `groupName` marker specifies the API group name for the generated CRD.
2. Create a `_types.go` file in the API version directory. Following [gateway_parameters_types.go](/projects/gateway2/api/v1alpha1/gateway_parameters_types.go) as an example:
    - Define a struct for the resource (containing the metadata fields, `Spec`, and `Status`)
        - Tip: For spec fields, try to use pointer values when appropriate, as it makes inheritance easier (allows us to differentiate between zero values and nil).
        - Define getters for each field, as these are not generated automatically.
        - Include all the appropriate json and kubebuilder annotations on fields and structs.
        - Make sure to include a unique `shortName` in the kubebuilder annotation for the resource.
    - Define a struct for the resource list (containing the metadata fields and `Items`)
3. Run codegen via `make generated-code -B`. This will invoke the `controller-gen` command specified in [generate.go](/projects/gateway2/generate.go), which should result in the following:
    - A `zz_generated.deepcopy.go` file is created in the same directory as the Go types.
    - A CRD file is generated in [install/helm/gloo/crds](/install/helm/gloo/crds)

Note: You may need to manually add the new API to the install/helm/gloo/templates/44-rbac.yaml file.

## Background

Historically, we have defined Gloo Gateway custom resources using protobuf files, which are then converted to Go types via solo-kit or skv2 codegen. This was also the initial implementation for the kube gateway resources, however we pivoted to using Go types for a few reasons.

Some of our Gloo Gateway APIs depend on Kubernetes [Core](https://github.com/kubernetes/api) and [Apimachinery](https://github.com/kubernetes/apimachinery) APIs, because our user-facing APIs (e.g GatewayParameters) allow users to configure some parts of Kubernetes resources (e.g. pod/container security context, affinity, tolerations) directly.

The source of truth for the Kubernetes APIs are the Go types defined in the Kubernetes repos, and protobuf files get generated from the Go types. Initially, the Gloo Gateway protobuf files imported a copy of the generated Kubernetes protobuf files, but:

- There turned out to be inconsistencies between the structure of the generated protobuf files and the source APIs (e.g. extra embedded fields).
- Maintaining a copy of the generated files meant that we would need to remember to update them whenever we upgraded Kubernetes library versions in Gloo Gateway.
