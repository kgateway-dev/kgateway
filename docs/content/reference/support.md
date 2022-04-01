---
title: "Release Support"
description: Expected release cadence and support of Gloo Edge
weight: 52
---

Review the following information about supported release versions for Gloo Edge Enterprise and Gloo Edge OSS (open source).

## Supported versions

Gloo Edge Enterprise offers `n-3` patching support for bug and critical security fixes. In other words, the current release and the three previous releases are supported. For example, if the latest stable Gloo Edge Enterprise release is 1.11, then Gloo Edge Enterprise 1.10.x, 1.9.x, and 1.8.x are also supported.

| Gloo Edge | Kubernetes | Envoy | Helm | Istio |
|------|----------|---------|--------|------|
| 1.11.x | 1.19 - 1.22 | v3 xDS API | >= 3.0 | 1.7 - 1.11 |
| 1.10.x | 1.19 - 1.23 | v3 xDS API | >= 3.0 | 1.7 - 1.11 |
| 1.9.x | 1.19 - 1.22 | v3 xDS API | >= 3.0 | 1.7 - 1.11 |
| 1.8.x | 1.19 - 1.21 | v3 xDS API | >= 3.0 | 1.7 - 1.8 |

## Release cadence

Gloo Edge Enterprise releases are built on the OSS codebase and typically follow the equivalent Gloo Edge OSS release. The OSS version is always released as the latest build, while Enterprise version is always released as the first stable build of that version. For example, the latest build of Gloo Edge OSS is {{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}, while the latest stable build of Gloo Edge Enterprise is {{< readfile file="static/content/version_gee_latest.md" markdown="true">}}.

Stable builds for both Gloo Edge Enterprise and OSS are released as minor versions approximately every three months. A stable branch for a minor version, such as 1.10, is tagged from `master`, and stable builds for both Enterprise and OSS are supported from that branch. 

## Release development

### Beta release process

New features for Gloo Edge Enterprise and OSS are always developed on `master`. For Enterprise, new features are often released as `beta` builds of the latest Gloo Edge Enterprise version. You can use these beta builds to test new features, or wait until the feature is released with the next stable Enterprise minor version. For OSS, new features for the latest version are released as patches off of `master`.

### Stable release process

Development of a quality stable release on `master` typically follows this process:
1. New feature development is suspended on `master`.
2. Release candidates are created, such as `1.10.0-rc1`, `1.10.0-rc2`, and so on.
3. A full suite fo tests is performed for each release candidate. Testing includes all documented workflows, a test matrix of all supported platforms, and more.
4. Documentation for that release is prepared, vetted, and staged.
5. The stable minor version is released.
6. Feature development on `master` is resumed.

## Additional support information

### Kubernetes 
Gloo Edge Enterprise is supported and tested for the latest Kubernetes version and all Kubernetes versions released up to 1 year before the latest version.

### Envoy
Officially, Gloo Edge Enterprise offers support for `n-1` of Envoy community releases. In specific support situations, fixes can be backported to `n-2` or more without bumping the Envoy minor version. In other words, a fix can be developed based on the code that you deployed within the `n-2` release timeframe. 

### Gloo Edge
New features are not developed on or backported to stable branches. However, critical patches, bug fixes, and documentation fixes are backported to all actively supported branches.


Have other questions? Reach out to us [on slack](https://slack.solo.io) or email [sales@solo.io](mailto:sales@solo.io).