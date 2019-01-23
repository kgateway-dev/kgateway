# Concepts

- [Overview](#Overview)
- [Virtual Services](#virtual-services)
    - [Routes](#Routes)
    - [Matchers](#Matchers)
    - [Destinations](#Destinations)
- [Upstreams](#Upstreams)
    - [Functions](#Functions)
- [Secrets](#Secrets)



<a name="Overview"></a>

### Overview

The two top-level concepts in Gloo are **Virtual Services** and **Upstreams**.

- **Virtual Services** define a set of route rules that live under a domain or set of domains.
Route rules consist of a *matcher*, which specifies the kind of function calls to match (requests and events,  are currently supported), and the name of the destination (or destinations) to route them to.

- **Upstreams** define destinations for routes. Upstreams tell Gloo what to route to. Upstreams may also define 
[functions](../v1/github.com/solo-io/gloo/projects/gloo/api/v1/plugins/aws/aws.proto.sk.md#LambdaFunctionSpec) and [service specs](../v1/github.com/solo-io/gloo/projects/gloo/api/v1/plugins/service_spec.proto.sk.md) for *function-level routing*.




<a name="Virtual Services"></a>

### Virtual Services

**Virtual Services** define a set of route rules, an optional SNI configuration for a given domain or set of domains.

Gloo will select the appropriate virtual service (set of routes) based on the domain specified in a request's `Host` header
(in HTTP 1.1) or `:authority` header (HTTP 2.0). 

Virtual Services support wildcard domains (starting with `*`).

Gloo will create a `default` virtual service for the user if the user does not provide one. The `default` virtual service
matches the `*` domain, which will serve routes for any request that does not include a `Host`/`:authority` header,
or a request that requests a domain that does not match another virtual service.

The each domain specified for a virtualservice must be unique across the set of all virtual services provided to Gloo.

For many use cases, it may be sufficient to let all routes live on a single virtual service. In thise scenario,
Gloo will use the same set of route rules to for requests, regardless of their `Host` or `:authority` header.

Route rules consist of a *matcher*, which specifies the kind of function calls to match (requests and events, 
are currently supported), and the name of the destination (or destinations, for load balancing) to route them to.

A simple virtual service with a single route might look like this:

```yaml
name: my-app
routes:
- request_matcher:
    path_prefix: /
  single_destination:
    upstream:
      name: my-upstream
```

Note that `domains` is empty (not specified). That means this virtual service will act as the default virtual service, matching
all domains.


<a name="Routes"></a>

#### Routes

**Routes** are the primary building block of the virtual service. A route contains a single **matcher** and one of: a
**single destination**, or a **list of weighted destinations**.

In short, a route is essentially a rule which tells Gloo: *if* the request matches this matcher, *then* route it to this 
destination.

Because multiple matchers can match a single request, the order of routes in the virtual service matters. Gloo
will select the first route which matches the request when making routing decisions. It is therefore important to place
fallback routes (e.g. matching any request for path `/` with a custom 404 page) towards the bottom of the route list.



<a name="Matchers"></a>

#### Matchers

Matchers currently support two types of requests:

* **Request Matchers** match on properties of HTTP requests. This includes the request path (`:path` header in HTTP 2.0),
method (`:method` in HTTP 2.0) headers (their keys and optionally their values), and query parameters.

* **Event Matchers** match properties of HTTP events, as per the [CloudEvents specification](https://github.com/cloudevents/spec/blob/master/spec.md).
*Note: the CloudEvents spec is in version 0.1 and likely to be changed in the future*. The only property **Event Matcher**
currently matches on is the *event-type* of an event (specified by the `x-event-type` request header). 




<a name="Destinations"></a>

#### Destinations

Destinations specify where to route a request once a matching route has been selected. A route can point to a single destination,
or it can split traffic for that route among a series of weighted destinations.

A destination can be either an *upstream destination* or a *function destination*.

**Upstream Destinations** are analogous to [Envoy clusters](https://www.envoyproxy.io/docs/envoy/latest/api-v1/cluster_manager/cluster.html?highlight=cluster).
Requests routed to upstream destinations will be routed
to a server which is expected to handle the request once it has been admitted (and possibly transformed) by Gloo.

**Function Destinations** allow requests to be routed directly to *functions* that live on various upstreams. A function
can be a serverless function call (e.g. Lambda, Google Cloud Function, OpenFaaS function, etc.), an API call on a service
(e.g. a REST API call, OpenAPI operation, XML/SOAP request etc.), or publishing to a message queue (e.g. NATS, AMQP, etc.).
Function-level routing is enabled in Envoy by Gloo's function-level filters. Gloo supports the addition of new upstream
types as well as new function types through our plugin interface.



<a name="Upstreams"></a>

### Upstreams

**Upstreams** define destinations for routes. Upstreams tell Gloo what to route to and how to route to them. Gloo determines
how to handle routing for the upstream based on its `spec` field. Upstreams have a type-specific `spec` field which must 
be used to provide routing information to Gloo.

The most basic upstream type is the [`static` upstream type](../v1/github.com/solo-io/gloo/projects/gloo/api/v1/plugins/static/static.proto.sk.md), which tells Gloo
a list of static hosts or dns names logically grouped together for an upstream. 
More sophisticated upstream types include the kubernetes upstream and the 
[AWS Lambda upstream](../v1/github.com/solo-io/gloo/projects/gloo/api/v1/plugins/aws/aws.proto.sk.md).

Let's walk through an example of a kubernetes upstream in order to understand how this works.

Gloo reads in a configuration that looks like the following: 

```yaml
---
metadata:
  labels:
    app: redis
    discovered_by: kubernetesplugin
  name: default-redis-6379
  namespace: gloo-system
  resourceVersion: "7010"
status:
  reportedBy: gloo
  state: Accepted
upstreamSpec:
  kube:
    selector:
      gloo: redis
    serviceName: redis
    serviceNamespace: gloo-system
    servicePort: 6379

```

- `name` tells Gloo what the identifier for this upstream will be (for routes that point to it).
- `type: kubernetes` tells Gloo that the kubernetes plugin knows how to route to this upstream
- `spec: ...` tells the kubernetes plugin the service name and namespace, which is used by Gloo for routing  



<a name="Functions"></a>

#### Functions

Some upstream types support **functions**. For example, we can add some HTTP functions to this upstream, and
Gloo will be able to route to those functions, providing request transformation to format incoming requests to the 
parameters expected by the upstream service.

We can now route to the function in our virtual service:

An example of a virtual service with a route to this upstream:

```yaml

metadata:
  name: default
  namespace: default
  resourceVersion: "7306"
status:
  reportedBy: gateway
  state: Accepted
  subresourceStatuses:
    '*v1.Proxy gloo-system gateway-proxy': {}
virtualHost:
  domains:
  - '*'
  name: default.default
  routes:
  - matcher:
      prefix: /
    routeAction:
      single:
        upstream:
          name: gloo-system-redis-6379
          namespace: gloo-system
    routePlugins:
      prefixRewrite: {}

```

Note that it is necessary to specify `parameters` for this function invocation. Some function destinations
require extensions to be specified on the route they belong to. Documentation for each plugin can be found in the Plugins
section.




<a name="Secrets"></a>

### Secrets

Certain plugins such as the [AWS Lambda Plugin](../v1/github.com/solo-io/gloo/projects/gloo/api/v1/plugins/aws/aws.proto.sk.md) require the use of secrets for authentication,
configuration of SSL Certificates, and other data that should not be stored in plaintext configuration.

Gloo runs an independent (goroutine) controller to monitor secrets. Secrets are stored in their own secret storage layer.
Gloo can monitor secrets stored in the following secret storage services:

- [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)
- [Hashicorp Vault](https://www.vaultproject.io)
- Plaintext files (recommended only for testing)

Secrets must adhere to a structure, specified by the plugin that requires them.
