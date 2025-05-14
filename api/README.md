# APIs for kgateway

This directory contains Go types for kgateway APIs & custom resources. 

## Adding a new API / CRD

These are the steps required to add a new CRD to be used in the Kubernetes Gateway integration:

1. If creating a new API version (e.g. `v1`, `v2alpha1`), create a new directory for the version and create a `doc.go` file with the `// +kubebuilder:object:generate=true` annotation, so that Go types in that directory will be converted into CRDs when codegen is run.
    - The `groupName` marker specifies the API group name for the generated CRD.
2. Create a `_types.go` file in the API version directory. Following [gateway_parameters_types.go](/api/v1alpha1/gateway_parameters_types.go) as an example:
    - Define a struct for the resource (containing the metadata fields, `Spec`, and `Status`)
        - Tip: For spec fields, try to use pointer values when appropriate, as it makes inheritance easier (allows us to differentiate between zero values and nil).
        - Define getters for each field, as these are not generated automatically.
        - Include all the appropriate json and kubebuilder annotations on fields and structs.
        - Make sure to include a unique `shortName` in the kubebuilder annotation for the resource.
        - Avoid using slices with pointers. see: https://github.com/kubernetes/code-generator/issues/166
        - RBAC rules are defined in `doc.go` via `+kubebuilder:rbac` annotation (note: this annotation should not belong to the type, but rather the file or package).
    - Define a struct for the resource list (containing the metadata fields and `Items`)
3. Run codegen via `make generated-code -B`. This will invoke the `controller-gen` command specified in [generate.go](/hack/generate.go), which should result in the following:
    - A `zz_generated.deepcopy.go` file is created in the same directory as the Go types.
    - A `zz_generated.register.go` file is created in the same directory as the Go types. To help with registering the Go types with the scheme.
    - CRDs are generated in the CRD helm chart template dir: `install/helm/kgateway-crds/templates`
    - RBAC role is generated in `install/helm/kgateway/templates/role.yaml`
    - Updates the `api/applyconfiguration` `pkg/generated` and `pkg/client` folders with kube clients. These are used in plugin initialization and the fake client is used in tests.
