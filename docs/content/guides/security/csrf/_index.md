---
title: CSRF
weight: 10
description: Shield your applications from session-riding attacks.
---

### Understanding CSRF

According to [OWASP](https://owasp.org/www-community/attacks/csrf):

> Cross-Site Request Forgery (CSRF) is an attack that forces an end user to execute unwanted actions on a web application in which they’re currently authenticated. With a little help of social engineering (such as sending a link via email or chat), an attacker may trick the users of a web application into executing actions of the attacker’s choosing. If the victim is a normal user, a successful CSRF attack can force the user to perform state changing requests like transferring funds, changing their email address, and so forth. If the victim is an administrative account, CSRF can compromise the entire web application.

Application owners can battle CSRF attacks at multiple levels of their stack.  At the application level, most popular frameworks support one or more CSRF prevention [options](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html#use-built-in-or-existing-csrf-implementations-for-csrf-protection).

A primary benefit of API Gateways like Gloo Edge is that they provide these weapons at a controlled access point and thus can off-load responsibilities from the application team.

One option for Gloo Edge Enterprise users is to  activate its [Web Application Firewall]({{% versioned_link_path fromRoot="/guides/security/waf/" %}}) based on Apache ModSecurity.  It supports use of CSRF rules in the OWASP [Core Rule Set](https://coreruleset.org/).

Alternatively, Envoy provides a simple [CSRF filter](https://www.envoyproxy.io/docs/envoy/latest/api-v2/config/filter/http/csrf/v2/csrf.proto) that may be applied to an entire Gloo `Gateway`, a `VirtualService`, or even individual `Routes` within a `VirtualService`.  To understand more about how this filter works in Envoy, we recommend playing in their CSRF [sandbox](https://www.envoyproxy.io/docs/envoy/latest/start/sandboxes/csrf).

The purpose of this guide is to demonstrate how to apply the Envoy CSRF filter to a Gloo Edge `VirtualService`.

### Deploy the Httpbin Service

We'll use the popular [httpbin](http://httpbin.org/) service as our testbed.  Let's begin by deploying it on a Kubernetes cluster.   

{{< notice note >}}
This example was created and tested using a GKE cluster running k8s v1.16.15, but other platforms and versions should work as well.
{{< /notice >}}

```shell
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: httpbin
---
apiVersion: v1
kind: Service
metadata:
  name: httpbin
  labels:
    app: httpbin
spec:
  ports:
  - name: http
    port: 8000
    targetPort: 80
  selector:
    app: httpbin
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin
      version: v1
  template:
    metadata:
      labels:
        app: httpbin
        version: v1
    spec:
      serviceAccountName: httpbin
      containers:
      - image: docker.io/kennethreitz/httpbin
        imagePullPolicy: IfNotPresent
        name: httpbin
        ports:
        - containerPort: 80
EOF
```

### Verify the Upstream

Gloo Edge discovers Kubernetes services automatically.  So, running the `glooctl get upstreams` command, you should be able to see a new Gloo Edge `Upstream` `default-httpbin-8000` with an `Accepted` status.  The name of the discovered `Upstream` was generated automatically by Gloo Edge based on the naming convention `namespace-serviceName-portNumber`:

```shell
% glooctl get upstreams default-httpbin-8000
+----------------------+------------+----------+------------------------+
|       UPSTREAM       |    TYPE    |  STATUS  |        DETAILS         |
+----------------------+------------+----------+------------------------+
| default-httpbin-8000 | Kubernetes | Accepted | svc name:      httpbin |
|                      |            |          | svc namespace: default |
|                      |            |          | port:          8000    |
|                      |            |          |                        |
+----------------------+------------+----------+------------------------+
```

### Create the Virtual Service

Create the following Gloo Edge `VirtualService` that will route all its requests to the new `Upstream`.

```shell
cat <<EOF | kubectl apply -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: httpbin
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
            name: default-httpbin-8000
            namespace: gloo-system
EOF
```

Run the following `glooctl` command to confirm that the new `Route` was accepted by Gloo Edge.

```shell
% glooctl get virtualservice httpbin
+-----------------+--------------+---------+------+----------+-----------------+----------------------------------+
| VIRTUAL SERVICE | DISPLAY NAME | DOMAINS | SSL  |  STATUS  | LISTENERPLUGINS |              ROUTES              |
+-----------------+--------------+---------+------+----------+-----------------+----------------------------------+
| httpbin         |              | *       | none | Accepted |                 | / ->                             |
|                 |              |         |      |          |                 | gloo-system.default-httpbin-8000 |
|                 |              |         |      |          |                 | (upstream)                       |
+-----------------+--------------+---------+------+----------+-----------------+----------------------------------+
```

### Test the Service

We'll use `curl` to exercise the service in a few different ways.  First, we'll issue a proper invocation with an `origin` header that matches our target host.  Second, we'll mimic an improper request by issuing a mismatched `origin` header.  Third, we'll mimic a different type of improper request by eliminating the `origin` header altogether.

```shell
# Matching Origin Header
% root_url=${$(glooctl proxy url)%:*} # trim port from proxy url
% curl -X POST "$(glooctl proxy url)/post" -H "origin: $root_url" -i
HTTP/1.1 200 OK
server: envoy
date: Tue, 05 Jan 2021 20:46:13 GMT
content-type: application/json
content-length: 362
access-control-allow-origin: http://34.74.14.50:80
access-control-allow-credentials: true
x-envoy-upstream-service-time: 5

{
  "args": {},
  "data": "",
  "files": {},
  "form": {},
  "headers": {
    "Accept": "*/*",
    "Content-Length": "0",
    "Host": "34.74.14.50",
    "Origin": "http://34.74.14.50:80",
    "User-Agent": "curl/7.64.1",
    "X-Envoy-Expected-Rq-Timeout-Ms": "15000"
  },
  "json": null,
  "origin": "10.68.2.3",
  "url": "http://34.74.14.50/post"
}
```

Note that the `httpbin` service sends back information mirroring the proper request that we issued.

If you issue requests with mismatched headers and missing headers, like these:

```shell
# Mismatched Origin Header
curl -X POST "$(glooctl proxy url)/post" -H "origin: http://example.com" -i
```
```shell
# Missing Origin header
curl -X POST "$(glooctl proxy url)/post" -i
```

You'll notice that they also succeed.  Why?  Because we have not yet applied any CSRF policies.  We'll get to that shortly.

### Review the Envoy CSRF Metrics

Envoy publishes a set of three metrics that are updated when its CSRF filter is active.  They track the number of valid requests, the number of total invalid requests, and the number of invalid requests due to a missing origin.

In development mode, one easy way to review these metrics is to establish a `port-forward` from the Envoy gateway proxy pod to your local workstation, and then curl that endpoint for the CSRF metrics.

```shell
% kubectl port-forward deployment/gateway-proxy -n gloo-system 19000
Forwarding from 127.0.0.1:19000 -> 19000
Forwarding from [::1]:19000 -> 19000
Handling connection for 19000
```

```shell
% curl -s http://localhost:19000/stats | grep csrf
http.http.csrf.missing_source_origin: 0
http.http.csrf.request_invalid: 0
http.http.csrf.request_valid: 0
```

At this point, note that the metrics are all zero-valued since we haven't yet activated the CSRF filter in Envoy.

### Apply a Shadow CSRF Policy

The CSRF filter configuration supports both policy shadowing and policy enforcement.  In shadowing mode, the filter samples some percentage of incoming traffic but allows every request to proceed.  It does record the results of its evaluations using the CSRF metrics.

In this section, we will modify our `VirtualService` to apply the filter to all requests and report evaluation results using the Envoy metrics.  Note that we have added a `shadowEnabled` policy that evaluates 100% of the traffic and reports -- but does not block -- violations.

```shell
cat <<EOF | kubectl apply -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: httpbin
  namespace: gloo-system
spec:
  displayName: httpbin
  virtualHost:
    domains:
      - '*'
    routes:
      - matchers:
        - prefix: /
        routeAction:
          single:
            upstream:
              name: default-httpbin-8000
              namespace: gloo-system
    options:
      csrf:
        shadowEnabled:
          defaultValue:
            numerator: 100
            denominator: HUNDRED
EOF
```

Issuing the same three `curl` requests against the `httpbin` endpoint yields nearly the same results as before.  The requests all succeed, but only one of them was valid according to the policy.  Two of the three are flagged as invalid, and one of those invalid requests is flagged as having a missing `origin` header.  You can verify this from the published CSRF metrics.

```shell
% curl -s http://localhost:19000/stats | grep csrf
http.http.csrf.missing_source_origin: 1
http.http.csrf.request_invalid: 2
http.http.csrf.request_valid: 1
```

### CSRF Policy Scoping

Note that CSRF policies may be scoped at different levels of the Gloo Edge hierarchy.  In this example, we are applying the policy at the [virtual host level]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options.proto.sk/#virtualhostoptions" %}}).  In addition, we may scope them more broadly, at the [listener level]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options.proto.sk/#httplisteneroptions" %}}) for an entire gateway.  Or we may scope these policies more narrowly, even down to the [individual route level]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options.proto.sk/#routeoptions" %}}).

### Enforce the CSRF Policy

Next we enable CSRF policy enforcement for our `VirtualService`.  This requires only a single-line change to our policy from `shadowEnabled` to `filterEnabled`.

```shell
cat <<EOF | kubectl apply -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: httpbin
  namespace: gloo-system
spec:
  displayName: httpbin
  virtualHost:
    domains:
      - '*'
    routes:
      - matchers:
        - prefix: /
        routeAction:
          single:
            upstream:
              name: default-httpbin-8000
              namespace: gloo-system
    options:
      csrf:
        filterEnabled:
          defaultValue:
            numerator: 100
            denominator: HUNDRED
EOF
```

The original valid request with the matching `origin` header will work exactly as before, and the valid request metric is incremented.

```shell
# Matching Origin Header
% root_url=${$(glooctl proxy url)%:*} # trim port from proxy url
% curl -X POST "$(glooctl proxy url)/post" -H "origin: $root_url" -i
HTTP/1.1 200 OK
server: envoy
date: Tue, 05 Jan 2021 23:02:40 GMT
content-type: application/json
content-length: 362
access-control-allow-origin: http://34.74.14.50:80
access-control-allow-credentials: true
x-envoy-upstream-service-time: 5

{
  "args": {},
  "data": "",
  "files": {},
  "form": {},
  "headers": {
    "Accept": "*/*",
    "Content-Length": "0",
    "Host": "34.74.14.50",
    "Origin": "http://34.74.14.50:80",
    "User-Agent": "curl/7.64.1",
    "X-Envoy-Expected-Rq-Timeout-Ms": "15000"
  },
  "json": null,
  "origin": "10.68.2.3",
  "url": "http://34.74.14.50/post"
}
```

```shell
% curl -s http://localhost:19000/stats | grep csrf
http.http.csrf.missing_source_origin: 1
http.http.csrf.request_invalid: 2
http.http.csrf.request_valid: 2
```

However, applying a mismatched `origin` header now causes Envoy to reject the request with a `403 Forbidden` error.

```shell
# Mismatched Origin Header
curl -X POST "$(glooctl proxy url)/post" -H "origin: http://example.com" -i
HTTP/1.1 403 Forbidden
content-length: 14
content-type: text/plain
date: Tue, 05 Jan 2021 23:08:09 GMT
server: envoy

Invalid origin
```

And the invalid request metric increments by 1:
```shell
% curl -s http://localhost:19000/stats | grep csrf
http.http.csrf.missing_source_origin: 1
http.http.csrf.request_invalid: 3
http.http.csrf.request_valid: 2
```

Finally, a request with a missing `origin` header similarly fails, but increments both the invalid request and missing source origin metrics:

```shell
# Missing Origin header
curl -X POST "$(glooctl proxy url)/post" -i
HTTP/1.1 403 Forbidden
content-length: 14
content-type: text/plain
date: Tue, 05 Jan 2021 23:11:13 GMT
server: envoy

Invalid origin
```

```shell
% curl -s http://localhost:19000/stats | grep csrf
http.http.csrf.missing_source_origin: 2
http.http.csrf.request_invalid: 4
http.http.csrf.request_valid: 2
```

### Allow Additional Origins

The `origin` header and target host require a precise match by default. However, you may configure `additionalOrigins` as a filter option to allow alternative request sources.  For example, in our case assume we want to allow requests that originate from any derivative of example.com.  We could modify our `VirtualService` to supply a single `additionalOrigins` entry as follows.

```shell
cat <<EOF | kubectl apply -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: httpbin
  namespace: gloo-system
spec:
  displayName: httpbin
  virtualHost:
    domains:
      - '*'
    routes:
      - matchers:
        - prefix: /
        routeAction:
          single:
            upstream:
              name: default-httpbin-8000
              namespace: gloo-system
    options:
      csrf:
        filterEnabled:
          defaultValue:
            numerator: 100
            denominator: HUNDRED
        additionalOrigins:
          - suffix: example.com
EOF
```

Then the previous "mismatched" origin request will work as expected in our new configuration.

```shell
# Mismatched Origin Header NO MORE
% curl -X POST "$(glooctl proxy url)/post" -H "origin: http://anyapp.api.example.com" -i
HTTP/1.1 200 OK
server: envoy
date: Tue, 05 Jan 2021 23:27:33 GMT
content-type: application/json
content-length: 370
access-control-allow-origin: http://anyapp.api.example.com
access-control-allow-credentials: true
x-envoy-upstream-service-time: 2

{
  "args": {},
  "data": "",
  "files": {},
  "form": {},
  "headers": {
    "Accept": "*/*",
    "Content-Length": "0",
    "Host": "34.74.14.50",
    "Origin": "http://anyapp.api.example.com",
    "User-Agent": "curl/7.64.1",
    "X-Envoy-Expected-Rq-Timeout-Ms": "15000"
  },
  "json": null,
  "origin": "10.68.2.3",
  "url": "http://34.74.14.50/post"
}
```

### Summary

In this guide, we described what is Cross Site Request Forgery (CSRF) and approaches for dealing with these attacks.  We delved into one Gloo Edge approach that directly uses an integrated Envoy filter.

For more information, check out both the [Envoy docs](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/csrf_filter#config-http-filters-csrf) and [Gloo docs]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/external/envoy/extensions/filters/http/csrf/v3/csrf.proto.sk/" %}}).
