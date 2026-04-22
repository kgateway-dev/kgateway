# Rustformation

An Envoy dynamic module filter that performs request and response transformation using [MiniJinja](https://docs.rs/minijinja/) templates.

## Overview

`rustformation` implements the [`TransformationPolicy`](../../../../api/v1alpha1/kgateway/traffic_policy_types.go) CRD spec. It is attached to routes as a per-route config and can modify:

- Request/response headers (set, add, remove)
- Request/response bodies (rewrite using a template)
- Create Envoy dynamic metadata (consumed by other filters down the chain or access logs)

The filter buffers the full body before applying any body transformation, unless `parseAs: None` is set, in which case body transformations are skipped and only header transformations run.

WebSocket upgrade requests and HTTP CONNECT tunnels are never buffered — only header transformations apply.

## Json Config Schema

The filter config JSON is directly mapping the `transformation` field in [`TrafficPolicy`](../../../../api/v1alpha1/kgateway/traffic_policy_types.go). The `TransformationPolicy.Request` and `TransformationPolicy.Response` fields map directly to the `request` and `response` keys in this JSON schema.

### Add a header using a request header value

```json
{
  "request": {
    "set": [
      { "name": "x-forwarded-user", "value": "{{ header(\"x-user-id\") }}" }
    ]
  }
}
```

### Rewrite the request body using JSON fields

```json
{
  "request": {
    "body": {
      "parseAs": "AsJson",
      "value": "{ \"user\": \"{{ username }}\" }"
    }
  }
}
```

### Add a response header from a request header

```json
{
  "response": {
    "set": [
      { "name": "x-original-host", "value": "{{ request_header(\"host\") }}" }
    ]
  }
}
```

### Skip body buffering (headers only)

```json
{
  "request": {
    "set": [
      { "name": "x-trace-id", "value": "{{ header(\"x-request-id\") }}" }
    ],
    "body": { "parseAs": "None" }
  }
}
```

### Write to Envoy dynamic metadata

```json
{
  "request": {
    "dynamicMetadata": [
      {
        "namespace": "com.example.auth",
        "key": "user_id",
        "value": { "stringValue": "{{ header(\"x-user-id\") }}" }
      }
    ]
  }
}
```
