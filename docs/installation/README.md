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

- [*Gateway*](#2a.-Install-the-Gloo-Gateway-to-your-Kubernetes-Cluster-using-Glooctl): Gloo's full feature set is available via its v1/Gateway API. The Gateway API is modeled on Envoy's own API with the use of opinionated defaults to make complex configurations possible, while maintaining simplicity where desired.

- [*Ingress*](#2b.-Install-the-Gloo-Ingress-Controller-to-your-Kubernetes-Cluster-using-Glooctl
): Gloo will support configuration the Kubernetes Ingress resource, acting as a Kubernetes Ingress Controller. Note that ingress objects must have the annotation `"kubernetes.io/ingress.class": "gloo"` to be processed by the Gloo Ingress.

- *Knative*: Gloo will integrate automatically with Knative as a cluster-level ingress for [*Knative-Serving*](https://github.com/knative/serving). Gloo can be used in this way as a 
lightweight replacement for Istio when using Knative-Serving.


### 2a. Install the Gloo Gateway to your Kubernetes Cluster using Glooctl
        
        Once your Kubernetes cluster is up and running, run the following command to deploy Gloo and Envoy to the `gloo-system` namespace:
        
        ```bash
        glooctl install gateway 
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

TODO a ggetting started for gateway (rename of the kubernetes getting started)

### 2b. Install the Gloo Ingress Controller to your Kubernetes Cluster using Glooctl

        Once your Kubernetes cluster is up and running, run the following command to deploy Gloo and Envoy to the `gloo-system` namespace:
        
        ```bash
        glooctl install ingress 
        ```
        
        Check that the Gloo pods and services have been created:
        
        TODO
        
TODO a ggetting started for ingress 
        

### 2c. Install the Gloo Knative Cluster Ingress to your Kubernetes Cluster using Glooctl

 
Once your Kubernetes cluster is up and running, run the following command to deploy Knative-Serving components to the `knative-serving` namespace and Gloo to the `gloo-system` namespace:

`glooctl install knative`


Check that the Gloo and Knative pods and services have been created:

```bash
kubectl get all -n gloo-system

NAME                                        READY     STATUS    RESTARTS   AGE
pod/clusteringress-proxy-65485cd8f4-gg9qq   1/1       Running   0          10m
pod/discovery-5cf7c45fb7-ndj29              1/1       Running   0          10m
pod/gloo-5fc9f5c558-n6nlr                   1/1       Running   1          10m
pod/ingress-6d8d8f595c-smql8                1/1       Running   0          10m

NAME                           TYPE           CLUSTER-IP       EXTERNAL-IP   PORT(S)                      AGE
service/clusteringress-proxy   LoadBalancer   10.96.196.217    <pending>     80:31639/TCP,443:31025/TCP   14m
service/gateway-proxy          LoadBalancer   10.109.135.176   <pending>     8080:32722/TCP               14m
service/gloo                   ClusterIP      10.103.179.64    <none>        9977/TCP                     14m
service/ingress-proxy          LoadBalancer   10.110.100.99    <pending>     80:31738/TCP,443:31769/TCP   14m

NAME                                   DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/clusteringress-proxy   1         1         1            1           14m
deployment.apps/discovery              1         1         1            1           14m
deployment.apps/gloo                   1         1         1            1           14m
deployment.apps/ingress                1         1         1            1           14m


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

TODO a getting started for knative (move what we have)


### Uninstall 

TODO: real uninstall x
todo for knative - annotate what we install, use it to determine if its our namespace

```bash

glooctl uninstall X
```

<!-- end -->

glooctl install knative broken on latest release?
