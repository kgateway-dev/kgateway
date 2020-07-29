---
title: Read-only Console
description: Accessing the Gloo Federation read-only console
weight: 50
---

The installation of Gloo Federation includes a read-only console, providing a wealth of information about the Gloo instances being managed by Gloo Federation available at a glance. This guide will take you through the process of accessing the console and show you some of the features of the interface.

## Prerequisites

To successfully follow this guide, you will need the following software available and configured on your system.

* Kubectl - Used to execute commands against the clusters
* Glooctl - Used to register the Kubernetes clusters with Gloo Federation
* [Kind](https://kind.sigs.k8s.io/) - Required if using the `glooctl` federation demo environment
* Docker - Required if using the `glooctl` federation demo environment

In this guide we are going to use the Gloo Federation environment available from the `glooctl demo federation` command. You can follow the directions in the [Getting Started guide]({{% versioned_link_path fromRoot="/guides/gloo_federation/getting_started/" %}}) to set up the demonstration environment. Otherwise, you will need at least one Kubernetes cluster running Gloo Enterprise and Gloo Federation.

For the purposes of this example, we have two clusters `local` and `remote`. The local cluster is also running Gloo Federation in addition to Gloo Enterprise. The kubectl context for the local cluster is `kind-local` and the remote cluster is `kind-remote`.

## Configure access to the console

The Gloo Federation console is exposed by the `gloo-fed-console` service running in the `gloo-fed` namespace. The console is available on port 8090. In a production scenario, you could choose to create a new service exposing the console on an IP address available outside the cluster. For our example, we are going to port-forward the service to the local IP address of the machine running the demonstration environment.

```bash
# Get the local IP address
LOCAL_IP=$(ip a | awk '/inet / { print $2 }' | sed -n 2p | cut -d "/" -f1)

# Launch the port-forward for port 8090
kubectl port-forward svc/gloo-fed-console -n gloo-fed 8090:8090 --address=$LOCAL_IP
```

You should now be able to access the read-only console from the machine's local ip address.

## Exploring the console

### Overview and Gloo instances

When the console initially loads, you will see an overview page providing you with the overall status of your Gloo instances, clusters, Virtual Services, and Upstreams.

![Console Overview]({{% versioned_link_path fromRoot="/img/gloo-fed-console-overview.png" %}})

You can get more information about your Gloo instances by clicking on the **Gloo Instances** item in the navigation bar.

![Gloo Instances Nav Bar]({{% versioned_link_path fromRoot="/img/gloo-fed-console-instances-nav-bar.png" %}})

The Gloo Instances page will show you the status of each Gloo instance, including the cluster, namespace, and version of the Gloo instance. 

![Gloo Instances Overview]({{% versioned_link_path fromRoot="/img/gloo-fed-console-instances-overview.png" %}})

The Gloo instance view also provides you with a snapshot of the Virtual Services and Upstreams available on the Gloo instance. You can drill down for more information by clicking on **View Gloo Details**. The details view provides more information about each resource broken up across tabs.

![Gloo Instances Details]({{% versioned_link_path fromRoot="/img/gloo-fed-console-instances-details.png" %}})

If you would like to see further details about a particular resource, you can click on the resource name in the list. For instance, here are the details on the `fed-upstream` resource.

![Gloo Upstream Details]({{% versioned_link_path fromRoot="/img/gloo-fed-console-upstream-details.png" %}})

From this page you can download the yaml for the resource configuration, or view the configuration in the browser.

### Admin settings for a Gloo instance

From the **Gloo Instances** menu item, you will see a list of each Gloo Instance managed by Gloo Federation. 

![Gloo Instances Nav Bar]({{% versioned_link_path fromRoot="/img/gloo-fed-console-instances-nav-bar.png" %}})

You can click on **View Now** in the *Admin Settings* section of each instance to see more about Gateways, Proxies, and Gloo settings.

![Gloo Instances Admin Settings]({{% versioned_link_path fromRoot="/img/gloo-fed-console-instances-admin-link.png" %}})

The *Admin Settings* view will show the status of each Gateway and Proxy configuration associated with the Gloo instance. You can also view the general Gloo instance settings from this view.

![Gloo Instances Admin Settings]({{% versioned_link_path fromRoot="/img/gloo-fed-console-instances-admin-settings.png" %}})

You can drill down into each resource type by clicking on the link at the bottom of each setting. The **View Gateways** link, will take you to a view of the configured gateways on the Gloo instance and show the raw yaml configuration for each gateway.

![Gloo Instances Gateway Settings]({{% versioned_link_path fromRoot="/img/gloo-fed-console-instances-gateway-settings.png" %}})

From here, you can use the buttons on the side to view the Proxy configurations or the general settings for the Gloo instances.

![Gloo Instances Admin Settings]({{% versioned_link_path fromRoot="/img/gloo-fed-console-instances-admin-menu.png" %}})

### Exploring Virtual Services and Upstreams

There are two more menu items in the navigation bar, Virtual Services and Upstreams.

![Gloo Console Nav Bar]({{% versioned_link_path fromRoot="/img/gloo-fed-console-virtual-services-nav-bar.png" %}})

Clicking on the **Virtual Services** menu item will show us a unified view of all Virtual Services running across all managed Gloo instances:

![Gloo Virtual Services]({{% versioned_link_path fromRoot="/img/gloo-fed-console-virtual-services.png" %}})

You can search for a particular Virtual Service or filter on properties like *Accepted*, *Rejected*, or *Pending*. 

Clicking on the **Upstreams** menu item will show us a unified view of all Upstreams running across all managed Gloo instances:

![Gloo Upstreams]({{% versioned_link_path fromRoot="/img/gloo-fed-console-upstreams.png" %}})

Similar to the Virtual Services view, you can search for a particular Upstream or filter on properties like *Accepted*, *Rejected*, or *Pending*.

## Next steps

The read-only console provides a powerful view into the status of all your managed Gloo instances. If you'd like to see how to make changes that will be reflected in the console, we recommend following the [Federated Configuration]({{% versioned_link_path fromRoot="/guides/gloo_federation/federated_configuration/" %}}) or [Service Failover]({{% versioned_link_path fromRoot="/guides/gloo_federation/service_failover/" %}}) guides.