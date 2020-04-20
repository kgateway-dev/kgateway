---
title: Gloo and Istio mTLS
weight: 3
---

Serving as the Ingress for an Istio cluster -- without compromising on security -- means supporting mutual TLS communication between Gloo and the rest of the cluster. Mutual TLS means that the client proves its identity to the server (in addition to the server proving its identity to the client, which happens in regular TLS).

## Guide versions

### Istio versions

This guide was tested with Istio 1.0.9, 1.1.17, 1.3.6, 1.4.3, ad 1.5.1.

### Gloo versions

This guide was tested with Gloo v1.3.1.

{{% notice note %}}
Please note that for gloo versions 1.1.x and up, you must run: `kubectl label namespace default discovery.solo.io/function_discovery=disabled` before editing the Upstream. This prevents your changes from being overwritten.
{{% /notice %}}

### Kubernetes versions

This guide was tested with GKE v1.15.

Please note that if you are running Kubernetes > 1.12 in Minikube, you may run into several issues later on when installing Istio in SDS mode. This mode requires the projection of the istio-token service account tokens into volumes. We recommend installing Istio in a cluster which has this feature turned on by default (for example, GKE).

---

## Step 1 - Install Istio

For this exercise, you will need Istio installed with mTLS enabled.

### Download and install

To download and install the latest version of Istio, follow the installation instructions [here](https://istio.io/docs/setup/getting-started/). You will need to set the profile to sds for this guide.

Previous releases can be found for download [here](https://github.com/istio/istio/releases).

For a quick install of Istio 1.0.6 or 1.0.9 (prior to SDS mode option) with mTLS enabled, run the following commands:

```bash
kubectl apply -f install/kubernetes/helm/istio/templates/crds.yaml
kubectl apply -f install/kubernetes/istio-demo-auth.yaml
kubectl get pods -w -n istio-system
```

Use `kubectl get pods -n istio-system` to check the status on the Istio pods and wait until all the pods are **Running** or **Completed**.

### SDS mode

In Istio 1.1, a new option to configure certificates and keys was introduced based on [Envoy Proxy's Secret Discovery Service](https://www.envoyproxy.io/docs/envoy/v1.11.2/configuration/secret.html#secret-discovery-service-sds). This mode enables Istio to deliver the secrets via an API instead of mounting to the file system as with Istio 1.0. This has two big benefits:

* We don't need to hot-restart the proxy when certificates are rotated
* The keys for the services never travel over the network; they stay on a single node and are delivered to the service. 

For more information on [Istio's identity provisioning through SDS](https://istio.io/docs/tasks/security/auth-sds/) take a look at the [Istio documentation](https://istio.io/docs/tasks/security/auth-sds/).

---

## Step 2 - Install bookinfo

Before configuring gloo, you'll need to install the bookinfo sample app to be consistent with this guide, or you can use your preferred Upstream. Either way, you'll need to enable istio-injection in the default namespace:

```bash
kubectl label namespace default istio-injection=enabled
```

To install the bookinfo sample app, cd into your downloaded Istio directory and run this command:
```bash
kubectl apply -f samples/bookinfo/platform/kube/bookinfo.yaml
```

---

## Step 3 - Configure Gloo

This guide assumes that you have Gloo installed. Gloo is installed to the `gloo-system` namespace and should *not* be injected with the Istio sidecar. If you have automatic injection enabled for Istio, make sure the `istio-injection` label does *not* exist on the `gloo-system` namespace. See [the Istio docs on automatic sidecar injection](https://istio.io/docs/setup/kubernetes/additional-setup/sidecar-injection/#automatic-sidecar-injection) for more.

To quickly install Gloo, download *glooctl* and run `glooctl install gateway`. See the [quick start]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) guide for more information.

For Gloo to successfully send requests to an Istio Upstream with mTLS enabled, we need to addthe Istio mTLS secret to the gateway-proxy pod. The secret allows Gloo to authenticate with the Upstream service.

The last configuration step is to configure the relevant Gloo Upstreams with mTLS. We can be fine-grained about which Upstreams have these settings as not all Gloo Upstreams may need/want mTLS enabled. This gives us the flexibility to route to Upstreams
both with and without mTLS enabled - a common occurrence in a brown field environment or during a migration to Istio.

### Without SDS

Edit the gateway-proxy to add Istio certs as a volume mount:
```bash
kubectl edit deploy/gateway-proxy -n gloo-system
```

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

The Gloo gateway will now have access to Istio client secrets.

Let's edit the `productpage` Upstream and tell Gloo to use the secrets that we just mounted into the Gloo Gateway.

Edit the Upstream with this command:
```bash
kubectl edit upstream default-productpage-9080 --namespace gloo-system
```

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

At this point, we have the correct certificates/keys/CAs installed into the proxy and configured for the `productpage` service.

See the bottom of the page for instructions on [testing your configuration]({{% versioned_link_path fromRoot="/guides/integrations/service_mesh/gloo_istio_mtls/#test-your-configuration" %}}).

### With SDS mode

Gloo can easily and automatically plug into the Istio SDS architecture. To allow Gloo to do this, let's configure the Gloo gateway proxy (Envoy) to communicate with the Istio SDS over the Unix Domain Socket:

```bash
kubectl edit deploy/gateway-proxy -n gloo-system
```

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

In Istio 1.3 there were some changes to the token used to authenticate as well as how that projected token gets into the gateway. For Istio 1.3 and 1.4, let's also add the projected token:

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

Next, we need to update the `productpage` Upstream with the appropriate SDS configuration:

```bash
kubectl edit upstream default-productpage-9080 -n gloo-system
```

### Istio 1.1.x

Here's an example of the edited Upstream for Istio 1.1.

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

### Istio 1.3.x and 1.4.x

For Istio 1.3 and 1.4, we need to use the new header name as well as point to the new location of the projected token.

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

For either version, in the above snippet we configure the location of the Unix Domain Socket where the Istio node agent is listening. Istio's node agent is the one that generates the certificates/keys communicates with Istio Citadel to sign the certificate, and ultimately provides the SDS API for Envoy/Gloo's Gateway proxy. The other various configurations are the location of the JWT token for the service account under which the proxy runs so the node agent can verify what identity is being requested, and finally how the request will be sent (in a header, etc). 

At this point, the Gloo gateway-proxy can communicate with Istio's SDS and consume the correct certificates and keys to participate in mTLS with the rest of the Istio mesh.

---

## Test your configuration

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


### Istio 1.5.x

{{% notice warning %}}

The Gloo integration with Istio 1.5.x requires version 1.3.20 or 1.4.0-beta1, or higher.

{{% /notice %}}

The [recommended way](https://istio.io/blog/2020/proxy-cert/) to adding workloads to the mesh without a proxy, is to used a sidecar to provision the mesh certificates to a shared volume.

To do that, we will update our gateway-proxy deployment as follows:

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
        image: quay.io/solo-io/gloo-envoy-wrapper:v1.3.18
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
        - mountPath: /etc/istio-certs/
          name: istio-certs
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

These values were tested with a default istio 1.5.1 installation. If you have customized your installation your installation these may need adjustment. Please refer to the [instructions](https://istio.io/blog/2020/proxy-cert/) on the istio.io website.

upstream:

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
    sslFiles:
      tlsCert: /etc/istio-certs/cert-chain.pem
      tlsKey: /etc/istio-certs/key.pem
      rootCa: /etc/istio-certs/root-cert.pem
...
{{< /highlight >}}

Note that alpn_protocols is supported in upstreams starting gloo 1.3.18.

#### Test
As this is a bit involved, lets take a step by step approach to test this. This was tested with Istio 1.5.1.

First, create a kubernetes cluster. `kind` or `minikube` should work:
```
kind create cluster
```

Verify istio version:
```
istioctl version
client version: 1.5.1
```

Install istio and book info:
```
istioctl manifest apply --set profile=minimal
kubectl label namespace default istio-injection=enabled
kubectl apply -f https://raw.githubusercontent.com/istio/istio/1.5.1/samples/bookinfo/platform/kube/bookinfo.yaml
```

Verify gloo ≥ v1.4.0-beta1:
```
glooctl version 
```

Install gloo:
```
glooctl install gateway 
```
or with helm:
```
kubectl create ns gloo-system; helm install --namespace gloo-system --version 1.4.0-beta1 gloo gloo/gloo
```

Patch gloo's envoy deployment. Note that in this example we are using kind, so our parameters are different than outlined above (note that you may need to update the version of the gateway-proxy container to match your version. in this example it is set to `gloo-envoy-wrapper:1.4.0-beta1`):
```
kubectl patch deployment -n gloo-system gateway-proxy --type merge -p '{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"labels":{"app":"gloo","gateway-proxy-id":"gateway-proxy","gloo":"gateway-proxy"},"name":"gateway-proxy","namespace":"gloo-system"},"spec":{"selector":{"matchLabels":{"gateway-proxy-id":"gateway-proxy","gloo":"gateway-proxy"}},"template":{"metadata":{"labels":{"gateway-proxy":"live","gateway-proxy-id":"gateway-proxy","gloo":"gateway-proxy"}},"spec":{"containers":[{"args":["--disable-hot-restart"],"env":[{"name":"POD_NAMESPACE","valueFrom":{"fieldRef":{"apiVersion":"v1","fieldPath":"metadata.namespace"}}},{"name":"POD_NAME","valueFrom":{"fieldRef":{"apiVersion":"v1","fieldPath":"metadata.name"}}}],"image":"quay.io/solo-io/gloo-envoy-wrapper:1.4.0-beta1","imagePullPolicy":"IfNotPresent","name":"gateway-proxy","ports":[{"containerPort":8080,"name":"http","protocol":"TCP"},{"containerPort":8443,"name":"https","protocol":"TCP"}],"resources":{},"securityContext":{"allowPrivilegeEscalation":false,"capabilities":{"add":["NET_BIND_SERVICE"],"drop":["ALL"]},"readOnlyRootFilesystem":true},"terminationMessagePath":"/dev/termination-log","terminationMessagePolicy":"File","volumeMounts":[{"mountPath":"/etc/envoy","name":"envoy-config"},{"mountPath":"/etc/istio-certs/","name":"istio-certs"}]},{"name":"istio-proxy","image":"docker.io/istio/proxyv2:1.5.1","args":["proxy","sidecar","--domain","$(POD_NAMESPACE).svc.cluster.local","--configPath","/etc/istio/proxy","--binaryPath","/usr/local/bin/envoy","--serviceCluster","istio-proxy-prometheus","--drainDuration","45s","--parentShutdownDuration","1m0s","--discoveryAddress","istio-pilot.istio-system.svc:15012","--proxyLogLevel=warning","--proxyComponentLogLevel=misc:error","--connectTimeout","10s","--proxyAdminPort","15000","--controlPlaneAuthPolicy","NONE","--dnsRefreshRate","300s","--statusPort","15020","--trust-domain=cluster.local","--controlPlaneBootstrap=false"],"env":[{"name":"OUTPUT_CERTS","value":"/etc/istio-certs"},{"name":"JWT_POLICY","value":"first-party-jwt"},{"name":"PILOT_CERT_PROVIDER","value":"istiod"},{"name":"CA_ADDR","value":"istiod.istio-system.svc:15012"},{"name":"POD_NAME","valueFrom":{"fieldRef":{"fieldPath":"metadata.name"}}},{"name":"POD_NAMESPACE","valueFrom":{"fieldRef":{"fieldPath":"metadata.namespace"}}},{"name":"ISTIO_META_MESH_ID","value":"cluster.local"},{"name":"INSTANCE_IP","valueFrom":{"fieldRef":{"fieldPath":"status.podIP"}}},{"name":"SERVICE_ACCOUNT","valueFrom":{"fieldRef":{"fieldPath":"spec.serviceAccountName"}}},{"name":"HOST_IP","valueFrom":{"fieldRef":{"fieldPath":"status.hostIP"}}},{"name":"ISTIO_META_POD_NAME","valueFrom":{"fieldRef":{"apiVersion":"v1","fieldPath":"metadata.name"}}},{"name":"ISTIO_META_CONFIG_NAMESPACE","valueFrom":{"fieldRef":{"apiVersion":"v1","fieldPath":"metadata.namespace"}}}],"imagePullPolicy":"IfNotPresent","readinessProbe":{"failureThreshold":30,"httpGet":{"path":"/healthz/ready","port":15020,"scheme":"HTTP"},"initialDelaySeconds":1,"periodSeconds":2,"successThreshold":1,"timeoutSeconds":1},"volumeMounts":[{"mountPath":"/var/run/secrets/istio","name":"istiod-ca-cert"},{"mountPath":"/etc/istio/proxy","name":"istio-envoy"},{"mountPath":"/etc/istio-certs/","name":"istio-certs"}]}],"volumes":[{"configMap":{"defaultMode":420,"name":"gateway-proxy-envoy-config"},"name":"envoy-config"},{"name":"istio-certs","emptyDir":{"medium":"Memory"}},{"name":"istiod-ca-cert","configMap":{"defaultMode":420,"name":"istio-ca-root-cert"}},{"emptyDir":{"medium":"Memory"},"name":"istio-envoy"}]}}}}'
```

Create an mTLS enabled upstream:
```
kubectl apply -f - <<EOF
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  labels:
    app: productpage
    service: productpage
  name: default-productpage-9080-sds
  namespace: gloo-system
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
    sslFiles:
      tlsCert: /etc/istio-certs/cert-chain.pem
      tlsKey: /etc/istio-certs/key.pem
      rootCa: /etc/istio-certs/root-cert.pem
EOF
```

Add route
```
glooctl add route --dest-name default-productpage-9080-sds --dest-namespace gloo-system --path-prefix /
```

Test:
```
curl -v $(glooctl proxy url)/productpage
```
