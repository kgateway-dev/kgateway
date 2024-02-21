---
title: Envoy WASM filters with Gloo Edge
weight: 70
description: Using WASM filters in Envoy with Gloo Edge
---

Support for Envoy WASM filters has been added to Gloo Edge Enterprise as of version 1.6.0+. This guide is specifically for Gloo Edge 1.6.0. Note, there may have been some changes to the configuration API since prior experimental versions.

{{% notice note %}}
This feature is considered to be in a tech preview state of stability. While wasm functionality has
been merged to upstream envoy, wasm filters are not yet recommended for production use. This tech preview 
is meant to show off the potential of WASM filters, and how they will integrate with Gloo Edge going forward. <br/><br/>

Envoy Wasm filters are a Gloo Edge Enterprise feature.
{{% /notice %}}

---

## Configuration

Getting started with WASM is simple, first we install Gloo Edge Enterprise.

Gloo Edge Enterprise can be installed using either `glooctl` or `helm 3` as follows:
{{< tabs >}}
{{< tab name="glooctl" codelang="shell script">}}
glooctl install gateway enterprise --license-key YOUR_LICENSE_KEY
{{< /tab >}}
{{< tab name="helm" codelang="shell script">}}
helm repo add gloo https://storage.googleapis.com/solo-public-helm

