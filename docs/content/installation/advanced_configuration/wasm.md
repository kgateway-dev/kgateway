---
title: Envoy Wasm filters with Gloo Edge
weight: 70
description: Using Wasm filters in Envoy with Gloo Edge
---

You can use WebAssembly (Wasm) Envoy filters with Gloo Edge Enterprise. [WebAssembly](https://webassembly.org/) (Wasm) is an open standard, binary instruction format to enable high-performing web apps, for use cases such as customizing the endpoints and thresholds of your workloads.

{{% notice note %}}
The [upstream Envoy Wasm filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/wasm_filter) is experimental, and not yet recommended for production usage.
{{% /notice %}}

## Before you begin

1. [Create your environment]({{< versioned_link_path fromRoot="/installation/platform_configuration/" >}}), such as a Kubernetes cluster in a cloud provider.
2. [Install Gloo Edge Enterprise]({{< versioned_link_path fromRoot="/installation/enterprise/" >}}) in your environment.
3. Install a test app such as Pet Store from the [Hello World tutorial]({{< versioned_link_path fromRoot="/guides/traffic_management/hello_world/" >}}).

## Prepare your Wasm filter {#filter}

WebAssembly provides a safe, secure, and dynamic way of extending infrastructure with the programming language of your choice.

1. Get a Wasm image. Review the following resources to help.
   * [WebAssembly Hub](https://webassemblyhub.io/repositories/) to use an existing Wasm image repository.
   * [WebAssembly Developer's Guide](https://webassembly.org/getting-started/developers-guide/) for more information on building your own Wasm image.
   * [Solo's `wasme` CLI tool](https://docs.solo.io/web-assembly-hub/latest/tutorial_code/getting_started/) with starter kits that makes it easy to build and push Wasm modules to WebAssembly Hub.

   Example steps with `wasme` CLI: For more information, see the [Build tutorial](https://docs.solo.io/web-assembly-hub/latest/tutorial_code/build_tutorials/building_cpp_filters/).
   1. Start a C++ filter for Gloo.
      ```sh
      wasme init --language cpp --platform gloo --platform-version 1.13.x ./my-filter
      ```
   2. Build the filter into a Wasm image.
      ```sh
      cd my-filter
      wasme build cpp --store ./wasmstore . -t my-wasm-filter:v1.0
      ```

2. Prepare your Wasm image for use with Gloo Edge Enterprise. Review the following options.
   * **Store in an image repository like WebAssembly Hub**: Solo provides [WebAssembly Hub](https://webassemblyhub.io/) as the simplest way to share and consume Wasm Envoy repositories. When you use the `wasme` CLI tool, you can push the image directly to your WebAssembly Hub repository. The resulting image repository is in a format similar to the following: `webassemblyhub.io/<username>/<filter-name>:<tag>`.
   * **Load the Wasm file directly into the filter**: If your filter is not hosted in an image repository such as WebAssembly Hub, you can refer to the filepath directly, such as `<directory>/<filter-name>.wasm`.
   * **Use an init container**: In some circumstances, you might not be able to use an image repository due to enterprise networking restrictions. Instead, you can use an `initContainer` on the Gloo Edge `gatewayProxy` deployment to load a `.wasm` file into a shared `volume`.

## Configure Gloo Edge to use a Wasm filter {#configuration}

Now that Gloo Edge Enterprise is installed and you have your Wasm image, you are ready to configure Gloo Edge to use the Wasm filter. You add the filter to your gateway proxy configuration. For more information, check out the {{% protobuf name="wasm.options.gloo.solo.io.PluginSource" display="API docs"%}}.

{{< tabs >}} 
{{% tab name="From WebAssembly Hub" %}}
1. Get the configuration for your `gateway-proxy` gateway.
   ```shell
   kubectl get -n gloo-system gateways.gateway.solo.io gateway-proxy -o yaml > gateway-proxy.yaml
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
             image: webassemblyhub.io/yuval/add-header:v0.1
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

Now that your `gateway-proxy` gateway is updated, the hard work has been done. All traffic on the HTTP gateway calls the Wasm filter.

## Verify the Wasm filter

1. Enable port-forwarding for the `gateway-proxy` on the port for the Envoy Admin page, usually 19000.
   ```
   kubectl port-forward -n gloo-system pods/$(kubectl get pod -l gloo=gateway-proxy -n gloo-system -o jsonpath='{.items[0].metadata.name}') 19000:19000
   ```
2. Check the `config_dump` from the Envoy Admin page for the Wasm filter by opening this URL: [`localhost:19000/config_dump`](localhost:19000/config_dump).

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

## References

* [WebAssembly Hub](https://webassemblyhub.io/) for sharing and reusing Wasm filters.
* [Solo's `wasme` CLI tool](https://docs.solo.io/web-assembly-hub/latest/installation/) for building and deploying Wasm filters for Gloo Edge Enterprise, Istio, and Envoy.
* [Solo's `wasm` GitHub repo](https://github.com/solo-io/wasm) for the `wasme` tool.
