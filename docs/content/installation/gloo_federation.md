---
title: Gloo Edge Federation
weight: 65
---

Gloo Edge Federation allows users to manage the configuration for all of their Gloo Edge instances from one place, no matter what platform they run on. In addition Gloo Edge Federation elevates Gloo Edge’s powerful routing features beyond the environment they live in, allowing users to create all new global routing features between different Gloo Edge instances. Gloo Edge Federation enables consistent configuration, service failover, unified debugging, and automated Gloo Edge discovery across all of your Gloo Edge instances.

Gloo Edge Federation is installed using the `glooctl` command line tool or a Helm chart. The following document will take you through the process of performing the installation of Gloo Edge Federation, verifying the components, and removing Gloo Edge Federation if necessary.

## Prerequisites

Gloo Edge Federation is an enterprise feature of Gloo Edge. You will need at least one instance of Gloo Edge Enterprise running on a Kubernetes cluster to follow the installation guide. Full details on setting up your Kubernetes cluster are available [here]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" %}}) and installing Gloo Edge Enterprise [here]({{% versioned_link_path fromRoot="/installation/enterprise/" %}}).

You should also have `glooctl` and `kubectl` installed. The `glooctl` version should be the most recent release, as the federation features were added in version 1.5. 

You also need a license key to install Gloo Edge Federation. To request a license, [contact Sales](https://www.solo.io/company/contact/).

## Installation

Gloo Edge Federation is installed in an admin cluster, which may or may not include Gloo Edge instances. You can perform the installation using `glooctl` or helm. 

### Install using glooctl

The `glooctl` tool uses Helm in the background to perform the deployment of Gloo Edge Federation. By default, the deployment will create the `gloo-system` namespace and instantiate the Gloo Edge Federation components in that namespace.  You can override the default settings by specify the `--values` argument and providing a yaml file with the necessary values.

Gloo Edge Federation is installed alongside Gloo Enterprise automatically. With your kubectl context set to the admin cluster, run the following command:

```
glooctl install gateway enterprise --license-key <LICENSE_KEY>
```

The `--with-gloo-fed=false` flag can be used to install only Gloo Enterprise without Gloo Edge Federation.

Make sure to change the placeholder `<LICENSE_KEY>` to the license key you have procured for Gloo Edge Enterprise.

The installation will create the necessary Kubernetes components for running Gloo Edge Federation.

### Install using Helm

You can also install Gloo Edge Federation by using Helm directly. The default values of the chart can be overridden by using the `--set` argument.

With your kubectl context set to the admin cluster, run the following commands:

```bash
# Add the gloo-fed helm repo
helm repo add gloo-fed https://storage.googleapis.com/gloo-fed-helm

# Update your repos 
helm repo update

# Install using helm
helm install -n gloo-system --create-namespace gloo-fed gloo-fed/gloo-fed --set license_key=<LICENSE_KEY>
```

Make sure to change the placeholder `<LICENSE_KEY>` to the license key you have procured for Gloo Edge Federation.

The installation will create the necessary Kubernetes components for running Gloo Edge Federation.

## Verification

Once the deployment is complete, you can validate the installation by checking on the status of a few components. The following command will show you the status of the deployment itself:

```
kubectl -n gloo-system rollout status deployment gloo-fed
```

You should see output similar to the following:

```
deployment "gloo-fed" successfully rolled out
```

You can also view the resources in the `gloo-system` namespace by running:

```
kubectl get all -n gloo-system
```

You should see output similar to the following, with all pods running successfully.

```
NAME                                    READY   STATUS              RESTARTS   AGE
pod/gloo-fed-956f66f75-mwk24            1/1     Running             0          11h
pod/gloo-fed-console-78f6f4f696-hckw8   3/3     Running             0          11h

NAME                       TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)                       AGE
service/gloo-fed-console   ClusterIP   10.109.209.107   <none>        10101/TCP,8090/TCP,8081/TCP   11h

NAME                               READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/gloo-fed           1/1     1            1           11h
deployment.apps/gloo-fed-console   1/1     1            1           11h

NAME                                          DESIRED   CURRENT   READY   AGE
replicaset.apps/gloo-fed-956f66f75            1         1         1       11h
replicaset.apps/gloo-fed-console-78f6f4f696   1         1         1       11h
```

There are also a number of Custom Resource Definitions that can be viewed by running:

```
kubectl get crds -l app=gloo-fed
```

You should see the following list:

```
NAME                                               CREATED AT
failoverschemes.fed.solo.io                        2021-06-18T17:18:42Z
federatedauthconfigs.fed.enterprise.gloo.solo.io   2021-06-18T17:18:42Z
federatedgateways.fed.gateway.solo.io              2021-06-18T17:18:42Z
federatedratelimitconfigs.fed.ratelimit.solo.io    2021-06-18T17:18:42Z
federatedroutetables.fed.gateway.solo.io           2021-06-18T17:18:42Z
federatedsettings.fed.gloo.solo.io                 2021-06-18T17:18:42Z
federatedupstreamgroups.fed.gloo.solo.io           2021-06-18T17:18:42Z
federatedupstreams.fed.gloo.solo.io                2021-06-18T17:18:42Z
federatedvirtualservices.fed.gateway.solo.io       2021-06-18T17:18:42Z
glooinstances.fed.solo.io                          2021-06-18T17:18:42Z
multiclusterrolebindings.multicluster.solo.io      2021-06-18T17:18:42Z
multiclusterroles.multicluster.solo.io             2021-06-18T17:18:42Z
```

Your instance of Gloo Edge Federation has now been successfully deployed. The next step is to register clusters with Gloo Edge Federation.

## Uninstall {#uninstall}

To uninstall Gloo Edge Enterprise, Federation and all related components, simply run the following.

```shell
glooctl uninstall --all
```

## Next Steps

As a next step, we recommend [registering the Kubernetes clusters]({{% versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" %}}) running Gloo Edge instances with Gloo Edge Federation. Then you can move onto creating [federated configurations]({{% versioned_link_path fromRoot="/guides/gloo_federation/federated_configuration/" %}}) or [service failover]({{% versioned_link_path fromRoot="/guides/gloo_federation/service_failover/" %}}). You can also read more about Gloo Edge Federation in the [concepts area]({{% versioned_link_path fromRoot="/introduction/gloo_federation/" %}}) of the docs.

{{< readfile file="static/content/upgrade-note.md" markdown="true">}}
