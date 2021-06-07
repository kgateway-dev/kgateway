---
title: Gloo Edge and Istio mTLS
weight: 3
---

Serving as the Ingress for an Istio cluster -- without compromising on security -- means supporting mutual TLS (mTLS) communication between Gloo Edge and the rest of the cluster. Mutual TLS means that the client proves its identity to the server (in addition to the server proving its identity to the client, which happens in regular TLS).

## Guide versions

### Istio versions

This guide was tested with Istio 1.6.6, 1.7.2, and 1.8.1. For older versions of Istio, see [here]({{% versioned_link_path fromRoot="/guides/integrations/service_mesh/gloo_istio_mtls/older_istio_versions/" %}}).

### Gloo Edge versions

This guide was tested with Gloo Edge v1.5.0.

{{% notice warning %}}

The Gloo Edge integration with Istio 1.6.6+ requires Gloo Edge version 1.4.10+, or 1.5.0+.

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
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.8.1 sh -
cd istio-1.8.1
istioctl install --set profile=demo
```

Use `kubectl get pods -n istio-system` to check the status on the Istio pods and wait until all the pods are **Running** or **Completed**.


## Step 2 - Install bookinfo

Before configuring Gloo Edge, you'll need to install the bookinfo sample app to be consistent with this guide, or you can use your preferred Upstream. Either way, you'll need to enable istio-injection in the default namespace:

```bash
kubectl label namespace default istio-injection=enabled
```

To install the bookinfo sample app, cd into your downloaded Istio directory and run this command:
```bash
kubectl apply -f samples/bookinfo/platform/kube/bookinfo.yaml
```

---

## Step 3 - Configure Gloo Edge

If necessary, install Gloo Edge with either glooctl:
```
glooctl install gateway
```
or with helm:
```
kubectl create ns gloo-system; helm install --namespace gloo-system --version 1.5.0-beta25 gloo gloo/gloo
```
See the [quick start]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) guide for more information.

Gloo Edge is installed to the `gloo-system` namespace and should *not* be injected with the Istio sidecar. If you have automatic injection enabled for Istio, make sure the `istio-injection` label does *not* exist on the `gloo-system` namespace. See [the Istio docs on automatic sidecar injection](https://istio.io/docs/setup/kubernetes/additional-setup/sidecar-injection/#automatic-sidecar-injection) for more.

For Gloo Edge to successfully send requests to an Istio Upstream with mTLS enabled, we need to add the Istio mTLS secret to the gateway-proxy pod. The secret allows Gloo Edge to authenticate with the Upstream service. We will also add an SDS server container to the pod, to handle cert rotation when Istio updates its certs.

We can use `glooctl` to update our `gateway-proxy` deployment to handle Istio mTLS certs:
```bash
glooctl istio inject
```

Under the hood, this will update the deployment to add the SDS server sidecar, as well as the istio-proxy to generate the mTLS certs. It will also update the `configMap` used by the `gateway-proxy` pod to bootstrap envoy, so that it has the SDS server listed in its `static_resources`.

The last configuration step is to configure the relevant Gloo Edge Upstreams with mTLS. We can be fine-grained about which Upstreams have these settings as not all Gloo Edge Upstreams may need/want mTLS enabled. This gives us the flexibility to route to Upstreams both with and without mTLS enabled - a common occurrence in a brown-field environment or during a migration to Istio.

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

To test this out, we need a route in Gloo Edge:
```bash
glooctl add route --name prodpage --namespace gloo-system --path-prefix / --dest-name default-productpage-9080 --dest-namespace gloo-system
```

## Step 4 - Test it

We can curl the product page:

```bash
curl -v $(glooctl proxy url)/productpage
```

Or access it in the browser:
```bash
HTTP_GW=$(glooctl proxy url)
## Open the ingress url in the browser:
$([ "$(uname -s)" = "Linux" ] && echo xdg-open || echo open) $HTTP_GW/productpage
```



## Understanding the changes made

While the `glooctl` functions we've used in this guide are great for getting started quickly, it's also useful to know what changes they've actually made to our configuration under the hood. When we ran `glooctl inject istio`, this updated two resources:

##### ConfigMap changes
First, it updates the `gateway-proxy-envoy-config` `ConfigMap` resource used by the `gateway-proxy` deployment to bootstrap our envoy server. Specifically, it adds the `gateway_proxy_sds` cluster, so that envoy can receive the secrets over SDS:

{{< highlight yaml "hl_lines=20-31" >}}
apiVersion: v1
data:
  envoy.yaml: |
    (...)
    staticResources:
      clusters:
      - altStatName: xds_cluster
        connectTimeout: 5s
        http2ProtocolOptions: {}
        loadAssignment:
          clusterName: gloo.gloo-system.svc.cluster.local:9977
          endpoints:
          - lbEndpoints:
            - endpoint:
                address:
                  socketAddress:
                    address: gloo.gloo-system.svc.cluster.local
                    portValue: 9977
      (...)
      - connectTimeout: 0.250s
            http2ProtocolOptions: {}
            loadAssignment:
              clusterName: gateway_proxy_sds
              endpoints:
              - lbEndpoints:
                - endpoint:
                    address:
                      socketAddress:
                        address: 127.0.0.1
                        portValue: 8234
            name: gateway_proxy_sds
      (...)
{{< /highlight >}}

##### Deployment changes
Next, it updates our `gateway-proxy` deployment to add two sidecars and some volume mounts. It adds an `istio-proxy` sidecar, which is used to generate the certificates used for mTLS communication. Glooctl reads the currently installed version of `istiod` from your current cluster in order to determine which version of `istio-proxy` to install as a sidecar. Currently supported versions of Istio are 1.6.x-1.8.x. It also adds an `sds` sidecar, which is a running [sds server](https://www.envoyproxy.io/docs/envoy/latest/configuration/security/secret) that feeds any certificate changes into our `gateway-proxy` whenever the certs change. For example, this will happen when the istio mTLS certificates rotate, which is every 24 hours in the default istio installation. The certs are also added to a volumeMount at `/etc/istio-certs/`

{{< highlight yaml "hl_lines=65-192 202-216" >}}
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: gloo
    gateway-proxy-id: gateway-proxy
    gloo: gateway-proxy
  name: gateway-proxy
  namespace: gloo-system
spec:
  selector:
    matchLabels:
      gateway-proxy-id: gateway-proxy
      gloo: gateway-proxy
  template:
    metadata:
      annotations:
        prometheus.io/path: /metrics
        prometheus.io/port: "8081"
        prometheus.io/scrape: "true"
      creationTimestamp: null
      labels:
        gateway-proxy: live
        gateway-proxy-id: gateway-proxy
        gloo: gateway-proxy
    spec:
      containers:
      - args:
        - --disable-hot-restart
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
        - name: POD_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.name
        image: quay.io/solo-io/gloo-envoy-wrapper:1.5.0-beta24
        imagePullPolicy: IfNotPresent
        name: gateway-proxy
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        - containerPort: 8443
          name: https
          protocol: TCP
        resources: {}
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 10101
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config
      - env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
        - name: ISTIO_MTLS_SDS_ENABLED
          value: "true"
        image: quay.io/solo-io/sds:1.5.0-beta24
        imagePullPolicy: IfNotPresent
        name: sds
        ports:
        - containerPort: 8234
          name: sds
          protocol: TCP
        resources: {}
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /etc/istio-certs/
          name: istio-certs
        - mountPath: /etc/envoy
          name: envoy-config
      - args:
        - proxy
        - sidecar
        - --domain
        - $(POD_NAMESPACE).svc.cluster.local
        - --configPath
        - /etc/istio/proxy
        - --binaryPath
        - /usr/local/bin/envoy
        - --serviceCluster
        - istio-proxy-prometheus
        - --drainDuration
        - 45s
        - --parentShutdownDuration
        - 1m0s
        - --discoveryAddress
        - istiod.istio-system.svc:15012
        - --proxyLogLevel=warning
        - --proxyComponentLogLevel=misc:error
        - --connectTimeout
        - 10s
        - --proxyAdminPort
        - "15000"
        - --controlPlaneAuthPolicy
        - NONE
        - --dnsRefreshRate
        - 300s
        - --statusPort
        - "15021"
        - --trust-domain=cluster.local
        - --controlPlaneBootstrap=false
        env:
        - name: OUTPUT_CERTS
          value: /etc/istio-certs
        - name: JWT_POLICY
          value: first-party-jwt
        - name: PILOT_CERT_PROVIDER
          value: istiod
        - name: CA_ADDR
          value: istiod.istio-system.svc:15012
        - name: ISTIO_META_MESH_ID
          value: cluster.local
        - name: POD_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
        - name: INSTANCE_IP
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: status.podIP
        - name: SERVICE_ACCOUNT
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: spec.serviceAccountName
        - name: HOST_IP
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: status.hostIP
        - name: ISTIO_META_POD_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.name
        - name: ISTIO_META_CONFIG_NAMESPACE
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
        image: docker.io/istio/proxyv2:1.6.0
        imagePullPolicy: IfNotPresent
        name: istio-proxy
        readinessProbe:
          failureThreshold: 30
          httpGet:
            path: /healthz/ready
            port: 15021
            scheme: HTTP
          initialDelaySeconds: 1
          periodSeconds: 2
          successThreshold: 1
          timeoutSeconds: 1
        resources: {}
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /var/run/secrets/istio
          name: istiod-ca-cert
        - mountPath: /etc/istio/proxy
          name: istio-envoy
        - mountPath: /etc/istio-certs/
          name: istio-certs
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext:
        fsGroup: 10101
        runAsUser: 10101
      serviceAccount: gateway-proxy
      serviceAccountName: gateway-proxy
      terminationGracePeriodSeconds: 30
      volumes:
      - configMap:
          defaultMode: 420
          name: gateway-proxy-envoy-config
        name: envoy-config
      - emptyDir:
          medium: Memory
        name: istio-certs
      - configMap:
          defaultMode: 420
          name: istio-ca-root-cert
        name: istiod-ca-cert
      - emptyDir:
          medium: Memory
        name: istio-envoy
status:
(...)
{{< /highlight >}}

##### Upstream changes
Finally, running `glooctl istio enable-mtls --upstream default-productpage-9080` adds the `sslConfig` to our upstream so that Envoy knows to get the certs via SDS:

{{< highlight yaml "hl_lines=17-24" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: default-productpage-9080
  namespace: gloo-system
spec:
  discoveryMetadata:
    labels:
      app: productpage
      service: productpage
  kube:
    selector:
      app: productpage
    serviceName: productpage
    serviceNamespace: default
    servicePort: 9080
  sslConfig:
    alpnProtocols:
    - istio
    sds:
      certificatesSecretName: istio_server_cert
      clusterName: gateway_proxy_sds
      targetUri: 127.0.0.1:8234
      validationContextName: istio_validation_context
status:
(...)
{{< /highlight >}}

## Declarative Approach

The glooctl istio inject feature we have walked through above is great for testing out the istio integration and quickly getting started. However, given that it is a manual step, it may not be suitable for all environments - for example a helm-only install in a production cluster. In such cases, we also have the option of taking a declarative approach instead.

{{< tabs >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl install gateway --values <(echo '{"crds":{"create":true},"global":{"istioSDS":{"enabled":true}}}')
{{< /tab >}}
{{< tab name="helm" codelang="shell">}}
helm install gloo gloo/gloo --namespace gloo-system --set global.istioSDS.enabled=true
{{< /tab >}}
{{< /tabs >}}

This will set up the ConfigMap and gateway-proxy deployments to handle mTLS cert rotation.

Any upstreams using mTLS will need to be contain the sslConfig as described above in the [Upstream changes section]({{% versioned_link_path fromRoot="/guides/integrations/service_mesh/gloo_istio_mtls/#upstream-changes" %}}).

##### Custom Sidecars

The default istio-proxy image used as a sidecar by this declarative approach is `docker.io/istio/proxyv2:1.9.5`. If this image doesn't work for you (for example, your mesh is on a different, incompatible Istio version), you can override the default sidecar with your own.

To do this, you must set your custom sidecar in the helm value `global.istioSDS.customSidecars`. For example, if you wanted to use istio proxy v1.6.6 instead:

```yaml
global:
  istioSDS:
    enabled: true
    customSidecars:
      - name: istio-proxy
        image: docker.io/istio/proxyv2:1.6.6
        args:
        - proxy
        - sidecar
        - --domain
        - $(POD_NAMESPACE).svc.cluster.local
        - --configPath
        - /etc/istio/proxy
        - --binaryPath
        - /usr/local/bin/envoy
        - --serviceCluster
        - istio-proxy-prometheus
        - --drainDuration
        - 45s
        - --parentShutdownDuration
        - 1m0s
        - --discoveryAddress
        - istiod.istio-system.svc:15012
        - --proxyLogLevel=warning
        - --proxyComponentLogLevel=misc:error
        - --connectTimeout
        - 10s
        - --proxyAdminPort
        - "15000"
        - --controlPlaneAuthPolicy
        - NONE
        - --dnsRefreshRate
        - 300s
        - --statusPort
        - "15021"
        - --trust-domain=cluster.local
        - --controlPlaneBootstrap=false
        env:
          - name: OUTPUT_CERTS
            value: "/etc/istio-certs"
          - name: JWT_POLICY
            value: third-party-jwt
          - name: PILOT_CERT_PROVIDER
            value: istiod
          - name: CA_ADDR
            value: istiod.istio-system.svc:15012
          - name: ISTIO_META_MESH_ID
            value: cluster.local
          - name: POD_NAME
            valueFrom:
              fieldRef:
                fieldPath: metadata.name
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: INSTANCE_IP
            valueFrom:
              fieldRef:
                fieldPath: status.podIP
          - name: SERVICE_ACCOUNT
            valueFrom:
              fieldRef:
                fieldPath: spec.serviceAccountName
          - name: HOST_IP
            valueFrom:
              fieldRef:
                fieldPath: status.hostIP
          - name: ISTIO_META_POD_NAME
            valueFrom:
              fieldRef:
                apiVersion: v1
                fieldPath: metadata.name
          - name: ISTIO_META_CONFIG_NAMESPACE
            valueFrom:
              fieldRef:
                apiVersion: v1
                fieldPath: metadata.namespace
        imagePullPolicy: IfNotPresent
        readinessProbe:
          failureThreshold: 30
          httpGet:
            path: /healthz/ready
            port: 15021
            scheme: HTTP
          initialDelaySeconds: 1
          periodSeconds: 2
          successThreshold: 1
          timeoutSeconds: 1
        volumeMounts:
        - mountPath: /var/run/secrets/istio
          name: istiod-ca-cert
        - mountPath: /etc/istio/proxy
          name: istio-envoy
        - mountPath: /etc/istio-certs/
          name: istio-certs
        - mountPath: /var/run/secrets/tokens
          name: istio-token 
```

If any value is set in `global.istioSDS.customSidecars`, this is used instead of the default istio-proxy sidecar. Multiple custom sidecars can be added if necessary.

See also [helm values references doc]({{% versioned_link_path fromRoot="/reference/helm_chart_values/" %}}).

