---
title: Traffic Management
weight: 20
---

Gloo Edge acts as the control plane to manage traffic flowing between downstream clients and upstream services. Traffic management can take many forms as a request flows through the Envoy proxies managed by Gloo Edge. Requests from clients can be transformed, redirected, routed, and shadowed, to cite just a few examples.

---

## Fundamentals

The primary components that deal with traffic management in Gloo Edge are as follows:

* **Gateways** - Gloo Edge listens for incoming traffic on *Gateways*. The Gateway definition includes the protocols and ports on which Gloo Edge listens for traffic.
* **Virtual Services** - *Virtual Services* are bound to a Gateway and configured to respond for specific domains. Each contains a set of route rules, security configuration, rate limiting, transformations, and other core routing capabilities supported by Gloo Edge.
* **Routes** - Routes are associated with Virtual Services and direct traffic based on characteristics of the request and the upstream destination.
* **Upstreams** - Routes send traffic to destinations, called *Upstreams*. Upstreams take many forms, including Kubernetes services, AWS Lambda functions, or Consul services.

Additional information can be found in the [Gloo Edge Core Concepts document]({{% versioned_link_path fromRoot="/introduction/architecture/concepts/" %}}).

---

## Listener configuration

The Gateway component of Gloo Edge is what listens for incoming requests. An example configuration is shown below for an SSL Gateway. The `spec` portion defines the options for the Gateway.

{{< highlight proto "hl_lines=8-15" >}}
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
{{< /highlight >}}

A full listing of configuration options is available in the {{< protobuf name="gateway.solo.io.Gateway" display="API reference for Gateways.">}}

The listeners on a gateway typically listen for HTTP requests coming in on a specific address and port as defined by `bindAddress` and `bindPort`. Additional options can be configured by including an `options` section in the spec. SSL for a gateway is enabled by setting the `ssl` property to `true`.

Gloo Edge can be configured to act as a gateway on layer 7 (HTTP/S) or layer 4 (TCP). The majority of services will likely be using HTTP, but there may be some cases where applications either do not use HTTP or should be presented as a TCP endpoint. When Gloo Edge operates as a TCP Proxy, the options for traffic management are greatly reduced. Gloo Edge currently supports standard routing, SSL, and Server Name Indication (SNI) domain matching. Applications not using HTTP can be configured using the [TCP Proxy guide]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/tcp_proxy/" %}}).

Gloo Edge is meant to serve as an abstraction layer, simplifying the configuration of the underlying Envoy proxy and adding new functionality. The advanced options on Envoy are not exposed by default, but they can be accessed by adding an `httpGateway` section to your listener configuration. 

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway

spec:
  httpGateway:
    options:
      httpConnectionManagerSettings:
        tracing:
          verbose: true
          requestHeadersForTags:
            - path
            - origin
