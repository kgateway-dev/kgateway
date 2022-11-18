---
title: "Gloo Edge for Knative"
description: How to install Gloo Edge to run in Knative Mode on Kubernetes.
weight: 40
---

{{% notice warning %}}
Gloo Edge Knative Ingress is deprecated in 1.10 and will not be available in 1.11
{{% /notice %}}

For the purpose of running Knative, Gloo Edge can function as a complete replacement for Istio (supporting all documented Knative features), requiring less resource usage and operational overhead. 

This guide walks you through installing Gloo Edge and Knative using either `glooctl` (the Gloo Edge command line) or Helm. 

{{% notice note %}}
`glooctl` generates a manifest which can be piped to stdout or a file using the `--dry-run` flag. Alternatively, Gloo Edge can be installed via its [Helm Chart]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes#installing-on-kubernetes-with-helm" >}}), which will permit fine-grained configuration of installation parameters.
{{% /notice %}}

---

## Installing the Gloo Edge Knative Ingress on Kubernetes

These directions assume you've prepared your Kubernetes cluster appropriately. Full details on setting up your Kubernetes cluster can be found [here]({{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" >}}). You can install Gloo Edge Knative Ingress using using `glooctl` or through Helm. Helm is the recommended method for installing in a Production environment.

### Installing on Kubernetes with `glooctl`

Before you begin, make sure that you [install `glooctl`]({{< versioned_link_path fromRoot="/installation/preparation/" >}}), the Gloo Edge command line tool (CLI).

Using `glooctl` will install Knative Serving components to the `knative-serving` namespace if it does not already exist in your cluster and install Gloo Edge's Knative Ingress. The Knative installation is a modified version of the Knative Serving manifest with the dependencies on Istio removed. 

