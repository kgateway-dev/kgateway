---
title: Gloo and Istio mTLS
weight: 3
---

## Motivation

Serving as the Ingress for an Istio cluster -- without compromising on security -- means supporting 
mutual TLS communication between Gloo and the rest of the cluster. Mutual TLS means that the client 
proves its identity to the server (in addition to the server proving its identity to the client, which happens in regular TLS).

##### Istio versions

This guide was tested with Istio 1.0.9, 1.1.17, 1.3.6, and 1.4.3.

##### Gloo versions

This guide was tested with Gloo v1.3.1.

{{% notice note %}}
Please note that for gloo versions 1.1.x and up, you must run: `kubectl label namespace default discovery.solo.io/function_discovery=disabled`
before editing the upstream. This prevents your changes from being overwritten.
{{% /notice %}}

##### Kubernetes versions

This guide was tested with GKE v1.15.

Please note that if you are running Kubernetes > 1.12 in Minikube, you may run into several issues later on when installing
Istio in SDS mode. This mode requires the projection of the istio-token service account tokens into volumes. We recommend installing Istio in a GKE cluster, which has this feature turned on by default.

### Step 1 - Install Istio

For this exercise, you will need Istio installed with mTLS enabled.

#### Download and install

To download and install the latest version of Istio, follow the installation instructions [here](https://istio.io/docs/setup/getting-started/).
You will need to set the profile to sds for this guide.

Previous releases can be found for download [here](https://github.com/istio/istio/releases).

For a quick install of Istio 1.0.6 or 1.0.9 (prior to SDS mode option) with mTLS enabled, run the following commands:
```bash
kubectl apply -f install/kubernetes/helm/istio/templates/crds.yaml
kubectl apply -f install/kubernetes/istio-demo-auth.yaml
kubectl get pods -w -n istio-system
```

Use `kubectl get pods -n istio-system` to check the status on the Istio pods and wait until all the 
pods are **Running** or **Completed**.

#### SDS mode

In Istio 1.1, a new option to configure certificates and keys was introduced based on [Envoy Proxy's Secret Discovery Service](https://www.envoyproxy.io/docs/envoy/v1.11.2/configuration/secret.html#secret-discovery-service-sds). 
This mode enables Istio to deliver the secrets via an API instead of mounting to the file system as with Istio 1.0. This has two big benefits:

* We don't need to hot-restart the proxy when certificates are rotated
* The keys for the services never travel over the network; they stay on a single node and are delivered to the service. 

For more information on [Istio's identity provisioning through SDS](https://istio.io/docs/tasks/security/auth-sds/) take a look at the [Istio documentation](https://istio.io/docs/tasks/security/auth-sds/).


### Step 2 - Install bookinfo

Before configuring gloo, you'll need to install the bookinfo sample app to be consistent with this guide, 
or you can use your preferred upstream. Either way, you'll need to enable istio-injection in the default namespace:
```bash
kubectl label namespace default istio-injection=enabled
```

To install the bookinfo sample app, run this command:
```bash
kubectl apply -f samples/bookinfo/platform/kube/bookinfo.yaml
```

### Step 3 - Configure Gloo

This guide assumes that you have Gloo installed. Gloo is installed to the `gloo-system` namespace
and should *not* be injected with the Istio sidecar. If you have automatic injection enabled for Istio, make sure the `istio-injection` label does NOT exist on the `gloo-system` namespace. See [the Istio docs on automatic sidecar injection](https://istio.io/docs/setup/kubernetes/additional-setup/sidecar-injection/#automatic-sidecar-injection) for more. 

To quickly install Gloo, download *glooctl* and run `glooctl install gateway`. See the 
[quick start](../../../installation/gateway/kubernetes/) guide for more information.

For Gloo to successfully send requests to an Istio upstream with mTLS enabled, we need to add
the Istio mTLS secret to the gateway-proxy pod. The secret allows Gloo to authenticate with the 
upstream service.

The last configuration step is to configure the relevant Gloo upstreams with mTLS. We can be fine-grained about which upstreams have these settings as not all Gloo upstreams may need/want mTLS enabled. This gives us the flexibility to route to upstreams
both with and without mTLS enabled - a common occurrence in a brown field environment or during a migration to Istio.

#### Without SDS

We will need to add Istio certs as a volume mount.
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
Let's edit the `productpage` upstream and tell Gloo to use the secrets that we just mounted into the Gloo Gateway.

Edit the upstream with this command:
```bash
kubectl edit upstream default-productpage-9080 --namespace gloo-system
```

The updated upstream should look like this:
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

#### With SDS mode

Gloo can easily and automatically plug into the Istio SDS architecture. 
To configure Gloo to do this, let's configure the Gloo gateway proxy (Envoy) to communicate with the Istio SDS over Unix Domain Socket:
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

Next, we need to update the `productpage` upstream with the appropriate SDS configuration:
```bash
kubectl edit upstream default-productpage-9080 -n gloo-system
```

#### Istio 1.1.x

Here's an example of the edited upstream for Istio 1.1.

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
Note that Istio has a misspelling on version 1.1, using 'credentail' instead of 'credential' in the header.
This was fixed by Istio 1.3.6.
{{% /notice %}}

#### Istio > 1.3.6

For Istio 1.3 and 1.4, we need to use the new header name as well as point to the new location of the projected token.

Here's an example of the edited upstream for Istio 1.3 and 1.4:
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

In the above snippet we configure the location of the Unix Domain Socket where the Istio node agent is listening. Istio's node agent is the one that generates the certificates/keys communicates with Istio Citadel to sign the certificate, and ultimately provides the SDS API for Envoy/Gloo's Gateway proxy. The other various configurations are the location of the JWT token for the service account under which the proxy runs so the node agent can verify what identity is being requested, and finally how the request will be sent (in a header, etc). 

At this point, the Gloo gateway-proxy can communicate with Istio's SDS and consume the correct certificates and keys to participate in mTLS with the rest of the Istio mesh.

See the bottom of the page for instructions on [testing your configuration]({{% versioned_link_path fromRoot="/gloo_integrations/service_mesh/gloo_istio_mtls/#test-your-configuration" %}}).

### Test your configuration 

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