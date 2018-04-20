# Installing on Kubernetes

- [Simple Installation](#Simple Installation)
- [Advanced  Installation](#Advanced Installation)



<a name="Simple Installation"></a>
### Simple Installation

#### What you'll need

1. Kubernetes v1.8+ or higher deployed. We recommend using [minikube](https://kubernetes.io/docs/getting-started-guides/minikube/) to get a demo cluster up quickly.
1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/) installed on your local machine.

Once your Kubernetes cluster is up and running, run the following command to deploy Gloo and Envoy to the `gloo-system` namespace:

```bash
kubectl apply \
  --filename https://raw.githubusercontent.com/solo-io/gloo/master/install/kube/install.yaml
```

Check that the Gloo pods and services have been created:

```bash
kubectl get all -n gloo-system

NAME                           DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deploy/function-discovery      1         1         1            1           3m
deploy/gloo                    1         1         1            1           3m
deploy/ingress                 1         1         1            1           3m
deploy/ingress-controller      1         1         1            1           3m
deploy/upstream-discovery   1         1         1            1           3m

NAME                                  DESIRED   CURRENT   READY     AGE
rs/function-discovery-74cbdb66b5      1         1         1         3m
rs/gloo-6f68b9f7d6                    1         1         1         3m
rs/ingress-controller-78cfcd7f78      1         1         1         3m
rs/ingress-d5478d8c8                  1         1         1         3m
rs/upstream-discovery-84744c4676   1         1         1         3m

NAME                           DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deploy/function-discovery      1         1         1            1           3m
deploy/gloo                    1         1         1            1           3m
deploy/ingress                 1         1         1            1           3m
deploy/ingress-controller      1         1         1            1           3m
deploy/upstream-discovery   1         1         1            1           3m

NAME                                  DESIRED   CURRENT   READY     AGE
rs/function-discovery-74cbdb66b5      1         1         1         3m
rs/gloo-6f68b9f7d6                    1         1         1         3m
rs/ingress-controller-78cfcd7f78      1         1         1         3m
rs/ingress-d5478d8c8                  1         1         1         3m
rs/upstream-discovery-84744c4676   1         1         1         3m

NAME                                        READY     STATUS    RESTARTS   AGE
po/function-discovery-74cbdb66b5-zknxp      1/1       Running   0          3m
po/gloo-6f68b9f7d6-hn46t                    1/1       Running   0          3m
po/ingress-controller-78cfcd7f78-nmqm6      1/1       Running   0          3m
po/ingress-d5478d8c8-w6kxs                  1/1       Running   0          3m
po/upstream-discovery-84744c4676-sdtgt   1/1       Running   0          3m

NAME          TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)                         AGE
svc/gloo      ClusterIP   10.107.176.154   <none>        8081/TCP                        3m
svc/ingress   NodePort    10.96.48.30      <none>        8080:30145/TCP,8443:31071/TCP   3m
```

Everything should be up and running. If this process does not work, please [open an issue](https://github.com/solo-io/gloo/issues/new). We are happy to answer
questions on our [diligently staffed Slack channel](https://slack.solo.io/).

See [Getting Started on Kubernetes](../getting_started/kubernetes/1.md) to get started creating routes with Gloo.