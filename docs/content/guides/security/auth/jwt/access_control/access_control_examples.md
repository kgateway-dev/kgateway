---
title: Examples
weight: 2
description: Additional examples of JWT verification and Access Control (without an external auth server)
---

{{% notice note %}}
The JWT feature was introduced with **Gloo Edge Enterprise**, release 0.13.16. If you are using an earlier version, this tutorial will not work.
{{% /notice %}}

## Table of Contents
- [Setup](#setup)
- [Matching against nested JWT claims](#matching-against-nested-jwt-claims)
  - [Sample JWT](#sample-jwt-nested-claims)
  - [Virtual Service](#virtual-service-nested-claims)
- [Matching against non-string JWT claim values](#matching-against-non-string-jwt-claims)
  - [Matching boolean values](#matching-boolean-values)
  - [Matching list values](#matching-list-values)

## Setup
Before you begin, set up basic JWT authorization and configure a Virtual Service to verify JWTs by following the steps in [JWT and Access Control]({{% versioned_link_path fromRoot="/guides/security/auth/jwt/access_control/" %}}).

## Matching against nested JWT claims

By default, matching is supported for only top-level claims of the JWT.
To additionally enable matching against nested claims, or claims that are children of top-level claims, you must specify a `nestedClaimDelimiter`, such as `.`, in the RBAC [policy]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/rbac/rbac.proto.sk/#policy" %}}),
and specify the claim name as a path, such as `parent.child.foo: user`, in the `claims` field of the [`jwtPrincipal`]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/rbac/rbac.proto.sk/#jwtprincipal" %}}).

### Sample JWT (nested claims)

Consider an example JWT with the following claims:

```json
{
  "iss": "kubernetes/serviceaccount",
  "sub": "1234567890",
  "iat": 1516239022,
  "metadata": {
    "auth": {
      "role": "user"
    }
  }
}
```


### Virtual Service (nested claims)

To ensure that GET requests to the `/api/pets` endpoint are permitted only to users that have a JWT with the `role`
claim set to `user`, configure the Virtual Service with the following RBAC policy:  

{{< highlight shell "hl_lines=40 48" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petstore
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
          kube:
            ref:
              name: petstore
              namespace: default
            port: 8080
    options:
      jwt:
        providers:
          kube:
            issuer: kubernetes/serviceaccount
            jwks:
              local:
                key: |
                  -----BEGIN PUBLIC KEY-----
                  MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEApj2ac/hNZLm/66yoDQJ2
                  mNtQPX+3RXcTMhLnChtFEsvpDhoMlS0Gakqkmg78OGWs7U4f6m1nD/Jc7UThxxks
                  o9x676sxxLKxo8TC1w6t47HQHucJE0O5wFNtC8+4jwl4zOBVwnkAEeN+X9jJq2E7
                  AZ+K6hUycOkWo8ZtZx4rm1bnlDykOa9VCuG3MCKXNexujLIixHOeEOylp7wNedSZ
                  4Wfc5rM9Cich2F6pIoCwslHYcED+3FZ1ZmQ07h1GG7Aaak4N4XVeJLsDuO88eVkv
                  FHlGdkW6zSj9HCz10XkSPK7LENbgHxyP6Foqw10MANFBMDQpZfNUHVPSo8IaI+Ot
                  xQIDAQAB
                  -----END PUBLIC KEY-----
      rbac:
        policies:
          viewer:
            nestedClaimDelimiter: .
            permissions:
              methods:
              - GET
              pathPrefix: /api/pets
            principals:
            - jwtPrincipal:
                claims:
                  metadata.auth.role: user
{{< /highlight >}}

## Matching against non-string JWT claim values

By default, claims are matched against values by using exact string comparison. To instead match claims against non-string values, you must specify a [ClaimMatcher]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/rbac/rbac.proto.sk/#claimmatcher" %}})
in the `matcher` field of the [`jwtPrincipal`]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/rbac/rbac.proto.sk/#jwtprincipal" %}}).

### Matching boolean values

**Sample JWT (boolean)**

Consider an example JWT with the following claims:
```json
{
  "iss": "kubernetes/serviceaccount",
  "sub": "1234567890",
  "iat": 1516239022,
  "email_verified": true
}
```

**Virtual Service (boolean)**

To ensure that GET requests to the `/api/pets` endpoint are permitted only to users that have a JWT with the `email_verified`
claim set to `true`, configure the Virtual Service with the following RBAC policy: 

{{< highlight shell "hl_lines=47-49" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petstore
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
          kube:
            ref:
              name: petstore
              namespace: default
            port: 8080
    options:
      jwt:
        providers:
          kube:
            issuer: kubernetes/serviceaccount
            jwks:
              local:
                key: |
                  -----BEGIN PUBLIC KEY-----
                  MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEApj2ac/hNZLm/66yoDQJ2
                  mNtQPX+3RXcTMhLnChtFEsvpDhoMlS0Gakqkmg78OGWs7U4f6m1nD/Jc7UThxxks
                  o9x676sxxLKxo8TC1w6t47HQHucJE0O5wFNtC8+4jwl4zOBVwnkAEeN+X9jJq2E7
                  AZ+K6hUycOkWo8ZtZx4rm1bnlDykOa9VCuG3MCKXNexujLIixHOeEOylp7wNedSZ
                  4Wfc5rM9Cich2F6pIoCwslHYcED+3FZ1ZmQ07h1GG7Aaak4N4XVeJLsDuO88eVkv
                  FHlGdkW6zSj9HCz10XkSPK7LENbgHxyP6Foqw10MANFBMDQpZfNUHVPSo8IaI+Ot
                  xQIDAQAB
                  -----END PUBLIC KEY-----
      rbac:
        policies:
          viewer:
            permissions:
              methods:
              - GET
              pathPrefix: /api/pets
            principals:
            - jwtPrincipal:
                claims:
                  email_verified: true
                matcher: BOOLEAN
{{< /highlight >}}

### Matching list values

**Sample JWT (list)**

Consider an example JWT with the following claims:
```json
{
  "iss": "kubernetes/serviceaccount",
  "sub": "1234567890",
  "iat": 1516239022,
  "roles": [
    "super_user",
    "manage-account",
    "manage-account-links",
    "view-profile"
  ]
}
```

**Virtual Service (list)**

To ensure that GET requests to the `/api/pets` endpoint are permitted only to users that have a JWT with the `roles`
claim that contains `super_user` within its list, configure the Virtual Service with the following RBAC policy:

{{< highlight shell "hl_lines=47-49" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petstore
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
          kube:
            ref:
              name: petstore
              namespace: default
            port: 8080
    options:
      jwt:
        providers:
          kube:
            issuer: kubernetes/serviceaccount
            jwks:
              local:
                key: |
                  -----BEGIN PUBLIC KEY-----
                  MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEApj2ac/hNZLm/66yoDQJ2
                  mNtQPX+3RXcTMhLnChtFEsvpDhoMlS0Gakqkmg78OGWs7U4f6m1nD/Jc7UThxxks
                  o9x676sxxLKxo8TC1w6t47HQHucJE0O5wFNtC8+4jwl4zOBVwnkAEeN+X9jJq2E7
                  AZ+K6hUycOkWo8ZtZx4rm1bnlDykOa9VCuG3MCKXNexujLIixHOeEOylp7wNedSZ
                  4Wfc5rM9Cich2F6pIoCwslHYcED+3FZ1ZmQ07h1GG7Aaak4N4XVeJLsDuO88eVkv
                  FHlGdkW6zSj9HCz10XkSPK7LENbgHxyP6Foqw10MANFBMDQpZfNUHVPSo8IaI+Ot
                  xQIDAQAB
                  -----END PUBLIC KEY-----
      rbac:
        policies:
          viewer:
            permissions:
              methods:
              - GET
              pathPrefix: /api/pets
            principals:
            - jwtPrincipal:
                claims:
                  roles: super_user
                matcher: LIST_CONTAINS
{{< /highlight >}}
