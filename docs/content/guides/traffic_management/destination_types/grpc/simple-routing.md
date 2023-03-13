---
title: Set up routing to gRPC services
weight: 20
description: Use a demo app to explore how to set up routing to a gRPC upstream in Gloo Edge. 
---

In this guide, you learn how to expose a gRPC `Upstream` through a Gloo Edge `Virtual Service`, and connect to it with a gRPC client. Then, you explore how to secure the communication between the gRPC client and the Envoy proxy by using TLS certificates. 

The following tasks are included in this guide: 

- [Step 1: Deploy the demo gRPC store service and set up routing](#deploy-app)
- [Step 2: Validate connectivity with a gRPC client](#validate-connectivity)
- [Step 3: Secure the gRPC app](#secure-app).

## Before you begin

Make sure to complete the following tasks before you get started with this guide. 

- Create or use an existing [Kubernetes cluster]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" %}}). 
- [Install Gloo Edge 1.14 or later]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}).
- [Install `grpcurl`](https://github.com/fullstorydev/grpcurl) to act as the gRPC client. 
- Install `openssl` to generate self-signed TLS certificates. For example, to install `openssl` on a Mac, run `brew install openssl`. 

## Step 1: Deploy the demo gRPC store service and set up routing {#deploy-app}

Use the gRPC Store app to explore how to set up routing to gRPC services. 

1. Create the deployment and expose the deployment with a Kubernetes service. 
   ```shell
   kubectl create deployment grpcstore-demo --image=docker.io/soloio/grpcstore-demo
   kubectl expose deployment grpcstore-demo --port 80 --target-port=8080
   ```
   
2. Verify that Gloo Edge automatically discovered the gRPC app and created an upstream for it. 
   ```shell
   kubectl get upstream -n gloo-system default-grpcstore-demo-80
   ```
   
3. Enable Gloo Edge function discovery (FDS) so the proto descriptor can be found. The proto descriptor binary includes the gRPC functions that are available in the store service as well as any HTTP mappings that were added for HTTP to gRPC transcoding. For more information, see [gRPC transcoding]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc/about/#grpc-transcoding" %}}). 
   ```shell
   kubectl label upstream -n gloo-system default-grpcstore-demo-80 discovery.solo.io/function_discovery=enabled
   ```
   
4. Get the upstream for the gRPC store and verify that the proto descriptor was added to the YAMl file. 
   ```shell
   kubectl get upstream -n gloo-system default-grpcstore-demo-80 -o yaml
   ```
   
   Example output: 

   {{% notice note %}}
   The proto descriptor fields are truncated for brevity.
   {{% /notice %}}

   {{< highlight yaml "hl_lines=25-29" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  annotations:
    cloud.google.com/neg: '{"ingress":true}'
  creationTimestamp: "2023-03-06T16:47:54Z"
  generation: 4
  labels:
    discovered_by: kubernetesplugin
    discovery.solo.io/function_discovery: enabled
  name: default-grpcstore-demo-80
  namespace: gloo-system
  resourceVersion: "10533"
  uid: bee3ec08-a2c1-44c5-a632-ec53f0113f8c
spec:
  discoveryMetadata:
    labels:
      app: grpcstore-demo
  kube:
    selector:
      app: grpcstore-demo
    serviceName: grpcstore-demo
    serviceNamespace: default
    servicePort: 80
    serviceSpec:
      grpcJsonTranscoder:
        protoDescriptorBin: Cqw...90bzM=  # truncated
        services:
        - solo.examples.v1.StoreService
status:
  statuses:
    gloo-system:
      reportedBy: gloo
      state: 1
   {{< /highlight >}}

5. Optional: Decode the `spec.kube.serviceSpec.grpcJsonTanscoder` proto descriptor field. Note that the field is truncated in the example command. Make sure to add the entire `spec.kube.serviceSpec.grpcJsonTanscoder` value to this command. 
   ```shell
   echo "Cqw ... 90bzM=" | base64 -d
   ```
   
6. Change the communication protocol that the Envoy proxy uses to HTTP/2. This step is required so that incoming requests from gRPC clients can be forwarded to the gRPC app. You can choose between the following two options: 
   * Add the `gloo.solo.io/h2_service: true` annotation to the gRPC service. 
   * Name the port for the gRPC service one of the following: `grpc`, `http2`, or `h2`. 

   1. Get the YAML file for the current gRPC service and save it to a local file. 
      ```shell
      kubectl get service grpcstore-demo -o yaml > grpc-service.yaml
      ```
      
   2. Open the file and either add the annotation or port name. The following example changes the name of the `grpc` service port. 
      ```yaml
      spec:
        clusterIP: 10.101.199.96
        ports:
        - name: grpc
          port: 80
          protocol: TCP
          targetPort: 8080
      ```
      
   3. Wait a few seconds and then verify that the `useHttp2: true` value was added to your upstream. 
      ```
      kubectl get upstream -n gloo-system default-grpcstore-demo-80 -o yaml
      ```
      
      Example output: 
      {{< highlight yaml "hl_lines=4" >}}
      ...
              services:
              - solo.examples.v1.StoreService
        useHttp2: true
      status:
        statuses:
          gloo-system:
            reportedBy: gloo
            state: Accepted
      {{< /highlight >}}

7. Create the virtual service so that you can route incoming requests to the gRPC store app. The virtual service assumes that you use the `gloo-system` namespace for your Gloo Edge installation. In this configuration, the prefix `/` is matched for all domains. 
   ```
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: grpc
     namespace: gloo-system
   spec:
     virtualHost:
       routes:
         - matchers:
             - prefix: /
           routeAction:
             single:
               upstream:
                 name: default-grpcstore-demo-80
                 namespace: gloo-system
   EOF
   ```

## Step 2: Validate connectivity with a gRPC client {#validate-connectivity}

To test connectivity to the gRPC app, you use the `grpcurl` utility. Because the connection to the app is not secured via TLS, you must use the `-plaintext` option in your command to allow unencrypted traffic to the app. You learn how to secure the communication to the app via TLS later in this guide. 

1. Send a request to your gRPC app. The `grpcurl` utility requires a port number to be sent as part of the request. Because the store service has Server Reflection enabled, you do not have to specify a proto source file for `grpcurl` to use with the request. The `list` argument retrieves all the services that are availble in the gRPC app.
   ```shell
   grpcurl -plaintext $(glooctl proxy address --port http) list
   ```
   
   Example output: 
   ```
   grpc.reflection.v1alpha.ServerReflection
   solo.examples.v1.StoreService
   ```

2. Describe the store service to get a list of methods that are available in the app. 
   ```shell
   grpcurl -plaintext $(glooctl proxy address --port http) describe solo.examples.v1.StoreService
   ```
   
   Example output: 
   ```
   solo.examples.v1.StoreService is a service:
   service StoreService {
     rpc CreateItem ( .solo.examples.v1.CreateItemRequest ) returns ( .solo.examples.v1.CreateItemResponse );
     rpc DeleteItem ( .solo.examples.v1.DeleteItemRequest ) returns ( .solo.examples.v1.DeleteItemResponse );
     rpc GetItem ( .solo.examples.v1.GetItemRequest ) returns ( .solo.examples.v1.GetItemResponse );
     rpc ListItems ( .solo.examples.v1.ListItemsRequest ) returns ( .solo.examples.v1.ListItemsResponse );
   }
   ```
   
3. You can continue to describe the methods and responses that were returned in the previous command. For example, to get the details for the `solo.examples.v1.CreateItemRequest` method, run the following command. 
   ```shell
   grpcurl -plaintext $(glooctl proxy address --port http) describe solo.examples.v1.CreateItemRequest
   ``` 
   
   Example output: 
   ```
   solo.examples.v1.CreateItemRequest is a message:
   message CreateItemRequest {
     .solo.examples.v1.Item item = 1;
   }
   ```
   
4. Use the `CreateItem` method to add an item to the store app. 
   ```shell 
   grpcurl -plaintext -d '{"item":{"name":"item1"}}' $(glooctl proxy address --port http) solo.examples.v1.StoreService/CreateItem
   ```
   
   Example output: 
   ```json
   {
     "item": {
       "name": "item1"
     }
   }
   ```
   
5. Use the `ListItems` to retrieve all the items that are available in the store. 
   ```shell
   grpcurl -plaintext $(glooctl proxy address --port http) solo.examples.v1.StoreService/ListItems
   ```
   
   Example output: 
   ```json
   {
     "items": [
       {
         "name": "item1"
       }
     ]
   }
   ```

## Step 3: Secure the gRPC app {#secure-app}

Enable encryption between the gRPC client and the Envoy proxy on a specific domain. 

1. Edit the virtual service that you created earlier and add a `domain` entry to the `virtualHost` configuration. Instead of allowing requests from any domain, the gRPC store app now can receive requests from only the `store.example.com` domain. 
   ```bash
   kubectl edit vs grpc -n gloo-system
   ```

   Add the domain as shown in the highlighted lines. 

   {{< highlight yaml "hl_lines=3-4" >}}
   spec:
     virtualHost:
       domains:
       - store.example.com
       routes:
       - matchers:
         - prefix: /
         routeAction:
           single:
             upstream:
               name: default-grpcstore-demo-80
               namespace: gloo-system
   {{< /highlight >}}

   {{% notice warning %}} If you want to use a port number other than 443, you must append the port to the domain. For more information, see https://github.com/solo-io/gloo/issues/3505. 
   {{% /notice %}}

2. Send a request to list the items in the store. Because the app is now configured to only receive incoming requests on the `store.example.com` domain, you see an error from the Envoy proxy. Note that the error message is not strictly true, but rather is the best that Envoy can figure out because the gRPC request was sent without an `authority` header (the equivalent of an HTTP host header in `curl`). Instead, Envoy used the IP address that was returned with the `glooctl proxy address` command as the host name. 
   ```bash
   grpcurl -plaintext $(glooctl proxy address --port http) solo.examples.v1.StoreService/ListItems
   ```
   
   Example output: 
   ```
   Error invoking method "solo.examples.v1.StoreService/ListItems": failed to query for service descriptor "solo.examples.v1.StoreService": server does not support the reflection API
   ```

3. Add the `authority` flag to the request. This time the request succeeds. 
   ```shell
   grpcurl -plaintext -authority store.example.com $(glooctl proxy address --port http) solo.examples.v1.StoreService/ListItems
   ```
   
   Example output: 
   ```json
   {
     "items": [
       {
         "name": "item1"
       }
     ]
   }
   ```

   {{< notice note >}}
   In this example, the authority header is specified alongside the public IP address of the Envoy proxy in the format <code>X.X.X.X:80</code>. If you wanted to provide the domain and port directly, such as with <code>store.example.com:80</code>, you still need to specify the authority header to avoid issues as Envoy uses the domain name and the port as the host header. Because <code>store.example.com:80</code> does not match <code>store.example.com</code>, you see the same error as if no domain was provided. You can avoid this error by specifying the authority header explicitly. If you cannot specify the authority header, you can update the domain match on the Virtual Service to use <code>store.example.com*</code> instead.
   {{< /notice >}}
   
4. Create a self-signed TLS certificate for your domain. Note that self-signed certificates are not a recommended security practice for production. If you plan to use a gRPC app in production, create certificates that are signed by a trusted public or private certificate authority. 

   ```shell
   openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
      -keyout tls.key -out tls.crt -subj "/CN=store.example.com"
   ```
   
5. Store the certificate in a Kubernetes secret. 
   ```shell
   kubectl create secret tls grpc-tls --key tls.key \
      --cert tls.crt --namespace gloo-system
   ```
   
6. Configure the Virtual Service to use this certificate to authenticate gRPC clients. 
   ```shell
   glooctl edit virtualservice --name grpc --namespace gloo-system \
      --ssl-secret-name grpc-tls --ssl-secret-namespace gloo-system
   ```
   
7. Verify that the SSL configuration was added to the Virtual Service. 
   ```shell
   glooctl get virtualservice grpc -o kube-yaml
   ```

   {{< highlight yaml "hl_lines=7-10" >}}
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: grpc
     namespace: gloo-system
   spec:
     sslConfig:
       secretRef:
         name: grpc-tls
         namespace: gloo-system
     virtualHost:
       domains:
       - store.example.com
       routes:
       - matchers:
         - prefix: /
         routeAction:
           single:
             upstream:
               name: default-grpcstore-demo-80
               namespace: gloo-system
   status:
     reportedBy: gateway
     state: 1
     subresourceStatuses:
       '*v1.Proxy.gloo-system.gateway-proxy':
         reportedBy: gloo
         state: 1
   {{< /highlight >}}
   
8. Send another request. Update the `grpcurl` command to use the `-insecure` flag instead of the `-plaintext` flag. You also need to change the port from 80 (http) to 443 (https). Note that if you have a certificate that is trusted by the `grpcurl` client, you can skip the `-insecure` flag. 

   ```shell
   grpcurl -insecure -authority store.example.com $(glooctl proxy address --port https) solo.examples.v1.StoreService/ListItems
   ```
   
   Example output: 
   ```json
   {
     "items": [
       {
         "name": "item1"
       }
     ]
   }
   ```

## Summary and next steps

Excellent! In this guide, you explored how to connect to a gRPC Upstream from a gRPC client by using Gloo Edge. You also learned how to limit routing to certain domains and how to secure the connection between the gRPC client and your upstream with TLS certificates. 

To learn how to connect to a gRPC upstream through Gloo Edge by using a REST API, check out the [Transcode HTTP requests to gRPC]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc/grpc-transcoding" %}}) guide. For more information about how to further secure your Gloo Edge deployment, see the [Network Encryption]({{% versioned_link_path fromRoot="/guides/security/tls/" %}}) guides. 
