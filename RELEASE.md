# Kgateway releases

## Rolling `main` builds

Automation is in place to build and publish releases for all commits merged into the `main` branch.

This enables devs and users to have concrete artifacts for testing which contain features and bug fixes which have not yet made it into a patch or minor release.

The version is rolling, based on the next minor version release, e.g. `v2.2.0-main`.

The usable artifacts are pushed to GHCR and visible on the [packages page](https://github.com/orgs/kgateway-dev/packages?repo_name=kgateway).

Typically this will be consumed via the helm charts, and can be used directly, such as:
```bash
helm install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds --version v2.2.0-main --namespace kgateway-system --create-namespace
helm install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway --version v2.2.0-main --namespace kgateway-system --create-namespace
```

## Developer documentation

Please refer to [devel/contributing/releasing.md](devel/contributing/releasing.md).