---
title: Traffic Management
weight: 20
---

Gloo acts as the control plane to manage traffic flowing between downstream clients and udstream services. Traffic management can take many forms as a request flows through the Envoy proxies managed by Gloo. Requests from clients can be transformed, redirected, routed, and shadowed, to cite just a few examples.

---

## Fundamentals

The primary components that deal with traffic management in Gloo are as follows:

* **Gateways** - Gloo listens for incoming traffic on Gateways. With the Gateway definition are the protocols and ports on which Gloo listens for traffic.
* **Virtual Services** - Virtual Services are bound to a Gateway and configured to respond for specific domains. Each contains a set of route rules, security configuration, rate limiting, transformations, and other core routing capabilities supported by Gloo.
* **Routes** - Routes are associated with Virtual Services and define where traffic should be sent if it matches certain criteria.
* **Upstreams** - Routes send traffic to destinations, called Upstreams. Upstreams take many forms, including Kubernetes services, AWS Lambda functions, or Consul services.

Additional information can be found in the [Gloo Concepts document].

---

## Listener configuration

The Gateway component of Gloo is what listens for incoming requests. An example configuration is shown below for an SSL Gateway. The `spec` portion defines the options for the Gateway.

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

A full listing of configuration options is available in the [API reference for Gateways]. 

The listeners on a gateway typically listen for HTTP requests coming in on a specific address and port as defined by `bindAddress` and `bindPort`. Additional options can be configured by including an `options` section in the spec. SSL for a gateway is enabled by setting the `ssl` property to `true`.

Gloo Gateway can be configured as a layer 7 (HTTP/S) gateway or a layer 4 (TCP) gateway. Applications not using HTTP can be configured using the [TCP Proxy guide].

There may be times when you wish to configure some advanced options on the Envoy, and those options are exposed through the `httpGateway` section. More detail on how to perform advanced listener configuration can be found in the [HTTP Connection Manager guide].

By default, Gloo Gateway configures Envoy to automatically transcode traffic for gRPC web clients. This feature is one of the advanced options that can be disabled through the `httpGateway` section. The proper settings for disabling `grpcWeb` are documented in [the guide on gRPC Web].

Another advanced option is the use of websockets to enable full-duplex communication, which is enabled by default by Gloo. There may be times you would like a finer level of control over websockets, and that can be accomplished by following [the guide on Websockets].

---

## Traffic processing

Traffic that arrives at a listener is processed using one of the Virtual Services bound to the Gateway. The selection of a Virtual Service is based on the domain specified in the request. A Virtual Service contains rules regarding how a destination is selected and if the request should be altered in any way before sending it along.

### Destination selection

Routes are the primary building block of the Virtual Service. A route contains matchers and an upstream which could be a single destination, a list of weighted destinations, or an upstream group.

There are many types of matchers, including Path Matching, Header Matching, Query Parameter Matching, and HTTP Method Matching. Matchers can be combined in a single rule to further refine which requests will be matched against that rule.
More information on each type of matcher is available in the following guides.

* Path matching
* Header matching
* Query Parameter Matching
* HTTP Method Matching

---

### Destination types

There are many types of destinations that can be targeted by a route. Most commonly, a route destination is a single Gloo Upstream. It’s also possible to route to multiple Upstreams, by either specifying a multi destination, or by configuring an Upstream Group. Finally, it’s possible to route directly to Kubernetes or Consul services, without needing to use Gloo Upstreams or discovery.

When routing to an Upstream, you can take advantage of Gloo’s endpoint discovery system, and configure routes to specific functions, either on a REST or gRPC service, or on a cloud function. This is covered in Function Routing.

Upstreams can be added manually, creating what are called [Static Upstreams]. Gloo also has a discovery service that can monitor Kubernetes or Consul and [automatically add new services] as they are discovered.

There may be times that you want to specify multiple Upstreams for a given route. Perhaps you are performing Blue/Green testing, and want to send a certain percentage of traffic to an alternate service. You can specify [multiple Upstream destinations] in your route, [create an Upstream Group] for your route, or send traffic to a [subset of pods in Kubernetes].

Gloo can also use Upstream Groups to perform a canary release, by slowly and iteratively introducing a new destination for a percentage of the traffic on a Virtual Service. Gloo can be used with Flagger to automatically change the percentages in an Upstream Group as part of a canary release.

In addition to static and discovered Upstreams, the following Upstreams can be created to map directly a specialty construct:

* Kubernetes services
* Consul services
* AWS Lambda
* REST endpoint
* gRPC

---

### Request processing

One of the core features of any API Gateway is the ability to transform the traffic that it manages. To really enable the decoupling of your services, the API Gateway should be able to mutate requests before forwarding them to your Upstream services and do the same with the resulting responses before they reach the downstream clients. Gloo delivers on this promise by providing you with a powerful transformation API.

#### Transformations

Transformations can be applied to *VirtualHosts*, *Routes*, and *WeightedDestinations*. The guides included in the [Transformations] section can provide clear examples of how transformations can be used.

In addition to the transformations described above, Gloo can also make changes like [appending or removing headers] or [rewriting the prefix] on a request.

#### Direct response and redirects

Some requests should have a direct response or a redirect instead of an Upstream service. Gloo can perform an [HTTPS redirect from HTTP], a [Host Redirect], or produce a [Direct Response]. 

#### Faults and timeouts

Faults are a way to test the resilience of your services by injecting faults (errors and delays) into a percentage of your requests. Gloo can do this automatically by [following this guide].

Requests that linger without a response can degrade the performance of your services. Gloo includes options for each route that will drop the request after a certain amount of time or number of retries.

#### Traffic shadowing

You can control the rollout of changes using canary releases or blue-green deployments with Upstream Groups. Both of those options use live traffic to test out the changes. Traffic shadowing makes a copy of an incoming request and sends it out-of-band to the new version of our software, without altering the original request.

---

## Configuration validation

When configuring an API gateway or edge proxy, invalid configuration can quickly lead to bugs, service outages, and security vulnerabilities. Whenever Gloo configuration objects are updated, Gloo validates and processes the new configuration. This is achieved through a four-step process:

1. Admit or reject change with a Kubernetes Validating Webhook
1. Process a batch of changes and report any errors
1. Report the status on change
1. Process the changes and apply to Envoy

More detail on the validation process and how to configure it can be found in the [Configuration Validation guide].
