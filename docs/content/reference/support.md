---
title: "Release Support"
description: Expected release cadence and support of Gloo Edge
weight: 52
---

Review the following information about supported release versions for Gloo Edge Enterprise and Gloo Edge OSS (open source).

## Supported versions

Gloo Edge Enterprise offers `n-3` patching support for bug and critical security fixes. In other words, the current release and the three previous releases are supported. For example, if the latest stable Gloo Edge Enterprise release is 1.14, then Gloo Edge Enterprise 1.13.x, 1.12.x, and 1.11.x are also supported.

| Gloo Edge | Kubernetes`*` | Envoy | Helm | Istio`†`    |
|------|----------|---------|--------|-------------|
| 1.15.x | 1.23 - 1.27 | v3 xDS API | >= 3.11 | 1.13 - 1.18 |
| 1.14.x | 1.23 - 1.25 | v3 xDS API | >= 3.8 | 1.13 - 1.18 |
| 1.13.x | 1.21 - 1.24 | v3 xDS API | >= 3.0 | 1.11 - 1.15 |
| 1.12.x | 1.21 - 1.24 | v3 xDS API | >= 3.0 | 1.11 - 1.15 |

{{% notice note %}}`†` **Istio versions**: Gloo Edge is tested on Istio 1.11 - 1.12. Istio must run on a compatible version of Kubernetes. For example, you cannot run Istio 1.15 on Kubernetes 1.21. For more information, see the [Istio docs](https://istio.io/latest/docs/releases/supported-releases/). If you want hardened `n-4` versions of Istio for particular requirements such as FIPS, consider using [Gloo Platform](https://www.solo.io/products/gloo-platform/), which includes Gateway and Mesh components.{{% /notice %}}

<!--TO FIND VERSIONS
Go to the branch for the Edge version you want, like 1.11.x. In https://github.com/solo-io/gloo/blob/main/ci/kind/setup-kind.sh, search for CLUSTER_NODE_VERSION to see the max k8s version, and ISTIO_VERSION for max istio version. You will have to ask someone on the team to find out the minimum versions of each for a given Edge release. They do have an [issue](https://github.com/solo-io/gloo/issues/5358) open to run regular tests for min-max though.-->

## Release cadence

Gloo Edge Enterprise releases are built on the OSS codebase and typically follow the equivalent Gloo Edge OSS release. The OSS version is always released as the latest build, while Enterprise version is always released as the first stable build of that version. For example, the latest build of Gloo Edge OSS is {{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}, while the latest stable build of Gloo Edge Enterprise is {{< readfile file="static/content/version_gee_latest.md" markdown="true">}}.

Stable builds for both Gloo Edge Enterprise and OSS are released as minor versions approximately every three months. A stable branch for a minor version, such as 1.14, is tagged from `main`, and stable builds for both Enterprise and OSS are supported from that branch. 

## Release development

### Beta release process

New features for Gloo Edge Enterprise and OSS are always developed on `main`. For Enterprise, new features are often released as `beta` builds of the latest Gloo Edge Enterprise version. You can use these beta builds to test new features, or wait until the feature is released with the next stable Enterprise minor version. For OSS, new features for the latest version are released as patches off of `main`.

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
Gloo Edge Enterprise is supported and tested for the latest Kubernetes version and all Kubernetes versions released up to 1 year before the latest version.

### Envoy
Officially, Gloo Edge Enterprise offers support for `n-1` of Envoy community releases. In specific support situations, fixes can be backported to `n-2` or more without bumping the Envoy minor version. In other words, a fix can be developed based on the code that you deployed within the `n-2` release timeframe. 

### Gloo Edge
New features are not developed on or backported to stable branches. However, critical patches, bug fixes, and documentation fixes are backported to all actively supported branches.


Have other questions? Reach out to us [on slack](https://slack.solo.io) or email [sales@solo.io](mailto:sales@solo.io).
