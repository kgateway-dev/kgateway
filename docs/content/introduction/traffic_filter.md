---
title: Traffic processing
weight: 25
---

With Gloo Edge, you can configure the gateway listener along with custom Envoy filters to process the traffic that enters into and out of your environment. By mutating requests to and responses from your upstream services, you can decouple and scale your services more dynamically.

* [Types of request processing](#types-of-request-processing)
* [Filter flow](#filter-flow)

For an overview of Gloo Edge gateway, virtual service, and upstream configurations, see [Traffic management]({{% versioned_link_path fromRoot="/introduction/traffic_management/" %}}).

---

## Types of request processing

Review the following types of request processing that you can do, and see the linked guides for more configuration details.

### Transformations

Transformations can be applied to *VirtualHosts*, *Routes*, and *WeightedDestinations* parts of a Gloo Edge Virtual Service custom resource. Example transformations include the following.

* [Change the response status]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/change_response_status/" %}}) coming from an Upstream service.
* [Add headers to the body]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/add_headers_to_body/" %}}) of a request. 
* [Add custom attributes]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/enrich_access_logs//" %}}) for your access logs. 
 
For example steps, see the [Transformation guides]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/" %}}).

### Direct response and redirects

Not all requests should be sent to an Upstream destination. Review the following situations in which you might use a direct response or redirect.

* You want to redirect the response to another website ([Host Redirect]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/redirect_action/" %}})) or to another Virtual Service in Gloo Edge. 
* You want to redirect clients that request the HTTP version of a service to the [HTTPS version instead]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/https_redirect/" %}}). 
* You want to return a [direct response]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/direct_response_action/" %}}) from the Virtual Service to the client's request, such as a `404 not found` error message.

### Faults

Faults are a way to test the resilience of your services by injecting faults (errors and delays) into a percentage of your requests. Gloo Edge can do this automatically by [following this guide]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/faults/" %}}).

### Timeouts and retries

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

### Traffic shadowing

You can control the rollout of changes using canary releases or blue-green deployments with Upstream Groups. The downside to using either feature is that your are working with live traffic. Real clients are consuming the new version of your service, with potentially negative consequences. An alternative is to shadow the client traffic to your new release, while still processing the original request normally. [Traffic shadowing]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/shadowing/" %}}) makes a copy of an incoming request and sends it out-of-band to the new version of your service, without altering the original request.

---

## Inheritance rules

In general, options that you set in a parent object are inherited by a child object. Then, the child has both its own options and those of its parent appended. If the option in the child and parent conflict, the child option takes precedence and overwrites the parent option. You can change this behavior by setting the `inheritTransformation` option to `false` in the children objects.

Examples of parent and child objects:
* VirtualHost parent object options append to children objects like Routes and WeightedDestinations
* Route parent object options append to children objects like WeightedDestinations

For examples of inherited options, see the following guides:
* [Request processing transformation inheritance]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/" %}})
* [Header inheritance]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/append_remove_headers/#inheritance" %}})
* [Auth config inheritance]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/#configuration-format/" %}})

---

## Filter flow

The order that Envoy applies filters to traffic impacts how you configure your Gloo Edge resources. Review the following video and diagrams to understand the filter flow in Gloo Edge.

For more information on configuring traffic filters, see the [Transformation guides]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/" %}}).

### Video overview of the filter flow

<iframe width="560" height="315" src="https://www.youtube.com/embed/pN12nxpJ_EE" title="YouTube video player" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>

### Filter flow description

Review the following diagram of how Gloo Edge filters traffic, depending on what you configure. Notes on the filter policies that you can configure:
* The filters are applied in the order that is shown in the diagram. For example, if you apply both CORS and DLP security filters, a request is processed for CORS first, and then DLP. You cannot change the order.
* If you add a policy at both the `VirtualService` and `Route` levels, the `Route` policy takes precedence.

<figure><img src="{{% versioned_link_path fromRoot="/img/traffic-filter-flow.svg" %}}">
<figcaption style="text-align:center;font-style:italic">Figure: Filter flow.</figcaption></figure>

1.  **External auth**: When you enable the external authorization and authentication service in Gloo Edge Enterprise, you can secure access to your apps with authentication tools like OIDC, API keys, OAuth2, or OPA. External auth is used to organize the flow in this diagram so that you can quickly see how traffic can be manipulated before or after requiring the client to log in. For more information, see [Authentication and authorization]({{% versioned_link_path fromRoot="/guides/security/auth/" %}}).
2.  **Before or after external auth**: You can configure several traffic filters either before, after, or both before and after a client request is authorized.
    *  **JWT**: You can verify a JSON web token (JWT) signature, check the claims, and add them to new headers. To set JWT before and/or after external auth, use the `JwtStaged` setting. For more information, see [JWT and access control]({{% versioned_link_path fromRoot="/guides/security/auth/jwt/access_control/" %}}).
    *  **Transformation**: Apply transformation templates to the header or body request. If the body is a JSON payload, you can also extract values from it. The `clearRouteCache` setting clears the route that was initially selected by the HTTP connection manager, with the final route selected when the request reaches the Router filter. To set transformations before and/or after external auth, use the `stagedTransformation` setting. For more information, see [Transformations]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/" %}}).
    *  **Rate limiting**: Rate limiting can take place before or after external auth. You can use the `SetStyle` API to build complex rules for rate limiting. For more information, see [Rate limiting]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/" %}}).
3.  **Filters only before external auth**: Review the information about other filters that you can apply only before external auth.
    * **Fault**: See the [Faults guide]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/faults/" %}}).
    * **CORS**: See the [Cross-origin resources sharing security guide]({{% versioned_link_path fromRoot="/guides/security/cors/" %}}).
    * **DLP**: See the [Data loss prevention security guide]({{% versioned_link_path fromRoot="/guides/security/data_loss_prevention/" %}}).
    * **WAF**: See the [Web application firewall security guide]({{% versioned_link_path fromRoot="/guides/security/waf/" %}}).
    * **Sanitize**: See the [sanitize proto reference]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/external/envoy/extensions/extauth/sanitize.proto.sk/" %}}).
4.  **Filters only after external auth**: Review the information about other filters that you can apply only after external auth.
    * **RBAC**: Note that the RBAC filter requires the `JwtStaged` filter. See the [RBAC proto reference]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/rbac/rbac.proto.sk/" %}}).
    * **gRPC-web**: See the [gRPC web guide]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/grpc_web/" %}}).
    * **CSRF**: See the [Cross-site request forgery security guide]({{% versioned_link_path fromRoot="/guides/security/csrf/" %}}).
5.  **Router**: With the router filter, you can configure many different settings before the request reaches your upstream service, such as the following. For more information, see the [route proto reference]({{% versioned_link_path fromRoot="/reference/api/envoy/api/v2/route/route.proto.sk/" %}}).
    * Add or remove request headers
    * Add or remove response headers
    * Set upstream timeouts
    * Rewrite prefixes
    * Automatically rewrite hosts
    * Rewrite with regular expressions (regex)
    * Retry policies
    * Detect outliers
    * Shadow or mirror requests
