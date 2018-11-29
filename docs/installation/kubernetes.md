# Installing on Kubernetes

- [Simple Installation](#simple-installation)
- [Advanced  Installation](#advanced-installation)



<a name="Simple Installation"></a>
### Simple Installation

#### What you'll need

1. Kubernetes v1.8+ or higher deployed. We recommend using [minikube](https://kubernetes.io/docs/getting-started-guides/minikube/) to get a demo cluster up quickly.
1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/) installed on your local machine.

Once your Kubernetes cluster is up and running, run the following command to deploy Gloo and Envoy to the `gloo-system` namespace:

```bash
glooctl install kube 
```

Check that the Gloo pods and services have been created:

```bash
kubectl get all -n gloo-system

NAME                                           READY   STATUS    RESTARTS   AGE
pod/control-plane-6fc6dc7545-xrllk             1/1     Running   0          11m
pod/function-discovery-544c596dcd-gk8x7        1/1     Running   0          11m
pod/ingress-64f75ccb7-4z299                    1/1     Running   0          11m
pod/kube-ingress-controller-665d59bc7d-t6lwk   1/1     Running   0          11m
pod/upstream-discovery-74db4d7475-gqrst        1/1     Running   0          11m

NAME                    TYPE           CLUSTER-IP       EXTERNAL-IP   PORT(S)                         AGE
service/control-plane   ClusterIP      10.101.206.34    <none>        8081/TCP                        11m
service/ingress         LoadBalancer   10.108.115.187   <pending>     8080:32608/TCP,8443:30634/TCP   11m

NAME                                      DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/control-plane             1         1         1            1           11m
deployment.apps/function-discovery        1         1         1            1           11m
deployment.apps/ingress                   1         1         1            1           11m
deployment.apps/kube-ingress-controller   1         1         1            1           11m
deployment.apps/upstream-discovery        1         1         1            1           11m

NAME                                                 DESIRED   CURRENT   READY   AGE
replicaset.apps/control-plane-6fc6dc7545             1         1         1       11m
replicaset.apps/function-discovery-544c596dcd        1         1         1       11m
replicaset.apps/ingress-64f75ccb7                    1         1         1       11m
replicaset.apps/kube-ingress-controller-665d59bc7d   1         1         1       11m
replicaset.apps/upstream-discovery-74db4d7475        1         1         1       11m
```

Everything should be up and running. If this process does not work, please [open an issue](https://github.com/solo-io/gloo/issues/new). We are happy to answer
questions on our [diligently staffed Slack channel](https://slack.solo.io/).

See [Getting Started on Kubernetes](../getting_started/kubernetes/1.md) to get started creating routes with Gloo.
