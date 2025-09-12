# Envoy Dependency

## Envoy version

Currently, kgateway uses a custom build of envoy from [envoy-gloo](https://github.com/solo-io/envoy-gloo).
The [release page](https://github.com/solo-io/envoy-gloo/releases) lists the latest release versions.

## Upgrading

When a new envoy version is released, envoy-gloo will also be updated to use the latest upstream version.

The following files should be updated:

| File | Update |
|---|---|
| Makefile | Update ENVOY_IMAGE with the new version |
| internal/envoyinit/rustformations/Cargo.lock | Update the commit hash to match the [envoy](https://github.com/envoyproxy/envoy/releases) release commit hash |
| internal/envoyinit/rustformations/Cargo.toml | Update the commit hash to match the [envoy](https://github.com/envoyproxy/envoy/releases) release commit hash |
| pkg/validator/validator.go | Update the envoy-gloo image version (search for `envoy-gloo:`) |
| pkg/validator/validator_test.go | Update the envoy-gloo image version  (search for `envoy-gloo:`) |

### go-control-plane

When upgrading envoy to a new minor version, most likely the go-control-plane version also needs to be updated. Envoy has auto sync job that sync new envoy commits to [go-control-plane](https://github.com/envoyproxy/go-control-plane/actions/workflows/envoy-sync.yaml). It seems to only sync commit in main, so
find the commit hash that is closest to the envoy release date and do:

```
go get github.com/envoyproxy/go-control-plane@<commit_hash>
go mod tidy
make verify
```

Create a PR with all the changes. This [PR](https://github.com/kgateway-dev/kgateway/pull/12209) can be used as a reference.
