---
title: Gloo and Istio mTLS
weight: 3
---

## Motivation

Serving as the Ingress for an Istio cluster -- without compromising on security -- means supporting 
mutual TLS communication between Gloo and the rest of the cluster. Mutual TLS means that the client 
proves its identity to the server (in addition to the server proving its identity to the client, which happens in regular TLS).


For this exercise, you will need Istio installed with mTLS enabled. This guide was tested with istio 1.0.6 and Istio 1.1. 

This guide also assumes that you have Gloo installed. Gloo is installed to the `gloo-system` namespace
and should *not* be injected with the Istio sidecar. If you have automatic injection enabled for Istio, make sure the `istio-injection` label does NOT exist on the `gloo-system` namespace. See [the Istio docs on automatic sidecar injection](https://istio.io/docs/setup/kubernetes/additional-setup/sidecar-injection/#automatic-sidecar-injection) for more. 

To quickly install Gloo, download *glooctl* and run `glooctl install gateway`. See the 
[quick start](../../../installation/gateway/kubernetes/) guide for more information.

## Istio 1.0.x
For a quick install of Istio 1.0.6 on minikube with mTLS enabled, run the following commands:
```bash
kubectl apply -f install/kubernetes/helm/istio/templates/crds.yaml
kubectl apply -f install/kubernetes/istio-demo-auth.yaml
kubectl get pods -w -n istio-system
```

Use `kubectl get pods -n istio-system` to check the status on the Istio pods and wait until all the 
pods are **Running** or **Completed**.

Install bookinfo sample app:

```bash
kubectl label namespace default istio-injection=enabled
kubectl apply -f samples/bookinfo/platform/kube/bookinfo.yaml
```

### Configure Gloo
For Gloo to successfully send requests to an Istio upstream with mTLS enabled, we need to add
the Istio mTLS secret to the gateway-proxy pod. The secret allows Gloo to authenticate with the 
upstream service.

Edit the pod, with the command `kubectl edit -n gloo-system deploy/gateway-proxy`, 
and add istio certs volume and volume mounts. Here's an example of an edited deployment:
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

The Gloo gateway will now have access to Istio client secrets. The last configuration step is to 
configure the relevant Gloo upstreams with mTLS. We can be fine-grained about which upstreams have these settings as not all Gloo upstreams may need/want mTLS enabled. This gives us the flexibility to route to upstreams
both with and without mTLS enabled - a common occurrence in a brown field environment or during a migration to Istio.

Let's edit the `productpage` upstream and tell Gloo to use the secrets that are now mounted into the Gloo Gateway and we configured in the previous step.

Edit the upstream with the command `kubectl edit upstream default-productpage-9080 --namespace gloo-system`. The updated upstream should look like this:

{{< highlight yaml "hl_lines=20-24" >}}
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

At this point, we have the correct certificates/keys/CAs installed into the proxy and configured for the `productpage` service. We can now set up a Gloo route:

```bash
glooctl add route --name prodpage --namespace gloo-system --path-prefix / --dest-name default-productpage-9080 --dest-namespace gloo-system
```

Access the ingress url:
```
HTTP_GW=$(glooctl proxy url)
## Open the ingress url in the browser:
$([ "$(uname -s)" = "Linux" ] && echo xdg-open || echo open) $HTTP_GW
```

## Istio 1.1.x

With Istio 1.1, a new option to configure certificates and keys was introduced based on [Envoy Proxy's Secret Discovery Service](https://www.envoyproxy.io/docs/envoy/v1.11.2/configuration/secret.html#secret-discovery-service-sds). This mode enables Istio to deliver the secrets via an API instead of mounting to the file system as we saw in the previous section. This has two big benefits:

* We don't need to hot-restart the proxy when certificates are rotated
* The keys for the services never travel over the network; they stay on a single node and are delivered to the service. 

For more information on [Istio's identity provisioning through SDS](https://istio.io/docs/tasks/security/auth-sds/) take a look at the [Istio documentation](https://istio.io/docs/tasks/security/auth-sds/).

Just like in the previous section, we need Istio installed with SDS enabled, and the bookinfo example deployed. To install Istio with SDS you can [review their installation steps](https://istio.io/docs/tasks/security/auth-sds/). To install the bookinfo application, refer to the previous section.

### Configure Gloo

Gloo can easily and automatically plug into the Istio SDS architecture. To configure Gloo to do this, similarly to how we did in the previous section with the older Istio identity architecture, Let's configure the Gloo gateway proxy (Envoy) to communicate with the Istio SDS over Unix Domain Socket: 

```bash
kubectl edit deploy/gateway-proxy -n gloo-system
```
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

Next, we need to update the `productpage` upstream with the appropriate SDS configuration:

```bash
kubectl edit upstream default-productpage-9080  -n gloo-system
```

{{< highlight yaml "hl_lines=24-32" >}}
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
  upstreamSpec:
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
            header: istio_sds_credential_header-bin
            tokenFileName: /var/run/secrets/kubernetes.io/serviceaccount/token
        certificatesSecretName: default
        targetUri: unix:/var/run/sds/uds_path
        validationContextName: ROOTCA
status:
  reported_by: gloo
  state: 1
{{< /highlight >}}  

In the above snippet we configure the location of the Unix Domain Socket where the Istio node agent is listening. Istio's node agent is the one that generates the certificates/keys communicates with Istio Citadel to sign the certificate, and ultimately provides the SDS API for Envoy/Gloo's Gateway proxy. The other various configurations are the location of the JWT token for the service account under which the proxy runs so the node agent can verify what identity is being requested, and finally how the request will be sent (in a header, etc). 


At this point, the Gloo gateway-proxy can communicate with Istio's SDS and consume the correct certificates and keys to participate in mTLS with the rest of the Istio mesh. 

To test this out, we need a route in Gloo:

```bash
glooctl add route --name prodpage --namespace gloo-system --path-prefix / --dest-name default-productpage-9080 --dest-namespace gloo-system
```

And we can curl it:

```bash
curl -v $(glooctl proxy url)/productpage
```


## Changes for Istio 1.3.x


In Istio 1.3 there were some changes to the token used to authenticate as well as how that projected token gets into the gateway. For Istio 1.3, let's add the projected token:


```bash
kubectl edit deploy/gateway-proxy -n gloo-system
```
{{< highlight yaml "hl_lines=17-18 33-40" >}}
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

And in the upstream, point to the new location of the projected token:

```
kubectl edit upstream default-productpage-9080  -n gloo-system
```

{{< highlight yaml "hl_lines=17" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
spec:
  discoveryMetadata: {}
  upstreamSpec:
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
            header: istio_sds_credential_header-bin
            tokenFileName: /var/run/secrets/tokens/istio-token
        certificatesSecretName: default
        targetUri: unix:/var/run/sds/uds_path
        validationContextName: ROOTCA
{{< /highlight >}}
