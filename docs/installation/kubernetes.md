# Installing on Kubernetes

### What you'll need

1. Kubernetes v1.8+ or higher deployed. We recommend using [minikube](https://kubernetes.io/docs/getting-started-guides/minikube/) to get a demo cluster up quickly.
1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/) installed on your local machine.

### Install

#### 1. Install Glooctl

If this is your first time running Gloo, you’ll need to download the command-line interface (CLI) onto your local machine. 
You’ll use this CLI to interact with Gloo, including installing it onto your Kubernetes cluster.

To install the CLI, run:

`curl -sL https://run.solo.io/gloo/install | sh`

Alternatively, you can download the CLI directly via the github releases page. 

Next, add Gloo to your path with:

`export PATH=$HOME/.gloo/bin:$PATH`

Verify the CLI is installed and running correctly with:

`glooctl --version`

#### 2. Install Gloo to your Kubernetes Cluster using Glooctl

Once your Kubernetes cluster is up and running, run the following command to deploy Gloo and Envoy to the `gloo-system` namespace:

```bash
glooctl install kube 
```

Check that the Gloo pods and services have been created:

```bash
kubectl get all -n gloo-system

NAME                                 READY   STATUS    RESTARTS   AGE
pod/discovery-8497c769bd-ccz8h       1/1     Running   0          30s
pod/gateway-57d6bd8684-tqgw9         1/1     Running   0          30s
pod/gateway-proxy-798cbc584c-dm6p4   1/1     Running   0          30s
pod/gloo-868c6644c9-jl2x8            1/1     Running   0          30s

NAME                    TYPE           CLUSTER-IP       EXTERNAL-IP   PORT(S)          AGE
service/gateway-proxy   LoadBalancer   10.105.143.110   <pending>     8080:32218/TCP   30s
service/gloo            ClusterIP      10.101.197.139   <none>        9977/TCP         30s

NAME                            DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/discovery       1         1         1            1           30s
deployment.apps/gateway         1         1         1            1           30s
deployment.apps/gateway-proxy   1         1         1            1           30s
deployment.apps/gloo            1         1         1            1           31s

NAME                                       DESIRED   CURRENT   READY   AGE
replicaset.apps/discovery-8497c769bd       1         1         1       30s
replicaset.apps/gateway-57d6bd8684         1         1         1       30s
replicaset.apps/gateway-proxy-798cbc584c   1         1         1       30s
replicaset.apps/gloo-868c6644c9            1         1         1       30s
```

Everything should be up and running. If this process does not work, please [open an issue](https://github.com/solo-io/gloo/issues/new). We are happy to answer
questions on our [diligently staffed Slack channel](https://slack.solo.io/).

See [Getting Started on Kubernetes](../getting_started/kubernetes/1.md) to get started creating routes with Gloo.


### Uninstall 

To tear down the installation at any point, you can simply run

```bash

kubectl delete namespace gloo-system
```

<!-- end -->
