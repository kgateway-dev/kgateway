---
title: Gloo Federation
weight: 65
---

Gloo Federation allows users to manage the configuration for all of their Gloo instances from one place, no matter what platform they run on. In addition Gloo Federation elevates Gloo’s powerful routing features beyond the environment they live in, allowing users to create all new global routing features between different Gloo instances. Gloo Federation enables consistent configuration, service failover, unified debugging, and automated Gloo discovery across all of your Gloo instances.

Gloo Federation is installed using the `glooctl` command line tool or a Helm chart. The following docuement will take you through the process of performing the installation of Gloo Federation, verifying the components, and removing Gloo Federation if necessary.

## Prerequisites

Gloo Federation is an enterprise feature of Gloo. You will need at least one instance of Gloo Enterprise running on a Kubernetes cluster to follow the installation guide. Full details on setting up your Kubernetes cluster are available [here]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" %}}) and installing Gloo Enterprise [here]({{% versioned_link_path fromRoot="/installation/enterprise/" %}}).

You should also have `glooctl` and `kubectl` installed. The `glooctl` version should be the most recent release, as the federation features were added in version 1.4.

You will also need a license key to install Gloo Federation. The key can be procured by visting this page on the Solo.io website.

## Installation

Gloo Federation is installed in an administrative cluster, which may or may not include Gloo instances. The `glooctl` tool uses Helm to perform the deployment of Gloo Federation. By default, the deployment will create the `gloo-fed` namespace and instantiate the Gloo Federation components in that namespace. Additional information and flags can be found by running `glooctl install federation -h`.

With your kubectl context set to the administrative cluster, run the following command:

```
glooctl install federation --license-key <LICENSE_KEY>
```

Make sure to change the placeholder `<LICENSE_KEY>` to the license key you have procured for Gloo Federation.

The installation will create the necessary Kubernetes components for running Gloo Federation.

## Verification

Once the deployment is complete, you can validate the installation by checking on the status of a few components. The following command will show you the status of the deployment itself:

```
kubectl -n gloo-fed rollout status deployment gloo-fed --timeout=1m
```

You should see output similar to the following:

```

```

You can also view the resources in the `gloo-fed` namespace by running:

```
kubectl get all -n gloo-fed
```

You should see output similar to the following, with all pods running successfully.

```

```

There are also a number fo Custom Resource Definitions that can be viewed by running:

```
kubectl get crds -l app=gloo-fed
```

You should see the following list:

```

```

Your instance of Gloo Federation has now been successfully deployed. The next step is to register clusters with Gloo Federation.

## Next Steps

As a next step, we recommend registering the Kubernetes clusters running Gloo instances with Gloo Federation. Then you can move onto creating federated configurations or service failover. You can also read more about Gloo Federation in the concepts area of the docs.