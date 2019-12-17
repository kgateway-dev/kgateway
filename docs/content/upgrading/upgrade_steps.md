---
title: Upgrade Steps
weight: 3
description: Steps for upgrading Gloo components
---

{{% notice warning %}}
This upgrade process is not suitable in environments where downtime is unacceptable. This is mainly for upgrading
Gloo while experimenting in dev or staging environments. For zero-downtime upgrades, users have found success in performing
Gloo upgrades through blue-green deployments across different namespaces.
{{% /notice %}}

{{% notice note %}}
This guide will largely assume that you are running Gloo in Kubernetes.
{{% /notice %}}

In this guide, we'll walk you through how to upgrade Gloo. First you'll want to familiarize yourself
with our various [Changelog entry types](../../changelog/changelog_types) so that you can review
the changes that have been made in the release you are upgrading to. 

Once you have reviewed the changes in the new release, there are two components to upgrade:

* [`glooctl`](#upgrading-glooctl)
    * [Using `glooctl` itself](#using-glooctl-itself)
    * [Download release asset](#download-release-asset)
* [Gloo (server components)](#upgrading-the-server-components)
    * [Updating Gloo using `glooctl`](#using-glooctl)
    * [Updating Gloo using Helm](#using-helm)
        * [Helm 3](#helm-3)
        * [Helm 2](#helm-2)

Before upgrading, always make sure to check our changelogs (refer to our
[open-source](../../changelog/open_source) or [enterprise](../../changelog/enterprise) changelogs)
for any mention of breaking changes. In some cases, a breaking change may mean a slightly different upgrade 
procedure; if this is the case, then we will take care to explain what must be done in the changelog notes.

You may also want to scan our [frequently-asked questions](../faq) to see if any of those cases apply to you.
Also feel free to post in the `#gloo` or `#gloo-enterprise` rooms of our 
[public Slack](https://slack.solo.io/) if your use case doesn't quite fit the standard upgrade path. 

{{% notice note %}}
We version open-source Gloo separately from Gloo Enterprise. This is because Gloo Enterprise pulls in
open-source Gloo as a dependency. While the patch versions of Gloo and Gloo Enterprise may drift apart
from each other, we will maintain the same major/minor versions across the two projects. So for example,
we may be at version `x.y.a` in open-source Gloo and `x.y.b` in Gloo Enterprise. `x` and `y` will always
be the same, but `a` and `b` will often not be the same. This is why, if you are a Gloo Enterprise user,
you may see different versions reported by `glooctl version`. We will try to ensure that open-source Gloo
and Gloo Enterprise will be compatible each other across patch versions, but make no guarantees
about compatibility between minor or major versions.

<br> 

Visit https://semver.org/ for an explanation of semantic versioning if you
are unfamiliar with these concepts.

<br>

```bash
~ > glooctl version # snipped some content for brevity
Client: {"version":"0.20.13"} # glooctl is built from open-source Gloo, which is where its version comes from
Server: {"type":"Gateway","enterprise":true,"kubernetes":...,{"Tag":"0.20.8","Name":"grpcserver-ee","Registry":"quay.io/solo-io"},...,{"Tag":"0.20.13","Name":"discovery","Registry":"quay.io/solo-io"},...}

# above we see the Gloo Enterprise API server running enterprise version 0.20.8,
# which has pulled in open-source Gloo 0.20.13 as a dependency.
```

<br>

If you are an open-source user of Gloo, you will only need to be aware of open-source versions found
[in our open-source changelogs](../../changelog/open_source). If you are an enterprise user of Gloo,
you will be selecting versions of Gloo Enterprise from [our Enterprise changelogs](../../changelog/enterprise).
However, you may need to be aware of the version of open-source Gloo included as a dependency in
Gloo Enterprise, as most of our proto definitions are open-source. Changes to the open-source version
will be listed as "Dependency Bumps", and significant changes may be listed as "Breaking Changes" in
our [changelog entries](../../changelog/changelog_types).
{{% /notice %}}

## Upgrading Components

After upgrading a component, you should be sure to run `glooctl check` immediately afterwards.
`glooctl check` will scan Gloo for problems and report them back to you. A problem reported by
`glooctl check` means that Gloo is not working properly and that Envoy may not be receiving updated
configuration.

### Upgrading `glooctl`

{{% notice note %}}
It is important to try to keep the version of `glooctl` in alignment with the version of the Gloo
server components running in your cluster. Because `glooctl` can create resources in your cluster
(for example, with `glooctl add route`), you may see errors in Gloo if you create resources from a version
of `glooctl` that is incompatible with the version of the server components.
{{% /notice %}}

#### Using `glooctl` Itself

The easiest way to upgrade `glooctl` is to simply run `glooctl upgrade`, which will attempt to download
the latest binary. There are more fine-grained options available; those can be viewed by running
`glooctl upgrade --help`. One in particular to make note of is `glooctl upgrade --release`, which can
be useful in maintaining careful control over what version you are running.

Here is an example where we notice we have a version mismatch between `glooctl` and the version of Gloo
running in our minikube cluster (1.2.0 and 1.2.1 respectively), and we correct it:

```bash
~ > glooctl version
Client: {"version":"1.2.0"}
Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"1.2.1","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"1.2.1","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"1.2.1","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"1.2.1","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
```

```bash
~ > glooctl upgrade --release v1.2.1
downloading glooctl-darwin-amd64 from release tag v1.2.1
successfully downloaded and installed glooctl version v1.2.1 to /usr/local/bin/glooctl
```

```bash
~ > glooctl version
Client: {"version":"1.2.1"}
Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"1.2.1","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"1.2.1","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"1.2.1","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"1.2.1","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
```

```bash
~ > glooctl check
Checking deployments... OK
Checking pods... OK
Checking upstreams... OK
Checking upstream groups... OK
Checking secrets... OK
Checking virtual services... OK
Checking gateways... OK
Checking proxies... OK
No problems detected.
```

#### Download Release Asset

You can find `glooctl` built for every platform in our release artifacts. For example, see our release
assets for v1.0.0: https://github.com/solo-io/gloo/releases/tag/v1.0.0

### Upgrading the Server Components

There are two options for how to perform the server upgrade. Note that these options are not
mutually-exclusive; if you have used one in the past, you can freely choose to use a different one in the future.

Both installation methods allow you to provide overrides for the default chart values; however, installing through
Helm may give you more flexibility as you are working directly with Helm rather than `glooctl`, which, for
installation, is essentially just a wrapper around Helm.
See our [open-source installation docs](../../installation/gateway/kubernetes/#list-of-gloo-helm-chart-values) and
our [enterprise installation docs](../../installation/enterprise/#list-of-gloo-helm-chart-values)
for a complete list of Helm values that can be overridden.

{{% notice note %}}
We create a Kubernetes Job named `gateway-certgen` to generate a cert for the validation webhook.
We attempt to put a `ttlSecondsAfterFinished` value on the job so that it gets cleaned up automatically,
but as this setting is still in Alpha, your cluster may ignore this value. If that is the case, you
may run into an issue while upgrading where the upgrade attempts to change the `gateway-certgen` job,
but the update fails because the job is immutable. If you run into this, simply delete the job, which
should have completed long before, and re-apply the upgrade.
{{% /notice %}}

#### Using `glooctl`

You'll want to use the `glooctl install` command tree, the most common path in which is
`glooctl install gateway`. A good way to proceed in a simple case is a two-step process, which will ensure that
`glooctl`'s version is left matching the server components:

1. Upgrade the `glooctl` binary as described above
1. Run `glooctl install gateway`, which will pull down image versions matching `glooctl`'s version.

All `glooctl` commands can have `--help` appended to them to view helpful usage information.
Some useful flags to be aware of in particular:

* `--dry-run` (`-d`): lets you preview the YAML of the release manifest
* `--namespace` (`-n`): lets you customize the namespace being installed to, which defaults to `gloo-system`
* `--values`: provide a path to a values override file to use when rendering the Helm chart

Here we perform an upgrade from Gloo 1.2.0 to 1.2.1 in our minikube
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
~ > glooctl version
Client: {"version":"1.2.0"}
Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"1.2.0","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"1.2.0","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"1.2.0","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"1.2.0","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
```

```bash
~ > glooctl upgrade --release v1.2.1
downloading glooctl-darwin-amd64 from release tag v1.2.1
successfully downloaded and installed glooctl version v1.2.1 to /usr/local/bin/glooctl
```

```bash
~ > glooctl version
Client: {"version":"1.2.1"}
Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"1.2.0","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"1.2.0","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"1.2.0","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"1.2.0","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
```

```bash
~ > glooctl install gateway --dry-run | grep -o 'quay.*$'
quay.io/solo-io/certgen:1.2.1
quay.io/solo-io/gloo:1.2.1
quay.io/solo-io/discovery:1.2.1
quay.io/solo-io/gateway:1.2.1
quay.io/solo-io/gloo-envoy-wrapper:1.2.1
```

```bash
~ > glooctl install gateway --dry-run | kubectl apply -f - -n gloo-system
# output snipped for brevity
# you may see warnings along the lines of "kubectl apply should be used on resource created by either kubectl create or kubectl apply..."
# these lines are harmless
```

```bash
~ > kubectl -n gloo-system get pod -l gloo=gloo -ojsonpath='{.items[0].spec.containers[0].image}'
quay.io/solo-io/gloo:1.2.1
```

```bash
~ > glooctl check
Checking deployments... OK
Checking pods... OK
Checking upstreams... OK
Checking upstream groups... OK
Checking secrets... OK
Checking virtual services... OK
Checking gateways... OK
Checking proxies... OK
No problems detected.
```

#### Using Helm

{{% notice note %}}
Upgrading through Helm only (i.e., not through `glooctl`) will not ensure that the version of `glooctl` 
matches the server components. You may encounter errors in this state. Be sure to follow the 
["upgrading `glooctl`"](#upgrading-glooctl) steps above to match `glooctl`'s version to the server components. 
{{% /notice %}}

For Enterprise users of Gloo, the process with either Helm 2 or 3 is the same. You'll just need to set your license
key during installation by using `--set license_key="$license"` (or include the line `license_key: LICENSE-KEY` in
your values file).

Get a trial Enterprise license at https://www.solo.io/gloo-trial.

##### Helm 3

If we have Gloo released under the Helm release name `gloo` to `gloo-system`, upgrading the server components is easy:

```shell script
~ > helm upgrade -n gloo-system gloo gloo/gloo --version=v1.2.1
Release "gloo" has been upgraded. Happy Helming!
NAME: gloo
LAST DEPLOYED: Thu Dec 12 12:22:16 2019
NAMESPACE: gloo-system
STATUS: deployed
REVISION: 2
TEST SUITE: None
```

Verify that Gloo has the expected version:

```shell script
~ > kubectl -n gloo-system get pod -l gloo=gloo -ojsonpath='{.items[0].spec.containers[0].image}'
quay.io/solo-io/gloo:1.2.1
```

##### Helm 2

{{% notice warning %}}
Using Helm 2 with open source Gloo v1.2.3 and later or Gloo Enterprise v1.0.0 and later requires explicitly
setting `crds.create=true`, as this is how we are managing compatibility between Helm 2 and 3.
{{% /notice %}}

At the time of writing, Helm v2 [does not support managing CRDs](https://github.com/helm/helm/issues/5871#issuecomment-522096388).
As a result, if you try to upgrade through `helm install` or `helm upgrade`, you may encounter an error
stating that a CRD already exists.

```bash
~ > helm install gloo/gloo
Error: customresourcedefinitions.apiextensions.k8s.io "authconfigs.enterprise.gloo.solo.io" already exists
```

There are two options for resolving this problem. If there have not been changes to the CRDs (such a change
would be mentioned in our changelogs), then you can simply set the Helm value `global.glooRbac.create` to `false`
and skip CRD creation altogether. If there have been changes to the CRDs, then you could either delete the 
CRDs yourself, or you could render the chart yourself and then directly `kubectl apply` it. 
The rest of this section will assume the latter.

```bash
namespace=gloo-system # customize to your namespace
helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm fetch gloo/gloo --version "1.2.1"
```

We will perform the same upgrade of Gloo v1.2.0 to v1.2.1:

```bash
~ > glooctl version
Client: {"version":"1.2.0"}
Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"1.2.0","Name":"discovery","Registry":"quay.io/solo-io"},{"Tag":"1.2.0","Name":"gloo-envoy-wrapper","Registry":"quay.io/solo-io"},{"Tag":"1.2.0","Name":"gateway","Registry":"quay.io/solo-io"},{"Tag":"1.2.0","Name":"gloo","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
~ > kubectl delete job gateway-certgen # if the job has not already been removed by its TTL expiration
job.batch "gateway-certgen" deleted
```

```bash
~ > helm template ./gloo-1.2.1.tgz --namespace gloo-system --set crds.create=true | kubectl apply -f -
configmap/gloo-usage configured
configmap/gateway-proxy-envoy-config unchanged
serviceaccount/gloo unchanged
... # snipped for brevity
gateway.gateway.solo.io/gateway-proxy-ssl unchanged
settings.gloo.solo.io/default unchanged
validatingwebhookconfiguration.admissionregistration.k8s.io/gloo-gateway-validation-webhook-gloo-system configured
```

```bash
~ > kubectl get pod -l gloo=gloo -ojsonpath='{.items[0].spec.containers[0].image}'
quay.io/solo-io/gloo:1.2.1
```