1. Get a Wasm image. For more information on building your own Wasm image, see the [WebAssembly Developer's Guide](https://webassembly.org/getting-started/developers-guide/). 

2. Prepare your Wasm image for use with Gloo Edge Enterprise. Review the following options.

   * Store in an OCI-compliant image repository. This guide uses an example Wasm image from Solo's public Google Container Registry.
   * Load the Wasm file directly into the filter. If your filter is not hosted in an image repository, you can refer to the filepath directly, such as `<directory>/<filter-name>.wasm`.
   * Use an init container. In some circumstances, you might not be able to use an image repository due to enterprise networking restrictions. Instead, you can use an `initContainer` on the Gloo Edge `gatewayProxy` deployment to load a `.wasm` file into a shared `volume`.

## Configure Gloo Edge to use a Wasm filter {#configuration}

Now that Gloo Edge Enterprise is installed and you have your Wasm image, you are ready to configure Gloo Edge to use the Wasm filter. You add the filter to your gateway proxy configuration. For more information, check out the {{% protobuf name="wasm.options.gloo.solo.io.PluginSource" display="API docs"%}}.

{{< tabs >}} 
{{% tab name="From an image registry" %}}
1. Get the configuration for your `gateway-proxy` gateway.
   ```shell
   kubectl get -n gloo-system gateways.gateway.solo.io gateway-proxy -o yaml > gateway-proxy.yaml
   open gateway-proxy.yaml
   ```
2. Add the reference to your Wasm filter in the `httpGateway` section as follows.
   ```yaml
     httpGateway:
       options:
         wasm:
           filters:
           - config:
               '@type': type.googleapis.com/google.protobuf.StringValue
               value: "world"
             image: gcr.io/solo-public/docs/assemblyscript-test:istio-1.8
             name: add-header
             rootId: add_header
   ```
3. Update the `gateway-proxy` gateway.
   ```sh
   kubectl apply -n gloo-system -f gateway-proxy.yaml
   ```
{{% /tab %}} 
{{% tab name="From filepath" %}}
1. Get the configuration for your `gateway-proxy` gateway.
   ```shell
   kubectl get -n gloo-system gateways.gateway.solo.io gateway-proxy -o yaml  > gateway-proxy.yaml
   ```
2. Add the filepath reference to your `.wasm` file in the `httpGateway` section as follows.
   ```yaml
    httpGateway:
      options:
        wasm:
          filters:
          - config:
              '@type': type.googleapis.com/google.protobuf.StringValue
              value: "world"
            filePath: filters-dir/my-filter.wasm
            name: add-header
            rootId: add_header
   ```
3. Update the `gateway-proxy` gateway.
   ```sh
   kubectl apply -n gloo-system -f gateway-proxy.yaml
   ```
{{% /tab %}}
{{% tab name="From an init container" %}}
Build a Docker image that has the Wasm filter image you previously created and use this image in an init container that runs alongside the gateway proxy. 

{{% notice note %}}Note: If you use a C++ Wasm filter, make sure to upgrade to `proxy-wasm-cpp-sdk-b2e6b0759d34d760e527dadca413a285614f9e99`.{{% /notice %}}

1. Create a Dockerfile in the same location as your Wasm filter. The Dockerfile makes an image that has your Wasm filter, and copies the filter to the `/wasm-filters/` directory when the image runs. Later, you mount this directory in a shared volume. _Note: In the previous section, your Wasm file is called `filter.wasm` and is located at `.wasmstore/<uniqueId>/filter.wasm`. If built your filter with a different tool than `wasme` (such as `bazel`), your filter location might differ._
   ```
   FROM alpine

   COPY filter.wasm filter.wasm
   
   CMD ["cp", "filter.wasm", "/wasm-filters/"]
   ```
2. Build and tag a Docker image from this Dockerfile. Replace the example values with your repository URL and preferred image name in the following example command.
   ```sh
   docker build . -t localhost:8888/myorg/my-wasm-getter:1.0.0
   ```
3. Push the Docker image to an image repository that your enterprise network can access.
   ```
   docker push localhost:8888/myorg/my-wasm-getter:1.0.0
   ```
4. Edit your `gateway-proxy` deployment to add an init container and mount a shared volume. For a full example, see this [`gateway-proxy-wasm.yaml` file](https://github.com/solo-io/gloo-edge-use-cases/blob/main/docs/gateway-proxy-wasm.yaml).
   1. Get the configuration for the `gateway-proxy` deployment.
      ```sh
      kubectl get -n gloo-system deployment gateway-proxy -o yaml > gateway-proxy-wasm.yaml
      ```
   2. In the `spec.template.spec.volumes` section, add a volume named `wasm-filters` that all the containers in the template can access.
      ```yaml
            volumes:
            - configMap:
                defaultMode: 420
                name: gateway-proxy-envoy-config
              name: envoy-config
            - name: wasm-filters
      ```
   3. In the `spec.template.spec.containers` section, add a mount path to the `wasm-filters` volume that you just configured.
      ```yaml
            containers:
              volumeMounts:
              - mountPath: /etc/envoy
                name: envoy-config
              - mountPath: /wasm-filters
                name: wasm-filters
      ```
   4. In the `spec.template.spec` section, add the following init container stanza, which refers to the Wasm image that you just built and mounts the volume.
      ```yaml
            initContainers:
            - name: wasm-image
              image: localhost:8888/myorg/my-wasm-getter:1.0.0
              imagePullPolicy: IfNotPresent
              volumeMounts:
              - mountPath: /wasm-filters
                name: wasm-filters
      ```
   5. Apply the updated `gateway-proxy` deployment.
      ```sh
      kubectl apply -n gloo-system -f gateway-proxy-wasm.yaml
      ```
5. Now that the Wasm filter is in a shared mount filepath accessible by Envoy, get the configuration for your `gateway-proxy` gateway.
   ```shell
   kubectl get -n gloo-system gateways.gateway.solo.io gateway-proxy -o yaml  > gateway-proxy.yaml
   ```
6. Add the filepath reference to your `.wasm` file in the `httpGateway` section as follows.
   ```yaml
    httpGateway:
      options:
        wasm:
          filters:
          - config:
              '@type': type.googleapis.com/google.protobuf.StringValue
              value: "my test config"
            filePath: /wasm-filters/filter.wasm
            name: add-header
            rootId: add_header
   ```
7. Update the `gateway-proxy` gateway.
   ```sh
   kubectl apply -n gloo-system -f gateway-proxy.yaml
   ```
{{% /tab %}} 
{{< /tabs >}}

Once this process has been completed, gloo should be up and running in the `gloo-system` namespace.

To check run the following:
```shell script
kubectl get pods -n gloo-system
``` 
```shell script
NAME                                                  READY   STATUS    RESTARTS   AGE
api-server-5b57f68dc-dqfjk                            3/3     Running   0          77s
discovery-5ffdfb9898-js9j2                            1/1     Running   0          77s
extauth-6888f56db4-kgz2n                              1/1     Running   0          77s
gateway-569488695f-zpqnj                              1/1     Running   0          77s
gateway-proxy-9c954dc8-wt46j                          1/1     Running   0          77s
gloo-5984f6f655-ct97s                                 1/1     Running   0          77s
glooe-grafana-78c6f96db-qwnd2                         1/1     Running   0          77s
glooe-prometheus-kube-state-metrics-7f8fd8dd8-cfmqp   1/1     Running   0          77s
glooe-prometheus-server-6cc865559b-gl8wq              2/2     Running   0          76s
observability-6dd56c8468-xvwqc                        1/1     Running   0          77s
rate-limit-c4fb9fc5b-6gm4s                            1/1     Running   0          76s
redis-55d6dbb6b7-fg7wm                                1/1     Running   0          77s
```

Once all of the pods are up and running you are all ready to configure your first WASM filter. The API to configure the filter can be found {{% protobuf name="wasm.options.gloo.solo.io.PluginSource" display="here"%}}.

   * Example output in the `filter_chains` section:
   ```json
   ...
   {
       "name": "envoy.filters.http.wasm",
       "typed_config": {
           "@type": "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm",
           "config": {
               "name": "myfilter",
               "root_id": "add_header",
               "vm_config": {
                   "vm_id": "gloo-vm-id",
                   "runtime": "envoy.wasm.runtime.v8",
                   "code": {
                       "remote": {
                           "http_uri": {
                               "uri": "http://gloo/images/   8b3b05719379af3996d51bf6d5baed1103059fb908baec547f2136ed48aebd77"   ,
                               "cluster": "wasm-cache",
                               "timeout": "5s"
                           },
                           "sha256":    "8b3b05719379af3996d51bf6d5baed1103059fb908baec547f2136ed48aebd77"
                       }
                   },
                   "nack_on_code_cache_miss": true
               },
               "configuration": {
                   "@type": "type.googleapis.com/google.protobuf.StringValue",
                   "value": "my test config"
               }
           }
       }
   },
   ...
   ```
3. To check that the Wasm filter is applied, send a curl request to one of your endpoints. 
   ```
   curl -v $(glooctl proxy url)/all-pets
   ```
   Example output: Notice the header that your Wasm filter adds.
   {{< highlight yaml "hl_lines=13" >}}
   * TCP_NODELAY set
   * Connected to 34.30.251.229 (34.30.251.229) port 80 (#0)
   > GET /all-pets HTTP/1.1
   > Host: 34.30.251.229
   > User-Agent: curl/7.64.1
   > Accept: */*
   > 
   < HTTP/1.1 200 OK
   < content-type: text/xml
   < date: Thu, 02 Mar 2023 16:46:24 GMT
   < content-length: 86
   < x-envoy-upstream-service-time: 2
   < hello: world!
   < server: envoy
   < 
   [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
   {{< /highlight >}}
