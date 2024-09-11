---
title: CORS
weight: 70
description: Enforce client-side access controls by specifying external domains to access certain routes of your domain
---

Cross-Origin Resource Sharing (CORS) is a security feature that is implemented by web browsers and that controls how web pages in one domain can request and interact with resources that are hosted on a different domain. 

## How does CORS work? 

By default, web browsers only allow requests to resources that are hosted on the same domain as the web page that served the original request. Access to web pages or resources that are hosted on a different domain is restricted to prevent potential security vulnerabilities, such as cross-site request forgery (CRSF).

When CORS is enabled in a web browser and a request for a different domain comes in, the web browser checks whether this request is allowed or not. To do that, it typically sends a preflight request (HTTP `OPTIONS` method) to the server or service that serves the requested resource to get back the methods that are allowed to use when sending the actual cross-origin request, such as `GET`, `POST`, etc. If the request to the different domain is allowed, the response includes CORS-specific headers that instruct the web browser how to make the cross-origin request. For example, the CORS headers typically include the origin that is allowed to access the resource, and the credentials or headers that must be included in the cross-origin request. 

Note that the preflight request is optional. Web browsers can also be configured to send the cross-origin directly. However, access to the request resource is granted only if CORS headers were returned in the response. If no headers are returned during the preflight request, the web browser denies access to the resource in the other domain. 

CORS policies are typically implemented to limit access to server resources for JavaScripts that are embedded in a web page, such as:
* A JavaScript on a web page at `example.com` tries to access a different domain, such as `api.com`.
* A JavaScript on a web page at `example.com` tries to access a different subdomain, such as `api.example.com`.
* A JavaScript on a web page at `example.com` tries to access a different port, such as `example.com:3001`.
* A JavaScript on a web page at `https://example.com` tries to access the resources by using a different protocol, such as `http://example.com`. 

