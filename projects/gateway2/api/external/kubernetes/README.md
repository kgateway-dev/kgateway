## Kubernetes Protos in Gloo Gateway

Some of our Gloo Gateway protos depend on messages from the Kubernetes Core and Apimachinery APIs. This is because our user-facing APIs (e.g GatewayParameters) allow users to configure some parts of Kubernetes resources (e.g. pod/container security context, affinity, tolerations) directly.

Instead of mirroring/redefining these messages in our own protos, we import copies of the protos from the Kubernetes repo(s). Note, the files in those repos are named `generated.proto` since the protos were generated from Go structs.

### Usage

#### Go
The protos in this directory are compiled by the k8s gateway integration codegen in [generate.go](/projects/gateway2/generate.go), which produces Go types under [projects/gateway2/pkg/api/external/kubernetes/](/projects/gateway2/pkg/api/external/kubernetes/) which can be used in Go code.

#### Protobuf
To use these proto messages in other protos, add the appropriate import to your proto definition, e.g.
```
import "github.com/solo-io/gloo/projects/gateway2/api/external/kubernetes/api/core/v1/generated.proto";
```
and reference the messages with their package name, e.g. `k8s.io.api.core.v1.<message>`

### Updating the protos

Generally we should try to keep the protos in sync with the `k8s.io/api` version we depend on in `go.mod`. Near the top of each `generated.proto` is a comment like this which shows the source of the file, where `<version>` should correspond to the `k8s.io/api` version we are using.
```
// This is a copy of https://github.com/kubernetes/api/blob/<version>/core/v1/generated.proto
// with the imports and go_package changed to gloo paths.
// Ideally we should update this proto every time we upgrade our k8s.io/api dependency.
```

Whenever we upgrade our `k8s.io/api` version (to let's say, `<new-version>`) in gloo, we should:
1. Update each `generated.proto` in this directory with the `<new-version>` copy of the file, copied from the Kubernetes api or apimachinery repo.
2. Update the comment `This is a copy of...` in each file to have the new source file path.
3. Re-run codegen via `make install-go-tools generated-code -B`

### Adding new protos

If new Kubernetes protos need to be added:

1. Find the source file (usually look for `generated.proto` in https://github.com/kubernetes/api or https://github.com/kubernetes/apimachinery), on the branch/tag corresponding to the `k8s.io/api` version we are using.
2. Copy it here, keeping similar directory structure (e.g. if copying a file from `https://github.com/kubernetes/api/blob/<version>/batch/v1/generated.proto`, it should be copied to directory `projects/gateway2/api/external/kubernetes/api/batch/v1/generated.proto` in gloo)
3. Add a comment near the top of the file, similar to the one shown above in [Updating the protos](#updating-the-protos), which indicates the source of the copied file.
4. Change the import paths to use gloo repo paths (note, if the file is importing other dependencies that we don't have in this repo yet, those proto files need to be copied over as well).
5. Update the `go_package` to have the appropriate gloo path, e.g. `option go_package = "github.com/solo-io/gloo/projects/gateway2/pkg/api/external/kubernetes/api/core/v1";`
6. Re-run codegen via `make install-go-tools generated-code -B`
