---
title: "Building and Deploying Gloo Edge from Source"
weight: 6
---

## Building Gloo

### Setup

Before going through this guide, readers should go through the [setup guide](https://docs.solo.io/gloo-edge/latest/guides/dev/setting-up-dev-environment/) and ensure the dependencies in this section are installed. After this, all products in Gloo can be built from the top-level [Makefile](https://github.com/solo-io/gloo/blob/master/Makefile).

### Build Setup and Code Generation

Start by running

    make -B install-go-tools generated-code

The `install-go-tools` target will install dependencies necessary to build gloo in `_output/.bin`. The `generated-code` target does multiple things:

1. It generates the go source code from the `.proto` files in the `api` directories. For example, `.proto` files in `projects/gloo/api` will be generated in `projects/gloo/pkg/api`

2. It generates the CRDs used to interact with Gloo via kubernetes. These CRDs can be found in `install/helm/gloo/crds`.

3. It generates [solo-kit](https://github.com/solo-io/solo-kit) resources, event loops, emitters, snapshots, and resource clients. These are denoted by `.sk.go`.  These are built using `solo-kit.json` configuration files.

### Building Gloo and Docker Images

There are multiple products in the Gloo repository. The code for `gloo` itself can be found in `projects/gloo`. Each product has its own set of build targets. To compile the `gloo` binary, simply run

    make -B gloo

This will output a `gloo` binary in `_output/projects/gloo/`. To build the docker image, run

    make -B VERSION=0.0.1 gloo-docker

On `arm64` or `m1`, it is also necessary to specify the `IMAGE_REPO` like so:

    make -B VERSION=0.0.1 IMAGE_REPO=localhost:5000 gloo-docker

This will build a docker image and tag it as something like this:

    Successfully tagged quay.io/solo-io/gloo:0.0.1

Note that the `VERSION` value is optional and can be set to another value; however, it *must* be a valid semantic versioning](https://semver.org/) tag.

### Deployment

Gloo can be set up and installed in [many different ways](https://docs.solo.io/gloo-edge/latest/installation/preparation/#deployment-requirements). From this point, we will assume we want to deploy Gloo into a kubernetes cluster running locally via [kind](https://docs.solo.io/gloo-edge/latest/installation/platform_configuration/cluster_setup/#kind).

First, if using `arm64` or `m1` you will need the docker registry to upload and use images in kind.
You can build kind using `JUST_KIND=true ./ci/deploy-to-kind-cluster.sh`. 
NOTE: If kind is already runnning, please delete the cluster using `kind delete  cluster`. Then run the CI command above. This will build the registry for you, and set up kind for the registry. The docker registry is located at `localhost:5000`. The docker registry is a running container with the image name `registry:2`.

Then install `gloo` into the cluster if it is not already present:

    glooctl install gateway

Next, make the image accessible to the kind cluster. On `x86_64`:

    kind load docker-image quay.io/solo-io/gloo:0.0.1

Or on `arm64` or `m1`:

    docker push localhost:5000/gloo:1.13.0-beta13-2-g24ba2e1b8-dirty

Then update the kind cluster to use this new image as the template. On `x64_64`, replace `image-tag` with `quay.io/solo-io/gloo:0.0.1`, and on `arm64`, replace it with `localhost:5000/solo-io/gloo:0.0.1`

    kubectl -n gloo-system set image deployments/gloo gloo=<image-tag>

Now observe kubernetes as it updates the deployment, shutting down the old pod and start up the new one:

    $ kubectl -n gloo-system get pods
    NAME                            READY   STATUS    RESTARTS   AGE
    discovery-77d7cd499d-vg2rl      1/1     Running   0          82s
    gateway-proxy-5b88d8995-hzt27   1/1     Running   0          82s
    gloo-6578f95696-4fdth           1/1     Running   0          82s
    gloo-bdd7ddb9d-l977c            0/1     Running   0          9s

After a few seconds, the old pod is deleted:

    $ kubectl -n gloo-system get pods
    NAME                            READY   STATUS    RESTARTS   AGE
    discovery-77d7cd499d-vg2rl      1/1     Running   0          89s
    gateway-proxy-5b88d8995-hzt27   1/1     Running   0          89s
    gloo-bdd7ddb9d-l977c            1/1     Running   0          16s

Note that it's also easier to observe this update happening in real time by using a live monitoring tool like [k9s](https://k9scli.io/).

## Making and Debugging Changes

Now that we can build and deploy our own builds, we can start editing Gloo itself. An in-depth discussion on making code changes is outside the scope of this guide. However, a brief overview of Gloo's software architecture will make things easier to understand.

Gloo is primarily composed of *plugins*. The code for these plugins can be found in `projects/gloo/pkg/plugins`. There are a lot of them! Generally speaking, these plugins read in Gloo configuration and output Envoy configuration. There are two main ways to test implemented changes:

### Unit Tests

The fastest way to test and debug changes is by using the unit tests in these directories. They are fast to recompile and re-execute and integrate smoothly with the debugger of your choice. Tests for a specific plugin can be run by changing to the directory of a plugin and running `ginkgo`:

    cd projects/gloo/pkg/plugins/csrf/ && ginkgo

To run these tests in `gdb`, first run

    go test -c

This will create a file called `csrf.test`. Then run the file in gdb using

    gdb csrf.test

#### Dynamic Breakpoints

Dynamic breakpoints can be added via gdb without having to rebuild. Start by listing the available source files by running

    info sources plugin.go

The `plugin.go` argument is optional but helpful to filter down all the source files available.

Then specify the breakpoint by copying the *full path* of the filename followed by the line on which to place it. For example:

    b /home/user/go/src/github.com/solo-io/gloo/projects/gloo/pkg/plugins/csrf/plugin.go:91

#### Static Breakpoints

Breakpoints can be added statically by editing the source code and adding `runtime.Breakpoint()` at the desired point in the code. Then rebuild and re-run in gdb. gdb will break when it hits that line; from there, it is straightforward to go up the stack and inspect variables.

Note that adding `runtime.Breakpoint()` in the code will likely cause the go runtime to abort because it does not know how to handle the break signal. So it will only be possible to run the binary in gdb instead.

### Testing in Deployment

Once the unit tests pass, the next step is to rebuild the docker image and update the cluster as described above. Any output printed to stdout can be viewed by running

    kubectl -n gloo-system logs deployments/gloo

Since this is fairly laborious, it is a good idea to make sure that the tests are updated and passing before going to this step.