If you will be installing Gloo Edge Knative Ingress on a cluster that already has the Knative Serving components deployed, you can use the flag `--install-knative=false` to skip the Knative installation. More details can be found in the [Knative installation guide](https://knative.dev/docs/install/any-kubernetes-cluster/).

Once your Kubernetes cluster is up and running, run the following command to deploy the Gloo Edge Ingress to the `gloo-system` namespace and Knative-Serving components to the `knative-serving` namespace:

```bash
glooctl install knative
```

{{% notice note %}}
You can run the command with the flag `--dry-run` to output the Kubernetes manifests (as `yaml`) that `glooctl` will apply to the cluster instead of installing them.
{{% /notice %}}

### Installing on Kubernetes with Helm

This is the recommended method for installing Gloo Edge to your production environment as it offers rich customization to the Gloo Edge control plane and the proxies Gloo Edge manages. This guide assumes that you are using Helm version 3, and have already installed the Helm client on your local machine.

First, make sure you have Knative installed. If you do not, you can install Knative components without Gloo Edge using `glooctl`:

```shell script
glooctl install knative -g
```

Once the installation is complete, you can validate by checking the namespace `knative-serving`.

```shell
kubectl get all -n knative-serving
```

Now let's install Gloo Edge. If needed, add the Gloo Edge repository to the list of known chart repositories and perform a repository update:

```shell
helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm repo update
```

In the values for the Helm chart, you are going to set `gateway.enabled` to `false`, `settings.integrations.knative.enabled` to `true`, and configure the version of Knative at the path `settings.integrations.knative.version`. You can do this either by creating a `values.yaml` file with the proper settings or by defining the settings in line.

First you need to know what version of Knative you are running. You can find this by running the following command:

```console
kubectl describe namespace knative-serving
```

In the output, look for the `version` value in the Annotations section:

```noop
Name:         knative-serving
Labels:       istio-injection=enabled
              serving.knative.dev/release=v0.10.0
Annotations:  gloo.solo.io/glooctl_install_info: {"version":"0.10.0","monitoring":false,"eventing":false,"eventingVersion":"0.10.0"}
              kubectl.kubernetes.io/last-applied-configuration:
                {"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{},"labels":{"istio-injection":"enabled","serving.knative.dev/release":"v0...
Status:       Active
```

In our case, we are running `v0.10.0` of Knative.

#### Install using a values.yaml file

If you would like to define the settings with a file, create a `values.yaml` file with the following overrides:

```yaml
gateway:
  enabled: false
settings:
  integrations:
    knative:
      enabled: true
      version: {{ . }}  # put installed knative version here!
```

For our example, we would Replace the `{{ . }}` with `v0.10.0`. 

Save the file and then run the following commands to install the Gloo Edge components.

```shell
helm install gloo gloo/gloo --namespace gloo-system --create-namespace -f values.yaml
```

Gloo Edge can be installed to a namespace of your choosing with the `--namespace` flag.

#### Install using in-line settings

Instead of creating a `values.yaml` file, you can simply define the settings in-line. This is useful for a small number of values, but quickly becomes impractical if you want to override several values.

Run the following commands to install the Gloo Edge components with version `v0.10.0` of Knative.

```shell
helm install gloo gloo/gloo --namespace gloo-system --create-namespace \
  --set gateway.enabled=false,settings.integrations.knative.enabled=true,settings.integrations.knative.version=v0.10.0
```

---

## Verify your Installation

Check that the Gloo Edge pods and services have been created. Depending on your install options, you may see some differences from the following example. And if you choose to install Gloo Edge into a different namespace than the default `gloo-system`, then you will need to query your chosen namespace instead.

```shell
kubectl get all -n gloo-system
```

```noop
NAME                                          READY   STATUS    RESTARTS   AGE
pod/discovery-7b6684f57d-ldcvx                1/1     Running   0          73m
pod/gloo-6658f49f64-9zlgh                     1/1     Running   5          73m
pod/ingress-5476d956c7-rrr6m                  1/1     Running   0          73m
pod/knative-external-proxy-fdfc894fb-6w4m9    1/1     Running   0          73m
pod/knative-internal-proxy-745c9f6f86-t7h5j   1/1     Running   0          73m

NAME                             TYPE           CLUSTER-IP     EXTERNAL-IP     PORT(S)                      AGE
service/gloo                     ClusterIP      10.7.246.232   <none>          9977/TCP                     73m
service/knative-external-proxy   LoadBalancer   10.7.247.23    35.188.41.169   80:30388/TCP,443:32060/TCP   73m
service/knative-internal-proxy   ClusterIP      10.7.243.248   <none>          80/TCP,443/TCP               73m

NAME                                     READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/discovery                1/1     1            1           73m
deployment.apps/gloo                     1/1     1            1           73m
deployment.apps/ingress                  1/1     1            1           73m
deployment.apps/knative-external-proxy   1/1     1            1           73m
deployment.apps/knative-internal-proxy   1/1     1            1           73m

NAME                                                DESIRED   CURRENT   READY   AGE
replicaset.apps/discovery-7b6684f57d                1         1         1       73m
replicaset.apps/gloo-6658f49f64                     1         1         1       73m
replicaset.apps/ingress-5476d956c7                  1         1         1       73m
replicaset.apps/knative-external-proxy-fdfc894fb    1         1         1       73m
replicaset.apps/knative-internal-proxy-745c9f6f86   1         1         1       73m
```

---

## Uninstall {#uninstall}

To uninstall Gloo Edge and all related components, simply run the following.

{{% notice note %}}
This will also remove Knative-Serving, if it was installed by `glooctl`.
{{% /notice %}}

```shell
glooctl uninstall
```

If you installed Gloo Edge to a different namespace, you will have to specify that namespace using the `-n` option:

```shell
glooctl uninstall -n my-namespace
```

---

## Next Steps

To begin using Gloo Edge with Knative, check out the [Knative Getting Started Guide]({{< versioned_link_path fromRoot="/guides/integrations/knative/getting_started/" >}}).
