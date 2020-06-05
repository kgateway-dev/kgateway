---
title: Envoy WASM filters with Gloo
weight: 70
description: Using WASM filters in Envoy with Gloo
---

Experimental support for Envoy WASM filters has been added to Open Source Gloo as of version 1.2.6.

{{% notice note %}}
This feature is considered incredibly experimental. It uses a fork of Envoy which does not have any official support. It 
is meant to show off the potential of WASM filters, and how they will integrate with Gloo going forward.
{{% /notice %}}

---

## Configuration

Getting started with WASM is simple, it requires setting one new value in the gloo helm chart.

The easiest way to install with this value is using helm 3 as follows:
{{< tabs >}}
{{< tab name="helm" codelang="shell script">}}
helm repo add gloo https://storage.googleapis.com/solo-public-helm

helm repo update

helm install --namespace gloo-system --set global.wasm.enabled=true gloo gloo/gloo
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
        - config: |
            {}
          image: webassemblyhub.io/yuval-k/metrics:v1
          name: yuval
          root_id: stats_root_id
```

Once that is saved, the hard work has been done. All traffic on the http gateway will call the wasm filter.

To find our more information about WASM filters, and how to build/run them check out [`wasme`](https://github.com/solo-io/wasme).

`wasme` is a tool for building and deploying Envoy WASM filters, in Gloo, and in vanilla Envoy. Much more detailed information can be found there on how the filters work.

To find more information about WASM filters, and find more filters which can be included in Gloo check out [WebAssembly Hub!](https://webassemblyhub.io/).
