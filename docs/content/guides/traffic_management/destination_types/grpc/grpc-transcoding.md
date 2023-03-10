---
title: Transcode HTTP requests to gRPC
weight: 30
description: Explore gRPC transcoding and learn how to annotate your proto files with HTTP rules. You can then configure Gloo Edge to accept incoming HTTP requests and transform them into gRPC requests.
---

You can enable gRPC transcoding for Gloo Edge so that the proxy can accept incoming HTTP requests and transform them into gRPC requests before they are forwarded to the gRPC service. 

In this guide, you learn how to: 
- Deploy a gRPC demo service and transform its gRPC API to a REST API by using Gloo Edge.
- Annotate your gRPC proto files with HTTP rules.
- Generate and encode proto descriptors.
- Add proto descriptors to an upstream. 
- Set up routing to the gRPC app by using a Virtual Service. 
- Verify HTTP to gRPC request transcoding. 

## Before you begin

Make sure to complete the following tasks before you get started with this guide. 

- Create or use an existing [Kubernetes cluster]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" %}}). 
- [Install Gloo Edge version 1.14 or later]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}).
- [Install `curl`](https://everything.curl.dev/get) to act as the HTTP client. 
- [Install `protoc`](http://google.github.io/proto-lens/installing-protoc.html) to generate proto descriptors. 

{{% notice note %}}
The instructions in this guide assume that you did not or do not want to enable the Gloo Edge Function Discovery Service (FDS) to automatically generate proto descriptors for you and put them on the gRPC upstream. If you want to enable FDS, you can skip [Step 2](#generate-proto-descriptors) and [Step 3](#add-descriptors-to-upstream) in this guide, and go from [Step 1](#deploy-app) to [Step 4](#grpc-routing) directly. 
{{% /notice %}}

## Step 1: Deploy the demo gRPC Bookstore app {#deploy-app}

To explore gRPC transcoding, you can use the Bookstore demo app in the Gloo Edge GitHub repository. 

1. Clone the Gloo Edge GitHub repository. 
   ```shell
   git clone https://github.com/solo-io/gloo.git
   ```
   
2. Navigate to the Bookstore sample app. 
   ```shell
   cd gloo/docs/examples/grpc-json-transcoding/bookstore
   ```
   
3. Deploy the Bookstore app in your cluster. 
   ```shell
   kubectl apply -f Bookstore.yaml
   ```
   
   Example output: 
   ```
   deployment.apps/bookstore created
   service/bookstore created
   ```
   
4. Verify that the app is running. 
   ```shell
   kubectl get pods | grep bookstore
   ```
   
5. Verify that Gloo Edge automatically discovered the gRPC service and added an upstream for it. 
   ```shell
   kubectl get upstream -n gloo-system default-bookstore-8080 -o yaml
   ```
   

## Step 2: Generate proto descriptors {#generate-proto-descriptors}

Proto descriptors are created by using the `protoc` tool and are based on the functions and the HTTP mappings that you added to your proto files. 

{{% notice note %}}
The instructions in this guide assume that you did not enable the Gloo Edge Function Discovery Service (FDS) to automatically generate proto descriptors and put them on the gRPC upstream. If you want to enable FDS, you can skip [Step 2](#generate-proto-descriptors) and [Step 3](#add-descriptors-to-upstream) in this guide, and go to [Step 4](#grpc-routing) directly. To enable FDS, run the following command: `kubectl label upstream -n gloo-system default-bookstore-8080 discovery.solo.io/function_discovery=enable`. 
{{% /notice %}}


1. Explore HTTP mappings of the Bookstore app. The demo app has HTTP mappings and rules already added as `google.api.http` annotations to the `bookstore.proto` file. To learn more about HTTP mappings and find examples for how to annotate your proto files, see the [Transcoding reference]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc/transcoding-reference/" %}}). 
   ```shell
   cat bookstore.proto
   ```

2. Generate the proto descriptor binary. 
   ```shell script
   cd /tmp/
   git clone https://github.com/protocolbuffers/protobuf
   git clone http://github.com/googleapis/googleapis
   export PROTOBUF_HOME=$PWD/protobuf/src
   export GOOGLE_PROTOS_HOME=$PWD/googleapis
   cd -
   protoc -I${GOOGLE_PROTOS_HOME} -I${PROTOBUF_HOME} -I. --include_source_info --include_imports --descriptor_set_out=descriptors/proto.pb bookstore.proto
   ```
   
3. Verify that you see a `proto.pb` file in the `descriptors` directory. 
   ```shell 
   cd descriptors
   cat proto.pb
   ```

## Step 3: Add the proto descriptor binary to the gRPC upstream {#add-descriptors-to-upstream}

Now that you created the proto descriptors, you must encode the file to base64 and add it to the upstream YAML configuration. After the proto descriptor binary is added, Gloo Edge can translate incoming HTTP requests to gRPC requests. Note that you can skip this step if you enabled the Gloo Edge FDS feature. 

1. Navigate to the `gloo` root directory. 
2. From the root directory, encode the proto descriptor binary to base64. 
   ```shell
   cat docs/examples/grpc-json-transcoding/bookstore/descriptors/proto.pb | base64
   ```
   
3. Add the base64-encoded proto descriptor output to your gRPC upstream. 
   1. Get the YAML configuration for the gRPC upstream and save it to a local file. 
      ```shell
      kubectl get upstream -n gloo-system default-bookstore-8080 -o yaml > upstream.yaml
      ```
      
   2. Add the proto descriptor binary in the `serviceSpec` section. You must also add `main.Bookstore` as the service that is running inside the gRPC app. 
      {{< highlight yaml "hl_lines=26-30" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  annotations:
    cloud.google.com/neg: '{"ingress":true}'
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","kind":"Service","metadata":{"annotations":{},"labels":{"app":"bookstore"},"name":"bookstore","namespace":"default"},"spec":{"ports":[{"name":"grpc","port":8080,"protocol":"TCP"}],"selector":{"app":"bookstore"}}}
  creationTimestamp: "2023-03-09T16:29:28Z"
  generation: 2
  labels:
    discovered_by: kubernetesplugin
  name: default-bookstore-8080
  namespace: gloo-system
  resourceVersion: "2395636"
  uid: 48404cf6-febf-4385-af0b-acf127eac1b4
spec:
  discoveryMetadata:
    labels:
      app: bookstore
  kube:
    selector:
      app: bookstore
    serviceName: bookstore
    serviceNamespace: default
    servicePort: 8080
    serviceSpec:
      grpcJsonTranscoder:
        protoDescriptorBin: Ctd...3RvMw== // Value is truncated
        services:
        - main.Bookstore
  useHttp2: true
status:
  statuses:
    gloo-system:
      reportedBy: gloo
      state: Accepted
      {{< /highlight >}}
      
   3. Apply the change to your upstream. 
      ```
      kubectl apply -f upstream.yaml
      ```
      
{{% notice note %}}
Adding the proto descriptor binary to the upstream is the recommended gRPC transcoding practice. However, you can choose to add the proto descriptors to the gateway resource instead of the upstream. For instructions on how to do that, refer to the [Gloo Edge 1.13 docs](https://docs.solo.io/gloo-edge/v1.13.x/guides/traffic_management/destination_types/grpc_to_rest_advanced/). 
{{% /notice %}}


## Step 4: Set up routing to the gRPC upstream {#grpc-routing}

To route HTTP requests to your gRPC upstream, you must set up a gRPC route with a virtual service. 

1. Create a virtual service that allows routing to the gRPC upstream. 
   ```shell
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: json-to-grpc
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
       - foo.example.com
       routes:
         - matchers:
             - prefix: /
           routeAction:
             single:
               upstream:
                 name: default-bookstore-8080
                 namespace: gloo-system
   EOF
   ```
   
2. Verify that the virtual service was created. 
   ```shell
   kubectl get virtualservice json-to-grpc -n gloo-system
   ```     

## Step 5: Verify the HTTP to gRPC transcoding 

To test that Gloo Edge can successfully transform incoming HTTP requests to gRPC requests, you use `curl` as your HTTP client. 

1. Send a request to get all shelves from the Bookstore app. Because no shelves were created yet, you get back an empty response. 
   ```shell
   curl -H "Host: foo.example.com" $(glooctl proxy url)/shelves
   ```
   
   Example output: 
   ```
   {}
   ```

2. Add a shelf to your Bookstore. 
   ```shell
   curl -H "Host: foo.example.com" $(glooctl proxy url)/shelf -d '{"theme": "music"}'
   ```
   
   Example output: 
   ```
   {"theme":"music"}%   
   ```

3. Now list all the shelves again to confirm that the shelf was successfully created. 
   ```shell
   curl -H "Host: foo.example.com" $(glooctl proxy url)/shelves
   ```
   
   Example output: 
   ```
   {"shelves":[{"theme":"music"}]}
   ```

## Summary and next steps

Congratulations! You successfully enabled Gloo Edge to accept HTTP requests for your gRPC service and transform the HTTP request to gRPC requests so that the gRPC upstream can process it. You also explored how HTTP mappings are added to a gRPC API and what tools you can use to generate proto descriptors for your upstreams. This allows you to enjoy the benefits of using gRPC for your microservices while also having a traditional REST API that internet-facing clients can use without the need to maintain two sets of code. 

You can explore more HTTP mappings in the [Transcoding reference]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc/transcoding-reference/" %}}). Gloo Edge also supports the gRPC-Web protocol to allow browser clients that use HTTP/1 or HTTP/2 to access a gRPC service. For more information, see [gRPC for web clients]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/grpc_web/" %}}). 



