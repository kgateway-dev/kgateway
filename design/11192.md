# EP-11192: Unified JWT Support


* Issue: [#11192](https://github.com/kgateway-dev/kgateway/issues/11192)

## Background

[JSON Web Tokens](https://jwt.io/), or JWT for short, are a standard way to carry verifiable identity information. 
This identity can be used for authentication and provide some advantages since JWTs are a standard format and 
cryptographically signed, they can usually be verified without contacting an external authentication server. 

A JWT has three parts:
1. The header
2. The payload
3. The signature

Inside the JWT various claims are encoded; claims provide identity information. A few standard claims are:
- iss: The entity that issued the token
- sub: Subject of the token. This is usually a user id. 
- aud: The audience the token was issued for. This is an important security feature that makes sure that a token issued for one use cannot be used for other purposes.

See the [JWT specification](https://datatracker.ietf.org/doc/html/rfc7519#section-4.1) for a complete description of all standard claims.

We would like to add support for JWT in kgateway for configuring both the upstream
[Envoy JWT Authentication filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/jwt_authn_filter) 
and configuring [agentgateway JWT authentication](https://agentgateway.dev/docs/security/jwt/). 

## Motivation

JSON Web Tokens (JWT) are commonly used to offload authentication and authorization code from your apps.
- Authentication: Instead of storing user session information server-side which can lead to scalability issues, you set up a JWT issuer, such as an OpenID Connect (OIDC) or Single Sign-on (SSO) provider. After a user authenticates, the provider returns a JWT in the response that is stored client-side. Subsequent requests from the client include the JWT, which is faster to verify than performing the entire authentication and authorization process again.
- Authorization: JWTs can have custom claims that can define a user's scope, role, or other permissions. You can use these claims in combination with other policies to enforce fine-grained access control to your apps. By including the claim information within the JWT, the authorization process can happen faster and more scalably.
- Secure information exchange: Because the token is in JSON format, many otherwise incompatible systems and services can use the token to exchange information. The authentication and authorization features built into the token help these systems validate and trust the information.

### Goals
- Define a basic JWT policy to support configuration of Envoy-based proxies and Agent Gateway.
- Support defining the JWT policy at the Gateway, Route, or Listener level
- Support defining the providers and claims for Envoy and Agent Gateway. 
- You can select multiple targets together in the same policy (e.g. multiple HTTPRoutes can have one JWT policy)

### Non-Goals
- Extend the Agent Gateway to support all configuration currently supported by Envoy.
- Supporting postAuthz stage in the JWT policy. The initial implementation will only support the preAuthz stage (Because we want to support using the JWT policy for authorization, you cannot set the policy's phase to postAuthz)
- Supporting applying multiple JWT policies to the same route or listener. If multiple JWT policies select the same resource, only the oldest policy applies.


## Implementation Details

The agentgateway features for JWT include setting the local JWT based on a file path:
```yaml
policies:
  jwtAuth:
    issuer: agentgateway.dev
    audiences: [test.agentgateway.dev]
    jwks:
      # Relative to the folder the binary runs from, not the config file
      file: ./manifests/jwt/pub-key
```

Note: remove JWTs are not currently supported in agentgateway.

For authorization, agentgateway exposes the `rbac` field. This allows users to require a JWT token to authenticate requests 
and use an RBAC policy to authorize access to tools for JWT tokens that contain specific claims for MCP backends:
```yaml
    mcpAuthorization:
      rules:
        # Allow anyone to call 'echo'
        - 'mcp.tool.name == "echo"'
        # Only the test-user can call 'add'
        - 'jwt.sub == "test-user" && mcp.tool.name == "add"'
        # Any authenticated user with the claim `nested.key == value` can access 'printEnv'
        - 'mcp.tool.name == "printEnv" && jwt.nested.key == "value"'
```

All rbac rules in agentgateway are written as a list of [CEL](https://cel.dev/) rules. 

Envoy supports local jwks (fetch JWKS in local data source, such as a local file or embedded inline string), and 
remote jwks (fetch JWKS from a remote HTTP server, also specify cache duration).

Envoy also supports additional JWT configurations which agentgateway currently does not support, but may be expanded in the future:
- forward: if true, JWT will be forwarded to the upstream.
- from_headers: extract JWT from HTTP headers.
- from_params: extract JWT from query parameters.
- from_cookies: extract JWT from HTTP request cookies.
- forward_payload_header: forward the JWT payload in the specified HTTP header.
- claim_to_headers: copy JWT claim to HTTP header.
- jwt_cache_config: Enables JWT cache, its size can be specified by jwt_cache_size. Only valid JWTs are cached.
- clear_route_cache: Clears route cache in order to allow the JWT to correctly affect routing decisions.
- allow_missing/allow_missing_or_failed: Configures the behavior if the JWT is missing or the JWT verification fails  
- requires_all/requires_any: Configures the behavior of JWT requirements being AND'd/OR'd together. 

The proposal is to implement two new fields for JWT Authentication and RBAC Authorization on the existing TrafficPolicy,
along with a new GatewayExtension type for defining JWT Providers. 

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayExtension
metadata:
  name: basic-jwt-provider
spec:
  type: JWTProviders
  jwtProviders:
      my-example:
        claimsToHeaders:
          - name: org
            header: x-org
          - name: email
            header: x-email
        issuer: https://my.example.com
        jwks:
          local:
            key: |
              -----BEGIN PUBLIC KEY-----
              mykey
              -----END PUBLIC KEY-----
---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: route-test
  namespace: gwtest
spec:
  targetRefs:
    - kind: HTTPRoute
      group: gateway.networking.k8s.io
      name: route
  jwt:
    extensionRef:
      name: basic-jwt-provider
  rbac:
    action: Allow
    policy:
     matchExpressions:
      - "metadata.filter_metadata['envoy.filters.http.jwt_authn']['payload']['test'] == 'foo'"
      - "metadata.filter_metadata['envoy.filters.http.jwt_authn']['payload']['groups'].contains('group1')" 
```

Where the rbac rules are defined via flexible CEL expressions and can be expanded in the future to support other formats. 

#### Guidance on Grouping Multiple JWT Providers

The jwtProviders are a map of JWT provider names to provider. Users may have large numbers of JWT Providers, so having one JWT Provider per GatewayExtension
is bad UX since it forces the user to create a separate object for each provider.

When configuring JWT providers via the GatewayExtension resource, users have the option of placing multiple providers into a single GatewayExtension object, or defining each provider in its own GatewayExtension.

Grouping providers is recommended when:
- The providers are logically related (e.g., dev/test/CI providers from the same identity provider).
- You want to simplify policy management by referencing a single GatewayExtension from multiple TrafficPolicy resources.
- You want to reduce resource sprawl and centralize management of shared JWT configurations.

Grouping providers allows you to define a single GatewayExtension with a jwtProviders map:

```yaml
spec:
jwtProviders:
  dev-auth0:
    issuer: https://dev.example.com
    ...
  prod-auth0:
    issuer: https://prod.example.com
    ...
```

Separate GatewayExtension providers is recommended when:
- Providers are owned by different teams or have different lifecycle management.
- You want to restrict RBAC access differently for each provider (e.g., least privilege access to secret keys or token sources).
- Providers are used in isolation and unlikely to be shared across TrafficPolicies.

Placing many providers in a single GatewayExtension does not introduce performance overhead at the policy evaluation level.
Internally, all providers are translated into Envoy or agentgateway configurations in the TrafficPolicy plugin 
regardless of how they are grouped in the API.

However, very large single resources (e.g., hundreds of providers in one GatewayExtension) may make debugging and 
policy management more difficult, so it is important to consider the logical grouping for providers when writing the TrafficPolicy.

### Configuration
This feature will require a new field on the TrafficPolicy API to configure the JWT provider and RBAC. No new helm-level or GatewayParameters config is required.

### Plugin
This feature will be implemented via the existing plugin framework in kgateway. The existing traffic policy plugin and 
new jwt plugin on the agentgateway will be responsible for:
- Translate JWT provider configuration into the agentgateway and Envoy proxy configuration
- Translate the RBAC rules into the agentgateway and Envoy proxy configuration

The plugin will hook into the PreAuth stage in Envoy.

### Controllers
A new controller will be created to manage the new JWT providers and RBAC policies.
This controller will require additional RBAC for the new policies added (get, list, watch, update for statuses)

### Deployer
No deployer changes are required, but the crd helm chart will need to be updated to include the new JWT policies. 

### Translator and Proxy Syncer
No changes should be required to the translator and proxy syncer. The new policies should be handled by the plugin system
for the Envoy proxy and agentgateway proxy.

### Reporting
Status reporting will be added to the status field of the JWT policy:
- Accepted: The JWT policy has been successfully translated by the plugin
- Warning: The JWT policy has been translated, but there are some conditions that need to be addressed
- Error: The JWT policy has failed to translate

### Test Plan

Unit tests:
- Plugin level unit tests for the Envoy translation
- Plugin level unit tests for the agentgateway translation
- Setup level unit tests for the JWT policies for the Envoy proxy
- Setup level unit tests for the JWT policies when agentgatewayInterface is enabled

E2E Tests:
- E2E tests for the Envoy proxy with JWT Authentication at the Gateway and HTTPRoute level
- E2E tests for the Envoy proxy with JWT Authorization at the Gateway and HTTPRoute level
- E2E tests for the agentgateway proxy with JWT Authentication at the Gateway (MCP listener and A2A listener) and Tool level

## Alternatives

## Option one: Inline JWT Providers in TrafficPolicy

Instead of using the GatewayExtensions, we could inline the JWT Providers:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: jwt-policy
spec:
  jwt:
    providers:
      provider1:
        claimsToHeaders:
              - name: org
                header: x-org
              - name: email
                header: x-email
        issuer: https://dev.example.com
        jwks:
              local:
                key: mykey1
      provider2:
        claimsToHeaders:
          - name: org
            header: x-org
        issuer: https://dev.otherexample.com
        jwks:
          local:
            key: mykey2
```

However, the GatewayExtension approach enables users to have JWTs enabled on many TrafficPolicy without duplicate 
providers, versus this approach would require all TrafficPolicies to inline the Providers, potentially duplicating config.

## Option two: Separate policy for JWT Authentication and JWT Authorization

 The JWT Authentication policy would map of provider name to provider configuration on the TrafficPolicy that could target
a specific route rule, the entire HTTPRoute or Gateway. For example the same self-minited example on the HTTPRoute level:
```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: "jwt-example"
  namespace: foo
spec:
   targetRefs:
   - kind: HTTPRoute
     group: gateway.networking.k8s.io
     name: route
   jwt:
     providers:
     selfminted: # map of provider name to provider configuration
       issuer: kgateway.io
       jwks:
         local:
           key: |
            -----BEGIN PUBLIC KEY-----
            MyCoolKey
            -----END PUBLIC KEY-----
```

Provider example using auth0:
```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: "jwt-example"
  namespace: foo
spec:
  jwt:
    providers: 
        auth0: # map of provider name to provider configuration
          issuer: https://dev-1234567.us.auth0.com/
          tokenSource:
            headers:
              - header: jwt
          claimsToHeaders:
            - claim: email
              header: x-my-claim-email
          jwks:
            remote:
              url: https://dev-1234567.us.auth0.com/.well-known/jwks.json
              targetRef:
                name: auth0-jwks-backend
                namespace: test
                kind: Backend
```

Then the JWT Authorization policy would be configured separately in the rbac field:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: "jwt-example"
  namespace: foo
spec:
  targetRefs:
    - kind: HTTPRoute
      group: gateway.networking.k8s.io
      name: route
  rbac:
    policies:
      viewer:
        principals:
          - jwtPrincipal:
              claims:
                team: dev
  jwt:
    providers: 
        auth0: # map of provider name to provider configuration
          issuer: https://dev-1234567.us.auth0.com/
          tokenSource:
            headers:
              - header: jwt
          claimsToHeaders:
            - claim: email
              header: x-my-claim-email
          jwks:
            remote:
              url: https://dev-1234567.us.auth0.com/.well-known/jwks.json
              targetRef:
                name: auth0-jwks-backend
                namespace: test
                kind: Backend
```

Initially for agentgateway, only the HTTPListenerPolicy will be supported since there are no agentgateway routes.

Open Questions:
- agentgateway listeners are kgateway.dev/mcp or kgateway.dev/a2a. Is it confusing to put the JWT policy on the HTTPListenerPolicy?

## Option three: RequestAuthentication policy defines the jwtRules as list

Another option is to have a standalone JWT policy that defines the JWT providers. This approach follows the Istio JWT policy style
on the `RequestAuthentication` resource that uses `jwtRules` to define the JWT providers:

Istio JWT policy style:
```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: RequestAuthentication
metadata:
  name: "jwt-example"
  namespace: foo
spec:
  targetRefs:
  - name: mcp-gateway
    group: gateway.networking.k8s.io
    kind: Gateway
  jwtRules:
  - issuer: "testing@secure.kgateway.io"
    jwksUri: "https://raw.githubusercontent.com/istio/istio/release-1.25/security/tools/jwt/samples/jwks.json"
    outputClaimToHeaders:
      - header: "x-jwt-claim-foo"
        claim: "foo"
```

However, this would still require a separate JWT policy for authorization, similar to how Istio configures the `AuthorizationPolicy`
as part of the rules in the [Istio docs](https://istio.io/latest/docs/reference/config/security/authorization-policy/).

Istio-style AuthorizationPolicy:
```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: AuthorizationPolicy
metadata:
  name: "jwt-example"
  namespace: foo
spec:
  targetRefs:
  - name: mcp-gateway
    group: gateway.networking.k8s.io
    kind: Gateway
  action: Allow
  rules:
    - from:
        - source:
            principals: ["cluster.local/ns/default/sa/sleep"]
        - source:
            namespaces: ["test"]
      to:
        - operation:
            methods: ["GET"]
            paths: ["/info*"]
        - operation:
            methods: ["POST"]
            paths: ["/data"]
      when:
        - key: request.auth.claims[iss]
          values: ["https://accounts.google.com"]
```

One benefit of this approach is you can define the JWT providers separately from the rbac authorization policies. This
allows you to reuse the JWT providers across multiple policies, while still having fine-grained control over the
authorization policies.

In the agentgateway case, the JWT providers are defined targeting a `Gateway` resource that uses the agentgateway GatewayClass.
The rbac policy can be applied to a `Gateway` or `Tool` resource:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: AuthorizationPolicy
metadata:
  name: "agentgateway-example"
  namespace: foo
spec:
  targetRefs:
  - name: mcp-tool
    group: gateway.kgateway.dev
    kind: Tool
  action: Allow
  rules:
    - when:
        - key: request.auth.claims[iss]
          values: ["https://accounts.google.com"]
```

Alternatively to the Istio-style JWT AuthenticationRequest and AuthorizationPolicy, we could still have the
separate RequestAuthentication and AuthorizationPolicy, but use the map to define providers and rbac:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: RequestAuthentication
metadata:
  name: "agentgateway-example"
  namespace: foo
spec:
  targetRefs:
  - name: mcp-tool
    group: gateway.kgateway.dev
    kind: Tool
  jwt:
    providers:
      dev-example: # map of provider names to provider configuration
        issuer: https://dev.example.com
        local:
          inline: |-
            -----BEGIN PUBLIC KEY-----
            MyCoolKey
            -----END PUBLIC KEY-----
---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: AuthorizationPolicy
metadata:
  name: "agentgateway-example"
  namespace: foo
spec:
  targetRefs:
  - name: mcp-tool
    group: gateway.kgateway.dev
    kind: Tool
  action: Allow
  rbac:
    policies:
      dev-user: # map of policy names to policy config
        principals:
          - jwtPrincipal:
              claims:
                team: dev
```

This would simplify the rule definitions, replacing the complex `to`/`from`/`when` logic which agentgateway would not
initially support.

## Option four: Combine JWT Authentication and JWT Authorization in one policy

We can use a similar approach to create a generic JWT policy that can be applied at the Gateway or RouteLevel separate
from the TrafficPolicy or HTTPListenerPolicy.

This would be an additional resource, but would allow more flexibility in adding JWT functionality without changing
the underlying existing kgateway APIs.

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: JWTPolicy
metadata:
  name: jwt-policy
  namespace: httpbin
spec:
  targetRefs:
  - name: mcp-gateway
    group: gateway.networking.k8s.io
    kind: Gateway
  config:
    claims:
    - key: "name"
      values:
      - "kgateway"
    - key: "email"
      values:
      - "user2@kgateway.io"
    - key: "org"
      values:
      - "internal"
      - "partner"
    providers:
      dev-example:
        issuer: https://dev.example.com
        local:
          inline: |-
            -----BEGIN PUBLIC KEY-----
            MyCoolKey
            -----END PUBLIC KEY-----
```

For agentgateway tools, the JWT policy would be applied to the tools themselves via the `targetRefs` field:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: JWTPolicy
metadata:
  name: jwt-policy
  namespace: httpbin
spec:
  targetRefs:
  - name: mcp-tool # what if the tool is not in the same namespace as the policy?
    group: gateway.kgateway.dev
    kind: Tool
  config:
    claims:
    - key: "name"
      values:
      - "kgateway"
    - key: "email"
      values:
      - "user2@kgateway.io"
    - key: "org"
      values:
      - "internal"
      - "partner"
    providers:
      dev-example:
        issuer: https://dev.example.com
        local:
          inline: |-
            -----BEGIN PUBLIC KEY-----
            MyCoolKey
            -----END PUBLIC KEY-----
```

The downside of this approach is you cannot reuse the JWT providers across multiple policies (e.g. tools might use the same
provider but check different claims).

## Open Questions
- Q. Naming of policies? Should these be JWT-specific or more generic in the Istio-style (RequestAuthentication, AuthorizationPolicy)?

A. Having the `jwt` and `rbac` fields on the TrafficPolicy simplifies this configuration. 

- Q. Is defining providers in one policy and rbac rules in another policy useful? Or does it introduce additional overhead for users having to configure both?

A. If the TrafficPolicy has both the `jwt` and `rbac` fields, the user has flexibility to define a standalone JWT policy or combine the JWT and RBAC policies.

- Q. Should the targetRefs be local to the namespace or apply to all namespaces that the Gateway has access to?

A. `targetRefs` should follow the resource grant pattern in the rest of the Kubernetes Gateway API. 