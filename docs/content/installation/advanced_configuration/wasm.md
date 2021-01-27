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

helm repo update

helm install gloo glooe/gloo-ee --namespace gloo-system \
  --create-namespace --set-string license_key=YOUR_LICENSE_KEY
{{< /tab >}}
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

At the moment the config must live on the gateway level, this will change as the Envoy WASM api evolves. To configure a gateway
to add a WASM filter, the gateway must be edited like so.

```shell
kubectl edit -n gloo-system gateways.gateway.solo.io gateway-proxy
```

and change the `httpGateway` object to the following:

```yaml
  httpGateway:
    options:
      wasm:
        filters:
        - config:
            '@type': type.googleapis.com/google.protobuf.StringValue
            value: "world"
          image: webassemblyhub.io/sodman/example-filter:v0.5
          name: myfilter
          root_id: add_header_root_id
```

Once that is saved, the hard work has been done. All traffic on the http gateway will call the wasm filter.

If your image isn't hosted on an image registry, such as [WebAssembly Hub](https://webassemblyhub.io/), you can load the filter from the wasm file directly instead:

```yaml
  httpGateway:
    options:
      wasm:
        filters:
        - config:
            '@type': type.googleapis.com/google.protobuf.StringValue
            value: "world"
          filePath: filters-dir/my-filter.wasm
          name: myfilter
          root_id: add_header_root_id
```

When loading directly from file, you'll need to ensure that the given `filePath` contains your `.wasm` file. 

## Loading a wasm filter image from an initContainer

