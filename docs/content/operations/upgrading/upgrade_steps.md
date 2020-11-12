---
title: Upgrade Steps
weight: 10
description: Steps for upgrading Gloo Edge components
---

{{% notice warning %}}
This upgrade process is not suitable in environments where downtime is unacceptable. Depending on your Gloo Edge version, 
configured probes, and external infrastructure (e.g. external load-balancer to Gloo Edge) additional steps may need to be
taken. This guide is targeted toward users that are upgrading Gloo Edge while experimenting in dev or staging environments.
{{% /notice %}}

{{% notice note %}}
This guide assumes that you are running open source Gloo Edge in Kubernetes.
{{% /notice %}}

In this guide, we'll walk you through how to upgrade Gloo Edge. First you'll want to familiarize yourself with our various 
[Changelog entry types]({{% versioned_link_path fromRoot="/reference/changelog/changelog_types/" %}}) so that you can
review the changes that have been made in the release you are upgrading to. 

Once you have reviewed the changes in the new release, there are two components to upgrade:

* [`glooctl`](#upgrading-glooctl)
    * [Using `glooctl` itself](#using-glooctl-itself)
    * [Download release asset](#download-release-asset)
* [Gloo Edge (server components)](#upgrading-the-server-components)
    * [Helm 3](#helm-3)
    * [Helm 2](#helm-2)

Before upgrading, always make sure to check our changelogs (refer to our 
[open-source]({{% versioned_link_path fromRoot="/reference/changelog/open_source/" %}}) or 
[enterprise]({{% versioned_link_path fromRoot="/reference/changelog/enterprise/" %}}) changelogs) for any mention of
breaking changes. In some cases, a breaking change may mean a slightly different upgrade procedure; if this is the
case, then we will take care to explain what must be done in the changelog notes and an upgrade notice in the docs.

You may also want to scan our 
[frequently-asked questions]({{% versioned_link_path fromRoot="/operations/upgrading/faq/" %}}) to see if any of
those cases apply to you. Also feel free to post in the `#gloo` or `#gloo-enterprise` rooms of our
[public Slack](https://slack.solo.io/) if your use case doesn't quite fit the standard upgrade path. 

{{% notice note %}}
We version open-source Gloo Edge separately from Gloo Edge Enterprise. This is because Gloo Edge Enterprise pulls in open-source
Gloo Edge as a dependency. While the patch versions of Gloo Edge and Gloo Edge Enterprise may drift apart from each other, we will
maintain the same major/minor versions across the two projects. So for example, we may be at version `x.y.a` in
open-source Gloo Edge and `x.y.b` in Gloo Edge Enterprise. `x` and `y` will always be the same, but `a` and `b` will often not
be the same. This is why, if you are a Gloo Edge Enterprise user, you may see different versions reported by
`glooctl version`. We will try to ensure that open-source Gloo Edge and Gloo Edge Enterprise will be compatible each other
across patch versions, but make no guarantees about compatibility between minor or major versions.

<br> 

Visit https://semver.org/ for an explanation of semantic versioning if you are unfamiliar with these concepts.

<br>

```bash
~ > glooctl version # snipped some content for brevity
Client: {"version":"0.20.13"} # glooctl is built from open-source Gloo Edge, which is where its version comes from
Server: {"type":"Gateway","enterprise":true,"kubernetes":...,{"Tag":"0.20.8","Name":"grpcserver-ee","Registry":"quay.io/solo-io"},...,{"Tag":"0.20.13","Name":"discovery","Registry":"quay.io/solo-io"},...}

# above we see the Gloo Edge Enterprise API server running enterprise version 0.20.8,
# which has pulled in open-source Gloo Edge 0.20.13 as a dependency.
```

<br>

If you are an open-source user of Gloo Edge, you will only need to be aware of open-source versions found 
[in our open-source changelogs]({{% versioned_link_path fromRoot="/reference/changelog/open_source/" %}}). If you are
an enterprise user of Gloo Edge, you will be selecting versions of Gloo Edge Enterprise from 
[our Enterprise changelogs]({{% versioned_link_path fromRoot="/reference/changelog/enterprise/" %}}). However, you may
need to be aware of the version of open-source Gloo Edge included as a dependency in Gloo Edge Enterprise, as most of our proto
definitions are open-source. Changes to the open-source version will be listed as "Dependency Bumps", and significant
changes may be listed as "Breaking Changes" in our 
[changelog entries]({{% versioned_link_path fromRoot="/reference/changelog/changelog_types/" %}}).
{{% /notice %}}

## Upgrading Components

After upgrading a component, you should be sure to run `glooctl check` immediately afterwards. `glooctl check` will
scan Gloo Edge for problems and report them back to you. A problem reported by `glooctl check` means that Gloo Edge is not
working properly and that Envoy may not be receiving updated configuration.

### Upgrading `glooctl`

{{% notice note %}}
It is important to try to keep the version of `glooctl` in alignment with the version of the Gloo Edge server components
running in your cluster. Because `glooctl` can create resources in your cluster (for example, with 
`glooctl add route`), you may see errors in Gloo Edge if you create resources from a version of `glooctl` that is
incompatible with the version of the server components.
{{% /notice %}}

#### Using `glooctl` Itself

The easiest way to upgrade `glooctl` is to simply run `glooctl upgrade`, which will attempt to download the latest
binary. There are more fine-grained options available; those can be viewed by running `glooctl upgrade --help`. One in
particular to make note of is `glooctl upgrade --release`, which can be useful in maintaining careful control over
what version you are running.

Here is an example where we notice we have a version mismatch between `glooctl` and the version of Gloo Edge running in our
minikube cluster (1.2.0 and 1.2.1 respectively), and we correct it:

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

You can find `glooctl` built for every platform in our release artifacts. For example, see our release assets for
v1.0.0: https://github.com/solo-io/gloo/releases/tag/v1.0.0

### Upgrading the Server Components

{{% notice note %}}
We create a Kubernetes Job named `gateway-certgen` to generate a cert for the validation webhook. We attempt to put a
`ttlSecondsAfterFinished` value on the job so that it gets cleaned up automatically, but as this setting is still in
Alpha, your cluster may ignore this value. If that is the case, you may run into an issue while upgrading where the
upgrade attempts to change the `gateway-certgen` job, but the update fails because the job is immutable. If you run
into this, simply delete the job, which should have completed long before, and re-apply the upgrade.
{{% /notice %}}

#### Recommended settings to avoid downtime
If gloo is not running in kubernetes and using the kubernetes load-balancer, then properly configure 
[health checks]({{< versioned_link_path fromRoot="/guides/traffic_management/request_processing/health_checks" >}})
on envoy and configure your load-balancer to leverage these health checks, so requests stop going to envoy once it
begins draining connections.

If gloo is running in kubernetes and using the kubernetes load-balancer, enable envoy readiness and liveness probes 
during the upgrade. This will instruct kubernetes to send requests exclusively to the healthy envoy during upgrade,
preventing potential downtime. The probes are not enabled in default installations because they can provide for a poor
getting started experience. The following example upgrade assumes you're running in kubernetes with the kubernetes
load-balancer.

#### Using Helm

For Enterprise users of Gloo Edge, the process with either Helm 2 or 3 is the same. You'll just need to set your license
key during installation by using `--set license_key="$license"` (or include the line `license_key: $LICENSE-KEY` in
your values file).

Get a trial Enterprise license at https://www.solo.io/gloo-trial.

{{% notice note %}}
While it is possible to upgrade from open source Gloo Edge to Gloo Edge Enterprise using helm upgrade, you may have to take
additional steps to ensure there is no downtime because the charts do not have the exact same helm values.
{{% /notice %}}

##### Helm 3

If we have Gloo Edge released under the Helm release name `gloo` to `gloo-system`, upgrading the server components is easy:

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

Verify that Gloo Edge has the expected version:

```shell script
~ > kubectl -n gloo-system get pod -l gloo=gloo -ojsonpath='{.items[0].spec.containers[0].image}'
quay.io/solo-io/gloo:1.2.1
```

##### Helm 2

{{% notice warning %}}
Using Helm 2 with open source Gloo Edge v1.2.3 and later or Gloo Edge Enterprise v1.2.0 and later requires explicitly setting
`crds.create=true` on the first install, as this is how we are managing compatibility between Helm 2 and 3.
{{% /notice %}}

Helm upgrade command should just work:
```bash
~ > helm upgrade gloo gloo/gloo --namespace gloo-system
```

If you'd rather delete and reinstall to get a clean slate, take care to manage your CRDs as Helm v2 
[does not support managing CRDs](https://github.com/helm/helm/issues/5871#issuecomment-522096388). As a result, if you
try to upgrade through deleting a helm release (`helm delete --purge gloo`) and reinstalling via `helm install`, you
may encounter an error stating that a CRD already exists.

```bash
~ > helm install gloo/gloo --name gloo --namespace gloo-system --set crds.create=true
Error: customresourcedefinitions.apiextensions.k8s.io "authconfigs.enterprise.gloo.solo.io" already exists
```

To successfully reinstall, run the same install command with `crds.create=false`.