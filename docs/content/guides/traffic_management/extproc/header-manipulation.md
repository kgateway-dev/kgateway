---
title: Header manipulation
weight: 40
description: Walk through an example for how to manipulate request headers by using an extProc server. 
---

Set up an external processing (extProc) server that manipulates request headers for a sample app.

{{% notice note %}}
External processing is an Enterprise-only feature. 
{{% /notice %}}

{{% notice warning %}}
Envoy's external processing filter is considered a work in progress and has an unknown security posture. Use caution when using this feature in production environments. For more information, see the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_proc_filter#external-processing).
{{% /notice %}}

1. Before you begin, install [Gloo Edge Enterprise]({{% versioned_link_path fromRoot="/installation/enterprise/" %}}) in your cluster. 

2. Set up the extProc server. This example uses a prebuilt extProc server that manipulates request and response headers based on instructions that are sent in an `instructions` header. 
   
   {{< tabs >}}
{{% tab %}}
```yaml
kubectl apply -f- <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ext-proc-grpc
spec:
  selector:
    matchLabels:
      app: ext-proc-grpc
  replicas: 1
  template:
    metadata:
      labels:
        app: ext-proc-grpc
    spec:
      containers:
        - name: ext-proc-grpc
          image: gcr.io/solo-test-236622/ext-proc-example-basic-sink:0.0.2
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 18080
---
apiVersion: v1
kind: Service
metadata:
  name: ext-proc-grpc
  labels:
    app: ext-proc-grpc
  annotations:
    gloo.solo.io/h2_service: "true"
spec:
  ports:
  - port: 4444
    targetPort: 18080
    protocol: TCP
  selector:
    app: ext-proc-grpc
EOF
```
{{% /tab %}}
   {{< /tabs >}}

   The `instructions` header must be provided as a JSON string in the following format: 
  
   ```json 
   {
     "addHeaders": {
       "header1": "value1",
       "header2": "value2"
     },
     "removeHeaders": [ "header3", "header4" ],
     "setBody": "This is the new body",
     "setTrailers": {
       "trailer1": "value1",
       "trailer2": "value2"
     }
   }
   ```
   
3. Verify that the extProc server is up and running. 
   ```sh
   kubectl get pods 
   ```
   
   Example output: 
   ```
   NAME                             READY   STATUS    RESTARTS   AGE
   ext-proc-grpc-59d44ddf76-42q2x   1/1     Running   0          24m
   ```
   
4. Edit the default `Settings` custom resource to enable extProc in Gloo Edge. 
   ```
   kubectl edit settings default -n gloo-system
   ```
   
   Add the following extProc settings to the `spec` section: 
   ```yaml
   extProc: 
     grpcService: 
       extProcServerRef: 
         name: default-ext-proc-grpc-4444
         namespace: gloo-system
     filterStage: 
       stage: AuthZStage
       predicate: After
     failureModeAllow: false
     allowModeOverride: false
     processingMode: 
       requestHeaderMode: SEND
       responseHeaderMode: SKIP
       requestBodyMode: BUFFERED
       responseBodyMode: NONE
       requestTrailerMode: SKIP
       responseTrailerMode: SKIP
   ```
   
   |Setting|Description|
   |--|--|
   |`grpcService`| The configuration of the external processing server that you created earlier.|
   |`grpcService.exProcServerRef.name`| The name of the upstream that was created for the extProc server.|
   |`grpcService.exProcServerRef.namespace`| The namespace of the upstream that was created for the extProc server.|
   |`filterStage`|Where in the filter chain you want to apply the external processing.|
   |`failureModeAllow`|Allow the extProc server to continue when an error is detected during external processing. If set to `true`, the extProc server continues. If set to `false`, external processing is stopped and an error is returned to the Envoy proxy. |
   |`allowModeOverride`|Allow the extProc server to override the processing mode settings that you set. Default value is `false`. |
   |`processingMode`|Decide how you want the extProc server to process request and response information. |
   |`processingMode.requestHeaderMode`|Send (`SEND`) or skip sending (`SKIP`) request header information to the extProc server. |
   |`processingMode.responseHeaderMode`|Send (`SEND`) or skip sending (`SKIP`) response header information to the extProc server. |
   |`processingMode.requestBodyMode` </br>`processingMode.responseBodyMode`|Decide how to provide body information from the request or response to the extProc server. Available settings are: <ul><li> `NONE`: Do not send the body. This is the default value. </li><li> `STREAMED`: Stream the body to the extProc server in pieces as they arrive at the proxy. </li><li> `BUFFERED`: Buffer the body in memory and send the entire body at once. If the body exceeds the maximum buffer limit, an error is sent to the downstream service. </li><li> `BUFFERED_PARTIAL`: Buffer the body in memory and send the entire body at once. If the body exceeds the buffer limit, the body content is sent in pieces up to the buffer limit. </li></ul>|
   |`processingMode.requestTrailerMode` </br>`processingMode.responseTrailerMode`|Send (`SEND`) or skip sending (`SKIP`) request and response trailer information to the extProc server.|
   
