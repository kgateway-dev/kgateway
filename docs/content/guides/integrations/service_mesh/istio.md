---
title: Gloo Gateway and Istio
menuTitle: Configure your gateway to run an Istio sidecar 
weight: 1
---

### About Istio

The open source project Istio is the leading service mesh implementation that offers powerful features to secure, control, connect, and monitor cloud-native, distributed applications. Istio is designed for workloads that run in one or more Kubernetes clusters, but you can also extend your service mesh to include virtual machines and other endpoints that are hosted outside your cluster. The key benefits of Istio include: 

* Automatic load balancing for HTTP, gRPC, WebSocket, MongoDB, and TCP traffic
* Secure TLS encryption for service-to-service communication with identity-based authentication and authorization
* Advanced routing and traffic management policies, such as retries, failovers, and fault injection
* Fine-grained access control and quotas
* Automatic logs, metrics, and traces for traffic in the service mesh

### About the Gloo Gateway Istio integration

Gloo Gateway comes with an Istio integration that allows you to configure your gateway proxy with an Istio sidecar. The Istio sidecar uses mutual TLS (mTLS) to prove its identity and to secure the connection between your gateway and the services in your Istio service mesh. In addition, you can control and secure the traffic that enters the mesh by applying all the advanced routing, traffic management, security, and resiliency capabilities that Gloo Gateway offers. For example, you can set up end-user authentication and authorization, per-user rate limiting quotas, web application filters, and access logging to help prevent malicious attacks and audit service mesh usage. 

### Changes to the Istio integration in 1.17

In Gloo Gateway 1.17, a new auto-mTLS feature was introduced that simplifies the integration with Istio service meshes. The auto-mTLS feature automatically injects mTLS configuration into all Upstream resources in your cluster. Without auto-mTLS, every Upstream must be updated manually to add the mTLS configuration. 


## Set up an Istio service mesh

Use Solo.io's Gloo Mesh Enterprise product to install a managed Istio version by using the built-in Istio lifecycle manager, or manually install and manage your own Istio installation. 

{{< tabs >}}
{{% tab name="Managed Istio with Gloo Mesh Enterprise" %}}

Gloo Mesh Enterprise is a service mesh management plane that is based on hardened, open-source projects like Envoy and Istio. With Gloo Mesh, you can unify the configuration, operation, and visibility of service-to-service connectivity across your distributed applications. These apps can run in different virtual machines (VMs) or Kubernetes clusters on premises or in various cloud providers, and even in different service meshes.

