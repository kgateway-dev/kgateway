# Knative Networking with Gloo Edge Cluster Ingress

`Warning: Knative support is deprecated in Gloo Edge 1.10` and [will not be available in Gloo Edge 1.11](https://github.com/solo-io/gloo/issues/5708)

With Knative support enabled, Gloo Edge will configure Envoy using [Knative's Cluster Ingress Resource](https://github.com/knative/serving/blob/main/pkg/client/informers/externalversions/networking/v1alpha1/ingress.go).

The installation process detailed in this document provides a way of using Knative-Serving without needing to install Istio.

### What you'll need

1. Kubernetes v1.11.3. We recommend using [minikube](https://kubernetes.io/docs/getting-started-guides/minikube/) or 
[Kubernetes-in-Docker](https://github.com/kubernetes-sigs/kind) to get a local cluster up quickly.
1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/) installed on your local machine.

### Install

#### 1. Install glooctl

If this is your first time running Gloo Edge, you’ll need to download the command-line interface (CLI) onto your local machine. 
You’ll use this CLI to interact with Gloo Edge, including installing it onto your Kubernetes cluster.

To install the CLI, run:

##### Linux/MacOS

`curl -sL https://run.solo.io/gloo/install | sh`

##### Windows

`(New-Object System.Net.WebClient).DownloadString("https://run.solo.io/gloo/windows/install") | iex`

Alternatively, you can download the CLI directly via the github releases page. 

Next, add Gloo Edge to your path with:

##### Linux/MacOS

`export PATH=$HOME/.gloo/bin:$PATH`

##### Windows

`$env:Path += ";$env:userprofile/.gloo/bin/"`

Verify the CLI is installed and running correctly with:

`glooctl version`

#### 2. Install Knative and Gloo Edge to your Kubernetes Cluster using glooctl

Once your Kubernetes cluster is up and running, run the following command to deploy Knative-Serving components to the `knative-serving` namespace and Gloo Edge to the `gloo-system` namespace:

`glooctl install knative`


Check that the Gloo Edge and Knative pods and services have been created:

```bash
kubectl get all -n gloo-system

NAME                                        READY     STATUS    RESTARTS   AGE
pod/knative-proxy-65485cd8f4-gg9qq   1/1       Running   0          10m
pod/discovery-5cf7c45fb7-ndj29              1/1       Running   0          10m
pod/gateway-7b48fdfbd8-trwvg                1/1       Running   1          10m
pod/gateway-proxy-984bcf497-29jl8           1/1       Running   0          10m
pod/gloo-5fc9f5c558-n6nlr                   1/1       Running   1          10m
pod/ingress-6d8d8f595c-smql8                1/1       Running   0          10m
pod/ingress-proxy-5fc45b8f6d-cckw4          1/1       Running   0          10m

NAME                           TYPE           CLUSTER-IP       EXTERNAL-IP   PORT(S)                      AGE
service/knative-proxy   LoadBalancer   10.96.196.217    <pending>     80:31639/TCP,443:31025/TCP   14m
service/gateway-proxy          LoadBalancer   10.109.135.176   <pending>     8080:32722/TCP               14m
service/gloo                   ClusterIP      10.103.179.64    <none>        9977/TCP                     14m
service/ingress-proxy          LoadBalancer   10.110.100.99    <pending>     80:31738/TCP,443:31769/TCP   14m

NAME                                   DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/knative-proxy   1         1         1            1           14m
deployment.apps/discovery              1         1         1            1           14m
deployment.apps/gateway                1         1         1            1           14m
deployment.apps/gateway-proxy          1         1         1            1           14m
deployment.apps/gloo                   1         1         1            1           14m
deployment.apps/ingress                1         1         1            1           14m
deployment.apps/ingress-proxy          1         1         1            1           14m


```

```bash
kubectl get all -n knative-serving

NAME                              READY     STATUS    RESTARTS   AGE
pod/activator-5c4755585c-5wv26    1/1       Running   0          15m
pod/autoscaler-78cd88f869-dvsfr   1/1       Running   0          15m
pod/controller-8d5b85958-tcqn5    1/1       Running   0          15m
pod/webhook-7585d7488c-zk9wz      1/1       Running   0          15m

NAME                        TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)             AGE
service/activator-service   ClusterIP   10.109.189.12   <none>        80/TCP,9090/TCP     15m
service/autoscaler          ClusterIP   10.98.6.4       <none>        8080/TCP,9090/TCP   15m
service/controller          ClusterIP   10.108.42.33    <none>        9090/TCP            15m
service/webhook             ClusterIP   10.99.201.163   <none>        443/TCP             15m

NAME                         DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/activator    1         1         1            1           15m
deployment.apps/autoscaler   1         1         1            1           15m
deployment.apps/controller   1         1         1            1           15m
deployment.apps/webhook      1         1         1            1           15m

NAME                                    DESIRED   CURRENT   READY     AGE
replicaset.apps/activator-5c4755585c    1         1         1         15m
replicaset.apps/autoscaler-78cd88f869   1         1         1         15m
replicaset.apps/controller-8d5b85958    1         1         1         15m
replicaset.apps/webhook-7585d7488c      1         1         1         15m

NAME                                                 AGE
image.caching.internal.knative.dev/fluentd-sidecar   15m
image.caching.internal.knative.dev/queue-proxy       15m
```

#### 3. Send Requests to a Knative App  

Create a Knative App: 

```bash
# deploy a basic helloworld-go service
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/main/test/kube2e/artifacts/knative-hello-service.yaml
```

Get the URL of the Gloo Edge Knative Ingress:

```bash
export INGRESS=$(glooctl proxy url --name knative-proxy)
echo $INGRESS

http://172.17.0.2:31345
```

Note: if your cluster is running in minishift, you'll need to run the following command to get an externally accessible 
url: 

```bash
export INGRESS=$(glooctl proxy url --name knative-proxy --local-cluster)
echo $INGRESS

http://192.168.99.163:32220

```

Send a request to the app using `curl`:

```bash
curl -H "Host: helloworld-go.default.example.com" $INGRESS

Hello Go Sample v1!
```

Everything should be up and running. If this process does not work, please [open an issue](https://github.com/solo-io/gloo/issues/new). We are happy to answer
questions on our [diligently staffed Slack channel](https://slack.solo.io/).


### Uninstall 

To tear down the installation at any point, you can simply run

```bash

kubectl delete namespace gloo-system
kubectl delete namespace knative-serving
```

