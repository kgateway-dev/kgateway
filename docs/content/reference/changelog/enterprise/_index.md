---
title: Gloo Edge Enterprise
weight: 8
description: Changelogs for Gloo Edge Enterprise
---


### v1.8.0

#### [v1.8.0-beta1](https://github.com/solo-io/solo-projects/releases/tag/v1.8.0-beta1) (Uses OSS [v1.8.0-beta2](https://github.com/solo-io/gloo/releases/tag/v1.8.0-beta2))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.8.0-beta2.
- solo-io/go-utils has been upgraded to v0.21.3.
- solo-io/ext-auth-service has been upgraded to v0.14.0.
- solo-io/ext-auth-service has been upgraded to v0.15.0.

##### Helm Changes
- Remove duplicate values from value-template that can be solved without codegen changes. (https://github.com/solo-io/gloo/issues/3470)

##### New Features
- Add a new `user_id_attribute_name` attribute to the `AccessTokenValidation` API through which users can optionally select which attribute in an OAuth2.0 token introspection response contains the ID of the resource owner. The external auth server can then emit the user ID either as a header, as dynamic metadata, or both. (https://github.com/solo-io/gloo/issues/4505)
- Allow the user to define behaviors for when a token is provided with a key ID that is not contained in the local JWKS cache. (https://github.com/solo-io/gloo/issues/4507)

<details><summary><b>v1.7.0</b></summary>

#### [v1.7.0](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0) (Uses OSS [v1.7.0](https://github.com/solo-io/gloo/releases/tag/v1.7.0))

##### Breaking Changes
- (From OSS [v1.7.0-rc2](https://github.com/solo-io/gloo/releases/tag/v1.7.0-rc2)) Removes gloo UI with-admin-console flag for gloo installation in favor of new gloo-fed UI. (https://github.com/solo-io/gloo/issues/4267)

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.7.0.
- solo-io/solo-apis has been upgraded to gloo-v1.7.0.
- solo-io/go-utils has been upgraded to v0.21.3.
- solo-io/ext-auth-service has been upgraded to v0.14.1.

##### Fixes
- Allow the user to define behaviors for when a token is provided with a key ID that is not contained in the local JWKS cache. (https://github.com/solo-io/gloo/issues/4507)
- (From OSS [v1.7.0](https://github.com/solo-io/gloo/releases/tag/v1.7.0)) Fix allowWarnings setting so it is honored when set to false (the default) in configuration validation API and admission webhook. (https://github.com/solo-io/gloo/issues/4466)
- (From OSS [v1.7.0](https://github.com/solo-io/gloo/releases/tag/v1.7.0)) Uninstall gloo federation when the --all flag is specified. (https://github.com/solo-io/gloo/issues/4502)
- (From OSS [v1.7.0](https://github.com/solo-io/gloo/releases/tag/v1.7.0)) Fix noisy error logs on kubernetes EDS control loop, which will help with log noise and disk space. (https://github.com/solo-io/gloo/issues/3761)

##### Helm Changes
- (From OSS [v1.7.0](https://github.com/solo-io/gloo/releases/tag/v1.7.0)) Allow tracing to be configured via helm on the default gateway. (https://github.com/solo-io/gloo/issues/4494)

##### New Features
- Add a new `user_id_attribute_name` attribute to the `AccessTokenValidation` API through which users can optionally select which attribute in an OAuth2.0 token introspection response contains the ID of the resource owner. The external auth server can then emit the user ID either as a header, as dynamic metadata, or both. (https://github.com/solo-io/gloo/issues/4505)

#### [v1.7.0-rc2](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-rc2) (Uses OSS [v1.7.0-rc1](https://github.com/solo-io/gloo/releases/tag/v1.7.0-rc1))

##### Helm Changes
- (From OSS [v1.7.0-rc1](https://github.com/solo-io/gloo/releases/tag/v1.7.0-rc1)) Add support for a new helm value (`gatewayProxies.gatewayProxy.customStaticLayer`) that allows the customization of envoy's static layer bootstrap yaml. (https://github.com/solo-io/gloo/issues/4327)

##### New Features
- (From OSS [v1.7.0-rc1](https://github.com/solo-io/gloo/releases/tag/v1.7.0-rc1)) Add `oneWayTls` boolean configuration to the `SslConfig` (referenced on `VirtualService`s) to allow users to configure TLS termination to use one-way TLS rather than mTLS even if the root CA is provided (e.g., by default with TLS secrets from cert-manager). (https://github.com/solo-io/gloo/issues/4254)

##### Notes
- This release contained no user-facing changes.
</details>

#### [v1.7.0-rc1](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-rc1)

##### Breaking Changes
- Removes old gloo UI grpcserver helm files in favor of new gloo-fed apiserver. (https://github.com/solo-io/gloo/issues/4267)

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.7.0-rc1.
- linux/alpine has been upgraded to v3.12.1.

##### New Features
- Introduce a readinessProbe on the ext auth deployment, ensuring that the extauth pod is not marked as ready until it has received Gloo configuration. We had previously relied on envoy health checks, so this protects agains the edge case where k8s terminates a pod, and we dont want to direct traffic to the new one, until it has received configuration. (https://github.com/solo-io/gloo/issues/2549)

##### Notes
- *This release build failed.*

#### [v1.7.0-beta15](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta15) (Uses OSS [v1.7.0-beta32](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta32))

##### Breaking Changes
- Removes old gloo UI grpcserver in favor of new gloo-fed apiserver. (https://github.com/solo-io/gloo/issues/4267)
- Removes old gloo UI code in favor of new gloo-fed UI. (https://github.com/solo-io/gloo/issues/4267)

##### Dependency Bumps
- solo-io/rate-limiter has been upgraded to v0.3.1.
- solo-io/rate-limiter has been upgraded to v0.3.2.
- solo-io/gloo has been upgraded to v1.7.0-beta32.
- solo-io/solo-apis has been upgraded to gloo-v1.7.0-beta32.
- solo-io/ext-auth-service has been upgraded to v0.13.0.
- solo-io/go-utils has been upgraded to v0.21.0.
- solo-io/k8s-utils has been upgraded to v0.0.7.
- (From OSS [v1.7.0-beta31](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta31)) linux/alpine has been upgraded to v3.13.2.
- (From OSS [v1.7.0-beta30](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta30)) solo-io/k8s-utils has been upgraded to v0.0.7.
- (From OSS [v1.7.0-beta30](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta30)) solo-io/go-utils has been upgraded to v0.21.0.

##### Fixes
- Allow set-style rules to omit rate limits, similar to envoy style API. (https://github.com/solo-io/gloo/issues/4279)
- Expose a discovery_poll_interval which controls interval at which OIDC configuration is discovered at <issuerUrl>/.well-known/openid-configuration. The default value is 30 minutes. (https://github.com/solo-io/gloo/issues/4470)
- Fix possible cache key collisions, by changing the way the cache key is generated for rate limit rules and requests in redis/dynamodb. A side affect of this change is that upgrading this will _change_ the cache keys under the covers, thus any long rate limits (i.e. per day / hour) will be effectively _reset_ upon upgrade. Further, a couple characters are now disallowed in rate limit rules, namely the pipe character, back tic, and caret. (https://github.com/solo-io/gloo/issues/3801)

##### Helm Changes
- Define helm partial to inject enterprise-only settings in Open source settings manifest. Due to the nature of helm chart scoping, all values used in the partial must either already exist in the open source template, or be in `.Values.global`. (https://github.com/solo-io/gloo/issues/3470)
- Removes entries in the values-template.yaml file that are unchanged duplicates of values in gloo OS's template. Not all duplicatee values can be removed easily, but these can be removed without changing the resulting Enterprise manifest. (https://github.com/solo-io/gloo/issues/3470)
- (From OSS [v1.7.0-beta31](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta31)) Expose the `regexMaxProgramSize` settings field as a Helm chart value. (https://github.com/solo-io/gloo/issues/4419)

#### [v1.7.0-beta14](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta14) (Uses OSS [v1.7.0-beta29](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta29))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.12.0.
- solo-io/solo-kit has been upgraded to v0.18.2.
- solo-io/solo-apis has been upgraded to gloo-v1.7.0-beta29.
- solo-io/gloo has been upgraded to v1.7.0-beta29.
- (From OSS [v1.7.0-beta29](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta29)) solo-io/solo-kit has been upgraded to v0.18.2.

##### Fixes
- (From OSS [v1.7.0-beta28](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta28)) EDS updates will work even when previous envoy snapshot is missing. (https://github.com/solo-io/gloo/issues/4345)
- (From OSS [v1.7.0-beta27](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta27)) Fixes docs issue where clicking the "Copy" button next to code blocks causes unintentional scrolling behaviour. (https://github.com/solo-io/gloo/issues/4413)
- (From OSS [v1.7.0-beta27](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta27)) It was possible to install mTLS, enable it on upstreams, remove mTLS and the configuration still be defined on upstreams. This caused the data plane to become out of sync, since envoy clusters would be configured to get their secrets from a non-existent cluster. Add protection to the `glooctl istio uninject` command to prevent users from unknowingly causing the data plane to become out of sync. (https://github.com/solo-io/gloo/issues/4390)

##### Helm Changes
- (From OSS [v1.7.0-beta27](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta27)) Add partial to be defined in enterprise to allow enterprise-only settings in Open source manifest, without leaking enterprise structs/datatypes. (https://github.com/solo-io/gloo/issues/3470)

##### New Features
- Allow setting basic authentication credentials in OAuth2.0 introspection requests. This enables integration with introspection servers which require clients to be authenticated. Credentials can be provided by using the new IntrospectionValidation fields on the OAuth2 AccessTokenValidation message. Refer to https://docs.solo.io/gloo-edge/master/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/extauth/v1/extauth.proto.sk/#introspectionvalidation for more details. (https://github.com/solo-io/gloo/issues/4418)

#### [v1.7.0-beta13](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta13) (Uses OSS [v1.7.0-beta26](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta26))

##### Notes
- This release contained no user-facing changes.

#### [v1.7.0-beta12](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta12) (Uses OSS [v1.7.0-beta26](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta26))

##### Dependency Bumps
- solo-io/envoy-gloo-ee has been upgraded to v1.18.0-rc3.
- solo-io/ext-auth-service has been upgraded to v0.10.3.
- solo-io/ext-auth-service has been upgraded to v0.11.2.
- solo-io/ext-auth-service has been upgraded to v0.11.3.
- solo-io/gloo has been upgraded to v1.7.0-beta25.
- solo-io/solo-apis has been upgraded to v0.0.0-20210301203230-7f9c5f2a7536.
- solo-io/gloo has been upgraded to v1.7.0-beta24.
- solo-io/ext-auth-service has been upgraded to v0.11.1.
- solo-io/gloo has been upgraded to v1.7.0-beta23.
- solo-io/solo-apis has been upgraded to gloo-v1.7.0-beta23.
- solo-io/solo-kit has been upgraded to v0.18.0.
- solo-io/ext-auth-service has been upgraded to v0.10.2.
- (From OSS [v1.7.0-beta26](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta26)) solo-io/go-utils has been upgraded to v0.20.5.
- (From OSS [v1.7.0-beta26](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta26)) solo-io/envoy-gloo has been upgraded to 1.18.0-rc2.

##### Fixes
- Unescape the path that is included in the OIDC state (https://github.com/solo-io/gloo/issues/4339)
- Fix grpc healthchecks to hit the right grpc service name in extauth service. (https://github.com/solo-io/gloo/issues/4324)
- Redirect to the default app url upon OIDC logouts (https://github.com/solo-io/gloo/issues/4271)
- Re-enable REST EDS by default to avoid bug in upstream envoy with cluster updates from gRPC management plane. (https://github.com/solo-io/gloo/issues/4151)
- (From OSS [v1.7.0-beta26](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta26)) Improve CPU by using newer marshal / unmarshal functions in protov2 in the REST EDS server. (https://github.com/solo-io/gloo/issues/4343)
- (From OSS [v1.7.0-beta22](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta22)) Support using settings.UpstreamOptions on upstreams that define one-way tls (https://github.com/solo-io/gloo/issues/4285)
- (From OSS [v1.7.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta21)) Deep merge default gateway proxy values into the proxy templates (https://github.com/solo-io/gloo/issues/3142)
- (From OSS [v1.7.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta21)) Fix helm values mapping and test for .Values.settings.InvalidConfigPolicy (https://github.com/solo-io/gloo/issues/4321)

##### Helm Changes
- Bump Helm's Prometheus dependency up to version 11.16.9 (https://github.com/solo-io/gloo/issues/4289)
- (From OSS [v1.7.0-beta26](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta26)) Added LOG_LEVEL environment variables to deployments templates (https://github.com/solo-io/gloo/issues/4090)
- (From OSS [v1.7.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta21)) Added HorizontalPodAutoscaler helm values for the gateway-proxy. (https://github.com/solo-io/gloo/issues/2229)
- (From OSS [v1.7.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta21)) Expose the imagePullSecret in our 2 kubernetes jobs (https://github.com/solo-io/gloo/issues/4262)
- (From OSS [v1.7.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta21)) Added PodDisruptionBudget helm values for the gateway-proxy. (https://github.com/solo-io/gloo/issues/2229)

##### New Features
- Config can be passed in to Passthrough External Authentication through the `config` block in AuthConfig. This config data is now available under [FilterMetadata](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/base.proto#config-core-v3-metadata) under the key `solo.auth.passthrough.config`. (https://github.com/solo-io/gloo/issues/4293)
- (From OSS [v1.7.0-beta26](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta26)) Adds support for HTTP Connect upstreams by setting tunneling config on the upstream. This can be used to route to other proxies (such as ones in a DMZ). (https://github.com/solo-io/gloo/issues/3664)
- (From OSS [v1.7.0-beta26](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta26)) Adds a `init-plugin-manger` to bootstrap the plugin manager. (https://github.com/solo-io/gloo/issues/4306)
- (From OSS [v1.7.0-beta25](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta25)) Install gloo-fed along with gloo-enterprise (https://github.com/solo-io/gloo/issues/4267)
- (From OSS [v1.7.0-beta24](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta24)) Config can be passed in to the passthrough auth grpc service through AuthConfig. (https://github.com/solo-io/gloo/issues/4293)
- (From OSS [v1.7.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta21)) Added istioMetaMeshId and istioMetaClusterId helm values for the gateway-proxy as well as glooctl. (https://github.com/solo-io/gloo/issues/4325)

##### Pre-release
- (From OSS [v1.7.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta21)) This is a release due to the build-bot failing to start the release. Changes will be in v1.7.0-beta22 and up.

#### [v1.7.0-beta11](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta11) (Uses OSS [v1.7.0-beta18](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta18))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.10.1.
- solo-io/rate-limiter has been upgraded to v0.2.5.
- solo-io/solo-apis has been upgraded to gloo-v1.7.0-beta18.

#### [v1.7.0-beta10](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta10) (Uses OSS [v1.7.0-beta18](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta18))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.7.0-beta18.
- solo-io/go-list-licenses has been upgraded to v0.1.3.

##### Fixes
- Fix ratelimit and extauth logs to include correct gloo version (https://github.com/solo-io/solo-projects/issues/2091)
- Fix per value rate-limits in the set-style API. (i.e., when omitting the optional value from a simple descriptor, create a rate limit for each unique value instead of having the unique values share the same limit). (https://github.com/solo-io/gloo/issues/4257)

##### Helm Changes
- (From OSS [v1.7.0-beta18](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta18)) Added disableHttpGateway and disableHttpsGateway helm values for more fine grained control over gateway creation. (https://github.com/solo-io/gloo/issues/3450)
- (From OSS [v1.7.0-beta17](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta17)) Added a new accessLoggingService Helm value that allows users to define access logs from helm. (https://github.com/solo-io/gloo/issues/4096)
- (From OSS [v1.7.0-beta17](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta17)) Added a new affinity Helm value that allows users to define more fine grained affinity rules. (https://github.com/solo-io/gloo/issues/3995)

##### New Features
- Provides an enterprise-only option to use the leftmost IP address from the x-forwarded-for header and set it as the downstream address. This is useful if the network topology (load balancers, etc.) prior to gloo is unknown or dynamic. If using this option, be sure to sanitize this header from downstream requests to prevent security risks. (https://github.com/solo-io/gloo/issues/4014)
- (From OSS [v1.7.0-beta18](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta18)) Provides an option to define global SslParameters that will be applied to all upstreams by default. An individual upstream can override these properties by specifying SslParameters. (https://github.com/solo-io/gloo/issues/4285)
- (From OSS [v1.7.0-beta17](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta17)) Provides an enterprise-only option to use the leftmost IP address from the x-forwarded-for header and set it as the downstream address. This is useful if the network topology (load balancers, etc.) prior to gloo is unknown or dynamic. If using this option, be sure to sanitize this header from downstream requests to prevent security risks. (https://github.com/solo-io/gloo/issues/4014)
- (From OSS [v1.7.0-beta17](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta17)) Add new `regexRewrite` option to routes. This new field can be used to substitute matched regex patterns for alternate text in request paths, optionally including capture groups from the regex. (https://github.com/solo-io/gloo/issues/3321)

#### [v1.7.0-beta9](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta9) (Uses OSS [v1.7.0-beta16](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta16))

##### Dependency Bumps
- solo-io/k8s-utils has been upgraded to v0.0.6.
- solo-io/gloo has been upgraded to v1.7.0-beta16.
- solo-io/ext-auth-service has been upgraded to v0.10.0.
- solo-io/skv2 has been upgraded to v0.17.3.

##### Fixes
- (From OSS [v1.7.0-beta16](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta16)) Improve error message when outlier detection interval is erroneously configured as nil. (https://github.com/solo-io/gloo/issues/4217)

##### Helm Changes
- (From OSS [v1.7.0-beta16](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta16)) Added a new externalIPs Helm value that allows users to define a list of IP addresses for which nodes in the cluster will also accept traffic. (https://github.com/solo-io/gloo/issues/3791)

##### New Features
- Added glooctl fed CLI extension to make it easier to interact with federated Gloo Edge resources (e.g. federated upstreams, virtualservices, gateways). (https://github.com/solo-io/gloo/issues/4209)
- The Gloo Enterprise external auth server can now easily be configured to validate OAuth2.0 access tokens that conform to the [JSON Web Token (JWT) specification](https://tools.ietf.org/html/rfc7519) via the `AccessTokenValidation` API. Tokens are validated using a JSON Web Key Set (as defined in [Section 5 of RFC7517](https://tools.ietf.org/html/rfc7517#section-5)), which can be either inlined in the configuration or fetched from a remote location via HTTP. The server will validate both the JWT signature and the standard claims it contains. If the JWT has been successfully validated, its set of claims will be added to the `AuthorizationRequest` state under the "jwtAccessToken" key. Additionally, if the server has been configured accordingly, the identifier of the authenticated user will be added to the request streams as dynamic metadata and/or a header. For more information see the external auth [API reference](https://docs.solo.io/gloo-edge/master/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/extauth/v1/extauth.proto.sk/). (https://github.com/solo-io/gloo/issues/4224)

#### [v1.7.0-beta8](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta8) (Uses OSS [v1.7.0-beta15](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta15))

##### Fixes
- In OIDC, ensure validating JWT ID token audiences work when the JWT has an array of valid audiences rather than a single audience. (https://github.com/solo-io/gloo/issues/4211)

##### Helm Changes
- If userIdHeader is set in helm, ensure it gets set on the Gloo `Settings` resource generated in helm. (https://github.com/solo-io/gloo/issues/4162)
- (From OSS [v1.7.0-beta14](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta14)) Added a new extraVolumes and extraProxyVolumeMounts Helm value that allows users to define additional volumes and volume mounts on the gateway proxy container. (https://github.com/solo-io/gloo/issues/4198)

##### Upgrade Notes
- Remove envoy v2 references from glooE. This marks the complete transition from v2 to v3. (https://github.com/solo-io/gloo/issues/4042)
- (From OSS [v1.7.0-beta14](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta14)) Upgrade gloo's envoy api to remove v2 references. This marks the complete transition from v2 to v3. (https://github.com/solo-io/gloo/issues/4042)

#### [v1.7.0-beta7](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta7) (Uses OSS [v1.7.0-beta13](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta13))

##### Dependency Bumps
- solo-io/protoc-gen-ext has been upgraded to v0.0.15.
- solo-io/skv2 has been upgraded to v0.17.2.
- solo-io/solo-apis has been upgraded to gloo-v1.7.0-beta11.
- solo-io/gloo has been upgraded to v1.7.0-beta13.
- solo-io/solo-apis has been upgraded to gloo-v1.7.0-beta13.
- solo-io/ext-auth-server has been upgraded to v0.7.11.
- (From OSS [v1.7.0-beta13](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta13)) solo-io/skv2 has been upgraded to v0.17.2.
- (From OSS [v1.7.0-beta12](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta12)) solo-io/protoc-gen-ext has been upgraded to v0.0.15.
- (From OSS [v1.7.0-beta12](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta12)) solo-io/go-utils has been upgraded to v0.20.2.

##### Fixes
- Fix the host header behavior of failover endpoints. Failover endpoints will now use the hostname of the upstream address, rather than the envoy default. (https://github.com/solo-io/gloo/issues/4227)
- Fixed a bug where some protobufs were erroneously being considered

- This bug affected Gloo Edge 1.6.0 to 1.6.6 and 1.7.0-beta1 to 1.7.0-beta11 versions only. (https://github.com/solo-io/gloo/issues/4215)
- (From OSS [v1.7.0-beta12](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta12)) Fixed a bug where some protobufs were erroneously being considered

- (From OSS [v1.7.0-beta12](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta12)) This bug affected Gloo Edge 1.6.0 to 1.6.6 and 1.7.0-beta1 to 1.7.0-beta11 versions only. (https://github.com/solo-io/gloo/issues/4215)

##### New Features
- Added glooctl wasm CLI extension to make it easier to manage wasm filters deployed on Gloo Edge Gateway Proxies. (https://github.com/solo-io/solo-projects/issues/2051)
- Add ability for the Gloo Edge Enterprise external auth server to validate OAuth 2.0 access tokens based on access token scopes.  The new match_all field of AccessTokenValidation can be used to specify a list of required scopes for a token. (https://github.com/solo-io/gloo/issues/4224)
- (From OSS [v1.7.0-beta13](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta13)) Add ability for the Gloo Edge Enterprise external auth server to validate OAuth 2.0 access tokens based on access token scopes.  The new `requiredScopes` field of AccessTokenValidation can be used to specify a list of required scopes for a token. Omitting the field means that scope validation is skipped. (https://github.com/solo-io/gloo/issues/4224)

#### [v1.7.0-beta6](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta6) (Uses OSS [v1.7.0-beta11](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta11))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.7.0-beta11.
- solo-io/ext-auth-service has been upgraded to v0.7.10.
- solo-io/gloo has been upgraded to v1.7.0-beta10.
- solo-io/solo-apis has been upgraded to gloo-v1.7.0-beta10.
- solo-io/skv2 has been upgraded to v0.7.0.
- solo-io/skv2-enterprise has been upgraded to v0.7.0.
- solo-io/rate-limiter has been upgraded to v0.7.0.
- solo-io/solo-apis has been upgraded to v0.0.0-20210122142844-ac0df2dce136.
- helm/helm has been upgraded to v3.4.2.
- containerd/containerd has been upgraded to v1.4.3.
- k8s.io/kube-openapi has been upgraded to v0.0.0-20200805222855-6aeccd4b50c6.
- k8s.io/utils has been upgraded to v0.0.0-20201110183641-67b214c5f920.
- k8s.io/controller-runtime has been upgraded to v0.7.0.
- k8s.io/kubernetes has been upgraded to v1.19.6.
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) solo-io/skv1 has been upgraded to v0.7.0.
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) solo-io/solo-apis has been upgraded to v0.0.0-20210122142844-ac0df2dce136.
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) helm/helm has been upgraded to v3.4.2.
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) containerd/containerd has been upgraded to v1.4.3.
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) k8s.io/kube-openapi has been upgraded to v0.0.0-20200805222855-6aeccd4b50c6.
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) k8s.io/utils has been upgraded to v0.0.0-20201110183641-67b214c5f920.
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) k8s.io/controller-runtime has been upgraded to v0.7.0.
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) k8s.io/kubernetes has been upgraded to v1.19.6.

##### Fixes
- Fixes an issue where gloo would repeatedly send unchanged configs to the extauth service, triggering excessive logging and user confusion. This was caused by an inconsistent ordering of configurations when hashing them to determine if anything had changed. (https://github.com/solo-io/gloo/issues/3631)
- Set-style ratelimit API only: set the skip if empty on request headers in rate limiting so that descriptors are still sent to the rate limit server even if some headers are missing on the request. (https://github.com/solo-io/gloo/issues/4095)
- Set-style ratelimit API only: fix the cache key used in the rate limit API in cases where more descriptors were provided in the tuple than necessary to match the set rule. (https://github.com/solo-io/gloo/issues/4095)
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) Re-enable REST EDS. Change the helm template to avoid this helm issue (https://github.com/Masterminds/sprig/issues/111). (https://github.com/solo-io/gloo/issues/4151)
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) Changes helm to make rest_xds_cluster deployment optional. (https://github.com/solo-io/gloo/issues/4164)

##### Helm Changes
- Allow setting the API version of the `ext_authz` transport protocol via the new `global.extensions.extAuth.transportApiVersion` Helm value. The allowed values are `V2` and `V3`, with the latter being the default. Users who are running a custom external auth server should make sure that the server supports `V3` of the API. If it does not, `transportApiVersion` should be set to `V2` to maintain backwards compatibility. This does not apply to the default Gloo Edge Enterprise external auth server, which supports both protocol versions. Note that `transportApiVersion` needs to be `V3` in order for the external auth server to be able to emit dynamic metadata. (https://github.com/solo-io/gloo/issues/4160)

##### New Features
- (From OSS [v1.7.0-beta11](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta11)) Allow for the configuration of socket options on the envoy listener. This is useful, for example, to set TCP keep alive for downstream connections to envoy (e.g., NLB in front of envoy). (https://github.com/solo-io/gloo/issues/3758)
- (From OSS [v1.7.0-beta10](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta10)) Added the new `transport_api_version` field to the `extauth` settings. The field determines the API version for the `ext_authz` transport protocol that will be used by Envoy to communicate with the auth server. The currently allowed values are `V2` and `V3`, with the former being the default; this was done to maintain compatibility with existing custom auth servers. Note that in order for the external auth server to be able to emit dynamic metadata the field needs to be set to `V3`. For more info, see the `transport_api_version` field [here](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_authz/v3/ext_authz.proto#extensions-filters-http-ext-authz-v3-extauthz). (https://github.com/solo-io/gloo/issues/4160)
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) Added the new `envoy_metadata` route option. This field can be used to provide additional information which can be consumed by the Envoy filters that process requests that match the route. For more info about metadata, see [here](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/advanced/data_sharing_between_filters#metadata). (https://github.com/solo-io/gloo/issues/4160)
- (From OSS [v1.7.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta9)) Add support for [metadata actions](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#config-route-v3-ratelimit-action-metadata) to the rate limit API. The new `metadata` action type can now be used to generate rate limit descriptors based on both static and dynamic Envoy metadata. (https://github.com/solo-io/gloo/issues/4160)

#### [v1.7.0-beta5](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta5) (Uses OSS [v1.7.0-beta8](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta8))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.7.9.
- solo-io/gloo has been upgraded to v1.7.0-beta7.
- solo-io/skv2 has been upgraded to v0.16.1.

##### Fixes
- Set the extauth default LOG_LEVEL to INFO (https://github.com/solo-io/gloo/issues/4177)
- Copy over helm defaults from the Gloo helm chart to Gloo Enterprise helm chart. (https://github.com/solo-io/gloo/issues/4130)

##### New Features
- Allows users to use JWT with the [boolean expression extauth API](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/extauth/v1/extauth.proto.sk/#authconfig) to create authentication logic between JWT and other ext-auth services. Previously, JWT authentication only ran before extauth. This adds stages to the JWT authentication, which can now be configured to run before extauth and after extauth. (https://github.com/solo-io/gloo/issues/3207)

#### [v1.7.0-beta4](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta4) (Uses OSS [v1.7.0-beta8](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta8))

##### Fixes
- Fix the proxy memory leak in the Gloo pod. It was being caused by a map or resources with status updates never being cleared. Rather than have this map created and passed in at setup time, it will instead be an argument to the various functions. (https://github.com/solo-io/gloo/issues/4078)
- (From OSS [v1.7.0-beta8](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta8)) Fix a race condition in the gateway-validation-webhook, where resources applied concurrently can avoid validation. (https://github.com/solo-io/gloo/issues/4136)
- (From OSS [v1.7.0-beta7](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta7)) Stop leaking memory for a timer in consul EDS. (https://github.com/solo-io/gloo/issues/4112)

##### New Features
- (From OSS [v1.7.0-beta8](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta8)) On a Gloo OS release, push the open source protos to the solo-apis repository. (https://github.com/solo-io/gloo/issues/3518)
- (From OSS [v1.7.0-beta7](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta7)) Add the `glooctl get ratelimitconfig` command. (https://github.com/solo-io/gloo/issues/4085)
- (From OSS [v1.7.0-beta7](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta7)) Add warnings when users try to use enterprise-only Gloo Edge features when running the Open Source edition of Gloo Edge (https://github.com/solo-io/gloo/issues/4020)
- (From OSS [v1.7.0-beta6](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta6)) Adds API to support two different instances of JWT validation, before the extauth filter in envoy and after. (https://github.com/solo-io/gloo/issues/3207)

#### [v1.7.0-beta3](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta3) (Uses OSS [v1.7.0-beta5](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta5))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.7.0-beta5.

##### Fixes
- (From OSS [v1.7.0-beta5](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta5)) CPU profile of Gloo at scale (5000+ upstreams) indicated that the `generateXDSSnapshot` function was taking upwards of 5 seconds of CPU on a ~50 second sample. This change optimizes the code by using creating hashes for the XDS snapshot using deterministic proto marshalling and fnv hashing rather than the reflection-based `mitchellh/hashstructure` which was benchmarked to be several orders of magnitude slower. (https://github.com/solo-io/gloo/issues/4084)

#### [v1.7.0-beta2](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta2) (Uses OSS [v1.7.0-beta4](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta4))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.7.0-beta4.
- (From OSS [v1.7.0-beta2](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta2)) solo-io/protoc-gen-ext has been upgraded to v0.0.14.

##### Fixes
- (From OSS [v1.7.0-beta4](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta4)) CPU profile of Gloo at scale (5000+ upstreams) indicated that the `endpointsForUpstream` function was taking upwards of 5 seconds of CPU on a ~50 second sample. This change optimizes the code by using a map instead of looping over all endpoints for each upstream. (https://github.com/solo-io/gloo/issues/4084)
- (From OSS [v1.7.0-beta2](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta2)) Switching CSRF mode from enabled to shadow mode does not apply default enabled value to filter. (https://github.com/solo-io/gloo/issues/4053)

##### Helm Changes
- Have Gloo-EE's helm config make use of Gloo-OS's new Istio integration config and blacklist pods from Istio discovery. (https://github.com/solo-io/gloo/issues/3924)
- (From OSS [v1.7.0-beta3](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta3)) Add 3 configuration values under global.istioIntegration to control automatic discovery and sidecar injection for Gloo pods by Istio. LabelInstallNamespace adds a label to mark the namespace for Istio discovery if the namespace is designated to be created in the chart. WhitelistDiscovery explicitly annotates Gloo's discovery pod for Istio sidecar injection. DisableAutoinjection annotates all pods that aren't more specifically noted elsewhere never receive Istio sidecar injection. (https://github.com/solo-io/gloo/issues/3924)

##### New Features
- (From OSS [v1.7.0-beta2](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta2)) Gloo Edge now proactively reports warnings on virtual services that have matchers that are short-circuited.

- (From OSS [v1.7.0-beta2](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta2)) Possibility to configure custom auth server to retrieve request body as bytes (Support Envoys packAsBytes) (https://github.com/solo-io/gloo/issues/3937)

#### [v1.7.0-beta1](https://github.com/solo-io/solo-projects/releases/tag/v1.7.0-beta1) (Uses OSS [v1.7.0-beta1](https://github.com/solo-io/gloo/releases/tag/v1.7.0-beta1))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.7.0-beta1.

### v1.6.0

#### [v1.6.23](https://github.com/solo-io/solo-projects/releases/tag/v1.6.23) (Uses OSS [v1.6.19](https://github.com/solo-io/gloo/releases/tag/v1.6.19))

##### Dependency Bumps
- solo-io/solo-apis has been upgraded to gloo-v1.6.19.
- solo-io/gloo has been upgraded to v1.6.19.

##### Helm Changes
- (From OSS [v1.6.19](https://github.com/solo-io/gloo/releases/tag/v1.6.19)) Allow tracing to be configured via helm on the default gateway. (https://github.com/solo-io/gloo/issues/4494)

#### [v1.6.22](https://github.com/solo-io/solo-projects/releases/tag/v1.6.22) (Uses OSS [v1.6.18](https://github.com/solo-io/gloo/releases/tag/v1.6.18))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.7.21.
- solo-io/solo-apis has been upgraded to gloo-v1.6.18.
- solo-io/gloo has been upgraded to v1.6.18.
- (From OSS [v1.6.16](https://github.com/solo-io/gloo/releases/tag/v1.6.16)) linux/alpine has been upgraded to v3.13.2.
- (From OSS [v1.6.16](https://github.com/solo-io/gloo/releases/tag/v1.6.16)) solo-io/solo-kit has been upgraded to v0.17.4.

##### Fixes
- Expose a discovery_poll_interval which controls interval at which OIDC configuration is discovered at <issuerUrl>/.well-known/openid-configuration. The default value is 30 minutes. (https://github.com/solo-io/gloo/issues/4470)
- (From OSS [v1.6.16](https://github.com/solo-io/gloo/releases/tag/v1.6.16)) EDS updates will work even when previous envoy snapshot is missing. (https://github.com/solo-io/gloo/issues/4345)
- (From OSS [v1.6.15](https://github.com/solo-io/gloo/releases/tag/v1.6.15)) Fixes docs issue where clicking the "Copy" button next to code blocks causes unintentional scrolling behaviour. (https://github.com/solo-io/gloo/issues/4413)

##### Helm Changes
- (From OSS [v1.6.16](https://github.com/solo-io/gloo/releases/tag/v1.6.16)) Expose the `regexMaxProgramSize` settings field as a Helm chart value. (https://github.com/solo-io/gloo/issues/4419)

#### [v1.6.21](https://github.com/solo-io/solo-projects/releases/tag/v1.6.21) (Uses OSS [v1.6.14](https://github.com/solo-io/gloo/releases/tag/v1.6.14))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.7.20.

#### [v1.6.20](https://github.com/solo-io/solo-projects/releases/tag/v1.6.20) (Uses OSS [v1.6.14](https://github.com/solo-io/gloo/releases/tag/v1.6.14))

##### Dependency Bumps
- solo-io/rate-limiter has been upgraded to v0.1.12.

##### Fixes
- Allow set-style rules to omit rate limits, similar to envoy style API. (https://github.com/solo-io/gloo/issues/4279)

#### [v1.6.19](https://github.com/solo-io/solo-projects/releases/tag/v1.6.19) (Uses OSS [v1.6.14](https://github.com/solo-io/gloo/releases/tag/v1.6.14))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.7.19.

##### Fixes
- Redirect to the default app url upon OIDC logouts (https://github.com/solo-io/gloo/issues/4271)

#### [v1.6.18](https://github.com/solo-io/solo-projects/releases/tag/v1.6.18) (Uses OSS [v1.6.14](https://github.com/solo-io/gloo/releases/tag/v1.6.14))

##### Fixes
- Re-enable REST EDS by default to avoid bug in upstream envoy with cluster updates from gRPC management plane. (https://github.com/solo-io/gloo/issues/4151)
- (From OSS [v1.6.14](https://github.com/solo-io/gloo/releases/tag/v1.6.14)) Improve CPU by using newer marshal / unmarshal functions in protov2 in the REST EDS server. (https://github.com/solo-io/gloo/issues/4343)

#### [v1.6.17](https://github.com/solo-io/solo-projects/releases/tag/v1.6.17) (Uses OSS [v1.6.13](https://github.com/solo-io/gloo/releases/tag/v1.6.13))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.7.18.

##### Fixes
- Unescape the path that is included in the OIDC state (https://github.com/solo-io/gloo/issues/4339)

#### [v1.6.16](https://github.com/solo-io/solo-projects/releases/tag/v1.6.16) (Uses OSS [v1.6.13](https://github.com/solo-io/gloo/releases/tag/v1.6.13))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.6.13.
- solo-io/ext-auth-service has been upgraded to v0.7.17.

##### New Features
- Config can be passed in to Passthrough External Authentication through the `config` block in AuthConfig. This config data is now available under [FilterMetadata](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/base.proto#config-core-v3-metadata) under the key `solo.auth.passthrough.config`. (https://github.com/solo-io/gloo/issues/4293)
- (From OSS [v1.6.11](https://github.com/solo-io/gloo/releases/tag/v1.6.11)) Support for custom config to be passed in to the passthrough auth grpc service through AuthConfig. (https://github.com/solo-io/gloo/issues/4293)

#### [v1.6.15](https://github.com/solo-io/solo-projects/releases/tag/v1.6.15) (Uses OSS [v1.6.10](https://github.com/solo-io/gloo/releases/tag/v1.6.10))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.6.10.

##### Fixes
- (From OSS [v1.6.9](https://github.com/solo-io/gloo/releases/tag/v1.6.9)) Support using settings.UpstreamOptions on upstreams that define one-way tls (https://github.com/solo-io/gloo/issues/4285)

##### New Features
- (From OSS [v1.6.10](https://github.com/solo-io/gloo/releases/tag/v1.6.10)) Adds support for HTTP Connect upstreams by setting tunneling config on the upstream. This can be used to route to other proxies (such as ones in a DMZ). (https://github.com/solo-io/gloo/issues/3664)

#### [v1.6.14](https://github.com/solo-io/solo-projects/releases/tag/v1.6.14) (Uses OSS [v1.6.8](https://github.com/solo-io/gloo/releases/tag/v1.6.8))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.7.15.

#### [v1.6.13](https://github.com/solo-io/solo-projects/releases/tag/v1.6.13) (Uses OSS [v1.6.8](https://github.com/solo-io/gloo/releases/tag/v1.6.8))

##### Fixes
- Fix grpc healthchecks to hit the right grpc service name in extauth service. (https://github.com/solo-io/gloo/issues/4324)

#### [v1.6.12](https://github.com/solo-io/solo-projects/releases/tag/v1.6.12) (Uses OSS [v1.6.8](https://github.com/solo-io/gloo/releases/tag/v1.6.8))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.6.8.

##### Fixes
- (From OSS [v1.6.8](https://github.com/solo-io/gloo/releases/tag/v1.6.8)) Provides an option to define global SslParameters that will be applied to all upstreams by default. An individual upstream can override these properties by specifying SslParameters. (https://github.com/solo-io/gloo/issues/4285)

#### [v1.6.11](https://github.com/solo-io/solo-projects/releases/tag/v1.6.11) (Uses OSS [v1.6.7](https://github.com/solo-io/gloo/releases/tag/v1.6.7))

##### Fixes
- Fix ratelimit and extauth logs to include correct gloo version (https://github.com/solo-io/solo-projects/issues/2091)

#### [v1.6.10](https://github.com/solo-io/solo-projects/releases/tag/v1.6.10) (Uses OSS [v1.6.7](https://github.com/solo-io/gloo/releases/tag/v1.6.7))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.7.14.

#### [v1.6.9](https://github.com/solo-io/solo-projects/releases/tag/v1.6.9) (Uses OSS [v1.6.7](https://github.com/solo-io/gloo/releases/tag/v1.6.7))

##### Fixes
- Fix per value rate-limits in the set-style API. (i.e., when omitting the optional value from a simple descriptor, create a rate limit for each unique value instead of having the unique values share the same limit). (https://github.com/solo-io/gloo/issues/4257)

#### [v1.6.8](https://github.com/solo-io/solo-projects/releases/tag/v1.6.8) (Uses OSS [v1.6.7](https://github.com/solo-io/gloo/releases/tag/v1.6.7))

##### Dependency Bumps
- solo-io/protoc-gen-ext has been upgraded to v0.0.15.
- solo-io/gloo has been upgraded to v1.6.7.
- solo-io/k8s-utils has been upgraded to v0.0.5.
- (From OSS [v1.6.7](https://github.com/solo-io/gloo/releases/tag/v1.6.7)) solo-io/protoc-gen-ext has been upgraded to v0.0.15.

##### Fixes
- Fixed a bug where some protobufs were erroneously being considered

- This bug affected Gloo Edge 1.6.0 to 1.6.6 and 1.7.0-beta1 to 1.7.0-beta11 versions only. (https://github.com/solo-io/gloo/issues/4215)
- In OIDC, ensure validating JWT ID token audiences work when the JWT has an array of valid audiences rather than a single audience. (https://github.com/solo-io/gloo/issues/4211)
- (From OSS [v1.6.7](https://github.com/solo-io/gloo/releases/tag/v1.6.7)) Fixed a bug where some protobufs were erroneously being considered

- (From OSS [v1.6.7](https://github.com/solo-io/gloo/releases/tag/v1.6.7)) This bug affected Gloo Edge 1.6.0 to 1.6.6 and 1.7.0-beta1 to 1.7.0-beta11 versions only. (https://github.com/solo-io/gloo/issues/4215)

##### Helm Changes
- If userIdHeader is set in helm, ensure it gets set on the Gloo `Settings` resource generated in helm. (https://github.com/solo-io/gloo/issues/4162)

#### [v1.6.7](https://github.com/solo-io/solo-projects/releases/tag/v1.6.7) (Uses OSS [v1.6.6](https://github.com/solo-io/gloo/releases/tag/v1.6.6))

##### Fixes
- Fix the host header behavior of failover endpoints. Failover endpoints will now use the hostname of the upstream address, rather than the envoy default. (https://github.com/solo-io/gloo/issues/4227)

#### [v1.6.6](https://github.com/solo-io/solo-projects/releases/tag/v1.6.6) (Uses OSS [v1.6.6](https://github.com/solo-io/gloo/releases/tag/v1.6.6))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.6.6.

##### Fixes
- Fixes an issue where gloo would repeatedly send unchanged configs to the extauth service, triggering excessive logging and user confusion. This was caused by an inconsistent ordering of configurations when hashing them to determine if anything had changed. (https://github.com/solo-io/gloo/issues/3631)
- (From OSS [v1.6.6](https://github.com/solo-io/gloo/releases/tag/v1.6.6)) Allow for the configuration of socket options on the envoy listener. This is useful, for example, to set TCP keep alive for downstream connections to envoy (e.g., NLB in front of envoy). (https://github.com/solo-io/gloo/issues/3758)

#### [v1.6.5](https://github.com/solo-io/solo-projects/releases/tag/v1.6.5) (Uses OSS [v1.6.5](https://github.com/solo-io/gloo/releases/tag/v1.6.5))

##### Fixes
- Set-style ratelimit API only: set the skip if empty on request headers in rate limiting so that descriptors are still sent to the rate limit server even if some headers are missing on the request. (https://github.com/solo-io/gloo/issues/4095)
- Set-style ratelimit API only: fix the cache key used in the rate limit API in cases where more descriptors were provided in the tuple than necessary to match the set rule. (https://github.com/solo-io/gloo/issues/4095)
- (From OSS [v1.6.5](https://github.com/solo-io/gloo/releases/tag/v1.6.5)) Re-enable REST EDS. Change the helm template to avoid this helm issue (https://github.com/Masterminds/sprig/issues/111). (https://github.com/solo-io/gloo/issues/4151)
- (From OSS [v1.6.5](https://github.com/solo-io/gloo/releases/tag/v1.6.5)) Changes helm to make rest_xds_cluster deployment optional. (https://github.com/solo-io/gloo/issues/4164)

#### [v1.6.4](https://github.com/solo-io/solo-projects/releases/tag/v1.6.4) (Uses OSS [v1.6.4](https://github.com/solo-io/gloo/releases/tag/v1.6.4))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.7.9.

##### Fixes
- Set the extauth default LOG_LEVEL to INFO (https://github.com/solo-io/gloo/issues/4177)

#### [v1.6.3](https://github.com/solo-io/solo-projects/releases/tag/v1.6.3) (Uses OSS [v1.6.4](https://github.com/solo-io/gloo/releases/tag/v1.6.4))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.6.4.

##### New Features
- Allows users to use JWT with the [boolean expression extauth API](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/extauth/v1/extauth.proto.sk/#authconfig) to create authentication logic between JWT and other ext-auth services. Previously, JWT authentication only ran before extauth. This adds stages to the JWT authentication, which can now be configured to run before extauth and after extauth. (https://github.com/solo-io/gloo/issues/3207)
- (From OSS [v1.6.4](https://github.com/solo-io/gloo/releases/tag/v1.6.4)) Adds API to support two different instances of JWT validation, before the envoy extauth filter and after. (https://github.com/solo-io/gloo/issues/3207)

#### [v1.6.2](https://github.com/solo-io/solo-projects/releases/tag/v1.6.2) (Uses OSS [v1.6.3](https://github.com/solo-io/gloo/releases/tag/v1.6.3))

##### Dependency Bumps
- (From OSS [v1.6.2](https://github.com/solo-io/gloo/releases/tag/v1.6.2)) solo-io/protoc-gen-ext has been upgraded to v0.0.14.

##### Fixes
- Fix the proxy memory leak in the Gloo pod. It was being caused by a map or resources with status updates never being cleared. Rather than have this map created and passed in at setup time, it will instead be an argument to the various functions. (https://github.com/solo-io/gloo/issues/4078)
- (From OSS [v1.6.3](https://github.com/solo-io/gloo/releases/tag/v1.6.3)) CPU profile of Gloo at scale (5000+ upstreams) indicated that the `generateXDSSnapshot` function was taking upwards of 5 seconds of CPU on a ~50 second sample. This change optimizes the code by using creating hashes for the XDS snapshot using deterministic proto marshalling and fnv hashing rather than the reflection-based `mitchellh/hashstructure` which was benchmarked to be several orders of magnitude slower. (https://github.com/solo-io/gloo/issues/4084)
- (From OSS [v1.6.3](https://github.com/solo-io/gloo/releases/tag/v1.6.3)) CPU profile of Gloo at scale (5000+ upstreams) indicated that the `endpointsForUpstream` function was taking upwards of 5 seconds of CPU on a ~50 second sample. This change optimizes the code by using a map instead of looping over all endpoints for each upstream. (https://github.com/solo-io/gloo/issues/4084)
- (From OSS [v1.6.3](https://github.com/solo-io/gloo/releases/tag/v1.6.3)) Gloo Edge now proactively reports warnings on virtual services that have matchers that are short-circuited.

- (From OSS [v1.6.3](https://github.com/solo-io/gloo/releases/tag/v1.6.3)) Fix a race condition in the gateway-validation-webhook, where resources applied concurrently can avoid validation. (https://github.com/solo-io/gloo/issues/4136)
- (From OSS [v1.6.2](https://github.com/solo-io/gloo/releases/tag/v1.6.2)) Gloo Edge now proactively reports warnings on virtual services that have matchers that are short-circuited.

- (From OSS [v1.6.2](https://github.com/solo-io/gloo/releases/tag/v1.6.2)) Switching CSRF mode from enabled to shadow mode does not apply default enabled value to filter. (https://github.com/solo-io/gloo/issues/4053)

##### New Features
- (From OSS [v1.6.2](https://github.com/solo-io/gloo/releases/tag/v1.6.2)) Possibility to configure custom auth server to retrieve request body as bytes (Support Envoys packAsBytes) (https://github.com/solo-io/gloo/issues/3937)

#### [v1.6.1](https://github.com/solo-io/solo-projects/releases/tag/v1.6.1) (Uses OSS [v1.6.1](https://github.com/solo-io/gloo/releases/tag/v1.6.1))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.6.1.

##### Fixes
- (From OSS [v1.6.1](https://github.com/solo-io/gloo/releases/tag/v1.6.1)) Introduce LocalityWeightedLb API. This will be used to support locality weighted load balancing on clusters (https://github.com/solo-io/gloo/issues/3038)

##### Notes
- _marked as pre-release due to memory leak in Gloo that was fixed in v1.6.2, for more see https://github.com/solo-io/gloo/issues/4078_

#### [v1.6.0](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0) (Uses OSS [v1.6.0](https://github.com/solo-io/gloo/releases/tag/v1.6.0))

##### Dependency Bumps
- gloo/solo-io has been upgraded to v1.6.0.
- solo-io/gloo has been upgraded to v1.6.0-beta25.
- (From OSS [v1.6.0](https://github.com/solo-io/gloo/releases/tag/v1.6.0)) solo-io/envoy-gloo has been upgraded to v1.17.0-rc4.
- (From OSS [v1.6.0-beta24](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta24)) solo-io/envoy-gloo has been upgraded to v1.17.0-rc3.

##### Fixes
- Disable REST EDS server by default, which is no longer necessary now that upstream envoy has fixed https://github.com/envoyproxy/envoy/issues/13070 (https://github.com/solo-io/gloo/issues/3805)
- (From OSS [v1.6.0](https://github.com/solo-io/gloo/releases/tag/v1.6.0)) RateLimitConfig CRD is now removed with glooctl uninstall command. (https://github.com/solo-io/gloo/issues/4010)
- (From OSS [v1.6.0](https://github.com/solo-io/gloo/releases/tag/v1.6.0)) Envoy has deprecated gzip filter support in favor of the HTTP Compressor filter. Fixes gloo gzip filter to work with envoy's compressor filter. (https://github.com/solo-io/gloo/issues/4016)
- (From OSS [v1.6.0](https://github.com/solo-io/gloo/releases/tag/v1.6.0)) Kubernetes plugin reports error when encountering upstream with nonexistant ServiceNamespace  instead of crashing. (https://github.com/solo-io/gloo/issues/4006)
- (From OSS [v1.6.0-beta24](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta24)) Buffer envoy filter is now added to the filter chain correctly so it can be used other than on the Gateway level. Added end-to-end tests for buffer filter. (https://github.com/solo-io/gloo/issues/4000)
- (From OSS [v1.6.0-beta24](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta24)) Disable REST EDS server by default, which is no longer necessary now that upstream envoy has fixed https://github.com/envoyproxy/envoy/issues/13070 (https://github.com/solo-io/gloo/issues/3805)
- (From OSS [v1.6.0-beta24](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta24)) Csrf envoy filter shadow mode now gets correctly applied to the envoy config. (https://github.com/solo-io/gloo/issues/3898)

##### Helm Changes
- (From OSS [v1.6.0-beta24](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta24)) Add the helm value `gatewayProxies.gatewayProxy.readConfigMulticluster`, set to false by default. Setting this to true will add a gateway-proxy-config-dump-service Service to the gloo installation namespace. This service allows multicluster management planes to access the envoy config dump on port 8082 of the gateway-proxy. (https://github.com/solo-io/gloo/issues/4012)

##### New Features
- Observability deployment uses upstreams' dashboardFolderId values to place corresponding grafana dashboards in specified folders. (https://github.com/solo-io/gloo/issues/3920)
- Allows wasm filters to be loaded from a filepath. This allows for pre-loading wasm filters on pod startup, removing the need to make network requests at runtime to retrieve filters. (https://github.com/solo-io/gloo/issues/4025)
- (From OSS [v1.6.0](https://github.com/solo-io/gloo/releases/tag/v1.6.0)) Gloo Edge can now more proactively report warnings on virtual services that are likely misconfigured.

- (From OSS [v1.6.0-beta24](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta24)) Adds a new `headers_to_append` field to the HTTP request transformation API. This allows users to specify headers which can contain multiple values and to specify transformations for each of the values. (https://github.com/solo-io/gloo/issues/3901)

##### Upgrade Notes
- (From OSS [v1.6.0-beta25](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta25)) Experimental wasm features have been removed from Gloo Edge. Wasm support is now a feature of Gloo Edge Enterprise. (https://github.com/solo-io/gloo/issues/4025)

##### Notes
- _marked as pre-release due to memory leak in Gloo that was fixed in v1.6.2, for more see https://github.com/solo-io/gloo/issues/4078_

#### [v1.6.0-beta13](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta13) (Uses OSS [v1.6.0-beta23](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta23))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.6.0-beta23.
- solo-io/ext-auth-service has been upgraded to v0.7.8.
- solo-io/solo-kit has been upgraded to v0.17.0.
- (From OSS [v1.6.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta21)) solo-io/skv2 has been upgraded to v0.15.2.

##### Fixes
- Fix inconsistency in helm chart regarding aws service account behavior. Before this change, the settings had to be modified in the OS and enterprise chart, this changes it to be only the OS chart for consistency. (https://github.com/solo-io/gloo/issues/4011)
- Close connections grpc connections from rate-limit and extauth to the gloo pod if either fail to connect in the client backoff loop. (https://github.com/solo-io/gloo/issues/3993)
- (From OSS [v1.6.0-beta22](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta22)) Fix a bug where the `inheritableMatchers` value itself was being read from the parent route, rather than the child route (as documented and intended). (https://github.com/solo-io/gloo/issues/4008)
- (From OSS [v1.6.0-beta22](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta22)) Fix generated resource clients watch functions to not block infinitely, therefore leaking go-routines. (https://github.com/solo-io/gloo/issues/4001)
- (From OSS [v1.6.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta21)) Update `glooctl cluster register` and `glooctl cluster deregister` commands to use the default Kubernetes config when registering and deregistering clusters. (https://github.com/solo-io/gloo/issues/3972)

##### Helm Changes
- (From OSS [v1.6.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta21)) Update the version of Istio used by the Istio sidecar in the gateway-proxy pod for mTLS cert generation when the helm value global.istioSDS.enabled is set to true. New Istio version is 1.8.1. (https://github.com/solo-io/gloo/issues/3967)

##### New Features
- Support a passthrough GRPC ext auth service. This service delegates to a configured external service which implements the envoy external auth API (https://github.com/envoyproxy/go-control-plane/blob/master/envoy/service/auth/v3/external_auth.pb.go)

- (From OSS [v1.6.0-beta23](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta23)) Add the defaultDashboardFolderId value to the settings proto definition for use in gloo-E. (https://github.com/solo-io/gloo/issues/3920)
- (From OSS [v1.6.0-beta23](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta23)) Gloo now supports enabling the upstream Envoy CSRF filter by configuring `spec.httpGateway.options.csrf` of the desired Gateway. This can also be overridden on virtual services at the virtual host or route level, and on weighted destinations. See [envoy csrf](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/csrf/v3/csrf.proto) for more details. (https://github.com/solo-io/gloo/issues/3898)
- (From OSS [v1.6.0-beta22](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta22)) Add support for the new `inheritablePathMatchers` value on `Route` config. This new setting is similar to the `inheritableMatchers` boolean value that allows delegated routes (i.e., routes on route tables) to optionally opt into inheriting HTTP header, method, or query parameter matching from the parent route. The new `inheritablePathMatchers` is used to optionally opt into inheriting HTTP path matcher config from the parent. (https://github.com/solo-io/gloo/issues/3726)
- (From OSS [v1.6.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta21)) Adding case sensitivity option on the path matcher. (https://github.com/solo-io/gloo/issues/3976)
- (From OSS [v1.6.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta21)) Allows Jwt authentication to be compatible with other auth mechanisms in enterprise authentication. (https://github.com/solo-io/gloo/issues/3207)
- (From OSS [v1.6.0-beta21](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta21)) Introduce an API to support a passthrough grpc ext auth service. This service authorizes requests by delegating to an external service which implements the envoy external auth API: https://github.com/envoyproxy/envoy/blob/ae1ed1fa74f096dabe8dd5b19fc70333621b0309/api/envoy/service/auth/v3/external_auth.proto#L29


#### [v1.6.0-beta12](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta12) (Uses OSS [v1.6.0-beta20](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta20))

##### Fixes
- Fixed a bug where syncers were writing competing statuses to the proxy resource. (https://github.com/solo-io/gloo/issues/3815)
- Ripout gogo proto in favor of golang proto (https://github.com/solo-io/gloo/issues/3926)
- (From OSS [v1.6.0-beta19](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta19)) Ripout gogo proto in favor of golang proto (https://github.com/solo-io/gloo/issues/3926)

#### [v1.6.0-beta11](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta11) (Uses OSS [v1.6.0-beta18](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta18))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.7.4.
- solo-io/gloo has been upgraded to v1.6.0-beta18.
- solo-io/ext-auth-service has been upgraded to v0.7.5.
- (From OSS [v1.6.0-beta18](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta18)) solo-io/go-utils has been upgraded to v0.20.1.

##### Fixes
- (From OSS [v1.6.0-beta18](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta18)) Fix the proxycontroller docs example code, and the corresponding documentation. (https://github.com/solo-io/gloo/issues/3941)
- (From OSS [v1.6.0-beta18](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta18)) fixes bug where route validation marks redirect and direct response route destinations as invalid. (https://github.com/solo-io/gloo/issues/3975)
- (From OSS [v1.6.0-beta18](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta18)) Changed Istio's default discoveryAddress used by the glooctl istio commands and helm installations when istioSDS is enabled (https://github.com/solo-io/gloo/issues/3908)

##### Helm Changes
- (From OSS [v1.6.0-beta18](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta18)) Add a helm value for setting extauth field for gloo.solo.io.Settings. This allows to configure custom external auth server while installing Helm chart, without need to post-render or patch Settings object after helm chart was installed or upgraded. (https://github.com/solo-io/gloo/issues/1892)

##### New Features
- Implement API to support optional OIDC discovery override for ext-auth. OIDC Configuration is discovered at <issuerUrl>/.well-known/openid-configuration and this configuration can override those discovered values. (https://github.com/solo-io/gloo/issues/3879)
- Support refreshing the OIDC id-token automatically when it expires. (https://github.com/solo-io/gloo/issues/3824)

#### [v1.6.0-beta10](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta10) (Uses OSS [v1.6.0-beta17](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta17))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.6.0-beta17.
- (From OSS [v1.6.0-beta16](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta16)) solo-io/go-utils has been upgraded to v0.20.0.
- (From OSS [v1.6.0-beta13](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta13)) linux/alpine has been upgraded to v3.12.1.

##### Fixes
- Un-hardcode the gloo-system namespace is hardcoded in the 23-extauth-upstream.yaml template. (https://github.com/solo-io/gloo/issues/3938)
- (From OSS [v1.6.0-beta17](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta17)) Fixes a bug where routes that use a deleted lambda function as destination causes gloo to crash. (https://github.com/solo-io/gloo/issues/3895)
- (From OSS [v1.6.0-beta17](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta17)) When configuring tracing, you can specify a cluster where traces are collected. If the collector is an upstream, tracing works as expected. However, if the cluster is statically defined in the envoy bootstrap, traces do not get collected. This adds support for statically defined tracing collector clusters. (https://github.com/solo-io/gloo/issues/3954)
- (From OSS [v1.6.0-beta15](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta15)) In multi-proxy environments, resources that were invalid on one proxy (error or warning) but valid on another may have a status written of accepted, despite internally calculating (and logging) a warning. This is now fixed. (https://github.com/solo-io/gloo/issues/3935)
- (From OSS [v1.6.0-beta15](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta15)) cert-manager can be used to create a Certificate (https://cert-manager.io/docs/concepts/certificate/). This is used to generate a TLS key and certificate, and they are stored in a Kubernetes secret. This can be configured to include an optional property on the secret, ca.crt, which holds a root CA certificate. If cert-manager is used to generate this Kubernetes secret, and the root CA certificate is included, we were not including it when converting to a Gloo secret, causing Gloo to crash. (https://github.com/solo-io/gloo/issues/3652)
- (From OSS [v1.6.0-beta15](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta15)) Turn the certgen job into a no-op if the previously generated certs still exist, and are still valid. (https://github.com/solo-io/gloo/issues/3790)

##### Helm Changes
- Added the ability to enable customEnv, extraVolume and extraVolumeMount to extauth deployment (https://github.com/solo-io/gloo/issues/3922)
- (From OSS [v1.6.0-beta14](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta14)) When doing a helm install where `istioSDS.enabled` is set to `true`, the `ISTIO_META_CLUSTER_ID` environment variable is now initialized to "Kubernetes". (https://github.com/solo-io/gloo/issues/3881)
- (From OSS [v1.6.0-beta14](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta14)) Panic mode allows Envoy load balancing to disregard host's health status. (https://github.com/solo-io/gloo/issues/3747)

##### New Features
- (From OSS [v1.6.0-beta17](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta17)) Add the ability to configure the propagation of the tracing header x-envoy-decorator-operation, for me info: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto.html?highlight=decorator#config-route-v3-decorator (https://github.com/solo-io/gloo/issues/3931)
- (From OSS [v1.6.0-beta16](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta16)) Add the ability to add static clusters to the envoy bootstrap config via helm. This option can be accessed via "<proxy-name>.envoyStaticClusters". The value should be a list of static clusters which will be passed directly to envoy, so the yaml must be correct. The api can be found here: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto#config-cluster-v3-cluster. This is meant to be used for advanced use cases (https://github.com/solo-io/gloo/issues/3905)
- (From OSS [v1.6.0-beta16](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta16)) Add the ability to add bootstrap extensions to the envoy bootstrap config via helm. This option can be accessed via "<proxy-name>.envoyBootstrapExtensions". The value should be a list of bootstrap extensions which will be passed directly envoy, so the yaml must be correct. The main use case being wasm services, for the purpose of creating singletons. Bootstrap extensions is a list of typed extension config (https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/extension.proto#envoy-v3-api-msg-config-core-v3-typedextensionconfig) so the list must be passed with the name, and type url. The API for the wasm service specfically can be found here: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/wasm/v3/wasm.proto#extensions-wasm-v3-wasmservice. This is meant to be used for advanced use cases (https://github.com/solo-io/gloo/issues/3943)
- (From OSS [v1.6.0-beta15](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta15)) Add API to support optional OIDC configuration override for ext-auth. OIDC Configuration is discovered at <issuerUrl>/.well-known/openid-configuration and this configuration can override those discovered values. (https://github.com/solo-io/gloo/issues/3879)
- (From OSS [v1.6.0-beta15](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta15)) Support XDS V2/V3 APIs simultaneously from rate-limit server. (https://github.com/solo-io/gloo/issues/2815)

##### Notes
- _Marked as a pre-release to due a regression with redirectActions, see https://github.com/solo-io/gloo/issues/3975_

#### [v1.6.0-beta9](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta9) (Uses OSS [v1.6.0-beta12](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta12))

##### Dependency Bumps
- solo-io/rate-limiter has been upgraded to v0.1.2.
- solo-io/gloo has been upgraded to v1.6.0-beta12.
- solo-io/solo-apis has been upgraded to actual-rate-limiter-v0.1.2.
- linux/alpine has been upgraded to v3.12.1.

##### Fixes
- fixes an issue where vault is unable to be accessed by the apiserver due to an issue with the vault client cache never being configured correctly. (https://github.com/solo-io/gloo/issues/3735)

##### New Features
- Sanitize downstream headers that match envoy's cluster_header destination type. See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-cluster-header for more details about this field. (https://github.com/solo-io/gloo/issues/3749)
- Implement set-style rate limiting. The previous rate-limiting implementation uses a tree structure for descriptors. This adds capability to use a set structure instead, where the descriptors are treated as an unordered set such that a given rule will apply if all the relevant descriptors match, regardless of the values of the other descriptors and regardless of descriptor order. For example, the rule may require `type: a` and `number: 1` but the `remote_address` descriptor can have any value. This can also be understood as `remote_address: *` where * is a wildcard. (https://github.com/solo-io/gloo/issues/2695)
- Show list of wasm filters associated with gateways in the Admin Dashboard. (https://github.com/solo-io/solo-projects/issues/1966)
- (From OSS [v1.6.0-beta12](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta12)) Allow an external tracing provider to be configured on a listener via the Gloo API. See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/trace/v3/http_tracer.proto#envoy-v3-api-msg-config-trace-v3-tracing-http for more details on this setting. (https://github.com/solo-io/gloo/issues/3762)
- (From OSS [v1.6.0-beta11](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta11)) Expose envoy's cluster_header field in the gloo api See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-cluster-header for more details about this field. (https://github.com/solo-io/gloo/issues/3749)

#### [v1.6.0-beta8](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta8) (Uses OSS [v1.6.0-beta10](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta10))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.7.0.
- gloo/solo-io has been upgraded to v1.6.0-beta10.
- (From OSS [v1.6.0-beta8](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta8)) solo-io/solo-apis has been upgraded to rate-limiter-v0.1.2.

##### Fixes
- Modals in Gloo UI are able to be closed by hitting escape. Modal borders have been debugged. (https://github.com/solo-io/solo-projects/issues/1955)
- (From OSS [v1.6.0-beta10](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta10)) Disable gloo metrics service as it is unused, and CPU intensive. (https://github.com/solo-io/gloo/issues/3849)
- (From OSS [v1.6.0-beta10](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta10)) Fixed a bug where a bad authconfig which should invalidate a single virtual service was incorrectly invalidating the entire gateway (https://github.com/solo-io/gloo/issues/3538)
- (From OSS [v1.6.0-beta10](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta10)) Fixed a bug where invalid route replacement did not correctly replace routes that referred to a missing `UpstreamGroup`, which potentially resulted in incorrect config being sent to envoy. Now, the route will be replaced correctly according to the invalid config policy. (https://github.com/solo-io/gloo/issues/3818)
- (From OSS [v1.6.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta9)) fixes glooctl check error messages that are displayed when deployments checks have failed (https://github.com/solo-io/gloo/issues/2952)

##### Helm Changes
- Setting resource limits for the api server deployment no longer requires apiServer.Enterprise to be true. (https://github.com/solo-io/gloo/issues/3793)
- Fixes bug where rate limit gateway proxy pod is still being created even when gateway proxy is disabled (https://github.com/solo-io/gloo/issues/3751)
- Add a new helm value, `license_secret_name`, which defines the name of the license key kubernetes secret. This enables users to run a full gitops workflow where they manage their own kubernetes license key secrets. (https://github.com/solo-io/gloo/issues/3789)
- refactoring comments so that the comments don't appear in the rendered helm template. (https://github.com/solo-io/solo-projects/issues/1954)

##### New Features
- (From OSS [v1.6.0-beta10](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta10)) Add Istio 1.8.x support to the existing glooctl istio integrations. (https://github.com/solo-io/gloo/issues/3855)
- (From OSS [v1.6.0-beta9](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta9)) Expose the server_header_transformation setting via the Gloo API. See https://www.envoyproxy.io/docs/envoy/latest/api-v2/config/filter/network/http_connection_manager/v2/http_connection_manager.proto for more details on this setting. (https://github.com/solo-io/gloo/issues/3769)
- (From OSS [v1.6.0-beta8](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta8)) Define the API to allow for set-style rate limiting. The previous rate-limiting implementation uses a tree structure for descriptors. This adds capability to use a set structure instead, where the descriptors are treated as an unordered set such that a given rule will apply if all the relevant descriptors match, regardless of the values of the other descriptors and regardless of descriptor order. For example, the rule may require `type: a` and `number: 1` but the `remote_address` descriptor can have any value. This can also be understood as `remote_address: *` where * is a wildcard. (https://github.com/solo-io/gloo/issues/2695)

#### [v1.6.0-beta7](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta7) (Uses OSS [v1.6.0-beta7](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta7))

##### Dependency Bumps
- (From OSS [v1.6.0-beta6](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta6)) solo-kit/solo-io has been upgraded to v0.13.14.

##### Fixes
- (From OSS [v1.6.0-beta6](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta6)) Fix EDS so modifying configs on a TLS enabled Upstream no longer results in 503s (https://github.com/solo-io/gloo/issues/3673)
- (From OSS [v1.6.0-beta6](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta6)) Gloo will no longer remove upstreams after editing their spec. (https://github.com/solo-io/gloo/issues/3710)
- (From OSS [v1.6.0-beta6](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta6)) Fix EDS so modifying health checks on Upstream no longer results in 503s (https://github.com/solo-io/gloo/issues/3219)
- (From OSS [v1.6.0-beta6](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta6)) Allow static upstream endpoints have individual SNI entries (https://github.com/solo-io/gloo/issues/3806)

##### Helm Changes
- This change stops Upstream, Service, ConfigMap, and Deployment objects from being created for gateway proxies if "disabled = true" under the helm chart values override. (https://github.com/solo-io/gloo/issues/3751)
- added image pull secrets check to all deployments and refactored existing image pull secrets checks (https://github.com/solo-io/gloo/issues/3269)
- (From OSS [v1.6.0-beta6](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta6)) This change stops ConfigMap, Service, and Gateway objects from being created for gateway proxies if "disabled = true" under the helm chart values override. (https://github.com/solo-io/gloo/issues/3751)

##### New Features
- Allow forwarding ID token as upstream header (https://github.com/solo-io/gloo/issues/3816)
- (From OSS [v1.6.0-beta6](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta6)) Allow toggling of EDS to rest XDS to avoid the envoy issue described in the following issue: https://github.com/envoyproxy/envoy/issues/13070. Set to true by default starting in version > `v1.6.0` (https://github.com/solo-io/gloo/issues/3805)

#### [v1.6.0-beta6](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta6) (Uses OSS [v1.6.0-beta5](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta5))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.6.19.

##### Helm Changes
- Added the ability to enable client-side sharding for scaling Redis replicas. (https://github.com/solo-io/gloo/issues/3269)

##### New Features
- Make the token introspection response available in the request state for use by extauth plugins on the chain. (https://github.com/solo-io/gloo/issues/3767)

#### [v1.6.0-beta5](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta5) (Uses OSS [v1.6.0-beta5](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta5))

##### Dependency Bumps
- envoy-gloo-ee/solo-io has been upgraded to v1.17.0-rc1.
- gloo/solo-io has been upgraded to v1.16.0-beta3.
- (From OSS [v1.6.0-beta3](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta3)) envoy-gloo/solo-io has been upgraded to v1.17.0-rc1.

##### Fixes
- Fix the grpc service names in health checks. This fixes a regression that was introduced in Gloo enterprise v1.5.0-beta8 and v1.4.7. Without this fix, the rate-limit and ext-auth grpc services will fail health checks and go into panic mode (which by default, ignores health checks, so requests still work). (https://github.com/solo-io/gloo/issues/3745)
- Add logout url support to OIDC. (https://github.com/solo-io/gloo/issues/3328)
- URLs with query params will redirect correctly with OIDC. (https://github.com/solo-io/gloo/issues/3765)
- Correct observability watch namespace behavior to default to all namespaces if none are provided. (https://github.com/solo-io/gloo/issues/3060)
- Allow storing session data in redis session when using OIDC. (https://github.com/solo-io/gloo/issues/3656)
- No longer let the api-server create a default settings CRD when none is provided. (https://github.com/solo-io/gloo/issues/3677)
- When Proxies are viewed in the Gloo admin UI, render the entire Proxy spec, even if it is compressed. Compression is determined by the presence of the gloo.solo.io/compress annotation on the Proxy. (https://github.com/solo-io/gloo/issues/3718)
- (From OSS [v1.6.0-beta4](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta4)) Fix an issue where ssl configurations across different virtual services may be incorrectly cached if they ssl configurations only differ by ssl-parameters (e.g., min tls version). **After this change, ssl configurations that are only different by ssl parameters must have different sni domains.** Prior to this change, such a configuration would not error but could result in one ssl configuration being selected over another; now an explicit error is recorded on the virtual service. (https://github.com/solo-io/gloo/issues/3776)
- (From OSS [v1.6.0-beta4](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta4)) Fix the validation API to return all errors encountered while validating a list of resources, rather than immediately returning on the first unmarshal error encountered for a resource in a list resource. (https://github.com/solo-io/gloo/issues/3610)
- (From OSS [v1.6.0-beta4](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta4)) Fix the validation API error reporting to include the resource associated with the error returned. (https://github.com/solo-io/gloo/issues/3610)
- (From OSS [v1.6.0-beta3](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta3)) Fix the validation API to only return proxies that would be generated by proposed resources if requested. This change means the default behavior matches the kubernetes validation webhook API. By including the top-level value `returnProxies=true` in the json/yaml request to the API, you can signal the endpoint to return the proxies that would be generated (previously, always returning by default). (https://github.com/solo-io/gloo/issues/3613)

##### Helm Changes
- Add resource limits to helm config for all deployments. (https://github.com/solo-io/gloo/issues/2979)
- Removed the `global.wasm.enabled` HELM value for toggling experimental wasm support. Wasm is now enabled by default. This flag is no longer required as there is no more need for a separate gateway-proxy image since wasm support was merged into upstream envoy. (https://github.com/solo-io/gloo/issues/3753)
- (From OSS [v1.6.0-beta5](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta5)) Set route prefix_rewrite in ingress proxy and knative proxy configs from `global.glooStats.routePrefixRewrite` helm value. This allows Gloo to integrate with other monitoring systems instead of just Prometheus. (https://github.com/solo-io/gloo/issues/3752)
- (From OSS [v1.6.0-beta5](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta5)) Adds a single helm value that can be used to configure all sds/envoy-sidecar container resource usages. (https://github.com/solo-io/gloo/issues/2979)
- (From OSS [v1.6.0-beta4](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta4)) Add possibility to pass image pull secret to all deployments in helm chart (https://github.com/solo-io/gloo/issues/3729)
- (From OSS [v1.6.0-beta4](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta4)) Add a helm value for stats prefix rewrite. This allows Gloo to integrate with other monitoring systems instead of just Prometheus, by setting the `global.glooStats.routePrefixRewrite` helm value. (https://github.com/solo-io/gloo/issues/3752)
- (From OSS [v1.6.0-beta3](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta3)) Removed the `global.wasm.enabled` HELM value for toggling experimental wasm support. Wasm is now enabled by default. This flag is no longer required as there is no more need for a separate gateway-proxy image since wasm support was merged into upstream envoy. (https://github.com/solo-io/gloo/issues/3753)
- (From OSS [v1.6.0-beta3](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta3)) Addresses minor issue of adding consul configs into helm. (https://github.com/solo-io/gloo/issues/3698)

##### New Features
- Use official wasm support from upstream envoy, rather than envoy-wasm fork. (https://github.com/solo-io/gloo/issues/3753)
- Expose apiserver over HTTPS using custom certs. (https://github.com/solo-io/gloo/issues/3384)
- (From OSS [v1.6.0-beta3](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta3)) Use official wasm support from upstream envoy, rather than envoy-wasm fork. (https://github.com/solo-io/gloo/issues/3753)
- (From OSS [v1.6.0-beta3](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta3)) Allow automatic discovery of TLS when using consul services. Requires serveral changes to gloo's helm config to use:


#### [v1.6.0-beta4](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta4) (Uses OSS [v1.6.0-beta2](https://github.com/solo-io/gloo/releases/tag/v1.6.0-beta2))

##### Notes
- This release contained no user-facing changes.

#### [v1.6.0-beta3](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta3)

##### Notes
- *This release build failed.*
- This release contained no user-facing changes.

#### [v1.6.0-beta2](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta2)

##### Notes
- *This release build failed.*
- This release contained no user-facing changes.

#### [v1.6.0-beta1](https://github.com/solo-io/solo-projects/releases/tag/v1.6.0-beta1)

##### Fixes
- Ensure the rest of our docker containers run with user 10101 rather than root (https://github.com/solo-io/gloo/issues/3346)
- Fix NPE when secret ref is missing. (https://github.com/solo-io/solo-projects/issues/1908)
- Prohibit duplicate route names for routes with Basic rate limits configured. (https://github.com/solo-io/gloo/issues/3674)
- Upgrade base image for grpcserver-enovy image (https://github.com/solo-io/gloo/issues/3617)
- Upgrade base image for grpcserver-ui image (https://github.com/solo-io/gloo/issues/3616)

##### Helm Changes
- Fix helm chart to honor `.Values.settings.replaceInvalidRoutes` value. (https://github.com/solo-io/gloo/issues/3619)
- Add the ability to specify a number of replicas for the redis, rate-limit, apiserver, and observability deployments via helm values. Note: Extra redis replicas still won't scale properly until https://github.com/solo-io/gloo/issues/3269 is addressed. (https://github.com/solo-io/gloo/issues/3262)
- Add the ability to NOT specify a number of replicas for the redis, rate-limit, apiserver, observability and extAuth deployments explicitly in deployments. This prevents issues when using flux and horizontal autoscaling. See https://docs.fluxcd.io/en/1.18.0/faq.html#how-can-i-prevent-flux-overriding-the-replicas-when-using-hpa for more details. (https://github.com/solo-io/gloo/issues/2650)

##### New Features
- Expose apiserver over HTTPS using self-signed certs when running in glooMtls mode. (https://github.com/solo-io/gloo/issues/3384)
- With each release, we will additionally be publishing an alternate set of docker containers (tagged as usual but with the "-extended" suffix) that have some additional dependencies built in (e.g., `curl` for debugging). You can deploy these images by setting the helm value `global.image.extended=true`. (https://github.com/solo-io/gloo/issues/3399)
- Implement new `AuthConfig` API that allows users to specify a boolean expression to determine how to evaluate auth configs within an auth chain. Previously, each config on an auth config must be authorized for the entire request to be authorized. This remains the default, but now users can additionally specify a boolean expression (the `booleanExpr` field on an auth config) to reference the auth configs and AND/OR/NOT them together to achieve the desired access policy. (https://github.com/solo-io/gloo/issues/3207)

##### Notes
- *This release build failed.*

### v1.5.0

#### [v1.5.18](https://github.com/solo-io/solo-projects/releases/tag/v1.5.18) (Uses OSS [v1.5.18](https://github.com/solo-io/gloo/releases/tag/v1.5.18))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.6.23.
- solo-io/solo-apis has been upgraded to gloo-v1.5.18.
- solo-io/gloo has been upgraded to v1.5.18.

##### Fixes
- Allow the user to define behaviors for when a token is provided with a key ID that is not contained in the local JWKS cache. (https://github.com/solo-io/gloo/issues/4507)

#### [v1.5.17](https://github.com/solo-io/solo-projects/releases/tag/v1.5.17) (Uses OSS [v1.5.17](https://github.com/solo-io/gloo/releases/tag/v1.5.17))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.6.22.
- solo-io/go-utils has been upgraded to v0.16.7.
- solo-io/solo-apis has been upgraded to gloo-v1.5.17.
- solo-io/gloo has been upgraded to v1.5.17.

##### Fixes
- Expose a discovery_poll_interval which controls interval at which OIDC configuration is discovered at <issuerUrl>/.well-known/openid-configuration. The default value is 30 minutes. (https://github.com/solo-io/gloo/issues/4470)

#### [v1.5.16](https://github.com/solo-io/solo-projects/releases/tag/v1.5.16) (Uses OSS [v1.5.16](https://github.com/solo-io/gloo/releases/tag/v1.5.16))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.16.

##### Fixes
- (From OSS [v1.5.16](https://github.com/solo-io/gloo/releases/tag/v1.5.16)) Allow for the configuration of socket options on the envoy listener. This is useful, for example, to set TCP keep alive for downstream connections to envoy (e.g., NLB in front of envoy). (https://github.com/solo-io/gloo/issues/3758)

#### [v1.5.15](https://github.com/solo-io/solo-projects/releases/tag/v1.5.15) (Uses OSS [v1.5.15](https://github.com/solo-io/gloo/releases/tag/v1.5.15))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.15.

##### Fixes
- (From OSS [v1.5.15](https://github.com/solo-io/gloo/releases/tag/v1.5.15)) Fix a race condition in the gateway-validation-webhook, where resources applied concurrently can avoid validation. (https://github.com/solo-io/gloo/issues/4136)

#### [v1.5.14](https://github.com/solo-io/solo-projects/releases/tag/v1.5.14) (Uses OSS [v1.5.14](https://github.com/solo-io/gloo/releases/tag/v1.5.14))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.14.

##### Fixes
- (From OSS [v1.5.14](https://github.com/solo-io/gloo/releases/tag/v1.5.14)) fixes bug where route validation marks routes with redirect and direct response actions as invalid. (https://github.com/solo-io/gloo/issues/3975)

#### [v1.5.13](https://github.com/solo-io/solo-projects/releases/tag/v1.5.13) (Uses OSS [v1.5.13](https://github.com/solo-io/gloo/releases/tag/v1.5.13))

##### Helm Changes
- Added the ability to enable customEnv, extraVolume and extraVolumeMount to extauth deployment (https://github.com/solo-io/gloo/issues/3922)

##### Notes
- _Marked as a pre-release to due a regression with redirectActions, see https://github.com/solo-io/gloo/issues/3975_

#### [v1.5.12](https://github.com/solo-io/solo-projects/releases/tag/v1.5.12) (Uses OSS [v1.5.13](https://github.com/solo-io/gloo/releases/tag/v1.5.13))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.13.
- (From OSS [v1.5.13](https://github.com/solo-io/gloo/releases/tag/v1.5.13)) solo-io/envoy-gloo has been upgraded to v1.16.1-patch1.

##### Fixes
- (From OSS [v1.5.13](https://github.com/solo-io/gloo/releases/tag/v1.5.13)) Add the ability to configure the propagation of the tracing header x-envoy-decorator-operation, for more info: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto.html?highlight=decorator#config-route-v3-decorator (https://github.com/solo-io/gloo/issues/3931)
- (From OSS [v1.5.13](https://github.com/solo-io/gloo/releases/tag/v1.5.13)) Fixes a bug where routes that use a deleted lambda function as destination causes gloo to crash. (https://github.com/solo-io/gloo/issues/3895)

##### Notes
- _Marked as a pre-release to due a regression with redirectActions, see https://github.com/solo-io/gloo/issues/3975_

#### [v1.5.11](https://github.com/solo-io/solo-projects/releases/tag/v1.5.11) (Uses OSS [v1.5.12](https://github.com/solo-io/gloo/releases/tag/v1.5.12))

##### Dependency Bumps
- gloo/solo-io has been upgraded to v1.5.12.

##### Fixes
- Sanitize downstream headers that match envoy's cluster_header destination type. See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-cluster-header for more details about this field. (https://github.com/solo-io/gloo/issues/3749)
- (From OSS [v1.5.11](https://github.com/solo-io/gloo/releases/tag/v1.5.11)) Expose envoy's cluster_header field in the gloo api See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-cluster-header for more details about this field. (https://github.com/solo-io/gloo/issues/3749)

##### Helm Changes
- (From OSS [v1.5.12](https://github.com/solo-io/gloo/releases/tag/v1.5.12)) Fix helm template and documentation for configuring tracing (https://github.com/solo-io/gloo/issues/3896)

#### [v1.5.10](https://github.com/solo-io/solo-projects/releases/tag/v1.5.10) (Uses OSS [v1.5.10](https://github.com/solo-io/gloo/releases/tag/v1.5.10))

##### Dependency Bumps
- gloo/solo-io has been upgraded to v1.5.10.

##### Fixes
- (From OSS [v1.5.10](https://github.com/solo-io/gloo/releases/tag/v1.5.10)) Disable gloo metrics service as it is unused, and CPU intensive. (https://github.com/solo-io/gloo/issues/3849)
- (From OSS [v1.5.10](https://github.com/solo-io/gloo/releases/tag/v1.5.10)) Fixed a bug where a bad authconfig which should invalidate a single virtual service was incorrectly invalidating the entire gateway (https://github.com/solo-io/gloo/issues/3538)
- (From OSS [v1.5.10](https://github.com/solo-io/gloo/releases/tag/v1.5.10)) Fixed a bug where invalid route replacement did not correctly replace routes that referred to a missing `UpstreamGroup`, which potentially resulted in incorrect config being sent to envoy. Now, the route will be replaced correctly according to the invalid config policy. (https://github.com/solo-io/gloo/issues/3818)

#### [v1.5.9](https://github.com/solo-io/solo-projects/releases/tag/v1.5.9) (Uses OSS [v1.5.9](https://github.com/solo-io/gloo/releases/tag/v1.5.9))

##### Dependency Bumps
- gloo/solo-io has been upgraded to v1.5.9.

##### Fixes
- (From OSS [v1.5.9](https://github.com/solo-io/gloo/releases/tag/v1.5.9)) Expose the server_header_transformation setting via the Gloo API. See https://www.envoyproxy.io/docs/envoy/latest/api-v2/config/filter/network/http_connection_manager/v2/http_connection_manager.proto for more details on this setting. (https://github.com/solo-io/gloo/issues/3769)

##### Helm Changes
- Setting resource limits for the api server deployment no longer requires apiServer.Enterprise to be true. (https://github.com/solo-io/gloo/issues/3793)
- Fixes bug where rate limit gateway proxy pod is still being created even when gateway proxy is disabled (https://github.com/solo-io/gloo/issues/3751)

#### [v1.5.8](https://github.com/solo-io/solo-projects/releases/tag/v1.5.8) (Uses OSS [v1.5.8](https://github.com/solo-io/gloo/releases/tag/v1.5.8))

##### Fixes
- Add logout url support to OIDC. (https://github.com/solo-io/gloo/issues/3328)
- URLs with query params will redirect correctly with OIDC. (https://github.com/solo-io/gloo/issues/3765)
- Allow forwarding ID token as upstream header (https://github.com/solo-io/gloo/issues/3816)
- Allow storing session data in redis session when using OIDC. (https://github.com/solo-io/gloo/issues/3656)

#### [v1.5.7](https://github.com/solo-io/solo-projects/releases/tag/v1.5.7) (Uses OSS [v1.5.7](https://github.com/solo-io/gloo/releases/tag/v1.5.7))

##### Helm Changes
- This change stops Upstream, Service, ConfigMap, and Deployment objects from being created for gateway proxies if "disabled = true" under the helm chart values override. (https://github.com/solo-io/gloo/issues/3751)
- added image pull secrets check to all deployments and refactored existing image pull secrets checks (https://github.com/solo-io/gloo/issues/3269)
- (From OSS [v1.5.7](https://github.com/solo-io/gloo/releases/tag/v1.5.7)) This change stops ConfigMap, Service, and Gateway objects from being created for gateway proxies if "disabled = true" under the helm chart values override. (https://github.com/solo-io/gloo/issues/3751)

#### [v1.5.6](https://github.com/solo-io/solo-projects/releases/tag/v1.5.6) (Uses OSS [v1.5.6](https://github.com/solo-io/gloo/releases/tag/v1.5.6))

##### Dependency Bumps
- solo-kit/gloo has been upgraded to v1.5.6.
- (From OSS [v1.5.6](https://github.com/solo-io/gloo/releases/tag/v1.5.6)) solo-kit/solo-io has been upgraded to v0.13.14.

##### Fixes
- (From OSS [v1.5.6](https://github.com/solo-io/gloo/releases/tag/v1.5.6)) Fix EDS so modifying configs on a TLS enabled Upstream no longer results in 503s (https://github.com/solo-io/gloo/issues/3673)
- (From OSS [v1.5.6](https://github.com/solo-io/gloo/releases/tag/v1.5.6)) Gloo will no longer remove upstreams after editing their spec. (https://github.com/solo-io/gloo/issues/3710)
- (From OSS [v1.5.6](https://github.com/solo-io/gloo/releases/tag/v1.5.6)) Fix EDS so modifying health checks on Upstream no longer results in 503s (https://github.com/solo-io/gloo/issues/3219)
- (From OSS [v1.5.6](https://github.com/solo-io/gloo/releases/tag/v1.5.6)) Allow toggling of EDS to rest XDS to avoid the envoy issue described in the following issue: https://github.com/envoyproxy/envoy/issues/13070. Set to true by default starting in version > `v1.6.0` (https://github.com/solo-io/gloo/issues/3805)

##### Helm Changes
- Add resource limits to helm config for all deployments. (https://github.com/solo-io/gloo/issues/2979)

#### [v1.5.5](https://github.com/solo-io/solo-projects/releases/tag/v1.5.5) (Uses OSS [v1.5.5](https://github.com/solo-io/gloo/releases/tag/v1.5.5))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.5.

##### Fixes
- Expose apiserver over HTTPS using custom certs. (https://github.com/solo-io/gloo/issues/3384)
- (From OSS [v1.5.5](https://github.com/solo-io/gloo/releases/tag/v1.5.5)) Allow static upstream endpoints have individual SNI entries (https://github.com/solo-io/gloo/issues/3806)

##### Helm Changes
- (From OSS [v1.5.4](https://github.com/solo-io/gloo/releases/tag/v1.5.4)) Adds a two helm values that can be used to configure all sds/envoy-sidecar container resource usages. (https://github.com/solo-io/gloo/issues/2979)

#### [v1.5.4](https://github.com/solo-io/solo-projects/releases/tag/v1.5.4) (Uses OSS [v1.5.3](https://github.com/solo-io/gloo/releases/tag/v1.5.3))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.3.

##### Fixes
- (From OSS [v1.5.3](https://github.com/solo-io/gloo/releases/tag/v1.5.3)) Fix an issue where ssl configurations across different virtual services may be incorrectly cached if they ssl configurations only differ by ssl-parameters (e.g., min tls version). **After this change, ssl configurations that are only different by ssl parameters must have different sni domains.** Prior to this change, such a configuration would not error but could result in one ssl configuration being selected over another; now an explicit error is recorded on the virtual service. (https://github.com/solo-io/gloo/issues/3776)

#### [v1.5.3](https://github.com/solo-io/solo-projects/releases/tag/v1.5.3) (Uses OSS [v1.5.2](https://github.com/solo-io/gloo/releases/tag/v1.5.2))

##### Fixes
- Correct observability pod to watch all namespaces if none are provided. V1.5 backport. (https://github.com/solo-io/gloo/issues/3060)
- When Proxies are viewed in the Gloo admin UI, render the entire Proxy spec, even if it is compressed. Compression is determined by the presence of the gloo.solo.io/compress annotation on the Proxy. (https://github.com/solo-io/gloo/issues/3718)

#### [v1.5.2](https://github.com/solo-io/solo-projects/releases/tag/v1.5.2) (Uses OSS [v1.5.2](https://github.com/solo-io/gloo/releases/tag/v1.5.2))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.2.

##### Fixes
- No longer let the api-server create a default settings CRD when none is provided. (https://github.com/solo-io/gloo/issues/3677)
- Fix the grpc service names in health checks. This fixes a regression that was introduced in Gloo enterprise v1.5.0-beta8 and v1.4.7. Without this fix, the rate-limit and ext-auth grpc services will fail health checks and go into panic mode (which by default, ignores health checks, so requests still work). (https://github.com/solo-io/gloo/issues/3745)
- (From OSS [v1.5.2](https://github.com/solo-io/gloo/releases/tag/v1.5.2)) Fix the validation API to only return proxies that would be generated by proposed resources if requested. This change means the default behavior matches the kubernetes validation webhook API. By including the top-level value `returnProxies=true` in the json/yaml request to the API, you can signal the endpoint to return the proxies that would be generated (previously, always returning by default). (https://github.com/solo-io/gloo/issues/3613)
- (From OSS [v1.5.2](https://github.com/solo-io/gloo/releases/tag/v1.5.2)) Fix the validation API to return all errors encountered while validating a list of resources, rather than immediately returning on the first unmarshal error encountered for a resource in a list resource. (https://github.com/solo-io/gloo/issues/3610)
- (From OSS [v1.5.2](https://github.com/solo-io/gloo/releases/tag/v1.5.2)) Fix the validation API error reporting to include the resource associated with the error returned. (https://github.com/solo-io/gloo/issues/3610)

#### [v1.5.1](https://github.com/solo-io/solo-projects/releases/tag/v1.5.1) (Uses OSS [v1.5.1](https://github.com/solo-io/gloo/releases/tag/v1.5.1))

##### Fixes
- Expose apiserver over HTTPS using self-signed certs when running in glooMtls mode. (https://github.com/solo-io/gloo/issues/3384)
- Ensure the rest of our docker containers run with user 10101 rather than root (https://github.com/solo-io/gloo/issues/3346)
- With each release, we will additionally be publishing an alternate set of docker containers (tagged as usual but with the "-extended" suffix) that have some additional dependencies built in (e.g., `curl` for debugging). You can deploy these images by setting the helm value `global.image.extended=true`. (https://github.com/solo-io/gloo/issues/3399)
- Implement `AuthConfig` API that allows users to specify a boolean expression to determine how to evaluate auth configs within an auth chain. Previously, each config on an auth config must be authorized for the entire request to be authorized. This remains the default, but now users can additionally specify a boolean expression (the `booleanExpr` field on an auth config) to reference the auth configs and AND/OR/NOT them together to achieve the desired access policy. (https://github.com/solo-io/gloo/issues/3207)
- Prohibit duplicate route names for routes with Basic rate limits configured. (https://github.com/solo-io/gloo/issues/3674)
- Upgrade base image for grpcserver-enovy image (https://github.com/solo-io/gloo/issues/3617)
- Upgrade base image for grpcserver-ui image (https://github.com/solo-io/gloo/issues/3616)
- (From OSS [v1.5.1](https://github.com/solo-io/gloo/releases/tag/v1.5.1)) Ensure the rest of our docker containers run with user 10101 rather than root (https://github.com/solo-io/gloo/issues/3346)
- (From OSS [v1.5.1](https://github.com/solo-io/gloo/releases/tag/v1.5.1)) With each release, we will additionally be publishing an alternate set of docker containers (tagged as usual but with the "-extended" suffix) that have some additional dependencies built in (e.g., `curl` for debugging). You can deploy these images by setting the helm value `global.image.extended=true`. (https://github.com/solo-io/gloo/issues/3399)
- (From OSS [v1.5.1](https://github.com/solo-io/gloo/releases/tag/v1.5.1)) Fixed the max_connection_duration and max_stream_duration settings not being exposed the Gloo API. See https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#config-core-v3-httpprotocoloptions for more details on these settings. (https://github.com/solo-io/gloo/issues/3628)

##### Helm Changes
- Fix helm chart to honor `.Values.settings.replaceInvalidRoutes` value. (https://github.com/solo-io/gloo/issues/3619)
- Add the ability to specify a number of replicas for the redis, rate-limit, apiserver, and observability deployments via helm values. Note: Extra redis replicas still won't scale properly until https://github.com/solo-io/gloo/issues/3269 is addressed. (https://github.com/solo-io/gloo/issues/3262)
- Add the ability to NOT specify a number of replicas for the redis, rate-limit, apiserver, observability and extAuth deployments explicitly in deployments. This prevents issues when using flux and horizontal autoscaling. See https://docs.fluxcd.io/en/1.18.0/faq.html#how-can-i-prevent-flux-overriding-the-replicas-when-using-hpa for more details. (https://github.com/solo-io/gloo/issues/2650)
- (From OSS [v1.5.1](https://github.com/solo-io/gloo/releases/tag/v1.5.1)) Fix helm chart to honor `.Values.settings.replaceInvalidRoutes` value. This change makes the default invalid route behavior match what's documented (disabled by default). To enable again, set `.Values.settings.replaceInvalidRoutes=true` (https://github.com/solo-io/gloo/issues/3619)
- (From OSS [v1.5.1](https://github.com/solo-io/gloo/releases/tag/v1.5.1)) Remove duplicate helm values that are no longer needed to keep hook-created values in helm releases. Backport for v1.5. (https://github.com/solo-io/gloo/issues/3498)

#### [v1.5.0](https://github.com/solo-io/solo-projects/releases/tag/v1.5.0) (Uses OSS [v1.5.0](https://github.com/solo-io/gloo/releases/tag/v1.5.0))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.0.
- solo-io/envoy-gloo-ee has been upgraded to v1.16.0-rc6.

#### [v1.5.0-beta12](https://github.com/solo-io/solo-projects/releases/tag/v1.5.0-beta12) (Uses OSS [v1.5.0-beta26](https://github.com/solo-io/gloo/releases/tag/v1.5.0-beta26))

##### Dependency Bumps
- solo-io/envoy-gloo-ee has been upgraded to 1.16.0-rc5.
- solo-io/gloo has been upgraded to v1.5.0-beta26.

##### Fixes
- Upgrade envoy-gloo-ee version to handle CVE-2020-25017 and CVE-2020-25018 (https://github.com/solo-io/gloo/issues/3687)

##### New Features
- Allow users to specify rate limits at the route level using the Gloo basic rate limit API. (https://github.com/solo-io/gloo/issues/2517)

##### Notes
- _marked as a pre-release due to a regression that will crash Gloo if it has an AWS upstream_

#### [v1.5.0-beta11](https://github.com/solo-io/solo-projects/releases/tag/v1.5.0-beta11) (Uses OSS [v1.5.0-beta25](https://github.com/solo-io/gloo/releases/tag/v1.5.0-beta25))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.0-beta25.

##### Fixes
- Allow skipping the body processing in the WAF filter. (https://github.com/solo-io/gloo/issues/3540)

##### Notes
- _marked as a pre-release due to a regression that will crash Gloo if it has an AWS upstream_

#### [v1.5.0-beta10](https://github.com/solo-io/solo-projects/releases/tag/v1.5.0-beta10) (Uses OSS [v1.5.0-beta22](https://github.com/solo-io/gloo/releases/tag/v1.5.0-beta22))

##### Notes
- This release contained no user-facing changes.

#### [v1.5.0-beta9](https://github.com/solo-io/solo-projects/releases/tag/v1.5.0-beta9) (Uses OSS [v1.5.0-beta20](https://github.com/solo-io/gloo/releases/tag/v1.5.0-beta20))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.0-beta20.

##### Fixes
- The extauth service now supports tls connections to the extauth service itself using a kubernetes secret rather than using cert and key files. To enable extauth tls mode, set `TLS_ENABLED` to true in the extauth service by setting the  helm value `global.extensions.extAuth.tlsEnabled` to `true`. To pull the cert and key from a kubernetes secret, set the helm value `global.extensions.extAuth.secretName` to the name of the tls secret containing the tls.crt and tls.key data. Note that the secret must be in the same namespace as the extauth deployment. (https://github.com/solo-io/gloo/issues/3430)
- Add DNS resolution to Failover Endpoints (https://github.com/solo-io/gloo/issues/3565)
- Allow user to create resources even when running a namespaced version of glooE. (https://github.com/solo-io/solo-projects/issues/1847)
- Fix opentracing causing envoy failure. Adds type-checking for all envoy go-control-plane data structures. (https://github.com/solo-io/gloo/issues/3496)
- Remove the unused Rate Limit feature from the gloo UI. (https://github.com/solo-io/gloo/issues/3484)
- The virtual services list page's table view crashed when there was an error with the virtual service. (https://github.com/solo-io/gloo/issues/3521)

##### Helm Changes
- Can now add custom labels to the deployments added by gloo-E (that aren't from subcharts) (https://github.com/solo-io/gloo/issues/3441)

#### [v1.5.0-beta8](https://github.com/solo-io/solo-projects/releases/tag/v1.5.0-beta8) (Uses OSS [v1.5.0-beta19](https://github.com/solo-io/gloo/releases/tag/v1.5.0-beta19))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.0-beta19.
- solo-io/ext-auth-service has been upgraded to v0.6.15.
- serialize-javascript/gloo-ui has been upgraded to 3.1.0.
- dot-prop/gloo-ui has been upgraded to 4.2.1.

##### Fixes
- Removes crd permissions from the apiserver-ui Role so namespaced glooE can be installed by a namespaced user. (https://github.com/solo-io/gloo/issues/3424)
- Update the version of golang Gloo was built with from 1.14.0 to 1.14.6, to pickup patch fixes to go; most notably, a workaround in go for a bug in affected Linux kernels (5.2.x, 5.3.0-5.3.14, 5.4.0-5.4.1) that could result in a corrupted AVX register and crash Gloo. (https://github.com/solo-io/gloo/issues/3493)
- Display error messages when there is an unexpected error with the ListEnvoyDetails API call. (https://github.com/solo-io/gloo/issues/3512)
- Resolve helm warnings during installation regarding `customReadinessProbe` (https://github.com/solo-io/gloo/issues/3467)
- Allow path overrides in http health checks on a per-host basis. (https://github.com/solo-io/gloo/issues/2821)

##### Helm Changes
- The bootstrap configuration for the Envoy sidecar that handles traffic between the Gloo Enterprise Admin Dashboard and the API server is now exposed as a `ConfigMap` named `default-apiserver-envoy-config`. This `ConfigMap` is installed by default by the Gloo Enterprise Helm chart. Users can provide their own custom bootstrap configuration for the sidecar via the new `apiServer.deployment.envoy.bootstrapConfig.configMapName` Helm value. The value must contain the name of a `ConfigMap` that is present in the same namespace as the `api-server` deployment. This `ConfigMap` must contain the Envoy bootstrap configuration in YAMl format under a `data` entry named `config.yaml`. (https://github.com/solo-io/gloo/issues/3477)
- Add the new helm value `global.extensions.dataplanePerProxy` (default false). When true, Gloo will deploy a set of dataplane resources for each proxy deployment (i.e., gateway/ingress). These resources include the extauth server and rate limit server, as well as their dependent resources. Note that if `dataplanePerProxy` is enabled, that each `Gateway` resource will need to be updated to point to their respective dataplane, via the `gatewayProxies.NAME.gatewaySettings.customHttpGateway` and/or the `gatewayProxies.NAME.gatewaySettings.customHttpsGateway` helm values. (https://github.com/solo-io/gloo/issues/3236)
- Add helm value for rate limit descriptors in settings. (https://github.com/solo-io/gloo/issues/3422)

##### New Features
- Allow adding arbitrary API key secret data to the headers of successfully authorized requests. (https://github.com/solo-io/gloo/issues/3385)
- Allow users to change the name of the header that the Gloo Enterprise external auth server inspects for API keys. (https://github.com/solo-io/gloo/issues/3390)
- The API keys can now be provided as simple Kubernetes secrets. Instead of being nested in a YAML document inside the secret data, the key is now simply the value of the `api-key` data key. This change is backwards compatible, i.e. Gloo will still support existing secrets with the old format. `glooctl create secret apikey` will now generate secrets with the new format. (https://github.com/solo-io/gloo/issues/3472)

#### [v1.5.0-beta7](https://github.com/solo-io/solo-projects/releases/tag/v1.5.0-beta7) (Uses OSS [v1.5.0-beta16](https://github.com/solo-io/gloo/releases/tag/v1.5.0-beta16))

##### Dependency Bumps
- elliptic/elliptic has been upgraded to 4.11.9.
- solo-io/gloo has been upgraded to v1.5.0-beta16.
- solo-io/gloo has been upgraded to v1.5.0-beta14.

##### Fixes
- Correct the upstream name on auto-generated Observability dashboards for manually-added Kubernetes upstreams. (https://github.com/solo-io/gloo/issues/3061)
- An authconfig missing a clientSecretRef will no longer segfault (https://github.com/solo-io/gloo/issues/3358)

##### Helm Changes
- In v1.4.0-beta8 the api-server service was changed from a NodePort service to a ClusterIP service, so that it is not available outside of the cluster. Now the service type is configurable in case users still want to make the service accessible outside the cluster. (https://github.com/solo-io/gloo/issues/3318)

#### [v1.5.0-beta6](https://github.com/solo-io/solo-projects/releases/tag/v1.5.0-beta6) (Uses OSS [v1.5.0-beta12](https://github.com/solo-io/gloo/releases/tag/v1.5.0-beta12))

##### Dependency Bumps
- solo-io/envoy-gloo-ee has been upgraded to v1.15.0-patch1.
- solo-io/gloo has been upgraded to v1.5.0-beta12.

##### Fixes
- Update envoy to support emitting proxy latency timings to access log via dynamic metadata. (https://github.com/solo-io/gloo/issues/3392)

#### [v1.5.0-beta5](https://github.com/solo-io/solo-projects/releases/tag/v1.5.0-beta5)

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.0-beta11.

##### New Features
- Add support for wasm-enabled gloo-ee (https://github.com/solo-io/gloo/issues/3035)

##### Notes
- *This release build failed.* Some images weren't built and pushed properly, do not attempt to use this release.

#### [v1.5.0-beta4](https://github.com/solo-io/solo-projects/releases/tag/v1.5.0-beta4) (Uses OSS [v1.5.0-beta10](https://github.com/solo-io/gloo/releases/tag/v1.5.0-beta10))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.5.0-beta10.

### v1.4.0

#### [v1.4.16](https://github.com/solo-io/solo-projects/releases/tag/v1.4.16) (Uses OSS [v1.4.13](https://github.com/solo-io/gloo/releases/tag/v1.4.13))

##### Dependency Bumps
- solo-io/envoy-gloo-ee has been upgraded to 1.15.1-patch2.

##### Fixes
- Upgrade envoy-gloo version to fix seg fault in aws lambda filter (https://github.com/solo-io/gloo/issues/3684)
- Upgrade envoy-gloo-ee version to handle CVE-2020-25017 and CVE-2020-25018 (https://github.com/solo-io/gloo/issues/3687)

#### [v1.4.15](https://github.com/solo-io/solo-projects/releases/tag/v1.4.15) (Uses OSS [v1.4.13](https://github.com/solo-io/gloo/releases/tag/v1.4.13))

##### Dependency Bumps
- solo-io/ext-auth-service has been upgraded to v0.6.12-patch1.

##### Notes
- _marked as a pre-release due to a regression that will crash Gloo if it has an AWS upstream_

#### [v1.4.14](https://github.com/solo-io/solo-projects/releases/tag/v1.4.14) (Uses OSS [v1.4.13](https://github.com/solo-io/gloo/releases/tag/v1.4.13))

##### Fixes
- Allow skipping the body processing in the WAF filter. (https://github.com/solo-io/gloo/issues/3540)

##### Notes
- _marked as a pre-release due to a regression that will crash Gloo if it has an AWS upstream_

#### [v1.4.13](https://github.com/solo-io/solo-projects/releases/tag/v1.4.13) (Uses OSS [v1.4.12](https://github.com/solo-io/gloo/releases/tag/v1.4.12))

##### Fixes
- The virtual services list page's table view crashed when there was an error with the virtual service. (https://github.com/solo-io/gloo/issues/3521)

#### [v1.4.12](https://github.com/solo-io/solo-projects/releases/tag/v1.4.12) (Uses OSS [v1.4.12](https://github.com/solo-io/gloo/releases/tag/v1.4.12))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.4.12.

#### [v1.4.11](https://github.com/solo-io/solo-projects/releases/tag/v1.4.11) (Uses OSS [v1.4.11](https://github.com/solo-io/gloo/releases/tag/v1.4.11))

##### Fixes
- Removes crd permissions from the apiserver-ui Role so namespaced glooE can be installed by a namespaced user. (https://github.com/solo-io/gloo/issues/3424)
- The extauth service now supports tls connections to the extauth service itself using a kubernetes secret rather than using cert and key files. To enable extauth tls mode, set `TLS_ENABLED` to true in the extauth service by setting the  helm value `global.extensions.extAuth.tlsEnabled` to `true`. To pull the cert and key from a kubernetes secret, set the helm value `global.extensions.extAuth.secretName` to the name of the tls secret containing the tls.crt and tls.key data. Note that the secret must be in the same namespace as the extauth deployment. (https://github.com/solo-io/gloo/issues/3430)

##### Helm Changes
- Fix the multi dataplane per proxy helm functionality (`global.extensions.dataplanePerProxy`, default false) that was introduced in Gloo v1.4.7. Since Gloo v1.4.7, if users provided multiple proxies (not a default install) and `dataplanePerProxy` was false, then the Gloo Enterprise chart would also try to install duplicates of some extauth, ratelimit, and redis resources; this would fail those installations/upgrades. (https://github.com/solo-io/gloo/issues/3516)

#### [v1.4.10](https://github.com/solo-io/solo-projects/releases/tag/v1.4.10) (Uses OSS [v1.4.11](https://github.com/solo-io/gloo/releases/tag/v1.4.11))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.4.11.

#### [v1.4.9](https://github.com/solo-io/solo-projects/releases/tag/v1.4.9) (Uses OSS [v1.4.10](https://github.com/solo-io/gloo/releases/tag/v1.4.10))

##### Fixes
- Display error messages when there is an unexpected error with the ListEnvoyDetails API call. (https://github.com/solo-io/gloo/issues/3512)

##### Helm Changes
- The bootstrap configuration for the Envoy sidecar that handles traffic between the Gloo Enterprise Admin Dashboard and the API server is now exposed as a `ConfigMap` named `default-apiserver-envoy-config`. This `ConfigMap` is installed by default by the Gloo Enterprise Helm chart. Users can provide their own custom bootstrap configuration for the sidecar via the new `apiServer.deployment.envoy.bootstrapConfig.configMapName` Helm value. The value must contain the name of a `ConfigMap` that is present in the same namespace as the `api-server` deployment. This `ConfigMap` must contain the Envoy bootstrap configuration in YAMl format under a `data` entry named `config.yaml`. (https://github.com/solo-io/gloo/issues/3477)

#### [v1.4.8](https://github.com/solo-io/solo-projects/releases/tag/v1.4.8) (Uses OSS [v1.4.10](https://github.com/solo-io/gloo/releases/tag/v1.4.10))

##### Fixes
- Allow path overrides in http health checks on a per-host basis. (https://github.com/solo-io/gloo/issues/2821)

#### [v1.4.7](https://github.com/solo-io/solo-projects/releases/tag/v1.4.7) (Uses OSS [v1.4.9](https://github.com/solo-io/gloo/releases/tag/v1.4.9))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.4.9.

##### Fixes
- Update the version of golang Gloo was built with from 1.14.0 to 1.14.6, to pickup patch fixes to go; most notably, a workaround in go for a bug in affected Linux kernels (5.2.x, 5.3.0-5.3.14, 5.4.0-5.4.1) that could result in a corrupted AVX register and crash Gloo. (https://github.com/solo-io/gloo/issues/3493)

##### Helm Changes
- Add the new helm value `global.extensions.dataplanePerProxy` (default false). When true, Gloo will deploy a set of dataplane resources for each proxy deployment (i.e., gateway/ingress). These resources include the extauth server and rate limit server, as well as their dependent resources. Note that if `dataplanePerProxy` is enabled, that each `Gateway` resource will need to be updated to point to their respective dataplane, via the `gatewayProxies.NAME.gatewaySettings.customHttpGateway` and/or the `gatewayProxies.NAME.gatewaySettings.customHttpsGateway` helm values. (https://github.com/solo-io/gloo/issues/3236)
- Add helm value for rate limit descriptors in settings. (https://github.com/solo-io/gloo/issues/3422)

#### [v1.4.6](https://github.com/solo-io/solo-projects/releases/tag/v1.4.6) (Uses OSS [v1.4.8](https://github.com/solo-io/gloo/releases/tag/v1.4.8))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.4.8.

##### Fixes
- Correct the upstream name on auto-generated Observability dashboards for manually-added Kubernetes upstreams. (https://github.com/solo-io/gloo/issues/3061)
- An authconfig missing a clientSecretRef will no longer segfault (https://github.com/solo-io/gloo/issues/3358)

##### Helm Changes
- In v1.4.0-beta8 the api-server service was changed from a NodePort service to a ClusterIP service, so that it is not available outside of the cluster. Now the service type is configurable in case users still want to make the service accessible outside the cluster. (https://github.com/solo-io/gloo/issues/3318)

#### [v1.4.6-patch2](https://github.com/solo-io/solo-projects/releases/tag/v1.4.6-patch2) (Uses OSS [v1.4.8-patch1](https://github.com/solo-io/gloo/releases/tag/v1.4.8-patch1))

##### Notes
- This release contained no user-facing changes.

#### [v1.4.6-patch1](https://github.com/solo-io/solo-projects/releases/tag/v1.4.6-patch1) (Uses OSS [v1.4.8-patch1](https://github.com/solo-io/gloo/releases/tag/v1.4.8-patch1))

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.4.8-patch1.

#### [v1.4.5](https://github.com/solo-io/solo-projects/releases/tag/v1.4.5) (Uses OSS [v1.4.6](https://github.com/solo-io/gloo/releases/tag/v1.4.6))

##### Dependency Bumps
- solo-io/envoy-gloo-ee has been upgraded to v1.15.0-patch1.
- solo-io/gloo has been upgraded to v1.4.6.

##### Fixes
- Update envoy to support emitting proxy latency timings to access log via dynamic metadata. (https://github.com/solo-io/gloo/issues/3392)

### v1.3.0

#### [v1.3.14](https://github.com/solo-io/solo-projects/releases/tag/v1.3.14) (Uses OSS [v1.3.32](https://github.com/solo-io/gloo/releases/tag/v1.3.32))

##### Dependency Bumps
- solo-io/envoy-gloo-ee has been upgraded to 1.14.5-patch1.

##### Fixes
- Upgrade envoy-gloo version to fix seg fault in aws lambda filter (https://github.com/solo-io/gloo/issues/3684)
- Upgrade envoy-gloo-ee version to handle CVE-2020-25017 and CVE-2020-25018 (https://github.com/solo-io/gloo/issues/3687)

#### [v1.3.13](https://github.com/solo-io/solo-projects/releases/tag/v1.3.13)

##### Dependency Bumps
- solo-io/gloo has been upgraded to v1.3.32.
- solo-io/solo-kit has been upgraded to v0.13.8.



