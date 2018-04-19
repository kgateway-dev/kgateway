# Quick Start
Build and Deploy Gloo

This quick start builds and deploys Gloo to `minikube`. You need
[Docker](https://docs.docker.com/install/), [Helm](https://docs.helm.sh/using_helm/#installing-helm) and [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)

## Step 1 Install
Please follow the [install guide](install.md) to install `thetool`.

## Step 2 Initialize
Create a working directory for `thetool` and initialize the directory. Please pass your Docker user id.

    mkdir thetool-gloo
    cd thetool-gloo
    thetool init -u axhixh

This should show the following message.

    Initializing current directory...
    Adding default repositories...
    Added repository https://github.com/solo-io/gloo-plugins.git with commit hash 282a844ea3ed2527f5044408c9c98bc7ee027cd2
    Initialized.

You can verify it has initialized correctly by getting the feature list.

    thetool list

This should return a list of Gloo features that are enabled by default.

```
Repository:       https://github.com/solo-io/gloo-plugins.git
Name:             aws_lambda
Gloo Directory:   aws
Envoy Directory:  aws/envoy
Enabled:          true

Repository:       https://github.com/solo-io/gloo-plugins.git
Name:             google_functions
Gloo Directory:   google
Envoy Directory:  google/envoy
Enabled:          true

Repository:       https://github.com/solo-io/gloo-plugins.git
Name:             kubernetes
Gloo Directory:   kubernetes
Envoy Directory:  
Enabled:          true

Repository:       https://github.com/solo-io/gloo-plugins.git
Name:             transformation
Gloo Directory:   transformation
Envoy Directory:  transformation/envoy
Enabled:          true
```

## Step 3 Build

You can build Gloo and its components by using the `build` command. For the quick start we are going to build all the components.

    thetool build all

The `build` command builds and publishes newly created Docker images.

```
Building and publishing with 4 features
Building Envoy...
Publishing Envoy...
Pushed Envoy image axhixh/envoy:be0d5f72
Building Gloo...
Adding plugins to Gloo...
Constraining plugins to given revisions...
Publishing Gloo...
Pushed Gloo image axhixh/gloo:be0d5f72
Building gloo-function-discovery...
Publishing gloo-function-discovery...
Pushed gloo-function-discovery image axhixh/gloo-function-discovery:644fefd
Building gloo-ingress-controller...
Publishing gloo-ingress-controller...
Pushed gloo-ingress-controller image axhixh/gloo-ingress-controller:90f2b21
Building gloo-upstream-discovery...
Publishing gloo-upstream-discovery...
Pushed gloo-upstream-discovery image axhixh/gloo-upstream-discovery:12b4753
```

`thetool` builds Gloo and its components using Docker containers. The first build can take some time, specially on macOS. You can speed up the build by giving the Docker machine more memory and CPUs. Future build will use the cache and be a lot quicker.

You can also run `build` command in verbose mode to see the progress.

    thetool build all -v

<div class="tip"> When building Envoy, <a href="https://bazel.build">Bazel</a> build can fail with the error message: 
<code>gcc: internal compiler error: Killed (program cc1plus)</code>,
if the virtual machine is out of memory. You can fix it by either reducing the number of cores or increasing the RAM on Docker VM. You can set the VM to 2GB RAM and 2 CPUs for a working configuration.
</div>

## Step 4 Deploy

You can use the `deploy` command to deploy Gloo to Kubernetes.

    thetool deploy k8s 

You should see the following output.

```
Downloading Gloo chart from https://github.com/solo-io/gloo-install.git
Building with 4 features
Generating Helm Chart values...
```

You can verify this using Helm and Kubectl.

```
helm list
NAME       	REVISION	UPDATED                 	STATUS  	CHART     	NAMESPACE  
goodly-duck	1       	Thu Mar  8 13:38:21 2018	DEPLOYED	gloo-0.1.0	gloo-system
```

```
kubectl get pods -n gloo-system
NAME                                              READY     STATUS    RESTARTS   AGE
goodly-duck-function-discovery-84c67868bd-9hbtl   1/1       Running   0          1m
goodly-duck-gloo-5f7f975c75-754kf                 1/1       Running   0          1m
goodly-duck-ingress-6cfcbd4784-pbpzw              1/1       Running   0          1m
goodly-duck-ingress-controller-6fc9f58b74-h784s   1/1       Running   0          1m
goodly-duck-upstream-discovery-86dfcd79-vv79b      1/1       Running   0          1m
```