In some circumstances, using [WebAssembly Hub](https://webassemblyhub.io/) as your wasm filter image repository may not be possible, for example due to enterprise networking restrictions. One way to deploy a wasm filter without going through WebAssembly Hub, is to use an `initContainer` on your `gatewayProxy` deployment to load the `.wasm` file into a shared `volume`. This section will walk you through setting this up.

### Prerequisites

We are assuming you already have a wasm filter created and built locally. You can replicate this by running the two commands below. Alternatively, you can follow the more in-depth guide in the WebAssembly Hub docs [here](https://docs.solo.io/web-assembly-hub/latest/tutorial_code/build_tutorials/building_cpp_filters/).

```bash
wasme init --language cpp --platform gloo --platform-version 1.6.x ./my-filter
```
followed by

```bash
cd my-filter
wasme build cpp --store ./wasmstore . -t my-wasm-filter:v1.0
```

It's also assumed that you have a k8s cluster running, with Gloo Edge Enterprise installed in it, ideally with a route to an upstream we can hit for testing. The example we're using here uses Gloo Edge Enterprise 1.6.2, but any future version should work.

### Step 1 - Build a docker image containing our filter

First we need to build a docker image which contains our new wasm filter image. In our steps above this file is called `filter.wasm` and has been output at `.wasmstore/<uniqueId>/filter.wasm`. If you've built your `filter.wasm` using a tool other than wasme, for example using `bazel` instead - the location of the filter file may differ. Let's grab this `filter.wasm` file and put it in a folder somewhere that we can make changes.

We will create a file named `Dockerfile` as a sibling to this `filter.wasm`, this will be used to generate our docker image. The contents of `Dockerfile` should be as follows:

```
FROM alpine

COPY filter.wasm filter.wasm

CMD ["cp", "filter.wasm", "/wasm-filters/"]
```

This will create a basic docker image which just contains our `filter.wasm` file, and when run, will copy it to the `/wasm-filters/` folder, which we will mount later.

Next we want to build and tag the docker image from this Dockerfile, by running the following command (replacing your repository URL, and preferred image name):

```bash
docker build . -t localhost:8888/myorg/my-wasm-getter:1.0.0
```

We have now created a standard docker image named `localhost:8888/myorg/my-wasm-getter` with a tag of `1.0.0`. This can be pushed to whatever docker-compliant image repository you have:

```bash
docker push localhost:8888/myorg/my-wasm-getter:1.0.0
```

### Step 2 - Add the initContainer to the gateway-proxy Deployment

Now that we have a docker image containing our wasm filter, we will use it as an `initContainer` for our `gateway-proxy` pod, so that the filter is available to Envoy via a shared mounted volume.

Update the `gateway-proxy` `Deployment` resource as shown below, noting the extra "wasm-filters" `volume` and `volumeMount`, as well as the addition of `initContainers`:

{{< highlight yaml "hl_lines=60-68 83" >}}
apiVersion: apps/v1
kind: Deployment
metadata:
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
        image: quay.io/solo-io/gloo-envoy-wrapper:1.6.2
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
            drop:
            - ALL
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 10101
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config
        - mountPath: /wasm-filters
          name: wasm-filters
      initContainers:
      - name: wasm-image
        image: localhost:8888/myorg/my-wasm-getter:1.0.0
        imagePullPolicy: IfNotPresent
        volumeMounts:
        - mountPath: /wasm-filters
          name: wasm-filters
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext:
        fsGroup: 10101
        runAsUser: 10101
      serviceAccount: gateway-proxy
      serviceAccountName: gateway-proxy
      terminationGracePeriodSeconds: 30
      volumes:
      - configMap:
          defaultMode: 420
          name: gateway-proxy-envoy-config
        name: envoy-config
      - name: wasm-filters
{{< /highlight >}}

What we've done here, is specify the initContainer, which will run _before_ the gateway-proxy envoy starts up. It will add our `filter.wasm` file to the filepath at `/wasm-filters/filter.wasm`, and since that's mounted to our gateway-proxy container, it's now accessible to be read directly from the file.

### Step 3 - Configure the wasm filter in the gateway

Now that the filter is in a filepath accessible to Envoy, we need to tell envoy to load it from path. We do this in the `Gateway` type resource, named `gateway-proxy`:

{{< highlight yaml "hl_lines=12-21" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
    app.kubernetes.io/managed-by: Helm
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    options:
      wasm:
        filters:
        - config:
            '@type': type.googleapis.com/google.protobuf.StringValue
            value: "my test config"
          filePath: /wasm-filters/filter.wasm
          name: myfilter
          root_id: add_header_root_id
  proxyNames:
  - gateway-proxy
  ssl: false
  useProxyProto: false
{{< /highlight >}}

If you've been following along with this example, you should now be able to curl one of your endpoints and see the results of your filter being run - in this case, a newly added http header. You can also confirm that the filter has been loaded by Envoy if you check the `config_dump` from the Envoy Admin page. This is usually served on port `19000`:

```bash
kubectl port-forward -n gloo-system gateway-proxy-645bc75c67-xmfdz 19000:19000
```

When you open `localhost:19000/config_dump`, if you look under `filter_chains` you should see your deployed wasm filter with the name `envoy.filters.http.wasm`:

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
                            "uri": "http://gloo/images/8b3b05719379af3996d51bf6d5baed1103059fb908baec547f2136ed48aebd77",
                            "cluster": "wasm-cache",
                            "timeout": "5s"
                        },
                        "sha256": "8b3b05719379af3996d51bf6d5baed1103059fb908baec547f2136ed48aebd77"
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

If you don't have any test services to run against, you can install the petstore example used in our [hello world tutorial]({{< versioned_link_path fromRoot="/guides/traffic_management/hello_world/" >}}).

### References

To find our more information about WASM filters, and how to build/run them check out [`wasm`](https://github.com/solo-io/wasm).

In that repo you'll find `wasme`, a tool for building and deploying Envoy WASM filters. `wasme` works with Gloo Edge Enterprise, Istio, and vanilla Envoy. Much more detailed information can be found there on how the filters work.  Learn how to install the `wasme` CLI tool [here](https://docs.solo.io/web-assembly-hub/latest/installation/).

To find more information about WASM filters, and find more filters which can be included in Gloo Edge check out [WebAssembly Hub!](https://webassemblyhub.io/).
