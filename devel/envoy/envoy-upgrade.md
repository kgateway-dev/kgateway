# Envoy Dependency

## Envoy version

As of v2.3.0, kgateway uses vanilla upstream [envoy](https://github.com/envoyproxy/envoy).
The [release page](https://github.com/envoyproxy/envoy/releases) lists the latest release versions.

## Upgrading

When a new envoy version is released, the following files should be updated:

| File | Update |
|---|---|
| Makefile | Update ENVOY_IMAGE with the new version |
| internal/envoy_modules/Cargo.toml | Update the commit hash or version tag to match the [envoy](https://github.com/envoyproxy/envoy/releases) release commit hash |

then:

``` bash
(cd internal/envoy_modules && cargo update -p envoy-proxy-dynamic-modules-rust-sdk)
```

### go-control-plane

When upgrading envoy to a new minor version, most likely the go-control-plane envoy api module also needs to be updated. Envoy has auto sync job that sync new envoy commits to [go-control-plane](https://github.com/envoyproxy/go-control-plane/actions/workflows/envoy-sync.yaml). The control plane repo recently starts to tag the API with the envoy version as well. So, we can just do:
```
go get github.com/envoyproxy/go-control-plane/envoy@<envoy_version_tag>
go mod tidy
make verify
```

For the go-control-plane core module, usually it's safe to use the latest released version independent of the envoy api version. 

#### Envoy patch releases on a release branch

A release branch that takes an Envoy patch release, such as a security release, often does not need a go-control-plane bump. Check the Envoy release notes anyway: if the release adds an API field or an extension, bump the go-control-plane envoy api module to the matching tag as above.

The controller parses `GatewayParameters` bootstrap overlays with the go-control-plane types it links, then validates the result with the Envoy in `ENVOY_IMAGE`. When go-control-plane is behind that Envoy, an overlay that sets a field or `@type` only the newer Envoy knows fails with `InvalidOverlay`, even though the proxy would accept it. The Gateway keeps its last-good bootstrap, so the failure is safe, but the user cannot set the field through an overlay until a release updates go-control-plane. See [Bootstrap overlays](../architecture/bootstrap-overlays.md).

Create a PR with all the changes. This [PR](https://github.com/kgateway-dev/kgateway/pull/12209) can be used as a reference.
