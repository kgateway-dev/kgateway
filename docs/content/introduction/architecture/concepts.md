---
title: "Core Concepts"
weight: 10
description: An explanation of the bedrock concepts behind Gloo Edge.
---

There are several core concepts that serve as the bedrock on which Gloo Edge is built. This document will provide an explanation of the various concepts, how they are implemented in Gloo Edge, and links to the API documentation to learn more. Before you review this document, it may be useful to first read our [Introduction]({{% versioned_link_path fromRoot="/introduction/" %}}) page to get a sense of the purpose behind Gloo Edge and the problems it seeks to solve.

In this document we will start with a general overview and then expand on specific topics. Links are included in the list below if you wish to jump to a specific topic.

- [Overview](#overview)
- [Gateways](#gateways)
- [Virtual Services](#virtual-services)
  - [Routes](#routes)
  - [Matchers](#matchers)
  - [Destinations](#destinations)
- [Upstreams](#upstreams)
  - [Functions](#functions)
- [Secrets](#secrets)

---

## Overview

The two top-level concepts in Gloo Edge are *Virtual Services* and *Upstreams*.

- **Virtual Services** define a set of route rules that live under a domain or set of domains. Route rules consist
of *matchers*, which specify the kind of function calls to match (requests and events, are currently supported),
and the name of the destination (or destinations) where to route them.

- **Upstreams** define destinations for routes. Upstreams tell Gloo Edge what to route to. Upstreams may also define
{{< protobuf name="aws.options.gloo.solo.io.LambdaFunctionSpec" display="functions" >}}
and {{< protobuf name="plugins.gloo.solo.io.ServiceSpec" display="service specs">}} for *function-level routing*.

---

## Gateways

*Gateway* definitions set up the protocols and ports on which Gloo Edge listens for traffic.  For example, by default, Gloo Edge has a gateway configured for HTTP and HTTPS traffic:

```bash
kubectl --namespace=gloo-system get gateway
```

```shell
NAME                AGE
gateway-proxy       61s
gateway-proxy-ssl   61s
```

A single gateway definition looks like the following:

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: gateway-proxy-ssl
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8443
  httpGateway: {}
  proxyNames:
  - gateway-proxy
  ssl: true
  useProxyProto: false
```

In this case, we are setting up an HTTP listener on port 8443. When [VirtualServices](#virtual-services) define a TLS context, they'll automatically bind to this Gateway. You can explicitly configure the Gateway to which a [VirtualService](#virtual-services) binds. In addition, you can also create [TCP gateways]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/tcp_proxy/" %}}) that allow for binary traffic.

---

## Virtual Services

*Virtual Services* define a set of route rules, security configuration (including [TLS, mTLS, SNI]({{% versioned_link_path fromRoot="/guides/security/tls/" %}}), [WAF]({{% versioned_link_path fromRoot="/guides/security/waf/" %}}), [OAuth]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/oauth/" %}}), [Data Loss Prevention]({{% versioned_link_path fromRoot="/guides/security/data_loss_prevention/" %}}), and [others]({{% versioned_link_path fromRoot="/guides/security/" %}})), rate limiting, transformations, and other core routing capabilities supported by Gloo Edge.

Gloo Edge selects the appropriate Virtual Service (set of routes) based on the domain specified in a request's `Host` header (in HTTP 1.1) or `:authority` header (HTTP 2.0).

Virtual Services also support wildcard domains (starting with `*`).

Gloo Edge creates a `default` Virtual Service for the user if the user does not provide one. The `default` Virtual Service matches the `*` domain, which serves routes for any request that does not include a `Host`/`:authority` header, or a request that requests a domain that does not match another Virtual Service. You'll note in the [Hello World tutorial]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) we create a `virtualservice` named `default`.

Each domain specified for a Virtual Service must be unique across the set of all Virtual Services provided to Gloo Edge. In previous versions, we supported Virtual Service  merging, which means you could have multiple Virtual Services with the same domain, and we would just merge the routes. The preferred way to segment out routes and have multiple owners of the Virtual Service is to use [delegation]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/delegation//" %}}). Please see the [introduction to the decentralized Gloo Edge API]({{% versioned_link_path fromRoot="/introduction/architecture/decentralized_routing/" %}}) and [delegation]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/delegation//" %}}) for more.

For some use cases, it may be sufficient to let all routes live on a single Virtual Service. In this scenario, Gloo Edge uses the same set of route rules for requests, regardless of their `Host` or `:authority` header.

Route rules consist of *matchers*, which specify the kind of function calls to match (requests and events, are currently supported), and the name of the destination (or destinations, for load balancing) where to route them.

A simple Virtual Service with a single route might look like this:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: my-upstream
            namespace: gloo-system
```

Note that we could have omitted `domains`, which would default to `*`. This Virtual Service acts as the default Virtual Service, matching all domains. We could have also omitted `matchers` here, which would default to the `/` prefix matcher, which matches all requests.

### Routes

*Routes* are the primary building block of the Virtual Service. A route contains a list of *matchers* and one of:

- a single destination
- a list of weighted destinations
- an *Upstream Group*

In short, a route is essentially a rule which tells Gloo Edge: **if** the request matches a matcher on the route, **then**
route it to this destination.

Because multiple matchers can match a single request, the order of routes in the Virtual Service matters. Gloo Edge selects the first route that matches the request when making routing decisions. It is therefore essential to place fallback routes (e.g., matching any request for path `/` with a custom 404 page) towards the bottom of the route list.

### Matchers

Matchers currently support two types of requests:

- **Request Matchers** match on properties of HTTP requests. This includes the request path (`:path` header in HTTP 2.0), method (`:method` in HTTP 2.0) headers (their keys and optionally their values), and query parameters.

- **Event Matchers** match properties of HTTP events, as per the [CloudEvents specification](https://github.com/cloudevents/spec/blob/main/cloudevents/spec.md). The only property **Event Matcher** currently matches on is the *event-type* of an event (specified by the `x-event-type` request header).

### Destinations

Destinations specify where to route a request once a matching route is selected. A route can point to a single destination or it can split traffic for that route among a series of weighted destinations.

A destination can be either an *upstream destination* or a *function destination*.

**Upstream Destinations** are analogous to [Envoy clusters](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto). Requests routed to upstream destinations are routed to a server which handles the request once it has been admitted (and possibly transformed) by Gloo Edge.

**Function Destinations** allow requests to be routed directly to *functions* that live on various Upstreams. A function can be a serverless function call (e.g., AWS Lambda, Google Cloud Function, Microsoft Azure Function) or an API call on a service (e.g., a REST API call, OpenAPI operation, gRPC operation). Function-level routing is enabled in Envoy by Gloo Edge's function-level filters. Gloo Edge supports the addition of new Upstream types and new function types through our plugin interface.

---

## Upstreams

**Upstreams** define destinations for routes. Upstreams tell Gloo Edge what to route to and how to route to them. Gloo Edge determines how to handle routing for the Upstream based on its `spec` field. Upstreams have a type-specific `spec` field that provides routing information to Gloo Edge.

The most basic Upstream type is the {{< protobuf name="static.options.gloo.solo.io.UpstreamSpec" display="static upstream type" >}}, which tells Gloo Edge a list of static hosts or DNS names logically grouped for an Upstream. More sophisticated Upstream types include the Kubernetes Upstream and the {{< protobuf name="aws.options.gloo.solo.io.UpstreamSpec" display="AWS Lambda Upstream">}}.

Let's walk through an example of a Kubernetes Upstream to understand how this works.

Gloo Edge reads in a configuration that looks like the following:

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  labels:
    discovered_by: kubernetesplugin
  name: default-redis-6379
  namespace: gloo-system
spec:
  discoveryMetadata: {}
  kube:
    selector:
      gloo: redis
    serviceName: redis
    serviceNamespace: gloo-system
    servicePort: 6379
status:
  reportedBy: gloo
  state: 1 # Accepted
```

- `name` tells Gloo Edge what the identifier is for this Upstream (for routes that point to it).
- `spec: ...` tells the Kubernetes plugin the service name and namespace, which is used by Gloo Edge for routing

### Functions

Some Upstream types support **functions**. For example, we can add some HTTP functions to this Upstream, and Gloo Edge routes to those functions, providing request transformation to format incoming requests to the parameters expected by the Upstream service.

We can now route to the function in our Virtual Service. An example of a Virtual Service with a route to this Upstream:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: default
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
       - prefix: /petstore/findWithId
      routeAction:
        single:
          destinationSpec:
            rest:
              functionName: findPetById
              parameters:
                headers:
                  :path: /petstore/findWithId/{id}
          upstream:
            name: petstore
            namespace: gloo-system
      options:
        prefixRewrite: /api/pets
```

Note that it is necessary to specify `parameters` for this function invocation. Some function destinations require extensions specified on the route for which they belong. Documentation for each plugin is in the Plugins section.

---

## Secrets

Certain plugins such as the {{< protobuf name="aws.options.gloo.solo.io.UpstreamSpec" display="AWS Lambda Plugin">}} require the use of secrets for authentication, the configuration of SSL Certificates, and other data that should not be stored in plaintext configuration.

Gloo Edge runs an independent (goroutine) controller to monitor secrets. Secrets are stored in the secret storage layer. Gloo Edge can monitor secrets stored in the following secret storage services:

- [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)
- [HashiCorp Vault](https://www.vaultproject.io)
- Plaintext files (recommended only for testing)

Secrets must adhere to a structure specified by the plugin that requires them.