5. Deploy the `httpbin` sample app. 
   {{< tabs >}}
{{% tab %}}
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
{{% /tab %}}
   {{< /tabs>}}
   
6. Verify that the httpbin pod is up an running. 
   ```sh
   kubectl get pods | grep httpbin
   ```

7. Create a virtual service to expose the httpbin app on the gateway. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: vs
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

   
8. Send a simple request to the httpbin app and make sure that you get back a 200 HTTP response code. The following request passes in two headers `header1` and `header2` that you see in your reponse. 
   ```
   curl -vik $(glooctl proxy url)/get -H "header1: value1" -H "header2: value2"
   ```
   
   Example output: 
   ```
   HTTP/1.1 200 OK
   ...
   {
     "args": {}, 
     "headers": {
       "Accept": "*/*", 
       "Header1": "value1", 
       "Header2": "value2", 
       "Host": "example.com.com", 
       "User-Agent": "curl/7.77.0", 
       "X-Envoy-Expected-Rq-Timeout-Ms": "15000"
     }, 
     "origin": "10.0.11.109", 
     "url": "http://example.com/get"
   }
   ```

9. Send another request to the httpbin app. This time, you pass along instructions for the extProc server in an `instruction` header. In this example, you use the extProc server to add the `header3` header, and to remove `header2`. 
   ```sh
   curl -vik $(glooctl proxy url)/get -H "header1: value1" -H "header2: value2" -H 'instructions: {"addHeaders":{"header3":"value3","header4":"value4"},"removeHeaders":["instructions", "header2"]}'
   ```
   
   Example output: 
   ```
   {
     "args": {}, 
     "headers": {
       "Accept": "*/*", 
       "Header1": "value1", 
       "Header3": "value3", 
       "Header4": "value4", 
       "Host": "example.com", 
       "User-Agent": "curl/7.77.0", 
       "X-Envoy-Expected-Rq-Timeout-Ms": "15000"
     }, 
     "origin": "10.0.11.109", 
     "url": "http://example.com/get"
   }
   ```
   
10. Edit the `Settings` resource again and set `requestHeaderMode: SKIP`. This setting instructs the extProc filter to not send any request headers to the extProc server. 
    ```sh
    kubectl edit settings default -n gloo-system
    ```
    
11. Send the same request to the httpbin app. Note that this time, `header2` is not removed and `header3` and `header4` are not added to the request, because the request headers are not sent to the extProc server. 
    ```sh
    curl -vik $(glooctl proxy url)/get -H "header1: value1" -H "header2: value2" -H 'instructions: {"addHeaders":{"header3":"value3","header4":"value4"},"removeHeaders":["instructions", "header2"]}'
    ```
    
    Example output: 
    ```
    {
     "args": {}, 
     "headers": {
       "Accept": "*/*", 
       "Header1": "value1", 
       "Header2": "value2", 
       "Host": "example.com", 
       "Instructions": "{\"addHeaders\":{\"header3\":\"value3\",\"header4\":\"value4\"},\"removeHeaders\":[\"instructions\", \"header2\"]}", 
       "User-Agent": "curl/7.77.0", 
       "X-Envoy-Expected-Rq-Timeout-Ms": "15000"
     }, 
     "origin": "10.0.11.109", 
     "url": "http://example.com/get"
    }
    ```
   
## Cleanup

You can optionally remove the resources that you created as part of this guide. 

```sh
kubectl delete deployment ext-proc-grpc httpbin
kubectl delete service ext-proc-grpc httpbin
kubectl delete servicaccount httpbin
kubectl delete virtualservice vs -n gloo-system
```





   
   