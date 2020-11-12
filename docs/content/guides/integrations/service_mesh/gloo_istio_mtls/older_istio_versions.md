---
title: Gloo Edge and Istio mTLS with older versions of Istio
weight: 1
---

This reference guide contains instructions for older versions of Istio (1.0 to 1.5). If you are running Istio 1.6, you can use the latest documentation [here]({{% versioned_link_path fromRoot="/guides/integrations/service_mesh/gloo_istio_mtls/" %}}).

Serving as the Ingress for an Istio cluster -- without compromising on security -- means supporting mutual TLS (mTLS) communication between Gloo Edge and the rest of the cluster. Mutual TLS means that the client proves its identity to the server (in addition to the server proving its identity to the client, which happens in regular TLS).

## Guide versions

### Istio versions

This guide was tested with Istio 1.0.9, 1.1.17, 1.3.6, 1.4.3, and 1.5.1.

### Gloo Edge versions

This guide was tested with Gloo Edge v1.3.1 except where noted.

### Kubernetes versions

This guide was tested with GKE v1.15.


{{% notice note %}}
Please note that if you are running Kubernetes > 1.12 in Minikube, you may run into several issues later on when installing Istio in SDS mode. This mode requires the projection of the istio-token service account tokens into volumes. We recommend installing Istio in a cluster which has this feature turned on by default (for example, GKE).
{{% /notice %}}

---

## Step 1 - Install Istio

### Download and install