```

Some of the advanced options include [enabling tracing]({{% versioned_link_path fromRoot="/guides/observability/tracing/" %}}), [access log configuration]({{% versioned_link_path fromRoot="/guides/security/access_logging//" %}}), disabling [gRPC Web transcoding]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/grpc_web/" %}}), and fine-grained control over [Websockets]({{% versioned_link_path fromRoot="http://localhost:1313/guides/traffic_management/listener_configuration/websockets/" %}}). More detail on how to perform advanced listener configuration can be found in the [HTTP Connection Manager guide]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/http_connection_manager/" %}}).

---

## Traffic processing

Traffic that arrives at a listener is processed using one of the Virtual Services bound to the Gateway. The selection of a Virtual Service is based on the domain specified in the request. A Virtual Service contains rules regarding how a destination is selected and if the request should be altered in any way before sending it along.

### Destination selection

Routes are the primary building block of a Virtual Service. Routes contain matchers and an Upstream which could be a single destination, a list of weighted destinations, or an Upstream Group.

{{< highlight proto "hl_lines=8-15" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService

spec:
  virtualHost:
    domains:
      - 'example.com'
    routes:
      - matchers:
         - prefix: /app/cart
        routeAction:
          single:
            upstream:
              name: shopping-cart
              namespace: gloo-system
{{< /highlight >}}

Matchers inspect information about a request and determine if the data in the request matches a value defined in the rule. The content inspected can include the request path, header, query, and method. Matchers can be combined in a single rule to further refine which requests will be matched against that rule. For instance, a request could be using the POST method and reference the path `/app/cart`. The combination of an HTTP Method matcher and a Path matcher could identify the request, and send it to a shopping cart Upstream.

More information on each type of matcher is available in the following guides.

* [Path matching]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_selection/path_matching/" %}})
* [Header matching]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_selection/header_matching/" %}})
* [Query Parameter Matching]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_selection/query_parameter_matching/" %}})
* [HTTP Method Matching]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_selection/http_method_matching/" %}})

---

### Destination types

Once an incoming request has been matched by a route rule, the traffic can either be sent to a destination or processed locally. The most common destination for a route is a single Gloo Edge Upstream. It’s also possible to route to multiple Upstreams, by either specifying multiple destinations, or by configuring an Upstream Group. Finally, it’s possible to route directly to Kubernetes or Consul services, without needing to use Gloo Edge Upstreams or discovery.

#### Single Upstreams

Upstreams can be added manually, creating what are called [Static Upstreams]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/static_upstream//" %}}). Gloo Edge also has a discovery service that can monitor Kubernetes or Consul and [automatically add new services]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/discovered_upstream/" %}}) as they are discovered. When routing to an Upstream, you can take advantage of Gloo Edge’s endpoint discovery system, and configure routes to specific functions, either on a REST or gRPC service, or on a cloud function.

#### Multiple Upstreams

There may be times you want to specify multiple Upstreams for a given route. Perhaps you are performing Blue/Green testing, and want to send a certain percentage of traffic to an alternate version of a service. You can specify [multiple Upstream destinations]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/multi_destination/" %}}) in your route, [create an Upstream Group]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/upstream_groups//" %}}) for your route, or send traffic to a [subset of pods in Kubernetes]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/upstream_groups//" %}}).

Gloo Edge can also use Upstream Groups to perform a [canary release]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/canary/" %}}), by slowly and iteratively introducing a new destination for a percentage of the traffic on a Virtual Service. Gloo Edge can be used with [Flagger](https://docs.flagger.app/tutorials/gloo-progressive-delivery) to automatically change the percentages in an Upstream Group as part of a canary release.

In addition to static and discovered Upstreams, the following Upstreams can be created to map directly a specialty construct:

* [Kubernetes services]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/kubernetes_services/" %}})
* [Consul services]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/consul_services/" %}})
* [AWS Lambda]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/aws_lambda/" %}})
* [REST endpoint]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/rest_endpoint/" %}})
* [gRPC]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc_to_rest/" %}})

#### Route Delegation

While it is possible to have a single Virtual Service directly route all traffic for a domain in its main route configuration, that may not always be the best approach. Let's say you have a domain called example.com which runs a shopping cart API and a community forum. The shopping cart API is handled by one team and the community forum is handled by another. You want to enable each team to make updates on their routing rules, without stepping on each other toes or messing with the main Virtual Service for the domain. Sounds like a job for route delegation!

In a route delegation, a prefix of the main Virtual Service can be delegated to a *Route Table*.  The Route Table is a collection of routes, just like in the main Virtual Service, but the permissions of the Route Table could be scoped to your shopping cart API team. In our example, you can create route delegations for the `/api/cart` prefix and the `/community` prefix to route tables managed by the respective teams. Now each team is free to manage their own set of routing rule, and you have the freedom to expand this model as new services are added to the domain. You can find out more in the [route delegation guide]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/delegation//" %}}).

---

### Request processing

One of the core features of any API Gateway is the ability to transform the traffic that it manages. To really enable the decoupling of your services, the API Gateway should be able to mutate requests before forwarding them to your Upstream services and do the same with the resulting responses before they reach the downstream clients. Gloo Edge delivers on this promise by providing you with a powerful transformation API.

#### Transformations

Transformations can be applied to *VirtualHosts*, *Routes*, and *WeightedDestinations*. You might [change the response status]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/change_response_status/" %}}) coming from an Upstream service, [add headers to the body]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/add_headers_to_body/" %}}) of a request, or [add custom attributes]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/enrich_access_logs//" %}}) for your access logs. The guides included in the [Transformations]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/" %}}) section can provide clear examples of how to implement those transformations and more.

#### Direct response and redirects

Not all requests should be sent to an Upstream destination. In some cases, the request should be redirected either to another site ([Host Redirect]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/redirect_action/" %}})) or to another Virtual Service in Gloo Edge. You may also wish to redirect clients requesting the HTTP version of a service to the [HTTPS version instead]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/https_redirect/" %}}). Other requests should have a [direct response]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/direct_response_action/" %}}) from the Virtual Service, such as a 404 not found.

#### Faults

Faults are a way to test the resilience of your services by injecting faults (errors and delays) into a percentage of your requests. Gloo Edge can do this automatically by [following this guide]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/faults/" %}}).

#### Timeouts and retries

Gloo Edge will attempt to send requests to the proper Upstream, but there may be times when that Upstream service is unable to handle additional requests. The `timeout` and `retry` portions of the `options` section for a route define how long to wait for a response from the Upstream service and what type of retry strategy should be used.

```yaml
      options:
        timeout: '20s'
        retries:
          retryOn: 'connect-failure'
          numRetries: 3
          perTryTimeout: '5s'
```

More information about configuring the [timeout]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/timeout/" %}}) and [retry]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/retries/" %}}) can be found in their respective guides.

#### Traffic shadowing

You can control the rollout of changes using canary releases or blue-green deployments with Upstream Groups. The downside to using either feature is that your are working with live traffic. Real clients are consuming the new version of your service, with potentially negative consequences. An alternative is to shadow the client traffic to your new release, while still processing the original request normally. [Traffic shadowing]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/shadowing//" %}}) makes a copy of an incoming request and sends it out-of-band to the new version of your service, without altering the original request.

---

## Configuration validation

When configuring an API gateway or edge proxy, invalid configurations can quickly lead to bugs, service outages, and security vulnerabilities. Whenever Gloo Edge configuration objects are updated, Gloo Edge validates and processes the new configuration. This is achieved through a four-step process:

1. Admit or reject change with a Kubernetes Validating Webhook
1. Process a batch of changes and report any errors
1. Report the status on change
1. Process the changes and apply to Envoy

More detail on the validation process and its settings can be found in the [Configuration Validation guide]({{% versioned_link_path fromRoot="/guides/traffic_management/configuration_validation/" %}}).

---

## Next Steps

Now that you have an understanding of how Gloo Edge handles traffic management we have a few suggested paths:

* **[Security]({{% versioned_link_path fromRoot="/introduction/security/" %}})** - learn more about Gloo Edge and its security features
* **[Setup]({{% versioned_link_path fromRoot="/installation/" %}})** - Deploy your own instance of Gloo Edge
* **[Traffic management guides]({{% versioned_link_path fromRoot="/guides/traffic_management/" %}})** - Try out the traffic management guides to learn more

