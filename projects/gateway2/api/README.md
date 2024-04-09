## APIs for Gloo Gateway - Kubernetes Gateway API Integration

This directory contains protobuf files used by the Gloo Gateway integration with the Kubernetes Gateway API.

### Codegen
The protos in this directory are compiled by the k8s gateway integration codegen in [generate.go](/projects/gateway2/generate.go) which can be invoked via `make install-go-tools generated-code -B`.

The output of codegen includes:
- Go types generated in [projects/gateway2/pkg/api/](/projects/gateway2/pkg/api/) which can be used in Go code
- CRDs generated in [install/helm/gloo/crds](/install/helm/gloo/crds)

### Adding a new CRD

Note that the codegen for k8s gateway integration uses the skv2 library instead of solo-kit (which Gloo Edge classic uses), and thus there are some differences in how CRDs are defined. The k8s gateway integration only runs on Kubernetes and follows the convention of having a `spec` and `status` defined for each CRD.

To add a new CRD:
1. Create its .proto definition in this directory
2. Add the `model.Resource` to the codegen command in [generate.go](/projects/gateway2/generate.go). Include a kind, spec type name, status type name, and (optionally) short names for the CRD.
3. Run codegen as described [above](#codegen)
