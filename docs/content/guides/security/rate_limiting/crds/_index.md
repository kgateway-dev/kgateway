---
title: RateLimitConfigs (Enterprise)
description: Powerful, reusable configuration for Gloo Edge Enterprise's rate-limit service.
weight: 20
---

{{% notice note %}}
Rate limit configuration via `RateLimitConfig` resources was introduced with **Gloo Edge Enterprise**, release `v1.5.0-beta3`. 
If you are using an earlier version, this feature will not be available.
{{% /notice %}}

As we saw in the [Envoy API guide]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/" %}}), 
Gloo Edge Enterprise exposes a fine-grained API that allows you to configure a vast number of rate limiting use cases.
The two main objects that make up the API are:
1. the [`descriptors`]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy//#descriptors" %}}), 
  which configure the rate limit server and are defined on the global `Settings` resource, and
2. the [`actions`]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy//#actions" %}}) that 
  determine how Envoy composes the descriptors that are sent to the server to check whether a request should be 
  rate-limited; `actions` are defined either on the `Route` or on the `VirtualHost` `options`.
  
Although powerful, this API has some drawbacks:
- The `descriptors` that define the rate limit policies you want to enforce are defined in a single, central location. 
  The global nature of the `Settings` does not guarantee isolation between different rate limits: if you make a mistake 
  while updating a policy, you might end up impacting other policies. This centralized configuration also makes it 
  harder to safely manage you rate limit policies in an automated fashion.
- Since `actions` are defined directly on your `Virtual Services`, your rate limits are tightly coupled to your routing 
  configuration. If you need to apply the same policy on different routes, you will need to redefine the same 
  configuration in multiple places. This can be a significant source of configuration bloat, especially if your policies 
  are complex.
  
To address these shortcomings, we introduced a new custom resource.

### RateLimitConfig resources
Starting with Gloo Edge Enterprise `v1.5.0-beta3` you can define you rate limits by creating `RateLimitConfig` resources. 
A `RateLimitConfig` resource represents a self-contained rate limit policy; this means that Gloo Edge will use the resource 
to configure both the Envoy proxies and the Gloo Edge Enterprise rate limit server they communicate with. 
Gloo Edge guarantees that rate limit rules defined on different `RateLimitConfig` resources are completely independent of each other.

Here is a simple example of a `RateLimitConfig` resource:

```yaml
apiVersion: ratelimit.solo.io/v1alpha1
kind: RateLimitConfig
metadata:
  name: my-rate-limit-policy
  namespace: gloo-system
spec:
  raw:
    descriptors:
    - key: generic_key
      value: counter
      rateLimit:
        requestsPerUnit: 10
        unit: MINUTE
    rateLimits:
    - actions:
      - genericKey:
          descriptorValue: counter
```

Once an `RateLimitConfig` is created, it can be used to enforce rate limits on your `VirtualService`s (or on the 
lower-level `Proxy` resources). You can do that be referencing the resource at two different levels:

- on **VirtualHosts** and
- on **Routes**,

The configuration format is the same in both cases. It must be specified under the relevant `options` attribute 
(on Virtual Hosts or Routes). This snippet shows an example configuration that uses the above `RateLimitConfig`:

```yaml
options:
  rateLimitConfigs:
    refs:
    - name: my-rate-limit-policy
      namespace: gloo-system
```

`RateLimitConfig`s defined on a `VirtualHost` is inherited by all the `Route`s that belong to that `VirtualHost`, 
unless a route itself references its own `RateLimitConfig`s.

#### Configuration format
Each `RateLimitConfig` is an instance of one specific configuration type. Currently, only the `raw` configuration type 
is implemented, but we are planning on adding more high-level configuration formats to support specific use cases 
(e.g. limiting requests based on the presence and value of a header, or on a per-upstream, per-client basis, etc.).

The `raw` configuration allows you to specify rate limit policies using the raw configuration formats used by the 
server and the client (Envoy). It consists of two elements:

- a list of `descriptors`, and
- a list of `rateLimits`.

These two objects have the exact some format as the `descriptors` and `ratelimits` that are explained in detail in the 
[Envoy API guide]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/" %}}). 

### Example
Let's run through an example that uses `RateLimitConfig` resources to enforce rate limit policies on your `Virtual Services`. 
As mentioned earlier, all the examples that are listed in the [Envoy API guide]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/" %}}) 
apply to `RateLimitConfig`s as well, so please be sure to check them out.

#### Initial setup
First, we need to install Gloo Edge Enterprise (minimum version `1.5.0-beta3`). Please refer to the corresponding
[installation guide]({{< versioned_link_path fromRoot="/installation/enterprise" >}}) for details.
 
Let's also deploy a simple application which returns "Hello World" when receiving HTTP requests:

```yaml
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: http-echo
  name: http-echo
  namespace: default
spec:
  selector:
    matchLabels:
      app: http-echo
  replicas: 1
  template:
    metadata:
      labels:
        app: http-echo
    spec:
      containers:
      - image: hashicorp/http-echo:latest
        name: http-echo
        args: ["-text=Hello World!"]
        ports:
        - containerPort: 5678
          name: http
---
apiVersion: v1
kind: Service
metadata:
  name: http-echo
  namespace: default
  labels:
    service: http-echo
spec:
  ports:
  - port: 5678
    protocol: TCP
  selector:
    app: http-echo
EOF
```

For the purpose of this example, let's create two different upstreams that point to the same service. You'll soon see why we do this. 

