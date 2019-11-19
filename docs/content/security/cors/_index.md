---
title: CORS
weight: 50
description: Cross-Origin Resource Sharing (CORS) is a method of enforcing client-side access controls on resources by 
specifying external domains that are able to access certain or all routes of your domain.
---

### Understanding CORS

Cross-Origin Resource Sharing (CORS) is a method of enforcing client-side access controls on resources by specifying
external domains that are able to access certain or all routes of your domain. Browsers use the presence of HTTP headers
to determine if a response from a different origin is allowed.

It is a mechanism which aims to allow requests made on behalf of you and at the same time block requests made by rogue
JS. As an example, it is triggered whenever scenarios like the ones below occur:

- a different domain (eg. site at example.com calls api.com)
- a different sub domain (eg. site at example.com calls api.example.com)
- a different port (eg. site at example.com calls example.com:3001)
- a different protocol (eg. site at `https://example.com` calls `http://example.com`)

For more details, see [this article](https://medium.com/@baphemot/understanding-cors-18ad6b478e2b).

### Where to Use It

In order to allow your `VirtualService` to work with CORS, you need to add a new set of configuration options in
the `VirtualHost` part of your `VirtualService`

{{< highlight yaml "hl_lines=9-10" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: corsexample
  namespace: gloo-system
spec:
  displayName: corsexample
  virtualHost:
    corsPolicy:
      (...)
    domains:
    - '*'
{{< /highlight >}}

### Available Fields

The following fields are available when specifying CORS on your `VirtualService`:

| Field              | Type       | Description                                                                                                                                                      | Default |
| ------------------ | ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `allowOrigin`      | `[]string` | Specifies the origins that will be allowed to make CORS requests. An origin is allowed if either allow_origin or allow_origin_regex match.                       |         |
| `allowOriginRegex` | `[]string` | Specifies regex patterns that match origins that will be allowed to make CORS requests. An origin is allowed if either allow_origin or allow_origin_regex match. |         |
| `allowMethods`     | `[]string` | Specifies the content for the *access-control-allow-methods* header.                                                                                             |         |
| `allowHeaders`     | `[]string` | Specifies the content for the *access-control-allow-headers* header.                                                                                             |         |
| `exposeHeaders`    | `[]string` | Specifies the content for the *access-control-expose-headers* header.                                                                                            |         |
| `maxAge`           | `string`   | Specifies the content for the *access-control-max-age* header.                                                                                                   |         |
| `allowCredentials` | `bool`     | Specifies whether the resource allows credentials.                                                                                                               |         |


#### Regex Grammar

Note that Gloo uses [ECMAScript](https://en.cppreference.com/w/cpp/regex/ecmascript) regex grammar.

For example, in order to match all subdomains:

  - Do not use: `*.example.com`
  - Instead, use: `[a-zA-Z0-9]*.example.com`

### Example

In the example below, the virtual service, through CORS parameters, will inform your browser that it should also allow
`GET` and `POST` calls from services located on `*.supergloo.dev`. This could allow you to host scripts or
other needed resources on the `'*.supergloo.dev'`, even if your application is not being server from that location.

{{< highlight yaml "hl_lines=9-22" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: corsexample
  namespace: gloo-system
spec:
  displayName: corsexample
  virtualHost:
    corsPolicy:
      allowCredentials: true
      allowHeaders:
      - origin
      allowMethods:
      - GET
      - POST
      allowOrigin:
      - solo.io
      allowOriginRegex:
      - '[a-zA-Z0-9]*.supergloo.dev'
      exposeHeaders:
      - origin
      maxAge: 1d
    domains:
    - '*'
{{< /highlight >}}
