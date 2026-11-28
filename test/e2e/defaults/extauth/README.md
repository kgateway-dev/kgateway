# ExtAuth Metadata Test Server

This is a minimal gRPC external authorization server used by e2e tests. By
default it allows a request only when Envoy dynamic metadata contains a verified
JWT claim, which proves that kgateway's `metadataContextNamespaces` setting
forwards that metadata to the auth service.

## What It Does

The server implements Envoy's gRPC `Authorization` service. For each `Check`
request it reads `--metadata-namespace` (default
`envoy.filters.http.jwt_authn`) and looks under the `payload` key for
`--claim-key` set to `--claim-value`. A match returns an OK response with
`x-ext-authz-check-result: allow`; otherwise the request is denied with HTTP
403 and a body explaining which claim was missing.

An optional `--require-header=authorization` check makes the fixtures cover
backward compatibility: with `--skip-jwt-metadata=true`, it proves an omitted
`metadataContextNamespaces` leaves gRPC ext-auth before JWT, where the bearer
token is still available.

Unlike the Istio sample ext-auth server, the metadata checks ignore request
headers, so a passing test proves the claim came from dynamic metadata rather
than a spoofable client header.

## Building and Publishing

The image is built and loaded into the kind/k3d cluster automatically by
`make e2e-test`. To build and load only this server:

```bash
make extauth-server-docker kind-load-extauth-server
```

This produces `ghcr.io/kgateway-dev/extauth-metadata-server:0.0.1`.
