---
title: Traffic tapping
weight: 100
description: Copy contents of HTTP requests and responses to an external tap server. 
---

Copy contents of HTTP or gRPC requests and responses to an external tap server. A tap server is a simple device that connects directly to your infrastructure and receives copies of actual traffic from your network so that this traffic can be further monitored, analyzed, or tested with. 

{{% notice warning %}}
Traffic tapping support in Gloo Gateway is introduced as an **alpha feature**. Alpha features are likely to change, are not fully tested, and are not supported for production.
{{% /notice %}}

{{% notice note %}}
Traffic tapping is an Enterprise-only feature. 
{{% /notice %}}

## About traffic tapping in Gloo Gateway

* Traffic tapping can be applied to a listener in a `Gateway` resource. As such, traffic tapping is applied to all routes that the gateway serves. Tapping traffic for a specific route is not currently supported.
* Users are responsible for writing their own tap servers. The tap server definitions can be found in the [`tap-extension-examples` repository](https://github.com/solo-io/tap-extension-examples). To receive tap traces, the tap server must implement the [tap service protobuf definitions](https://github.com/solo-io/tap-extension-examples/tree/main/pkg/tap_service) and be configured to receive data over the gRPC or HTTP protocol.
  {{% notice note %}}
  The `tap-extension-examples` repository is provided as an implementation reference. The repository is **not intended to be used in production**.
  {{% /notice %}}
* In the current implementation, the data plane buffers all trace data in memory before sending it to the tap server.
* You cannot tap traffic to a local file. 

{{% notice warning %}}
Data that is tapped from the data plane might contain sensitive information, including credentials or Personal Identifiable Information (PII). Before using traffic tapping in your environment, make sure that all data is encrypted during transit and that sensitive data is masked or removed by the tap server before it is written to permanent storage or forwarded to another service. Note that you cannot use the Data Loss Preventation (DLP) plug-in to prevent sensitive data from being leaked via the tap filter. 
{{% /notice %}}

## Before you begin

1. [Set up Gloo Gateway Enterprise]({{< versioned_link_path fromRoot="/installation/enterprise/" >}}) in your cluster. During the Gloo Gateway installation, a gateway resource is created for you that you later use to configure traffic tapping. 
2. Follow the steps to [deploy and expose the Petstore sample app]({{< versioned_link_path fromRoot="/guides/traffic_management/hello_world/" >}}).

## Deploy a tap server

1. Create the deployment, service, and upstream for the tap server in your cluster. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     labels:
       app: sample-tap-server-http
     name: sample-tap-server-http
     namespace: gloo-system
   spec:
     selector:
       matchLabels:
         app: sample-tap-server-http
     replicas: 1
     template:
       metadata:
         labels:
           app: sample-tap-server-http
       spec:
         containers:
         - image: gcr.io/solo-test-236622/sample-tap-server-http:0.0.2
           name: sample-tap-server-http
           # args: ["-text='Hello World!'"]
           ports:
           - containerPort: 8080
             name: grpc
   ---
   apiVersion: v1
   kind: Service
   metadata:
     name: sample-tap-server-http
     namespace: gloo-system
     labels:
       service: sample-tap-server-http
   spec:
     ports:
     - port: 8080
       protocol: TCP
     selector:
       app: sample-tap-server-http
   ---
   apiVersion: gloo.solo.io/v1
   kind: Upstream
   metadata:
     name: sample-tap-server-http
     namespace: gloo-system
   spec:
     # useHttp2: true
     static:
       hosts:
       - addr: sample-tap-server-http
         port: 8080
   EOF
   ```

2. Verify that the tap server is up and running. 
   ```sh
   kubectl get pods -n gloo-system | grep tap
   ```

## Set up traffic tapping

1. Configure the `gateway-proxy` gateway resource for traffic tapping. In the following example, you instruct Gloo Gateway to tap all incoming traffic on the gateway and to send it to the tap server that you previously configured by using the HTTP protocol. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-proxy
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8080
     httpGateway:
       options:
         tap:
           sinks:
           - httpService:
               tapServer:
                 name: sample-tap-server-http
                 namespace: gloo-system
               timeout: '20s'
   EOF
   ```

2. In a terminal window, tail the logs of the tap server. 
   ```sh
   kubectl -n gloo-system logs deployments/sample-tap-server-http -f
   ```

3. In another terminal window, send a request to the Petstore app. 
   ```sh
   curl $(glooctl proxy url)/all-pets
   ```

   Example output: 
   ```
   [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
   ```

4. Go back to the logs of the tap server and verify that you can see the request to the Petstore app. 
   
   Example output: 
   ```
   2023/12/12 20:54:19 got a request on /
   2023/12/12 20:54:19 Message contents were: {
     "trace_data": {
        "Trace": {
         "HttpBufferedTrace": {
           "request": {
             "headers": [
               {
                 "key": ":authority",
                 "value": "acbde4f6bb5a349eb8f83697b1295dbc-1834725307.us-east-2.elb.amazonaws.com"
               },
               {
                 "key": ":path",
                 "value": "/api/pets"
               },
               {
                 "key": ":method",
                 "value": "GET"
               },
               {
                 "key": ":scheme",
                 "value": "http"
               },
               {
                 "key": "user-agent",
                 "value": "curl/7.77.0"
               },
               {
                 "key": "accept",
                 "value": "*/*"
               },
               {
                 "key": "x-forwarded-proto",
                 "value": "http"
               },
               {
                 "key": "x-request-id",
                 "value": "08d56457-e8ea-4273-80c7-a448bb42733e"
               },
               {
                 "key": "x-envoy-expected-rq-timeout-ms",
                 "value": "15000"
               },
               {
                 "key": "x-envoy-original-path",
                 "value": "/all-pets"
               }
             ]
           },
           "response": {
             "headers": [
               {
                 "key": ":status",
                 "value": "200"
               },
               {
                  "key": "content-type",
                 "value": "application/xml"
               },
               {
                 "key": "date",
                 "value": "Tue, 12 Dec 2023 20:54:19 GMT"
               },
               {
                 "key": "content-length",
                 "value": "86"
               },
               {
                 "key": "x-envoy-upstream-service-time",
                 "value": "1"
               },
               {
                 "key": "server",
                 "value": "envoy"
               }
             ],
             "body": {
               "BodyType": {
                 "AsBytes": "W3siaWQiOjEsIm5hbWUiOiJEb2ciLCJzdGF0dXMiOiJhdmFpbGFibGUifSx7ImlkIjoyLCJuYW1lIjoiQ2F0Iiwic3RhdHVzIjoicGVuZGluZyJ9XQo="
               }
             }
           }
         }
       }
     }
   }
   ```

## Clean up

You can optionally remove the resources that you created as part of this guide. 

```sh
kubectl delete deployment sample-tap-server-http -n gloo-system
kubectl delete service sample-tap-server-http -n gloo-system
kubectl delete upstream sample-tap-server-http -n gloo-system
kubectl delete -f https://raw.githubusercontent.com/solo-io/gloo/v1.13.x/example/petstore/petstore.yaml
```

