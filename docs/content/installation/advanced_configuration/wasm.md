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

When loading directly from file, you'll need to ensure that the given `filePath` contains your `.wasm` file. One way to do this for example, would be using an `initContainer` on your `gatewayProxy` deployment to load the `.wasm` file into a shared `volume`.

To find our more information about WASM filters, and how to build/run them check out [`wasm`](https://github.com/solo-io/wasm).

In that repo you'll find `wasme`, a tool for building and deploying Envoy WASM filters. `wasme` works with Gloo Edge Enterprise, Istio, and vanilla Envoy. Much more detailed information can be found there on how the filters work.  Learn how to install the `wasme` CLI tool [here](https://docs.solo.io/web-assembly-hub/latest/installation/).

To find more information about WASM filters, and find more filters which can be included in Gloo Edge check out [WebAssembly Hub!](https://webassemblyhub.io/).
