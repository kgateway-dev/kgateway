---
title: Developer Guide
weight: 6
---

Developing on Gloo can be a fun and rewarding experience. 

Gloo is designed to be highly pluggable and easily extendable in each of the following domains:

- Routing Controllers (e.g. defining new APIs, automatic config in response to events, etc.)
- Service Discovery backends
- Configuring custom or upstream Envoy Filters

The reason Gloo is extendable in so many directions is due in part to its Kubernetes Operator-style design. By interacting with two CRDs, it is possible to customize Gloo to virtually any environment and use case:

* [`v1.Proxies`](../../v1/github.com/solo-io/gloo/projects/gloo/api/v1/upstream.proto.sk) provide the routing configuration which Gloo will translate and apply to Envoy.
* [`v1.Upstreams`](../../v1/github.com/solo-io/gloo/projects/gloo/api/v1/upstream.proto.sk) describe routable destinations for Gloo.

* **Proxies** represent a unified configuration to be applied to one or more instances of a proxy. You can think of the proxy of as tree like such:

        proxy
        ├─ bind-address
        │  ├── domain-set
        │  │  ├── /route
        │  │  ├── /route
        │  │  ├── /route
        │  │  └── tls-config
        │  └── domain-set
        │     ├── /route
        │     ├── /route
        │     ├── /route
        │     └── tls-config
        └─ bind-address
           ├── domain-set
           │  ├── /route
           │  ├── /route
           │  ├── /route
           │  └── tls-config
           └── domain-set
              ├── /route
              ├── /route
              ├── /route
              └── tls-config

  A single proxy CRD contains all the configuration necessary to be applied to an instance of Envoy. In the Gloo system, Proxies are treated as an intermediary representation of config, while user-facing config is imported from simpler, more opinionated resources such as the [`gateway.VirtualService`](../../v1/github.com/solo-io/gloo/projects/gateway/api/v1/virtual_service.proto.sk) or [Kubernetes Ingress objects](https://kubernetes.io/docs/concepts/services-networking/ingress/).
  
  For this reason, a standard Gloo deployment contains one or more controllers which programatically generate and write these CRDs to provide simpler, use-case specific APIs such as API Gateway and Ingress. [Sqoop](https://sqoop.solo.io/) is an advanced controller which creates routing configuration for Gloo from [**GraphQL Schemas**](https://graphql.org/). 
  
  [Click here for a tutorial providing a simple example utilizing this lower-level Proxy API](example-proxy-controller). This tutorial will walk you through building a Kubernetes controller to automatically configure Gloo without any user interaction](example-proxy-controller.go).

* **Upstreams** represent destinations for routing requests in Gloo. Routes in Gloo specify one or more Upstreams (by name) as their destination. Upstreams have a `type` which is provided in their `upstreamSpec` field. Each type of upstream corresponds to an **Upstream Plugin**, which tells Gloo how to translate upstreams of that type to Envoy clusters. When a route is declared for an upstream, Gloo invokes the corresponding plugin for that type 

More tutorials and design documentation are coming soon!