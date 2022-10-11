---
title: Prefix Rewrite
weight: 80
description: Prefix-rewriting when routing to upstreams
---

{{< protobuf name="gloo.solo.io.RouteOptions" display="PrefixRewrite" >}}
is a route feature that allows you to replace (rewrite) the matched request path with a specified value before sending it upstream.

Routes are processed in order, so the first matching request path is the only one that will be processed.

{{% notice note %}}
Setting prefixRewrite to "" is ignored. It's interpreted the same as if you did not provide any value 
at all, i.e., do NOT rewrite the path.
{{% /notice %}}

### Example

Install gloo gateway
```shell script
glooctl install gateway
```

Install the petstore demo
```shell script
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.11.x/example/petstore/petstore.yaml
```

Create a virtual service with routes for `/foo` and `/bar`
```yaml
kubectl apply -f - << EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'default'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
       - prefix: '/foo'
      routeAction:
        single:
          upstream:
            name: 'default-petstore-8080'
            namespace: 'gloo-system'
      options:
        prefixRewrite: '/api/invalid'
    - matchers:
       - prefix: '/bar'
      routeAction:
        single:
          upstream:
            name: 'default-petstore-8080'
            namespace: 'gloo-system'
      options:
        prefixRewrite: '/api/pets'
status: {}
EOF
```

These routes use prefix rewrite to change the request path before sending it upstream to the petstore microservice.

The petstore microservice lacks the `/api/invalid` endpoint, so the following command fails when handled upstream.
```shell script
curl "$(glooctl proxy url)/foo"
```
returns
```json
{"code":404,"message":"path /api/invalid was not found"}
```

Meanwhile the following command rewrites the `/bar` to the `/api/pets` endpoint, which the petstore microservice supports.
```shell script
curl "$(glooctl proxy url)/bar"
```

returns

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

We have successfully shown how you can change the external API of your services without changing the services themselves.

### Avoiding the `//` rewrite problem

When rewriting routes you must be careful with trailing backslashes. For example, the following naïve virtual service
looks fine
```yaml
kubectl apply -f - << EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'default'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
       - prefix: '/foo'
      routeAction:
        single:
          upstream:
            name: 'default-petstore-8080'
            namespace: 'gloo-system'
      options:
        prefixRewrite: '/'
status: {}
EOF
```
When you curl `/foo` the request path is rewritten to `/` as you would expect
```shell script
curl "$(glooctl proxy url)/foo"
```
returns
```json
{"code":404,"message":"path / was not found"}
```

But requests to `/foo/` (often a valid request) may surprise you
```shell script
curl "$(glooctl proxy url)/foo/"
```
returns
```json
{"code":404,"message":"path // was not found"}
```

To avoid this problem, we (and [Envoy](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route.proto.html#envoy-api-field-route-routeaction-prefix-rewrite))
recommend two matchers, one for each case (order matters)
```yaml
kubectl apply -f - << EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'default'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
       - prefix: '/foo/'
      routeAction:
        single:
          upstream:
            name: 'default-petstore-8080'
            namespace: 'gloo-system'
      options:
        prefixRewrite: '/'
    - matchers:
       - prefix: '/foo'
      routeAction:
        single:
          upstream:
            name: 'default-petstore-8080'
            namespace: 'gloo-system'
      options:
        prefixRewrite: '/'
status: {}
EOF
```

Now `/foo/` rewrites to `/`, as desired
```shell script
curl "$(glooctl proxy url)/foo/"
```
returns
```json
{"code":404,"message":"path / was not found"}
```

### Cleanup

```shell script
glooctl uninstall
kubectl delete -f https://raw.githubusercontent.com/solo-io/gloo/v1.11.x/example/petstore/petstore.yaml
```