```yaml
kubectl apply -f - <<EOF
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: echo-1
  namespace: gloo-system
spec:
  static:
    hosts:
      - addr: http-echo.default.svc.cluster.local
        port: 5678
---
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: echo-2
  namespace: gloo-system
spec:
  static:
    hosts:
      - addr: http-echo.default.svc.cluster.local
        port: 5678
EOF
```
 
Now let's create a Virtual Service with two different routes. Requests with the `/echo-1` and `/echo-2` path prefixes 
will be routed to `http-echo` service.

```yaml
kubectl apply -f - << EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: echo
  namespace: gloo-system
spec:
  displayName: echo
  virtualHost:
    domains:
      - '*'
    routes:
      - matchers:
        - prefix: /echo-1
        routeAction:
          single:
            upstream:
              name: echo-1
              namespace: gloo-system
      - matchers:
        - prefix: /echo-2
        routeAction:
          single:
            upstream:
              name: echo-2
              namespace: gloo-system
EOF
```

To verify that the Virtual Service works, let's send a request to `/echo`:

```bash
curl $(glooctl proxy url)/echo-1
curl $(glooctl proxy url)/echo-2
```

Both should return the expected response:
```
Hello World!
```

#### Apply rate limit policies
Now let's create two `RateLimitConfig` resources.

```yaml
kubectl apply -f - << EOF
apiVersion: ratelimit.solo.io/v1alpha1
kind: RateLimitConfig
metadata:
  name: global-limit
  namespace: gloo-system
spec:
  raw:
    descriptors:
    - key: generic_key
      value: count
      rateLimit:
        requestsPerUnit: 4
        unit: MINUTE
    rateLimits:
    - actions:
      - genericKey:
          descriptorValue: count
---
apiVersion: ratelimit.solo.io/v1alpha1
kind: RateLimitConfig
metadata:
  name: per-upstream-counter
  namespace: gloo-system
spec:
  raw:
    descriptors:
    - key: destination_cluster
      rateLimit:
        requestsPerUnit: 3
        unit: MINUTE
    rateLimits:
    - actions:
      - destinationCluster: {}
EOF
```

Let's see what each of these resources represents:

- `global-limit` defines a simple counter. Every time a request matches a route that references this resource, 
  the counter will be increased. After the counter has been increased **4 times within a 1-minute time window**, successive 
  requests in the same time window will be rejected with a `429` response code;
- `per-upstream-counter` defines a set of counters. Each counter tracks requests to a specific `cluster` (the Envoy 
  equivalent of a Gloo Edge `Upstream`). After an `Upstream` has received **3 requests within a 1-minute time window**, successive 
  requests to the same upstream in the same time window will be rejected with a `429` response code.
  
Now let's apply these policies to our `VirtualService`:

{{< highlight yaml "hl_lines=12-19" >}}
kubectl apply -f - << EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: echo
  namespace: gloo-system
spec:
  displayName: echo
  virtualHost:
    domains:
      - '*'
    options:
      rateLimitConfigs:
        refs:
        - name: global-limit
          namespace: gloo-system
        - name: per-upstream-counter
          namespace: gloo-system
    routes:
      - matchers:
        - prefix: /echo-1
        routeAction:
          single:
            upstream:
              name: echo-1
              namespace: gloo-system
      - matchers:
        - prefix: /echo-2
        routeAction:
          single:
            upstream:
              name: echo-2
              namespace: gloo-system
EOF
{{< /highlight >}}

We have applied these two policies to the `VirtualHost`, so they apply to both of the routes that belong to 
the `VirtualHost`. This will cause requests to be rate-limited either when:

- one of the two upstreams is hit more than **3 times within a minute**, or
- the aggregate of both upstreams is hit more than **4 times within a minute**.

You can verify that Gloo Edge has been correctly configured by port-forwarding the rate limit server and requesting a 
config dump. First run:

```shell script
kubectl port-forward -n gloo-system deploy/rate-limit 9091
```

Then - from a separate shell - run:

```shell script
curl http://localhost:9091/rlconfig/
```

You should get the following response:

```
solo.io.generic_key_gloo-system.global-limit.generic_key_count: unit=MINUTE requests_per_unit=4 weight=0 always_apply=false
solo.io.generic_key_gloo-system.per-upstream-counter.destination_cluster: unit=MINUTE requests_per_unit=3 weight=0 always_apply=false
```

#### Test our configuration
Let's verify that our rate limit policies are correctly enforced.

First, let's try sending some requests to the `echo-1` upstream. Submit the following command multiple times in rapid succession:

```shell script
curl -v $(glooctl proxy url)/echo-1
```

On the **fourth attempt** you should receive the following response:

```shell script
< HTTP/1.1 429 Too Many Requests
< x-envoy-ratelimited: true
< date: Tue, 14 Jul 2020 23:13:18 GMT
< server: envoy
< content-length: 0
```

This demonstrates that the per-upstream rate limit in enforced. Now let's wait for a minute for the counter to reset 
and then submit the same command again, but this time only **2 times**:

```shell script
curl -v $(glooctl proxy url)/echo-1
```

You should get two successful `Hello World!` responses. 
After the second attempt, let's start sending requests to the `echo-2` upstream:

```shell script
curl -v $(glooctl proxy url)/echo-2
```

The **third attempt** should return the `429 Too Many Reqeusts` response:

```shell script
< HTTP/1.1 429 Too Many Requests
< x-envoy-ratelimited: true
< date: Tue, 14 Jul 2020 23:13:18 GMT
< server: envoy
< content-length: 0
```

This is because, although we get 3 requests per minute on the `Upstream`, we have already reached the `global-limit` of 
4 requests per minute across both `Upstreams`.