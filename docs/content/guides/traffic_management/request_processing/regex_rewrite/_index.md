---
title: Regex Rewrite
weight: 80
description: Regex-rewriting when routing to upstreams
---

{{< protobuf name="gloo.solo.io.RouteOptions" display="RegexRewrite" >}}
is a route feature that allows you to replace (rewrite) the matched request path with a specified value before sending it upstream.

Routes are processed in order, so the first matching request path is the only one that will be processed.

### Example

Install gloo gateway
```shell script
glooctl install gateway
```

Install the petstore demo
```shell script
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
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
        regexRewrite: 
          pattern:
            regex: 'bar'
          substitution: 'baz'
    - matchers:
       - prefix: '/pre'
      routeAction:
        single:
          upstream:
            name: 'default-petstore-8080'
            namespace: 'gloo-system'
      options:
        regexRewrite: 
          pattern:
            regex: '^/pre/([^/]+)(/.*)$'
          substitution: '\2/swap/\1'
status: {}
EOF
```

These routes use regex rewrite to change the request path before sending it upstream to the petstore microservice.

The petstore microservice lacks the `/foo/baz` endpoint, so the following command fails when handled upstream.
```shell script
curl "$(glooctl proxy url)/foo/bar"
```
returns
```json
{"code":404,"message":"path /foo/baz was not found"}
```

A more complex example uses capture groups to route to a different "not found" endpoint:
```shell script
curl "$(glooctl proxy url)/pre/mid/end"
```
returns

```json
{"code":404,"message":"path /end/swap/mid was not found"}
```

We have successfully shown how you can change the external API of your services without changing the services themselves.

### Cleanup

```shell script
glooctl uninstall
kubectl delete -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

### Regex Engine

The Google RE2 engine is used by default. The user may specify another engine, but only the Google RE2 engine is supported.

The user may supply a `max_program_size` parameter to the Google RE2 engine like so:
```yaml
        regexRewrite: 
          pattern:
            google_re2:
              max_program_size: 1024
            regex: 'bar'
          substitution: 'baz'
```
If the global `regexMaxProgramSize` specified via the {{< protobuf name="gloo.solo.io.Settings" display="Settings">}} resource is more restrictive than that specified in the config, the global setting will be used.

{{% notice note %}} Max program size is enforced by Envoy. If a given regex exceeds the max program size, from either config or global setting, it will not be reflected in the virtual service status. {{% /notice %}}
