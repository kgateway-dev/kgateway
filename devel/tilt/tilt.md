## Development with Tilt
This section describes how to use [kind](https://kind.sigs.k8s.io) and [Tilt](https://tilt.dev) for a simplified
workflow that offers easy deployments and rapid iterative builds.

### Prerequisites

1. [Docker](https://docs.docker.com/install/): v19.03 or newer
2. [kind](https://kind.sigs.k8s.io): v0.20.0 or newer
3. [Tilt](https://docs.tilt.dev/install.html): v0.30.8 or newer
4. [helm](https://github.com/helm/helm): v3.7.1 or newer
5. [ctlptl](https://github.com/tilt-dev/ctlptl)
6. [homebrew-macos-cross-toolchains](https://github.com/messense/homebrew-macos-cross-toolchains) - to allow building linux binaries without docker

### Getting started

### Create a kind cluster with a local registry

To create a pre-configured cluster run:

```bash
ctlptl create cluster kind --name kind-kgateway --registry=ctlptl-registry
```

You can see the status of the cluster with:

```bash
kubectl cluster-info --context kind-kgateway
```

### Build and load your docker images

When you switch branches, you'll need to rebuild the images (unless the changes are in the enabled providers list). Run
```bash
VERSION=1.0.0-ci1 CLUSTER_NAME=kgateway IMAGE_VARIANT=standard make kind-build-and-load
```

### Run tilt!

Run :
```bash
tilt up
```

If there are any issues, manually triggering an update on the problematic resource should fix it

### Run the Delve debugger
The [Delve ](https://github.com/go-delve/delve/tree/master/Documentation/installation) debugger can be run on the kgateway pod by editing the `tilt-settings.yaml` file to set the debug_port and add it to port_forwarding.
The file currently has the settings to use port 50100 commented out.

The following configuration can then can be added to `launch.json` in VSCode to attach to the debugger.

```
        {
            "name": "Attach to Delve",
            "type": "go",
            "request": "attach",
            "mode": "remote",
            "port": 50100
        }
```

### Providers config

The list of enabled providers is specified in the `enabled_providers` array in `tilt-settings.yaml`

The providers have the following format :
```yaml
  gloo:                                          # Service name
    context: _output/projects/gloo               # The output dir of the binary
    image: quay.io/solo-io/gloo                  # Image name of the container in the deployment
    live_reload_deps:                            # files / folders to watch. Changes here will trigger a rebuild and reload
    - projects/gloo
    label: gloo                                  # The service name
    build_binary: make -B gloo                   # Command to build the binary
    binary_name: gloo-linux-arm64                # Name of the binary file when built
    dockerfile_contents: ...                     # A custom docker file. This might be required when the base image is different or managing an external project such as envoy
    debug_port: 50100                            # To enable debugging, just add the `debug_port` param to a provider (provided it supports debugging)
    port_forwards":                              # Custom port forwarding. This can include non debug ports such as the envoy admin port
      - 50100
```