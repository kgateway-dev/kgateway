# Sleeper

The purpose of this simple app is to allow delayed responses. This can be useful for investigating the impact of a long-running request on Envoy's config update behavior.

## Usage

- The query parameter `time` is interpreted as a `time.Duration` value. The server will sleep for this long before responding.

```
curl localhost:8080/?time=1ms
curl localhost:8080/?time=1s
curl localhost:8080/?time=100s
```

- sample route config for use with gloo:
  - after creating this route, you can access it with `curl $(glooctl proxy url)/sleep?time=1s`

```yaml
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /sleep
      routeAction:
        single:
          upstream:
            name: default-sleeper-80
            namespace: gloo-system
```

## Demos

### How does Envoy handle config updates during long-running requests

- Summary: Envoy will continue to service long-running requests even after the config has changed. New requests (issued after the config change) will be handled according to the new configuration.
- This makes sense, because for busy servers, Envoy may never see a moment where there are no active requests. If Envoy had to wait for all requests to drain, it would never be able to route new requests according to the updated config.

#### Steps for demonstrating this behavior

- Apply [config A](#route-config-a)
- Issue [request](#curl-command)
  - expect to wait 10 seconds and then see the response from the sleep server
- In a separate terminal, before 10 seconds elapse, apply [config B](#route-config-b) and re-issue the [request](#curl-command)
  - expect to see the redirect response, from the new config
  - when 10 seconds have elapsed since the first request, expect to see the response from the sleep server

##### Route Config A

```yaml
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /sleep
      routeAction:
        single:
          upstream:
            name: default-sleeper-80
            namespace: gloo-system
```

##### Route Config B

```yaml
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /sleep
      redirectAction:
        hostRedirect: solo.io
```

##### Curl command

```bash
curl -v $(glooctl proxy url)/sleep?time=10s
```
