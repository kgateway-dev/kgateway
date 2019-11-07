---
title: Upgrade Steps
weight: 2
description: Steps for upgrading Gloo components
---

{{% notice note %}}
This guide will largely assume that you are running the Gloo control plane in Kubernetes.
{{% /notice %}}

In this guide, we'll walk you through how to upgrade Gloo. There are two components that need to be updated:

* [`glooctl`](#upgrading-glooctl)
* [Gloo (control plane)](#upgrading-the-control-plane)
    * [Updating Gloo using `glooctl`](#using-glooctl)
    * [Updating Gloo using Helm](#using-helm)

Before upgrading, always make sure to check our changelogs (refer to our
[open-source](../../changelog/open_source) or [enterprise](../../changelog/enterprise) changelogs)
for any mention of breaking changes. In some cases, a breaking change may mean a slightly different upgrade 
procedure; if this is the case, then we will take care to explain what must be done in the changelog notes.

You may also want to scan our [frequently-asked questions](../faq) to see if any of those cases apply to you.
Also feel free to post in the `#gloo` or `#gloo-enterprise` rooms of our 
[public Slack](https://slack.solo.io/) if your use case doesn't quite fit the standard upgrade path.

### Upgrading Components

After upgrading a component, you should be sure to run `glooctl check` immediately afterwards.
`glooctl check` will scan Gloo for problems and report them back to you. A problem reported by
`glooctl check` means that Gloo is not working properly and that Envoy may not be receiving updated
configuration.

#### Upgrading `glooctl`

{{% notice note %}}
It is important to try to keep the version of `glooctl` in alignment with the version of the Gloo
control-plane running in your cluster. Because `glooctl` can create resources in your cluster
(for example, with `glooctl add route`), you may see errors in Gloo if you create resources from a version
of `glooctl` that is incompatible with the version of your control plane.
{{% /notice %}}

The easiest way to upgrade `glooctl` is to simply run `glooctl upgrade`, which will attempt to download
the latest binary. There are more fine-grained options available; those can be viewed by running
`glooctl upgrade --help`. One in particular to make note of is `glooctl upgrade --release`, which can
be useful in maintaining careful control over what version you are running.

Here is an example where we notice we have a version mismatch between `glooctl` and the version of Gloo
running in our minikube cluster (0.20.12 and 0.20.13 respectively), and we correct it:

```bash
(⎈ |minikube:gloo-system)~ > glooctl version
Client: {"version":"0.20.12"}
Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"0.20.13","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"0.20.13","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"0.20.13","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"0.20.13","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
(⎈ |minikube:gloo-system)~ > glooctl upgrade --release v0.20.13
downloading glooctl-darwin-amd64 from release tag v0.20.13
successfully downloaded and installed glooctl version v0.20.13 to /usr/local/bin/glooctl
(⎈ |minikube:gloo-system)~ > glooctl version
Client: {"version":"0.20.13"}
Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"0.20.13","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"0.20.13","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"0.20.13","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"0.20.13","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
```

#### Upgrading the Control Plane

There are two options for how to perform the control plane upgrade. Note that these options are not
mutually-exclusive; if you have used one in the past, you can freely choose to use a different one in the future.

Both installation methods allow you to provide overrides for the default chart values; however, installing through
Helm may give you more flexibility as you are working directly with Helm rather than `glooctl`, which, for
installation, is essentially just a wrapper around Helm.
See our [open-source installation docs](../../installation/gateway/kubernetes/#list-of-gloo-helm-chart-values) and
our [enterprise installation docs](../../installation/enterprise/#list-of-gloo-helm-chart-values)
for a complete list of Helm values that can be overridden.

##### Using `glooctl`

You'll want to use the `glooctl install` command tree, the most common path in which is
`glooctl install gateway`. A good way to proceed in a simple case is a two-step process, which will ensure that
`glooctl`'s version is left matching the control plane:

1. Upgrade the `glooctl` binary as described above
1. Run `glooctl install gateway`, which will pull down image versions matching `glooctl`'s version.

All `glooctl` commands can have `--help` appended to them to view helpful usage information.
Some useful flags to be aware of in particular:

* `--dry-run` (`-d`): lets you preview the YAML that is about to be handed to `kubectl apply`
* `--namespace` (`-n`): lets you customize the namespace being installed to, which defaults to `gloo-system`
* `--values`: provide a path to a values override file to use when rendering the Helm chart

Here we perform an upgrade from Gloo 0.20.12 to 0.20.13 in our minikube
cluster, confirming along the way (just as a demonstration) that the new images `glooctl` is referencing 
match its own version. Along the way you may need to delete the completed `gateway-certgen` job.

{{% notice note %}}
For Enterprise users of Gloo, this process is largely the same. You'll just need to change your `glooctl`
invocation to

```bash
glooctl install gateway enterprise --license-key ${license}
```
Get a trial Enterprise license at https://www.solo.io/gloo-trial.
{{% /notice %}}

```bash
(⎈ |minikube:gloo-system)~ > glooctl version
Client: {"version":"0.20.12"}
Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"0.20.12","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"0.20.12","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"0.20.12","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"0.20.12","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
(⎈ |minikube:gloo-system)~ > glooctl upgrade --release v0.20.13
downloading glooctl-darwin-amd64 from release tag v0.20.13
successfully downloaded and installed glooctl version v0.20.13 to /usr/local/bin/glooctl
(⎈ |minikube:gloo-system)~ > glooctl version
Client: {"version":"0.20.13"}
Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"0.20.12","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"0.20.12","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"0.20.12","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"0.20.12","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
(⎈ |minikube:gloo-system)~ > glooctl install gateway --dry-run | grep -o 'quay.*$'
quay.io/solo-io/certgen:0.20.13
quay.io/solo-io/gloo:0.20.13
quay.io/solo-io/discovery:0.20.13
quay.io/solo-io/gateway:0.20.13
quay.io/solo-io/gloo-envoy-wrapper:0.20.13
(⎈ |minikube:gloo-system)~ > kubectl delete job gateway-certgen # the job is immutable, so if the new release changes it, you may need to delete it
job.batch "gateway-certgen" deleted
(⎈ |minikube:gloo-system)~ > glooctl install gateway
Starting Gloo installation...
Installing CRDs...
Preparing namespace and other pre-install tasks...
Installing...

Gloo was successfully installed!
(⎈ |minikube:gloo-system)~ > kubectl get pod -l gloo=gloo -ojsonpath='{.items[0].spec.containers[0].image}'
quay.io/solo-io/gloo:0.20.13
```


##### Using Helm

{{% notice note %}}
Upgrading through Helm only (i.e., not through `glooctl`) will not ensure that the version of `glooctl` 
matches the control plane. You may encounter errors in this state. Be sure to follow the 
["upgrading `glooctl`"](#upgrading-glooctl) steps above to match `glooctl`'s version to the control plane. 
{{% /notice %}}

At the time of writing, Helm v2 [does not support managing CRDs](https://github.com/helm/helm/issues/5871#issuecomment-522096388).
As a result, if you try to upgrade through `helm install` or `helm upgrade`, you may encounter an error
stating that a CRD already exists.

```bash
(⎈ |minikube:gloo-system)~ > helm install gloo/gloo
Error: customresourcedefinitions.apiextensions.k8s.io "authconfigs.enterprise.gloo.solo.io" already exists
```

You could delete the CRDs yourself, or you could simply render chart yourself and then
`kubectl apply` it. The rest of this section will assume the latter.

```bash
namespace=gloo-system # customize to your namespace
helm template <(curl https://storage.googleapis.com/solo-public-helm/charts/gloo-0.20.13.tgz) \
    --namespace "$namespace" \
    -f path/to/your/values.yaml
```

We will perform the same upgrade of Gloo v0.20.12 to v0.20.13:

{{% notice note %}}
For Enterprise users of Gloo, this process is largely the same. You'll just need to change your `helm`
invocation to

```bash
helm template <(curl https://storage.googleapis.com/gloo-ee-helm/charts/gloo-ee-0.20.8.tgz) \
    --license-key "$license"
    -f path/to/your/values.yaml
```
Get a trial Enterprise license at https://www.solo.io/gloo-trial.
{{% /notice %}}

```bash
(⎈ |minikube:gloo-system)~ > glooctl version
Client: {"version":"0.20.12"}
Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"0.20.12","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"0.20.12","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"0.20.12","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"0.20.12","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
(⎈ |minikube:gloo-system)~ > kubectl delete job gateway-certgen # the job is immutable, so if the new release changes it, you may need to delete it
job.batch "gateway-certgen" deleted
(⎈ |minikube:gloo-system)~ > helm template <(curl https://storage.googleapis.com/solo-public-helm/charts/gloo-0.20.13.tgz) --namespace gloo-system | kubectl apply -f -
configmap/gloo-usage configured
configmap/gateway-proxy-v2-envoy-config unchanged
serviceaccount/gloo unchanged
... # snipped for brevity
gateway.gateway.solo.io.v2/gateway-proxy-v2-ssl unchanged
settings.gloo.solo.io/default unchanged
validatingwebhookconfiguration.admissionregistration.k8s.io/gloo-gateway-validation-webhook-gloo-system configured
(⎈ |minikube:gloo-system)~ > kubectl get pod -l gloo=gloo -ojsonpath='{.items[0].spec.containers[0].image}'
quay.io/solo-io/gloo:0.20.13
```
