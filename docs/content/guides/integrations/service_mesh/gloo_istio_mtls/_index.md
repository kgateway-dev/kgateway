---
title: Gloo and Istio mTLS
weight: 3
---

Serving as the Ingress for an Istio cluster -- without compromising on security -- means supporting mutual TLS (mTLS) communication between Gloo and the rest of the cluster. Mutual TLS means that the client proves its identity to the server (in addition to the server proving its identity to the client, which happens in regular TLS).

## Guide versions

### Istio versions

This guide was tested with Istio 1.6.6 and 1.7. For older versions of Istio, see [here]({{% versioned_link_path fromRoot="/guides/integrations/service_mesh/gloo_istio_mtls/older_istio_versions/" %}}).

### Gloo versions

This guide was tested with Gloo v1.5.0-beta23.

{{% notice warning %}}

The Gloo integration with Istio 1.6.x requires Gloo version 1.4.10, or 1.5.0-beta23 or higher.

{{% /notice %}}

### Kubernetes versions

This guide was tested with GKE v1.15.


{{% notice note %}}
Please note that if you are running Kubernetes > 1.12 in Minikube, you may run into several issues later on when installing Istio in SDS mode. This mode requires the projection of the istio-token service account tokens into volumes. We recommend installing Istio in a cluster which has this feature turned on by default (for example, GKE). See [token-authentication](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#webhook-token-authentication) for more details.

For local development and testing, if you remove the istio-token mount then Istio will default to using the service account token instead of the projected token. You will also need to set the Environment variable JWT_POLICY to "first-party-jwt" in the istio sidecar container attached to gateway-proxy.
{{% /notice %}}

---

## Step 1 - Install Istio

### Download and install

To download and install the latest version of Istio, we will be following the installation instructions [here](https://istio.io/docs/setup/getting-started/).

```bash
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.6.6 sh -
cd istio-1.6.6
istioctl install --set profile=demo
```

Use `kubectl get pods -n istio-system` to check the status on the Istio pods and wait until all the pods are **Running** or **Completed**.


## Step 2 - Install bookinfo

Before configuring Gloo, you'll need to install the bookinfo sample app to be consistent with this guide, or you can use your preferred Upstream. Either way, you'll need to enable istio-injection in the default namespace:

```bash
kubectl label namespace default istio-injection=enabled
```

To install the bookinfo sample app, cd into your downloaded Istio directory and run this command:
```bash
kubectl apply -f samples/bookinfo/platform/kube/bookinfo.yaml
```

---

## Step 3 - Configure Gloo

If necessary, install Gloo with either glooctl:
```
glooctl install gateway
```
or with helm:
```
kubectl create ns gloo-system; helm install --namespace gloo-system --version 1.5.0-beta23 gloo gloo/gloo
```
See the [quick start]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) guide for more information.

Gloo is installed to the `gloo-system` namespace and should *not* be injected with the Istio sidecar. If you have automatic injection enabled for Istio, make sure the `istio-injection` label does *not* exist on the `gloo-system` namespace. See [the Istio docs on automatic sidecar injection](https://istio.io/docs/setup/kubernetes/additional-setup/sidecar-injection/#automatic-sidecar-injection) for more.

For Gloo to successfully send requests to an Istio Upstream with mTLS enabled, we need to add the Istio mTLS secret to the gateway-proxy pod. The secret allows Gloo to authenticate with the Upstream service. We will also add an SDS server container to the pod, to handle cert rotation when Istio updates its certs.

We can use `glooctl` to update our `gateway-proxy` deployment to handle Istio mTLS certs:
```bash
glooctl istio inject
```

Under the hood, this will update the deployment to add the SDS server sidecar, as well as the istio-proxy to generate the mTLS certs. It will also update the `configMap` used by the `gateway-proxy` pod to bootstrap envoy, so that it has the SDS server listed in its `static_resources`.

The last configuration step is to configure the relevant Gloo Upstreams with mTLS. We can be fine-grained about which Upstreams have these settings as not all Gloo Upstreams may need/want mTLS enabled. This gives us the flexibility to route to Upstreams both with and without mTLS enabled - a common occurrence in a brown-field environment or during a migration to Istio.

Edit the Upstream with this command:
```bash
glooctl istio enable-mtls --upstream default-productpage-9080
```

This will add the sslConfig to the upstream.

Next we're going to lock down the mesh so that only mTLS traffic is allowed:
```bash
kubectl apply -n istio-system -f - <<EOF
apiVersion: "security.istio.io/v1beta1"
kind: "PeerAuthentication"
metadata:
  name: "default"
spec:
  mtls:
    mode: STRICT
EOF
```

Alternatively, if you're not ready to lock down your entire mesh, you can change settings on a namespace or workload level. To enable stict mTLS for just the productpage app we've configured, we can run:
```bash
cat <<EOF | kubectl apply -f -
apiVersion: "security.istio.io/v1beta1"
kind: "PeerAuthentication"
metadata:
  name: "productpage"
  namespace: "default"
spec:
  selector:
    matchLabels:
      app: productpage
  mtls:
    mode: STRICT
EOF
```

More details on configuring PeerAuthentication policies can be found [here](https://istio.io/latest/docs/tasks/security/authentication/authn-policy/).

To test this out, we need a route in Gloo:
```bash
glooctl add route --name prodpage --namespace gloo-system --path-prefix / --dest-name default-productpage-9080 --dest-namespace gloo-system
```

And we can curl it:

```bash
curl -v $(glooctl proxy url)/productpage
```

Or access it in the browser:
```bash
HTTP_GW=$(glooctl proxy url)
## Open the ingress url in the browser:
$([ "$(uname -s)" = "Linux" ] && echo xdg-open || echo open) $HTTP_GW/productpage
```
