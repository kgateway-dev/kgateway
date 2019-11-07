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

Before upgrading, always make sure to check our changelogs (either 
[open-source](../../changelog/open_source) or [enterprise](../../changelog/enterprise) Gloo)
for any mention of breaking changes. In some cases, a breaking change may mean a slightly different upgrade 
procedure; if this is the case, then we will take care to explain what must be done in the changelog notes.

You may also want to scan our [frequently-asked questions](#faq) to see if any of those cases apply to you.
Also feel free to post in the `#gloo` or `#gloo-enterprise` rooms of our 
[public Slack](https://slack.solo.io/) if your use case doesn't quite fit the standard upgrade path.

### Upgrading Components

After upgrading a component, you should be sure to run `glooctl check` immediately afterwards.
`glooctl check` will scan Gloo for problems and report them back to you. A problem reported by
`glooctl check` means that Gloo is not working properly and that Envoy may not be receiving updated
configuration.

#### Upgrading `glooctl`

The easiest way to upgrade `glooctl` is to simply run `glooctl upgrade`, which will attempt to download
the latest binary. There are more fine-grained options available; those can be viewed by running
`glooctl upgrade --help`.

It is important to try to keep the version of `glooctl` in alignment with the version of the Gloo
control-plane running in your cluster.

#### Upgrading the Control Plane

Two options for how to upgrade:

1. Through glooctl
1. Through helm (need to mention `helm upgrade`? Haven't seen that before). Should mention both rendering the chart directly with helm or using upgrade

### FAQ

1. Is the upgrade procedure any different if I'm playing with Gloo in a non-production/sandbox environment?
1. What is the recommended way to upgrade if I'm running Gloo in a production environment, where downtime is unacceptable?
1. What will happen to my upstreams, virtual services, settings, and Gloo state in general?
1. How do I handle upgrading across a breaking change?
1. Is the upgrade procedure any different if I am not an administrator of the cluster being installed to?


