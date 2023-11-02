### Envoy Protos in Gloo Edge

The envoy api in Gloo Edge is now taken directly from the envoy protos and then copied into the relevant directory
here in the Gloo Edge repo. In order to allow for seamless addition of envoy protos directly into the gloo repo simply
change the go_package option to the appropriate `go-control-plane` package, or if none exists than add the `go-control-plane` one

For example:
 * health_check.proto: `option go_package = "github.com/envoyproxy/go-control-plane/envoy/api/v2/core";`
 * outlier_detection.proto: `option go_package = "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster";`

These protos will not have any code generated for them as they are not in directory tree containing a `solo-kit.json`.
Any protos which should have code generated for them should either live in the extensions directory, or perhaps a new
more appropriate directory. The extensions directory has all of the custom envoy filter protos which do not have `go`
code pre generated for them.

### Envoy-gloo and Envoy-gloo-ee Protos

This directory also contains .proto files that are stored in the `envoy-gloo` and `envoy-gloo-ee` repositories. We store the
.proto files here so that the corresponding Go protobuf files are generated from these definitions during the generation phase of
the Gloo build. This is necessary so that we can instantiate these objects in our control plane code so that they can be written
over to Envoy via XDS.
