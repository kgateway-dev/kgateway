---
title: Appending and Removing Request/Response Headers
menuTitle: Header Control
weight: 70
description: Append and Remove Headers from Requests and Responses using Route configuration.
---

Gloo Edge can add and remove headers to/from requests and responses. We refer to this feature as "Header Manipulation".

Header Manipulation is configured via the 
{{< protobuf name="headers.options.gloo.solo.io.HeaderManipulation" display="headerManipulation">}} struct.

This struct can be added to {{< protobuf name="gloo.solo.io.RouteOptions" display="Route Options">}}, {{< protobuf name="gloo.solo.io.VirtualHostOptions" display="Virtual Host Options">}}, and {{< protobuf name="gloo.solo.io.WeightedDestinationOptions" display="Weighted Destination Options" >}}.

The `headerManipulation` struct contains four optional fields `requestHeadersToAdd`, `requestHeadersToRemove`,  `responseHeadersToAdd`, and `responseHeadersToRemove`. The key and value for the header can be specified directly in the manifest, or in the case of `requestHeadersToAdd` it can be a reference to a secret of the type `gloo.solo.io/header` or `Opaque`.

```yaml
headerManipulation:

  # add headers to request
  requestHeadersToAdd:
  - header:
      key: HEADER_NAME
      value: HEADER_VALUE
    # if the header HEADER_NAME is already present,
    # append the value.
    append: true
  - header:
      key: HEADER_NAME
      value: HEADER_VALUE
    # if the header HEADER_NAME is already present,
    # overwrite the value.
    append: false
  - headerSecretRef:
      name: SECRET_NAME
      namespace: SECRET_NAMESPACE
    # The type of the secret must be gloo.solo.io/header or Opaque
    # Each key/value pair in the secret will be added

  # remove headers from request
  requestHeadersToRemove:
  - "HEADER_NAME"
  - "HEADER_NAME"

  # add headers to response
  responseHeadersToAdd:
  - header:
      key: HEADER_NAME
      value: HEADER_VALUE
    # if the header HEADER_NAME is already present,
    # append the value.
    append: true
  - header:
      key: HEADER_NAME
      value: HEADER_VALUE
    # if the header HEADER_NAME is already present,
    # overwrite the value.
    append: false

  # remove headers from response
  responseHeadersToRemove:
  - "HEADER_NAME"
  - "HEADER_NAME"
  

```

Depending on where the `headerManipulation` struct is added, the header manipulation will be applied on that level.

* When using `headerManipulation` in route `options`,
headers will be manipulated for all traffic matching that route.

* When using `headerManipulation` in virtual host `options`,
headers will be manipulated for all traffic handled by the virtual host.

* When using `headerManipulation` in weighted destination `options`,
headers will be manipulated for all traffic that is sent to the specific destination when it is selected for load balancing.

Envoy supports adding dynamic values to request and response headers. The percent symbol (%) is used to 
delimit variable names. See a list of the dynamic variables supported by Envoy in the [envoy docs](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/headers#custom-request-response-headers).

If you would like to use the `headerSecretRef` with `requestHeadersToAdd`, you can create the correct Secret type by using `glooctl`. For example, the following command will create a secret named `my-headers`:

```bash
glooctl create secret header my-headers --headers x-header-1=one,x-header-2=two
```

The secret will be created in the same namespace as the Gloo Edge installation by default. Inspecting the secret will show that the type is set to `gloo.solo.io/header`. Each key/value pair in the secret will be added as a header to the request.

## Example: Manipulating Headers on a Route


{{< highlight yaml "hl_lines=24-30" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  creationTimestamp: null
  name: 'default'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
       - prefix: '/petstore'
      routeAction:
        single:
          upstream:
            name: 'default-petstore-8080'
            namespace: 'gloo-system'
      options:
        prefixRewrite: '/api/pets'
        headerManipulation:
          # add headers to all responses 
          # returned by this route
          responseHeadersToAdd:
          - header:
              key: HEADER_NAME
              value: HEADER_VALUE
          - headerSecretRef
              name: my-headers
              namespace: gloo-system
status: {}
{{< /highlight >}}


## Example: Manipulating Headers on a VirtualHost

{{< highlight yaml "hl_lines=22-27" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  creationTimestamp: null
  name: 'default'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
       - prefix: '/petstore'
      routeAction:
        single:
          upstream:
            name: 'default-petstore-8080'
            namespace: 'gloo-system'
      options:
        prefixRewrite: '/api/pets'
    options:
      headerManipulation:
        # remove headers from all requests 
        # handled by this virtual host
        requestHeadersToRemove:
        - "x-my-header"
        - "x-your-header"
status: {}
{{< /highlight >}}



## Example: Manipulating Headers on a Weighted Destination

{{< highlight yaml "hl_lines=28-35" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  creationTimestamp: null
  name: 'default'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
       - prefix: /myservice
      routeAction:
        multi:
          destinations:
          - weight: 9
            destination:
              upstream:
                name: default-myservice-v1-8080
                namespace: gloo-system
          - weight: 1
            destination:
              upstream:
                name: default-myservice-v2-8080
                namespace: gloo-system
            options:
              headerManipulation:
                # add headers to all requests
                # that are load balanced to `default-myservice-v2-8080`
                # on this route 
                requestHeadersToAdd:
                - header:
                    key: HEADER_NAME
                    value: HEADER_VALUE
status: {}
{{< /highlight >}}


