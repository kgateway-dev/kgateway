# Envoy Dependency

## Envoy version

As of v2.3.0, kgateway uses vanilla upstream [envoy](https://github.com/envoyproxy/envoy).
The [release page](https://github.com/envoyproxy/envoy/releases) lists the latest release versions.

## Upgrading

When a new envoy version is released, the following files should be updated:

| File | Update |
|---|---|
| Makefile | Update ENVOY_IMAGE with the new version |
| internal/envoyinit/rustformations/Cargo.toml | Update the commit hash or version tag to match the [envoy](https://github.com/envoyproxy/envoy/releases) release commit hash |

then:

``` bash
(cd internal/envoyinit && cargo update -p envoy-proxy-dynamic-modules-rust-sdk)
```

### go-control-plane

When upgrading envoy to a new minor version, most likely the go-control-plane envoy api module also needs to be updated. Envoy has auto sync job that sync new envoy commits to [go-control-plane](https://github.com/envoyproxy/go-control-plane/actions/workflows/envoy-sync.yaml). The control plane repo recently starts to tag the API with the envoy version as well. So, we can just do:
```
go get github.com/envoyproxy/go-control-plane/envoy@<envoy_version_tag>
go mod tidy
make verify
```

For the go-control-plane core module, usually it's safe to use the latest released version independent of the envoy api version. 

Create a PR with all the changes. This [PR](https://github.com/kgateway-dev/kgateway/pull/12209) can be used as a reference.
