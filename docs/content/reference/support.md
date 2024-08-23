---
title: "Release Support"
description: Expected release cadence and support of Gloo Gateway
weight: 52
---

Review the following information about supported release versions for Gloo Gateway Enterprise and Gloo Gateway OSS (open source).

## Supported versions

Gloo Gateway Enterprise offers `n-3` patching support for bug and critical security fixes. In other words, the current release and the three previous releases are supported.

| Gloo Gateway | Kubernetes | Envoy | Helm | Istio`†`    |
|------|----------|---------|--------|-------------|
| 1.17.x | 1.25 - 1.29 | v3 xDS API | >= 3.12 | 1.16 - 1.22 |
| 1.16.x | 1.24 - 1.28 | v3 xDS API | >= 3.12 | 1.14 - 1.20 |
| 1.15.x | 1.23 - 1.27 | v3 xDS API | >= 3.11 | 1.13 - 1.18 |
| 1.14.x | 1.23 - 1.25 | v3 xDS API | >= 3.8 | 1.13 - 1.18 |

{{% notice note %}}`†` **Istio versions**: Istio must run on a compatible version of Kubernetes. For example, Istio 1.22 is tested, but not supported, on Kubernetes 1.26. For more information, see the [Istio docs](https://istio.io/latest/docs/releases/supported-releases/). If you want hardened `n-4` versions of Istio for particular requirements such as FIPS, consider using [Gloo Mesh Enterprise](https://www.solo.io/products/gloo-mesh/), which includes ingress gateway and service mesh components.{{% /notice %}}

<!--TO FIND VERSIONS
For 1.17 and later, go to the version branch, such as v1.17.x. In the .github/workflows.env/nightly-tests directory, open the min_versions.env and max_versions.env files. Example on main: https://github.com/solo-io/gloo/tree/main/.github/workflows/.env/nightly-tests -->

## Image variants

For some Gloo Gateway component images, the following image variants are supported. Note that the `fips` and `fips-distroless` image variants are supported for Enterprise only.

* **Standard**: The default image variant provided by Gloo Gateway. The standard variant does not require a tag on the image. 
* **Distroless**: An image tagged with `-distroless` is a slimmed-down distribution with the minimum set of binary dependencies to run the image, for enhanced performance and security. Distroless images do not contain package managers, shells, or any other programs that are generally found in a standard Linux distribution. The use of distroless variants is a standard practice adopted by various open source projects and proprietary applications.
* **FIPS** (Enterprise only): An image tagged with `-fips` complies with National Institute of Standards and Technology (NIST) Federal Information Processing Standards (FIPS), for use cases that require federal information processing capabilities. For example, you might provide a cloud service that runs in a Federal Risk and Authorization Management Program ([FedRAMP](https://www.gsa.gov/technology/government-it-initiatives/fedramp)) regulated environment. In such cases, you can use Gloo Gateway's FIPS images without the need for any additional tooling or CLIs.
* **FIPS and distroless** (Enterprise only): An image tagged with `-fips-distroless` follows the same characteristics of both the distroless and FIPS image variants.

Gloo Gateway supports image variants for the following component images:
- `access-logger`
- `certgen`
- `discovery`
- `gloo`
- `gloo-envoy-wrapper`
- `ingress`
- `kubectl`
- `sds`

You have two options for specifying the variant for a Gloo Gateway image in your Helm values:
* Specify the image variant for all Gloo Gateway components in the `global.image.variant` Helm field. Supported values include `standard`, `distroless`, `fips`, and `fips-distroless`. If unset, the default value is `standard`.
* Specify images for individual components by using variant tags in the `gloo.<component>.deployment.image.tag` field of the component's Helm settings, such as `quay.io/solo-io/gloo-ee:v{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}-distroless`.

## Release cadence

Gloo Gateway Enterprise releases are built on the OSS codebase and typically follow the equivalent Gloo Gateway OSS release. The OSS version is always released as the latest build, while Enterprise version is always released as the first stable build of that version. For example, the latest build of Gloo Gateway OSS is {{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}, while the latest stable build of Gloo Gateway Enterprise is {{< readfile file="static/content/version_gee_latest.md" markdown="true">}}.

Stable builds for both Gloo Gateway Enterprise and OSS are released as minor versions approximately every three months. A stable branch for a minor version, such as 1.14, is tagged from `main`, and stable builds for both Enterprise and OSS are supported from that branch.

## Release development

### Beta release process

New features for Gloo Gateway Enterprise and OSS are always developed on `main`. For Enterprise, new features are often released as `beta` builds of the latest Gloo Gateway Enterprise version. You can use these beta builds to test new features, or wait until the feature is released with the next stable Enterprise minor version. For OSS, new features for the latest version are released as patches off of `main`.

### Stable release process

Development of a quality stable release on `main` typically follows this process:
1. New feature development is suspended on `main`.
2. Release candidates are created, such as `1.14.0-rc1`, `1.14.0-rc2`, and so on.
3. A full suite fo tests is performed for each release candidate. Testing includes all documented workflows, a test matrix of all supported platforms, and more.
4. Documentation for that release is prepared, vetted, and staged.
5. The stable minor version is released.
6. Feature development on `main` is resumed.

## Additional support information

### Kubernetes
Gloo Gateway Enterprise is supported and tested for the latest Kubernetes version and all Kubernetes versions released up to 1 year before the latest version.

### Envoy
Officially, Gloo Gateway Enterprise offers support for `n-1` of Envoy community releases. In specific support situations, fixes can be backported to `n-2` or more without bumping the Envoy minor version. In other words, a fix can be developed based on the code that you deployed within the `n-2` release timeframe.

### Gloo Gateway
New features are not developed on or backported to stable branches. However, critical patches, bug fixes, and documentation fixes are backported to all actively supported branches.


Have other questions? Reach out to us [on slack](https://slack.solo.io) or email [sales@solo.io](mailto:sales@solo.io).
