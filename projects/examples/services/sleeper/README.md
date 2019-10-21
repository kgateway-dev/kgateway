# Sleeper

The purpose of this simple app is to allow delayed responses. This can be useful for investigating the impact of a long-running request on Envoy's config update behavior.

## Usage

- The query parameter `time` is interpreted as a `time.Duration` value. The server will sleep for this long before responding.

```
curl localhost:8080/?time=1ms
curl localhost:8080/?time=1s
curl localhost:8080/?time=100s
```
