---
title: Upgrade Steps
weight: 2
description: Steps for upgrading Gloo components
---

In this guide, we'll walk you through how to upgrade Gloo. There are two components that need to be updated:

* [`glooctl`](#upgrading-glooctl)
* [Gloo (control plane)](#upgrading-the-control-plane)

Before upgrading, always make sure to check our changelogs (either 
[open-source](../../changelog/open_source) or [enterprise](../../changelog/enterprise) Gloo)
for any mention of breaking changes. A breaking change may mean a slightly different upgrade procedure; 
if this is the case, then we will take care to explain what must be done in the changelog notes.

## Upgrading Components

After upgrading a component, you should be sure to run `glooctl check` immediately afterwards.
`check` will scan Gloo for problems and report them back to you.

### Upgrading `glooctl`

* note about `glooctl upgrade`
* also point out additional options available to see at `glooctl upgrade --help`

### Upgrading the Control Plane

Two options for how to upgrade:

1. Through glooctl
1. Through helm (need to mention `helm upgrade`? Haven't seen that before). Should mention both rendering the chart directly with helm or using upgrade

## FAQ

1. Is the upgrade procedure any different if I'm playing with Gloo in a non-production/sandbox environment?
1. What is the recommended way to upgrade if I'm running Gloo in a production environment, where downtime is unacceptable?
1. What will happen to my upstreams, virtual services, settings, and Gloo state in general?
1. How do I handle upgrading across a breaking change?
1. Is the upgrade procedure any different if I am not an administrator of the cluster being installed to?


