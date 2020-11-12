---
title: Envoy WASM filters with Gloo Edge
weight: 70
description: Using WASM filters in Envoy with Gloo Edge
---

Support for Envoy WASM filters has been added to Open Source Gloo Edge as of version 1.6.0-beta3+. This guide is specifically for Gloo Edge 1.6.0-beta3, as there have been some changes to the configuration API since prior versions.

{{% notice note %}}
This feature is considered to be in a tech preview state of stability. While wasm functionality has
been merged to upstream envoy, wasm filters are not yet recommended for production use. This tech preview 
is meant to show off the potential of WASM filters, and how they will integrate with Gloo Edge going forward.
{{% /notice %}}

---

## Configuration

Getting started with WASM is simple, it requires setting one new value in the gloo helm chart.

Gloo Edge can be installed with this value set using either `glooctl` or `helm 3` as follows:
{{< tabs >}}
{{< tab name="glooctl" codelang="shell script">}}
glooctl install gateway --values <(echo '{"crds":{"create":true}}')
{{< /tab >}}
{{< tab name="helm" codelang="shell script">}}
helm repo add gloo https://storage.googleapis.com/solo-public-helm

helm repo update
kubectl create ns gloo-system
helm install --namespace gloo-system --version v1.6.0-beta3
{{< /tab >}}
{{< /tabs >}}

Once this process has been completed, gloo should be up and running in the `gloo-system` namespace.

To check run the following:
```shell script
kubectl get pods -n gloo-system
``` 
```shell script
NAME                            READY   STATUS    RESTARTS   AGE
discovery-5ff9ddbc8f-p7njb      1/1     Running   0          45s
gateway-578f5b7d9d-khw8m        1/1     Running   0          45s
gateway-proxy-c9b4cc476-x6h5j   1/1     Running   0          45s
gloo-6889d56b5c-f28gv           1/1     Running   0          45s
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
          image: webassemblyhub.io/sodman/example-filter:v0.3
          name: myfilter
          root_id: add_header_root_id
```

Once that is saved, the hard work has been done. All traffic on the http gateway will call the wasm filter.

To find our more information about WASM filters, and how to build/run them check out [`wasm`](https://github.com/solo-io/wasm).

`wasme` is a tool for building and deploying Envoy WASM filters, in Gloo Edge, and in vanilla Envoy. Much more detailed information can be found there on how the filters work.

To find more information about WASM filters, and find more filters which can be included in Gloo Edge check out [WebAssembly Hub!](https://webassemblyhub.io/).