To download and install the latest version of Istio, follow the installation instructions [here](https://istio.io/docs/setup/getting-started/). You will need to set the profile to `sds` for this guide.

Previous releases can be found for download [here](https://github.com/istio/istio/releases).

For a quick install of Istio 1.0.6 or 1.0.9 (prior to SDS mode option) with mTLS enabled, run the following commands:

```bash
kubectl apply -f install/kubernetes/helm/istio/templates/crds.yaml
kubectl apply -f install/kubernetes/istio-demo-auth.yaml
kubectl get pods -w -n istio-system
```

Use `kubectl get pods -n istio-system` to check the status on the Istio pods and wait until all the pods are **Running** or **Completed**.

### SDS mode

In Istio 1.1, a new option to configure certificates and keys was introduced based on [Envoy Proxy's Secret Discovery Service](https://www.envoyproxy.io/docs/envoy/v1.11.2/configuration/secret.html#secret-discovery-service-sds) (SDS). This mode enables Istio to deliver the secrets via an API instead of mounting to the file system as with Istio 1.0. This has two big benefits:

* We don't need to hot-restart the proxy when certificates are rotated.
* The keys for the services never travel over the network; they stay on a single node and are delivered to the service. 

For more information on [Istio's identity provisioning through SDS](https://istio.io/docs/tasks/security/auth-sds/) take a look at the [Istio documentation](https://istio.io/docs/tasks/security/auth-sds/).

---

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
kubectl create ns gloo-system; helm install --namespace gloo-system --version 1.3.20 gloo gloo/gloo
```
See the [quick start]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) guide for more information.

Gloo Edge is installed to the `gloo-system` namespace and should *not* be injected with the Istio sidecar. If you have automatic injection enabled for Istio, make sure the `istio-injection` label does *not* exist on the `gloo-system` namespace. See [the Istio docs on automatic sidecar injection](https://istio.io/docs/setup/kubernetes/additional-setup/sidecar-injection/#automatic-sidecar-injection) for more.

For Gloo Edge to successfully send requests to an Istio Upstream with mTLS enabled, we need to add the Istio mTLS secret to the gateway-proxy pod. The secret allows Gloo Edge to authenticate with the Upstream service.

The last configuration step is to configure the relevant Gloo Edge Upstreams with mTLS. We can be fine-grained about which Upstreams have these settings as not all Gloo Edge Upstreams may need/want mTLS enabled. This gives us the flexibility to route to Upstreams both with and without mTLS enabled - a common occurrence in a brown-field environment or during a migration to Istio.

Version-specific configurations for the gateway-proxy and the sample Upstream can be found below:
- [Istio 1.0.x](#istio-10x)
- [Istio 1.1.x](#istio-11x)
- [Istio 1.3.x and 1.4.x](#istio-13x-and-14x)
- [Istio 1.5.x](#istio-15x)

Edit the gateway-proxy with this command:
```bash
kubectl edit deploy/gateway-proxy -n gloo-system
```

Edit the Upstream with this command:
```bash
kubectl edit upstream default-productpage-9080 --namespace gloo-system
```

For Gloo Edge versions 1.1.x and up, you must disable function discovery before editing the Upstream to prevent your change from being overwritten by Gloo Edge:

```bash
kubectl label namespace default discovery.solo.io/function_discovery=disabled
```

To test this out, we need a route in Gloo Edge:
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

### Istio 1.0.x
{{% expand "Click to see configurations for Istio 1.0.x." %}}
Here's an example of an edited deployment:

{{< highlight yaml "hl_lines=43-45 50-54" >}}
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    app: gloo
    gloo: gateway-proxy
  name: gateway-proxy
  namespace: gloo-system
spec:
  replicas: 1
  selector:
    matchLabels:
      gloo: gateway-proxy
  template:
    metadata:
      labels:
        gloo: gateway-proxy
    spec:
      containers:
      - args: ["--disable-hot-restart"]
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        image: soloio/gloo-envoy-wrapper:0.8.6
        imagePullPolicy: Always
        name: gateway-proxy
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        - containerPort: 8443
          name: https
          protocol: TCP
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config
        - mountPath: /etc/certs/
          name: istio-certs
          readOnly: true
      volumes:
      - configMap:
          name: gateway-envoy-config
        name: envoy-config
      - name: istio-certs
        secret:
          defaultMode: 420
          optional: true
          secretName: istio.default
{{< /highlight >}}


The updated Upstream should look like this:
{{< highlight yaml "hl_lines=19-23" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","kind":"Service","metadata":{"annotations":{},"labels":{"app":"productpage"},"name":"productpage","namespace":"default"},"spec":{"ports":[{"name":"http","port":9080}],"selector":{"app":"productpage"}}}
  creationTimestamp: 2019-02-27T03:00:44Z
  generation: 1
  labels:
    app: productpage
    discovered_by: kubernetesplugin
  name: default-productpage-9080
  namespace: gloo-system
  resourceVersion: "3409"
  selfLink: /apis/gloo.solo.io/v1/namespaces/gloo-system/upstreams/default-productpage-9080
  uid: dfd33b6c-3a3b-11e9-98c6-02425fecee06
spec:
  discoveryMetadata: {}
  sslConfig:
    sslFiles:
      tlsCert: /etc/certs/cert-chain.pem
      tlsKey: /etc/certs/key.pem
      rootCa: /etc/certs/root-cert.pem
  kube:
    selector:
      app: productpage
    serviceName: productpage
    serviceNamespace: default
    servicePort: 9080
status:
  reported_by: gloo
  state: 1
{{< /highlight >}}

{{% /expand %}}

### Istio 1.1.x

{{% expand "Click to see instructions for Istio 1.1.x." %}}

Gloo Edge can easily and automatically plug into the Istio SDS architecture. To allow Gloo Edge to do this, let's configure the Gloo Edge gateway proxy (Envoy) to communicate with the Istio SDS over the Unix Domain Socket:

Here's an example of an edited deployment:
{{< highlight yaml "hl_lines=51-52 63-66" >}}
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    app: gloo
    gloo: gateway-proxy
  name: gateway-proxy
  namespace: gloo-system
spec:
  replicas: 1
  selector:
    matchLabels:
      gloo: gateway-proxy
  strategy:
  template:
    metadata:
      creationTimestamp: null
      labels:
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
        image: quay.io/solo-io/gloo-envoy-wrapper:0.11.1
        imagePullPolicy: Always
        name: gateway-proxy
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        - containerPort: 8443
          name: https
          protocol: TCP
        resources: {}
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config
        - mountPath: /var/run/sds
          name: sds-uds-path
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
      volumes:
      - configMap:
          defaultMode: 420
          name: gateway-envoy-config
        name: envoy-config
      - hostPath:
          path: /var/run/sds
          type: ""
        name: sds-uds-path
{{< /highlight >}}

Here's an example of the edited Upstream for Istio 1.1:

{{< highlight yaml "hl_lines=23-31" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  creationTimestamp: "2019-03-19T22:27:20Z"
  generation: 3
  labels:
    app: productpage
    discovered_by: kubernetesplugin
    service: productpage
  name: default-productpage-9080
  namespace: gloo-system
  resourceVersion: "7643"
  selfLink: /apis/gloo.solo.io/v1/namespaces/gloo-system/upstreams/default-productpage-9080
  uid: 28d7d8d5-4a96-11e9-b355-d2c82e77d7fe
spec:
  discoveryMetadata: {}
  kube:
    selector:
      app: productpage
    serviceName: productpage
    serviceNamespace: default
    servicePort: 9080
  sslConfig:
    sds:
      callCredentials:
        fileCredentialSource:
          header: istio_sds_credentail_header-bin
          tokenFileName: /var/run/secrets/kubernetes.io/serviceaccount/token
      certificatesSecretName: default
      targetUri: unix:/var/run/sds/uds_path
      validationContextName: ROOTCA
status:
  reported_by: gloo
  state: 1
{{< /highlight >}}

{{% notice note %}}
Note that Istio has a misspelling on version 1.1.17, using 'credentail' instead of 'credential' in the header.
This was fixed by Istio 1.3.6.
{{% /notice %}}

{{% /expand %}}

### Istio 1.3.x and 1.4.x

{{% expand "Click to see configuration for Istio 1.3.x/1.4.x." %}}

In Istio 1.3 there were some changes to the token used to authenticate as well as how that projected token gets into the gateway. For the gateway proxy, we need to use a new header name as well as point to the new location of the projected token:

{{< highlight yaml "hl_lines=16-17 32-39" >}}
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    app: gloo
    gloo: gateway-proxy
  name: gateway-proxy
  namespace: gloo-system
spec:
...
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config
        - mountPath: /var/run/sds
          name: sds-uds-path
        - mountPath: /var/run/secrets/tokens
          name: istio-token
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
      volumes:
      - configMap:
          defaultMode: 420
          name: gateway-envoy-config
        name: envoy-config
      - hostPath:
          path: /var/run/sds
          type: ""
        name: sds-uds-path
      - name: istio-token
        projected:
          defaultMode: 420
          sources:
          - serviceAccountToken:
              audience: istio-ca
              expirationSeconds: 43200
              path: istio-token
...        
{{< /highlight >}}

Here's an example of the edited Upstream for Istio 1.3 and 1.4:
{{< highlight yaml "hl_lines=15-23" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: default-productpage-9080
  namespace: gloo-system
...
spec:
  discoveryMetadata: {}
  kube:
    selector:
      app: productpage
    serviceName: productpage
    serviceNamespace: default
    servicePort: 9080
  sslConfig:
    sds:
      callCredentials:
        fileCredentialSource:
          header: istio_sds_credentials_header-bin
          tokenFileName: /var/run/secrets/tokens/istio-token
      certificatesSecretName: default
      targetUri: unix:/var/run/sds/uds_path
      validationContextName: ROOTCA
...
{{< /highlight >}}

For either version, in the above snippet we configure the location of the Unix Domain Socket where the Istio node agent is listening. Istio's node agent is the one that generates the certificates/keys, communicates with Istio Citadel to sign the certificate, and ultimately provides the SDS API for Envoy/Gloo Edge's Gateway proxy. The other configurations control the location of the JWT token for the service account under which the proxy runs (so the node agent can verify what identity is being requested) and how the request will be sent (in a header, etc). 

---

{{% /expand %}}

### Istio 1.5.x

{{% expand "Click to see configuration for Istio 1.5.x." %}}

{{% notice warning %}}

The Gloo Edge integration with Istio 1.5.x requires Gloo Edge version 1.3.20 or 1.4.0-beta1, or higher.

{{% /notice %}}

We will update our gateway-proxy deployment as follows:

{{< highlight yaml "hl_lines=61-156 162-179" >}}
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
        image: quay.io/solo-io/gloo-envoy-wrapper:1.5.0-beta23
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
            add:
            - NET_BIND_SERVICE
            drop:
            - ALL
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config
      - name: sds
        image: quay.io/solo-io/sds:1.5.0-beta23
        imagePullPolicy: Always
        ports:
        - containerPort: 8234
          name: sds
          protocol: TCP
        volumeMounts:
        - mountPath: /etc/istio-certs/
          name: istio-certs
        - mountPath: /etc/envoy
          name: envoy-config
        env:
          - name: POD_NAME
            valueFrom:
              fieldRef:
                fieldPath: metadata.name
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: ISTIO_MTLS_SDS_ENABLED
            value: "true"
      - name: istio-proxy
        image: docker.io/istio/proxyv2:1.5.1
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
        - istio-pilot.istio-system.svc:15012
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
        - "15020"
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
            port: 15020
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
      volumes:
      - configMap:
          defaultMode: 420
          name: gateway-proxy-envoy-config
        name: envoy-config
      - name: istio-certs
        emptyDir:
          medium: Memory
      - name: istiod-ca-cert
        configMap:
          defaultMode: 420
          name: istio-ca-root-cert
      - emptyDir:
          medium: Memory
        name: istio-envoy
      - name: istio-token
        projected:
          defaultMode: 420
          sources:
          - serviceAccountToken:
              audience: istio-ca
              expirationSeconds: 43200
              path: istio-token
{{</highlight>}}

These values were tested with a default Istio 1.5.1 installation. If you have customized your installation your installation these may need adjustment. Please refer to the [instructions](https://istio.io/blog/2020/proxy-cert/) on the istio.io website.

Here's an example of the edited Upstream for Istio 1.5.1:

{{< highlight yaml "hl_lines=15-21" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: default-productpage-9080
  namespace: gloo-system
...
spec:
  discoveryMetadata: {}
  kube:
    selector:
      app: productpage
    serviceName: productpage
    serviceNamespace: default
    servicePort: 9080
  sslConfig:
    alpn_protocols:
    - istio
    sds:
      targetUri: 127.0.0.1:8234
      certificatesSecretName: istio_server_cert
      validationContextName: istio_validation_context
...
{{< /highlight >}}

Note that `alpn_protocols` is supported in Upstreams starting in Gloo Edge 1.3.20.

{{% /expand %}}


### Istio 1.6.x

{{% expand "Click to see configuration for Istio 1.6.x." %}}

{{% notice warning %}}

The Gloo Edge integration with Istio 1.6.x requires Gloo Edge version 1.5.0-beta23, or higher.

{{% /notice %}}

We will update our gateway-proxy deployment as follows:

{{< highlight yaml "hl_lines=61-177 183-200" >}}
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
        image: quay.io/solo-io/gloo-envoy-wrapper:1.5.0-beta23
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
            add:
            - NET_BIND_SERVICE
            drop:
            - ALL
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config
      - name: sds
        image: quay.io/solo-io/sds:1.5.0-beta23
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 8234
          name: sds
          protocol: TCP
        volumeMounts:
        - mountPath: /etc/istio-certs/
          name: istio-certs
        - mountPath: /etc/envoy
          name: envoy-config
        env:
          - name: POD_NAME
            valueFrom:
              fieldRef:
                fieldPath: metadata.name
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: ISTIO_MTLS_SDS_ENABLED
            value: "true"
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
        - istio-pilot.istio-system.svc:15012
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
      volumes:
      - configMap:
          defaultMode: 420
          name: gateway-proxy-envoy-config
        name: envoy-config
      - name: istio-certs
        emptyDir:
          medium: Memory
      - name: istiod-ca-cert
        configMap:
          defaultMode: 420
          name: istio-ca-root-cert
      - emptyDir:
          medium: Memory
        name: istio-envoy
      - name: istio-token
        projected:
          defaultMode: 420
          sources:
          - serviceAccountToken:
              audience: istio-ca
              expirationSeconds: 43200
              path: istio-token
{{</highlight>}}

These values were tested with a default Istio 1.6.8 installation. If you have customized your installation your installation these may need adjustment. Please refer to the [instructions](https://istio.io/blog/2020/proxy-cert/) on the istio.io website.

Here's an example of the edited Upstream for Istio 1.6.8:

{{< highlight yaml "hl_lines=15-21" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: default-productpage-9080
  namespace: gloo-system
...
spec:
  discoveryMetadata: {}
  kube:
    selector:
      app: productpage
    serviceName: productpage
    serviceNamespace: default
    servicePort: 9080
  sslConfig:
    alpn_protocols:
    - istio
    sds:
      targetUri: 127.0.0.1:8234
      certificatesSecretName: istio_server_cert
      validationContextName: istio_validation_context
...
{{< /highlight >}}

Note that `alpn_protocols` is supported in Upstreams starting in Gloo Edge 1.3.20.

{{% /expand %}}