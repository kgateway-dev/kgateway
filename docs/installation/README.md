---
title: Installation
weight: 5
chapter: true
---
# Installing Gloo

## 1. Install Glooctl

If this is your first time running Gloo, you’ll need to download the command-line interface (CLI) onto your local machine. 
You’ll use this CLI to interact with Gloo, including installing it onto your Kubernetes cluster.

To install the CLI, run:

`curl -sL https://run.solo.io/gloo/install | sh`

Alternatively, you can download the CLI directly via the github releases page. 

Next, add Gloo to your path with:

`export PATH=$HOME/.gloo/bin:$PATH`

Verify the CLI is installed and running correctly with:

`glooctl --version`

## 2. Choosing a deployment option

There currently exist several options for deploying Gloo depending on your use case and 
deployment platform.

- [*Gateway*](#gateway): Gloo's full feature set is 
available via its v1/Gateway API. The Gateway API is modeled on Envoy's own API with the use of opinionated defaults 
to make complex configurations possible, while maintaining simplicity where desired.

- [*Ingress*](#ingress): Gloo will support configuration the Kubernetes Ingress resource, acting as a Kubernetes Ingress Controller. 
Note that ingress objects must have the annotation `"kubernetes.io/ingress.class": "gloo"` to be processed by the Gloo Ingress.

- [*Knative*](#knative): Gloo will integrate 
automatically with Knative as a cluster-level ingress for [*Knative-Serving*](https://github.com/knative/serving). 
Gloo can be used in this way as a lightweight replacement for Istio when using Knative-Serving.


<a name="gateway"></a> 
### 2a. Install the Gloo Gateway to your Kubernetes Cluster using Glooctl
        
Once your Kubernetes cluster is up and running, run the following command to deploy the Gloo Gateway to the `gloo-system` namespace:

```bash
glooctl install gateway 
```

Check that the Gloo pods and services have been created:

```bash
kubectl get all -n gloo-system

NAME                                READY     STATUS    RESTARTS   AGE
pod/discovery-f7548d984-slddk       1/1       Running   0          5m
pod/gateway-5689fd59d7-wsg7f        1/1       Running   0          5m
pod/gateway-proxy-9d79d48cd-wg8b8   1/1       Running   0          5m
pod/gloo-5b7b748dbf-jdsvg           1/1       Running   0          5m

NAME                    TYPE           CLUSTER-IP      EXTERNAL-IP   PORT(S)          AGE
service/gateway-proxy   LoadBalancer   10.97.232.107   <pending>     8080:31800/TCP   5m
service/gloo            ClusterIP      10.100.64.166   <none>        9977/TCP         5m

NAME                            DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/discovery       1         1         1            1           5m
deployment.apps/gateway         1         1         1            1           5m
deployment.apps/gateway-proxy   1         1         1            1           5m
deployment.apps/gloo            1         1         1            1           5m

NAME                                      DESIRED   CURRENT   READY     AGE
replicaset.apps/discovery-f7548d984       1         1         1         5m
replicaset.apps/gateway-5689fd59d7        1         1         1         5m
replicaset.apps/gateway-proxy-9d79d48cd   1         1         1         5m
replicaset.apps/gloo-5b7b748dbf           1         1         1         5m
```


See [Getting Started on Kubernetes](../getting_started/kubernetes/basic_routing.md) to get started using the Gloo Gateway.

<a name="ingress"></a> 
### 2b. Install the Gloo Ingress Controller to your Kubernetes Cluster using Glooctl

Once your Kubernetes cluster is up and running, run the following command to deploy the Gloo Ingress to the `gloo-system` namespace:

```bash
glooctl install ingress 
```

Check that the Gloo pods and services have been created:

```bash
kubectl get all -n gloo-system

NAME                                READY     STATUS    RESTARTS   AGE
pod/discovery-f7548d984-lfhsz       1/1       Running   0          3s
pod/gloo-5b7b748dbf-vtjvx           1/1       Running   0          4s
pod/ingress-9c59ffc64-ndsj9         1/1       Running   0          4s
pod/ingress-proxy-7b676c5b7-tqnlq   1/1       Running   0          4s

NAME                    TYPE           CLUSTER-IP       EXTERNAL-IP   PORT(S)                      AGE
service/gloo            ClusterIP      10.101.127.201   <none>        9977/TCP                     4s
service/ingress-proxy   LoadBalancer   10.106.91.246    <pending>     80:30999/TCP,443:31628/TCP   4s

NAME                            DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/discovery       1         1         1            1           4s
deployment.apps/gloo            1         1         1            1           4s
deployment.apps/ingress         1         1         1            1           4s
deployment.apps/ingress-proxy   1         1         1            1           4s

NAME                                      DESIRED   CURRENT   READY     AGE
replicaset.apps/discovery-f7548d984       1         1         1         4s
replicaset.apps/gloo-5b7b748dbf           1         1         1         4s
replicaset.apps/ingress-9c59ffc64         1         1         1         4s
replicaset.apps/ingress-proxy-7b676c5b7   1         1         1         4s

```

See [Getting Started with Kubernetes Ingress](../getting_started/kubernetes/basic_ingress.md) to get started using the Gloo Ingress Controller.
        

<a name="knative"></a> 
### 2c. Install the Gloo Knative Cluster Ingress to your Kubernetes Cluster using Glooctl

 
Once your Kubernetes cluster is up and running, run the following command to deploy Knative-Serving components to the `knative-serving` namespace and Gloo to the `gloo-system` namespace:

`glooctl install knative`


Check that the Gloo and Knative pods and services have been created:

```bash
kubectl get all -n gloo-system

NAME                                       READY     STATUS    RESTARTS   AGE
pod/clusteringress-proxy-cc5c6db57-2jtgd   1/1       Running   0          13s
pod/discovery-f7548d984-lqj6t              1/1       Running   0          13s
pod/gloo-5b7b748dbf-g42cg                  1/1       Running   0          13s
pod/ingress-54fcb854f9-z5bmv               1/1       Running   0          13s

NAME                           TYPE           CLUSTER-IP       EXTERNAL-IP   PORT(S)                      AGE
service/clusteringress-proxy   LoadBalancer   10.106.92.134    <pending>     80:30602/TCP,443:31006/TCP   14s
service/gloo                   ClusterIP      10.111.161.176   <none>        9977/TCP                     14s

NAME                                   DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/clusteringress-proxy   1         1         1            1           14s
deployment.apps/discovery              1         1         1            1           14s
deployment.apps/gloo                   1         1         1            1           14s
deployment.apps/ingress                1         1         1            1           14s

NAME                                                DESIRED   CURRENT   READY     AGE
replicaset.apps/clusteringress-proxy-cc5c6db57   1         1         1         14s
replicaset.apps/discovery-f7548d984              1         1         1         14s
replicaset.apps/gloo-5b7b748dbf                  1         1         1         14s
replicaset.apps/ingress-54fcb854f9               1         1         1         14s


```

```bash
kubectl get all -n knative-serving

NAME                              READY     STATUS    RESTARTS   AGE
pod/activator-5c8d977d45-6x9s4    1/1       Running   0          2m
pod/autoscaler-5cd4bb6dbc-kwt4q   1/1       Running   0          2m
pod/controller-66cd7d99df-c9fnx   1/1       Running   0          30s
pod/webhook-6d9568d-q27vv         1/1       Running   0          2m

NAME                        TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)             AGE
service/activator-service   ClusterIP   10.104.212.24   <none>        80/TCP,9090/TCP     2m
service/autoscaler          ClusterIP   10.98.232.40    <none>        8080/TCP,9090/TCP   2m
service/controller          ClusterIP   10.102.58.151   <none>        9090/TCP            2m
service/webhook             ClusterIP   10.106.233.95   <none>        443/TCP             2m

NAME                         DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/activator    1         1         1            1           2m
deployment.apps/autoscaler   1         1         1            1           2m
deployment.apps/controller   1         1         1            1           2m
deployment.apps/webhook      1         1         1            1           2m

NAME                                    DESIRED   CURRENT   READY     AGE
replicaset.apps/activator-5c8d977d45    1         1         1         2m
replicaset.apps/autoscaler-5cd4bb6dbc   1         1         1         2m
replicaset.apps/controller-66cd7d99df   1         1         1         2m
replicaset.apps/webhook-6d9568d         1         1         1         2m

NAME                                                 AGE
image.caching.internal.knative.dev/fluentd-sidecar   2m
image.caching.internal.knative.dev/queue-proxy       2m
```

See [Getting Started with Gloo and Knative](../getting_started/kubernetes/gloo_with_knative.md) to get started using Gloo as your Knative Ingress.

### Next steps

Everything should be up and running. If this process does not work, please [open an issue](https://github.com/solo-io/gloo/issues/new). We are happy to answer
questions on our [diligently staffed Slack channel](https://slack.solo.io/).


### Uninstall 

To uninstall Gloo and all related components, simply run

```bash
glooctl uninstall
```

Note that this will also remove Knative-Serving, if it was installed by Glooctl.

<!-- end -->