For more details, see [this article](https://medium.com/@baphemot/understanding-cors-18ad6b478e2b).

## Configuration options

You can configure the CORS policy at two levels in the VirtualService:

* [Virtual host](#virtual-host): By applying the CORS policy in the `virtualHost.options.cors` section, each route in the VirtualService gets the policy.
* [Route](#route): Configure separate CORS policies per route in the `routes.options.cors` section.

By default, the configuration of the route option take precedence over the virtual host. However, you can change this behavior by using the `corsPolicyMergeSettings` field in the virtual host options. For more information about the supported merge strategies, see the [API docs]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options/cors/cors.proto.sk/#corspolicymergesettings" %}}).

{{% notice note %}} 
Some apps, such as `httpbin`, have built-in CORS policies that allow all origins. These policies take precedence over CORS policies that you might configure in Gloo Gateway. 
{{% /notice %}}

### Configure CORS for a virtual host {#virtual-host}

To configure the `virtualHost` part of your VirtualService for a CORS policy, review the following example.

{{< highlight yaml "hl_lines=9-11" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: corsexample
  namespace: gloo-system
spec:
  displayName: corsexample
  virtualHost:
    options:
      cors:
        (...)
    domains:
    - '*'
{{< /highlight >}}

### Configure CORS for a particular route {#route}

To configure a particular `route` of your VirtualService for a CORS policy, review the following example.

{{< highlight yaml "hl_lines=15-29" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: corsexample
  namespace: gloo-system
spec:
  displayName: corsexample
  virtualHost:
  domains:
  - '*'
  routes:
  - matchers:
    - exact: /all-pets
    options:
      cors:
        allowCredentials: true
        allowHeaders:
        - origin
        allowMethods:
        - GET
        - POST
        - OPTIONS
        allowOrigin:
        - https://example.com
        allowOriginRegex:
        - https://[a-zA-Z0-9]*\.example\.com
        exposeHeaders:
        - origin
        maxAge: 1d
      prefixRewrite: /api/pets
{{< /highlight >}}

### Available fields to configure CORS {#available-fields}

The following fields are available when configuring a CORS policy for your `VirtualService`. For more information, see the [API docs]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options/cors/cors.proto.sk/" %}}).

| Field              | Type       | Description                                                                                                                                                      | Default |
| ------------------ | ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `allowOrigin`      | `[]string` | Specifies the origins that will be allowed to make CORS requests. An origin is allowed if either allow_origin or allow_origin_regex match.                       |         |
| `allowOriginRegex` | `[]string` | Specifies regex patterns that match origins that will be allowed to make CORS requests. An origin is allowed if either `allow_origin` or `allow_origin_regex` match. Note that Gloo Gateway uses [ECMAScript](https://en.cppreference.com/w/cpp/regex/ecmascript) regex grammar. For example, to match all subdomains `https://example.com`, do not use `https://*.example.com`, but instead use `https://[a-zA-Z0-9]*\.example\.com`.   |         |
| `allowMethods`     | `[]string` | Specifies the content for the *access-control-allow-methods* header.                                                                                             |         |
| `allowHeaders`     | `[]string` | Specifies the content for the *access-control-allow-headers* header.                                                                                             |         |
| `exposeHeaders`    | `[]string` | Specifies the content for the *access-control-expose-headers* header.                                                                                            |         |
| `maxAge`           | `string`   | Specifies the content for the *access-control-max-age* header.                                                                                                   |         |
| `allowCredentials` | `bool`     | Specifies whether the resource allows credentials.                                                                                                               |         |
| `disableForRoute` | `bool` | If set, the CORS Policy (specified on the virtual host) is disabled for this route. | false |
 

## Try out a CORS policy {#cors-steps}

1. Follow the steps to [deploy the Petstore Hello World app]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) in your cluster. 
2. Edit the virtual service that exposes the Petstore app to add in a CORS policy. 
   ```sh
   kubectl edit virtualservice default -n gloo-system
   ```
   
3. Add the following CORS configuration to the `spec.virtualHostoptions` section of your virtual service. The CORS policy in this example configures the Petstore to allow cross-origin requests for the `https://example.com` and `https://*.gloo.dev` domains. With this setup, you can host scripts or other resources on the `https://*.gloo.dev` or `https://solo.io` domains, even if your application is not being served from that location.
   ```yaml
   ...
   spec:
     virtualHost:
       domains:
       - '*'
       options:
         cors:
           allowCredentials: true
           allowHeaders:
           - origin
           allowMethods:
           - GET
           - POST
           - OPTIONS
           allowOrigin:
           - https://example.com
           allowOriginRegex:
           - https://[a-zA-Z0-9]*\.example\.com
           exposeHeaders:
           - origin
           maxAge: 1d
   ```

4. Send a request to the Petstore app for the origin `https://example.com` and verify that the CORS headers are returned. The presence of these CORS headers instruct a web browser to grant access to the remote resource. 
   {{% notice note %}}
   A preflight request to the Petstore sample app cannot be simulated as part of this guide because the Petstore app does not support the `OPTIONS` method. 
   {{% /notice %}}
   ```
   curl -vik -H "Origin: https://example.com" \
   -H "Access-Control-Request-Method: GET" \
   -X GET $(glooctl proxy url)/all-pets  
   ```
   
   Example output: 
   ```
   > GET /all-pets HTTP/1.1
   > User-Agent: curl/7.77.0
   > Accept: */*
   > Origin: https://example.com
   > Access-Control-Request-Method: GET
   > 
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   < access-control-allow-origin: https://example.com
   access-control-allow-origin: https://example.com
   < access-control-allow-credentials: true
   access-control-allow-credentials: true
   < access-control-expose-headers: origin
   access-control-expose-headers: origin
   < server: envoy
   server: envoy

   < 
   [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
   ```
   
5. Send another request to the Petstore app. This time, include the origin `https://notallowed.com` that is not configured in your virtual service. Verify that no CORS headers are returned for the provided origin. 

   {{% notice note %}}
   The request still returns a 200 HTTP response code, because a curl client is used to make the request in this example. However, CORS policies are enforced in a web browser. If this type of request is sent through a web browser and no CORS headers are returned in the response, the web browser denies access to the requested resource as the cross-origin instructions are missing. 
   {{% /notice %}}

   ```
   curl -vik -H "Origin: https://notallowed.com" \
   -H "Access-Control-Request-Method: GET" \
   -X GET $(glooctl proxy url)/all-pets 
   ``` 
   
   Example output: 
   ```
   GET /all-pets HTTP/1.1
   > Host: ab2e5d3c1c8f0466b9cee8494e87a90d-1027513527.us-east-1.elb.amazonaws.com
   > User-Agent: curl/7.77.0
   > Accept: */*
   > Origin: https://notallowed.com
   > Access-Control-Request-Method: GET
   > 
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   < content-type: text/xml
   content-type: text/xml   
   < content-length: 86
   content-length: 86
   < x-envoy-upstream-service-time: 1
   x-envoy-upstream-service-time: 1
   < server: envoy
   server: envoy

   < 
   [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
   ```

6. Update the VirtualService with a CORS policy at the route level that conflicts with the CORS policy at the virtual host level. Keep in mind that route configuration overrides the virtual host configuration.

   {{< highlight yaml "hl_lines=16 21 27-32" >}}
   ...
   spec:
     virtualHost:
       domains:
       - '*'
       options:
         cors:
           allowCredentials: true
           allowHeaders:
           - origin
           allowMethods:
           - GET
           - POST
           - OPTIONS
           allowOrigin:
           - https://fake.com
           allowOriginRegex:
           - https://[a-zA-Z0-9]*\.example\.com
           exposeHeaders:
           - origin
           - vh-header
           maxAge: 1d
     routes:
       - matchers:
           - exact: /all-pets
         options:
           cors:
             allowOrigin:
               - https://example.com
             exposeHeaders:
               - origin
               - route-header
           prefixRewrite: /api/pets
   ...
   {{< /highlight >}}

7. Repeat the previous request. This time, notice that the origin is `https://example.com`, which is set by the route configuration (but not the virtual host configuration, which is `https://fake.com`). Also, the `access-control-expose-headers` header contains only the values specified on the route level, `origin` and `route-header` (and not the virtual host, `vh-header`).

   ```
   curl -vik -H "Origin: https://notallowed.com" \
   -H "Access-Control-Request-Method: GET" \
   -X GET $(glooctl proxy url)/all-pets 
   ``` 

   Example output:
   ```
   > GET /all-pets HTTP/1.1
   > Host: localhost:8080
   > User-Agent: curl/8.1.2
   > Accept: */*
   > Origin: https://example.com
   > Access-Control-Request-Method: GET
   >
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   < access-control-allow-origin: https://example.com
   access-control-allow-origin: https://example.com
   < access-control-allow-credentials: true
   access-control-allow-credentials: true
   < access-control-expose-headers: origin,route-header
   access-control-expose-headers: origin,route-header
   < server: envoy
   server: envoy
   
   <
   [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat",   "status":"pending"}]
   ```

8. To change how conflicting CORS policies are handled, update the VirtualService with the `corsPolicyMergeSettings` in the virtual host. In the following example, you configure a `UNION` merge strategy for the `exposeHeaders` field. Now, the CORS policy applies to requests so that all the `exposeHeaders` values from both the virtual host and route are included. For more information, see the [API docs]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options/cors/cors.proto.sk/#corspolicymergesettings" %}}).

   {{< highlight yaml "hl_lines=7-8" >}}
   ...
   spec:
     virtualHost:
       domains:
       - '*'
       options:
         corsPolicyMergeSettings:
           exposeHeaders: UNION
         cors:
           allowCredentials: true
           allowHeaders:
           - origin
           allowMethods:
           - GET
           - POST
           - OPTIONS
           allowOrigin:
           - https://fake.com
           allowOriginRegex:
           - https://[a-zA-Z0-9]*\.example\.com
           exposeHeaders:
           - origin
           - vh-header
           maxAge: 1d
     routes:
       - matchers:
           - exact: /all-pets
         options:
           cors:
             allowOrigin:
               - https://example.com
             exposeHeaders:
               - origin
               - route-header
           prefixRewrite: /api/pets
   ...
   {{< /highlight >}}

9. Repeat the request. This time, notice that the `access-control-expose-headers` header has the union of the values set on both the virtual host and the route (`origin`, `vh-header`, and `route-header`).

   ```
   curl -vik -H "Origin: https://notallowed.com" \
   -H "Access-Control-Request-Method: GET" \
   -X GET $(glooctl proxy url)/all-pets 
   ``` 

   Example output:
   ```
   > GET /all-pets HTTP/1.1
   > Host: localhost:8080
   > User-Agent: curl/8.1.2
   > Accept: */*
   > Origin: https://example.com
   > Access-Control-Request-Method: GET
   >
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   < access-control-allow-origin: https://example.com
   access-control-allow-origin: https://example.com
   < access-control-allow-credentials: true
   access-control-allow-credentials: true
   < access-control-expose-headers: origin,vh-header,route-header
   access-control-expose-headers: origin,vh-header,route-header
   < server: envoy
   server: envoy
   
   <
   [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat",   "status":"pending"}]
   ```
