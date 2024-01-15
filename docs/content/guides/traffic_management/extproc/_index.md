---
title: External processing
weight: 40
description: 
---

Use the [Envoy external processing (extProc) filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_proc_filter) to connect an external gRPC processing server to the Envoy filter chain that manipulates headers, body, and trailers of a request or response before it is forwarded to an upstream or downstream service. 

{{% notice note %}}
External processing is an Enterprise-only feature. 
{{% /notice %}}

## About external processing


### How it works

The following diagram shows an example for how header manipulation in requests works when an external processing server is used. 

<figure><img src="{{% versioned_link_path fromRoot="/img/extproc.svg" %}}">
<figcaption style="text-align:center;font-style:italic">External processing for request headers</figcaption></figure>

1. The downstream service sends a request with headers to the Envoy gateway. 
2. The gateway extracts the header information and sends it to the external processing server. 
3. The external processing server manipulates, adds, or removes the request headers. 
4. The manipulated request headers are sent back to the gateway and are added to the original request. 
5. The headers are added to the request.
6. The request is forwarded to the upstream application. 

### Enable extProc in Gloo Edge

You can enable extProc for all requests and responses that the gateway processes the [Settings]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/settings.proto.sk/" %}}) custom resource. Alternatively, you can enable extProc for a certain listener, route, or virtual service. 

## Set up an external processing service

The external processing server 


## 

With external processing 

External processing can be configured on the options of a gateway and routes, virtual host, virtual service. 
Can also set it globally in settings 

The external service implements a gRPC interface that can respond to 

The external processing filter connects an external service, called an “external processor,” to the filter chain. The processing service itself implements a gRPC interface that allows it to respond to events in the lifecycle of an HTTP request / response by examining and modifying the headers, body, and trailers of each message, or by returning a brand-new response.

The protocol itself is based on a bidirectional gRPC stream. Envoy will send the external processor ProcessingRequest messages, and the processor must reply with ProcessingResponse messages.

Configuration options are provided to control which events are sent to the processor. This way, the processor may receive headers, body, and trailers for both request and response in any combination. The processor may also change this configuration on a message-by-message basis. This allows for the construction of sophisticated processors that decide how to respond to each message individually to eliminate unnecessary stream requests from the proxy.

This filter is a work in progress. Most of the major bits of functionality are complete. The updated list of supported features and implementation status may be found on the reference page.



## Header manipulation

Specify what 

The HeaderMutationRules structure specifies what headers may be
manipulated by a processing filter. This set of rules makes it
possible to control which modifications a filter may make.

By default, an external processing server may add, modify, or remove
any header except for an "Envoy internal" header (which is typically
denoted by an x-envoy prefix) or specific headers that may affect
further filter processing:

* ``host``
* ``:authority``
* ``:scheme``
* ``:method``

Every attempt to add, change, append, or remove a header will be
tested against the rules here. Disallowed header mutations will be
ignored unless ``disallow_is_error`` is set to true.

Attempts to remove headers are further constrained -- regardless of the
settings, system-defined headers (that start with ``:``) and the ``host``
header may never be removed.

In addition, a counter will be incremented whenever a mutation is
rejected. In the ext_proc filter, that counter is named
``rejected_header_mutations``.

Headers can be appended or removed

1. Deploy Hello world app 

1. Create a gateway resource. 
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
     proxyNames:
     - gateway-proxy
     httpGateway:
       options:
         extProc: 
            forwardRules: 
              mutationRules:  
                allowEnvoy: false
              processingMode: 
                requestHeaderMode: 
              requestAttributes: []
         caching:
           cachingServiceRef:
             name: caching-service
             namespace: gloo-system
   EOF
   ```

2. 