Follow the [Gloo Mesh Enterprise get started guide](https://docs.solo.io/gloo-mesh-enterprise/latest/getting_started/single/gs_single/) to quickly install a managed Solo distribution of Istio by using the built-in Istio lifecycle manager. 

{{% /tab %}}
{{% tab name="Manual Istio installation" %}}

Set up Istio. Choose between the following options to set up Istio: 
* [Manually install a Solo distribution of Istio](https://docs.solo.io/gloo-mesh-enterprise/latest/istio/manual/manual_deploy/). 
* Install an open source distribution of Istio by following the [Istio documentation](https://istio.io/latest/docs/setup/getting-started/). 

{{% /tab %}}
{{< /tabs >}}

## Deploy the httpbin app

1. Deploy the httpbin app.  
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: v1
   kind: ServiceAccount
   metadata:
     name: httpbin
   ---
   apiVersion: v1
   kind: Service
   metadata:
     name: httpbin
     labels:
       app: httpbin
   spec:
     ports:
     - name: http
       port: 8000
       targetPort: 80
     selector:
       app: httpbin
   ---
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: httpbin
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: httpbin
         version: v1
     template:
       metadata:
         labels:
           app: httpbin
           version: v1
       spec:
         serviceAccountName: httpbin
         containers:
         - image: docker.io/kennethreitz/httpbin
           imagePullPolicy: IfNotPresent
           name: httpbin
           ports:
           - containerPort: 80
   EOF
   ```

2. Verify that the httpbin app is running. 
   ```sh
   kubectl get pods 
   ```
   
3. Create a VirtualService to configure routing to the httpbin app. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: httpbin
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
       - '*'
       routes:
       - matchers:
         - prefix: /
         routeAction:
           single:
             upstream:
               name: default-httpbin-8000
               namespace: gloo-system
   EOF
   ```
   
4. Send a request to the httpbin app and verify that you get back a 200 HTTP response code. 
   ```sh
   curl -vik $(glooctl proxy url --name=gateway-proxy)/headers
   ```
   
   Example output: 
   ```
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   ...
   {
     "headers": {
       "Accept": "*/*", 
       "Host": "34.162.22.180", 
       "User-Agent": "curl/7.77.0", 
       "X-Envoy-Expected-Rq-Timeout-Ms": "15000"
     }
   }
   ```

## Enable the Istio integration in Gloo Gateway

Upgrade your Gloo Gateway installation to enable the Istio integration. 

1. Get the name of the istiod service. Depending on how you set up Istio, you might see a revisionless service name (`istiod`) or a service name with a revision, such as `istiod-1-21`. 
   ```sh
   kubectl get services -n istio-system
   ```
   
   Example output: 
   ```                          
   NAME          TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)                                 AGE
   istiod-1-21   ClusterIP   10.102.24.31   <none>        15010/TCP,15012/TCP,443/TCP,15014/TCP   3h49m
   ``` 

2. Derive the Kubernetes service address for your istiod deployment. The service address uses the format `<service-name>.<namespace>.svc:15012`. For example, if your service name is `istiod-1-21`, the full service address is `istiod-1-21.istio-system.svc:15012`.

3. Get the Helm values for your current Gloo Gateway installation. 
   ```sh
   helm get values gloo -n gloo-system -o yaml > gloo-gateway.yaml
   open gloo-gateway.yaml
   ```
   
4. Add the following values to the Helm value file to enable the Istio integration with auto-mTLS. Make sure that you change the `istioProxyContainer` values to the service address and cluster name of your Istio installation.

   {{< notice note >}}If you do not want auto-mTLS to be enabled, set this value to <code>false</code>.{{< /notice >}}
   {{< tabs  >}}
   {{% tab name="Open source" %}}
   ```yaml
   global:
     istioIntegration:
       enableAutoMtls: true
       enabled: true
     istioSDS:
       enabled: true
   gatewayProxies:
     gatewayProxy:
       istioDiscoveryAddress: istiod-1-21.istio-system.svc:15012
       istioMetaClusterId: mycluster
       istioMetaMeshId: mycluster
   ```
  
   {{% /tab %}}
   {{% tab name="Enterprise Edition" %}}
   
   ```yaml
   global:
     istioIntegration:
       enableAutoMtls: true
       enabled: true
     istioSDS:
       enabled: true
   gloo:
     gatewayProxies:
       gatewayProxy:
         istioDiscoveryAddress: istiod-1-21.istio-system.svc:15012
         istioMetaClusterId: mycluster
         istioMetaMeshId: mycluster
   ```
   {{% /tab %}}
   {{< /tabs >}}
   
   | Setting | Description |
   | -- | -- | 
   | `enableAutoMtls` | Automatically configure all Upstream resources in your cluster for mTLS. | 
   | `istioDiscoveryAddress` | The address of the istiod service. If omitted, `istiod.istio-system.svc:15012` is used. |
   | `istioMetaClusterId` </br> `istioMetaMeshId` | The name of the cluster where Gloo Gateway is installed. |
   
5. Upgrade your Gloo Gateway installation. 
   {{< tabs >}}
   {{% tab name="Open source" %}}
   ```sh
   helm upgrade -n gloo-system gloo gloo/gloo \
      -f gloo-gateway.yaml \
      --version={{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}
   ```
   {{% /tab %}}
   {{% tab name="Enterprise Edition" %}}
   ```sh
   helm upgrade -n gloo-system gloo glooe/gloo-ee \
    -f gloo-gateway.yaml \
    --version={{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   ```
   {{% /tab %}}
   {{< /tabs >}}

6. Verify that your `gateway-proxy` pod is restarted with 3 containers now, `gateway-proxy`, `istio-proxy`, and `sds`. 
   ```sh
   kubectl get pods -n gloo-system | grep gateway-proxy
   ```
   
   Example output: 
   ```
   gateway-proxy-f7cd596b7-tv5z7    3/3     Running            0              3h31m
   ```
   
## Set up mTLS routing to httpbin


1. Label the httpbin namespace for Istio sidecar injection. 
   ```sh
   export REVISION=$(kubectl get pod -L app=istiod -n istio-system -o jsonpath='{.items[0].metadata.labels.istio\.io/rev}')      
   echo $REVISION
   kubectl label ns default istio.io/rev=$REVISION --overwrite=true
   ```
  
2. Perform a rollout restart for the httpbin deployment so that an Istio sidecar is added to the httpbin app and the app is included in your service mesh. 
   ```sh
   kubectl rollout restart deployment httpbin 
   ```
   
3. Verify that the httpbin app comes up with an additional container. 
   ```sh
   kubectl get pods 
   ```
   
   Example output: 
   ```
   NAME                      READY   STATUS    RESTARTS   AGE
   httpbin-f798c698d-vpltn   2/2     Running   0          15s
   ```

4. Create a strict PeerAuthentication policy to require all traffic in the mesh to use mTLS.
   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: "security.istio.io/v1beta1"
   kind: "PeerAuthentication"
   metadata:
     name: "test"
     namespace: "istio-system"
   spec:
     mtls:
       mode: STRICT
   EOF
   ```

6. **Without auto-mTLS**: If you enabled auto-mTLS, the Upstream that represents the httpbin app is automatically configured for mTLS. However, if auto-mTLS is not enabled, you must manually configure the Upstream for mTLS. 
   ```sh
   glooctl istio enable-mtls --upstream default-httpbin-8000
   ```
   
   {{< notice note >}}
   If you do not add mTLS configuration to your Upstream and you try to send a request to the app, the request is denied with a 503 HTTP response code and you see an error message similar to the following: <code>upstream connect error or disconnect/reset before headers. reset reason: connection termination</code>. 
   {{< /notice >}}
   

7. Send a request to the httpbin app. Verify that you get back a 200 HTTP response and that an [`x-forwarded-client-cert`](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/headers#x-forwarded-client-cert) header is returned. The presence of this header indicates that the connection from the gateway to the httpbin app is now encrypted via mutual TLS. 

   ```sh
   curl -vik $(glooctl proxy url --name=gateway-proxy)/headers
   ```

   Example output: 
   {{< highlight yaml "hl_lines=21-22" >}}
   {
    "headers": {
      "Accept": [
        "*/*"
      ],
      "Host": [
        "www.example.com:8080"
      ],
      "User-Agent": [
        "curl/7.77.0"
      ],
      "X-B3-Sampled": [
        "0"
      ],
      "X-B3-Spanid": [
        "92744e97e79d8f22"
      ],
      "X-B3-Traceid": [
        "8189f0a6c4e3582792744e97e79d8f22"
      ],
      "X-Forwarded-Client-Cert": [
        "By=spiffe://gloo-gateway-docs-mgt/ns/httpbin/sa/httpbin;Hash=3a57f9d8fddea59614b4ade84fcc186edeffb47794c06608068a3553e811bdfe;Subject=\"\";URI=spiffe://gloo-gatewa-docs-mgt/ns/gloo-system/sa/gloo-proxy-http"
      ],
      "X-Forwarded-Proto": [
        "http"
      ],
      "X-Request-Id": [
        "7f1d6e38-3bf7-44fd-8298-a77c34e5b865"
      ]
    }
   }
   {{< /highlight >}}
   
   
## Cleanup

You can optionally remove the resources that you created. 

1. Follow the [Uninstall guide in the Gloo Mesh Enterprise documentation](https://docs.solo.io/gloo-mesh-enterprise/main/setup/uninstall/) to remove Gloo Mesh Enterprise. 
   
2. Upgrade your Gloo Gateway Helm installation and remove the Helm values that you added as part of this guide. 

3. Remove the Istio sidecar from the httpbin app. 
   1. Remove the Istio label from the httpbin namespace. 
      ```sh
      kubectl label ns default istio.io/rev-
      ```
   2. Perform a rollout restart for the httpbin deployment. 
      ```sh
      kubectl rollout restart deployment httpbin
      ```
   3. Verify that the Istio sidecar container is removed. 
      ```sh
      kubectl get pods 
      ```
      
      Example output: 
      ```
      NAME                       READY   STATUS        RESTARTS   AGE
      httpbin-7d4965fb6d-mslx2   1/1     Running       0          6s
      ```

4. Delete the VirtualService resource. 
   ```sh
   kubectl delete virtualservice httpbin -n gloo-system
   ```
   
5. Remove the httpbin app. 
   ```sh
   kubectl delete serviceaccount httpbin
   kubectl delete service httpbin
   kubectl delete deployment httpbin
   ```
