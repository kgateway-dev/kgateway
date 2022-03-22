---
title: Timeout defaults
weight: 60
description: Quick reference for timeout settings in Gloo Edge resources.
---

Review this page for a list of commonly used timeout settings in Gloo Edge, organized by custom resource. For more information, see the [API reference]({{< versioned_link_path fromRoot="/reference/api/" >}}) for each resource.


## Gateway CRD

- {{< protobuf name="gateway.solo.io.Gateway" display="Gateway" >}}
  - `httpGateway`
    - `options`
      - `httpConnectionManagerSettings` (see also [Envoy HCM](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-msg-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager) and the {{< protobuf name="hcm.options.gloo.solo.io.HttpConnectionManagerSettings" display="Gloo HCM" >}})
        - `idleTimeout` defaults to **1 hour** (see also [Envoy HttpProtocolOptions](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-msg-config-core-v3-httpprotocoloptions))
        - `streamIdleTimeout` defaults to **5 minutes**
        - `requestTimeout` (downstream) disabled by default, **unlimited**
        - `drainTimeout` defaults to **5 seconds**
        - `delayedCloseTimeout` defaults to **1 second**
        - `maxConnectionDuration` disabled by default, **unlimited** (see also [Envoy HttpProtocolOptions](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-msg-config-core-v3-httpprotocoloptions))
        - `maxStreamDuration` disabled by default, **unlimited** (see also [Envoy HttpProtocolOptions](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-msg-config-core-v3-httpprotocoloptions))
      - `dynamicForwardProxy` (see also {{< protobuf name="dfp.options.gloo.solo.io.FilterConfig" display="DynamicForwardProxy" >}})
        - `dnsCacheConfig`
          - `dnsRefreshRate` defaults to **60 seconds** for unresolved DNS hosts, or DNS TTL for resolved hosts
          - `hostTtl` defaults to **5 minutes**
          - `dnsFailureRefreshRate` 
            - `baseInterval` no default value
            - `maxInterval` defaults to **10 times the `baseInterval`**
          - `dnsQueryTimeout` defaults to to the underlying DNS implementation, or **5 seconds** max
  - `tcpGateway`
    - `options`
      - `tcpProxySettings`
        - `idleTimeout` disabled by default, **unlimited**
  - `options`
    - `socketOptions` (see also [Socket Options]({{< versioned_link_path fromRoot="/guides/integrations/aws/socket-options/" >}}))
      - **no downstream keep-alive** probes by default. AWS NLB default timeout is 350 seconds.


## Settings CRD

- {{< protobuf name="gloo.solo.io.Settings" display="Settings" >}}
  - `refreshRate` defaults to **60 seconds**
  - `ratelimitServer`
    - `requestTimeout` defaults to **100ms**
  - `extauth`
    - `requestTimeout` defaults to **200ms**
  - `gloo` (see also {{< protobuf name="gloo.solo.io.GlooOptions" display="GlooOptions" >}})
    - `endpointsWarmingTimeout` defaults to **5 minutes**
    - `awsOptions` (see also {{< protobuf name="gloo.solo.io.Settings" display="AwsOptions" >}} )
      - `credentialRefreshDelay` defaults to **not refreshing** on time period. Suggested is 15 minutes.


## AuthConfig CRD

- {{< protobuf name="enterprise.gloo.solo.io.AuthConfig" display="AuthConfig" >}}
  - `configs`
    - `oauth2`
      - `oidcAuthorizationCode` (see also {{< protobuf name="enterprise.gloo.solo.io.OidcAuthorizationCode" display="OidcAuthorizationCode" >}})
        - `discoveryPollInterval` defaults to **30 minutes**
        - `session`
          - `cookieOptions`
            - `maxAge` defaults to **30 days**
          - `redis`
            - `preExpiryBuffer` defaults to **2 seconds**
      - `accessTokenValidation` (see also {{< protobuf name="enterprise.gloo.solo.io.AccessTokenValidation" display="AccessTokenValidation" >}})
        - `jwt`
          - `remoteJwks`
            - `refreshInterval` defaults to **5 minutes**
        - `cacheTimeout` defaults to **10 minutes**
    - `passThroughAuth` (see also {{< protobuf name="enterprise.gloo.solo.io.PassThroughAuth" display="PassThroughAuth" >}})
      - `grpc`
        - `connectionTimeout` defaults to **5 seconds**
      - `http`
        - `connectionTimeout` defaults to **5 seconds**


## VirtualService CRD

- {{< protobuf name="gateway.solo.io.VirtualService" display="VirtualService" >}}
  - `sslConfig`
    - `transportSocketConnectTimeout` disabled by default, **unlimited** (or limited by connection/idle timeout). Suggested is 10 seconds.
  - `virtualHost`
    - `options` (see also {{< protobuf name="gloo.solo.io.VirtualHostOptions" display="VirtualHostOptions" >}} )
      - `retries`
        - `perTryTimeout` defaults to **15 seconds** (Route timeout)
      - `jwtStaged`
        - `beforeExtAuth`/`afterExtAuth`
          - `providers`
            - `jwks`
              - `remote`
                - `cacheDuration` defaults to **5 minutes**
    - `routes`
      - `options` (see also {{< protobuf name="gloo.solo.io.RouteOptions" display="RouteOptions" >}})
        - `timeout` defaults to **15 seconds**
        - `retries`
          - `perTryTimeout` defaults to **15 seconds** (Route timeout)


## Upstream CRD

- {{< protobuf name="gloo.solo.io.Upstream" display="Upstream" >}}
  - `loadBalancerConfig`
    - `updateMergeWindow` defaults to **1 second**
  - `outlierDetection`
    - `interval` defaults to **10 seconds**
    - `baseEjectionTime` defaults to **30 seconds**
  - `connectionConfig`
    - `connectTimeout` defaults to **5 seconds**
    - `tcpKeepalive` (see also [Envoy core config](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/address.proto#envoy-v3-api-msg-config-core-v3-tcpkeepalive))
      - `keepaliveTime` defaults to OS level configuration. Linux defaults to **2 hours**
      - `keepaliveInterval` defaults to OS level configuration. Linux defaults to **75 seconds**
    - `commonHttpProtocolOptions`
      - `idleTimeout` defaults to **1 hour** 
      - `maxStreamDuration` disabled by default, **unlimited**
  - `healthChecks`
    - `timeout` no default value
    - `interval` no default value
    - `initialJitter` no default value
    - `intervalJitter` no default value
    - `noTrafficInterval` defaults to **60 seconds**
    - `unhealthyInterval` defaults to `interval`'s value
    - `unhealthyEdgeInterval` defaults to `unhealthyInterval`'s value
    - `healthyEdgeInterval` defaults to `interval`'s value

  