---
title: "Release Support"
weight: 52
---


The following documents the expected release cadence and support of both Gloo EE (enterprise) and Gloo OSS (opensource). In general, Gloo EE releases will follow from the Gloo OSS releases (ie, Gloo EE 1.3 would come from Gloo OSS 1.3 code base). In other words, the community will always see the "latest releases" while enterprise would always see the first stable release of that version. 


## Stable releases every three months

For both GlooEE (enterprise) and open-source Gloo, we release stable builds approximately every three months. These builds will be tagged from `master` and will bump the minor version (ie, `1.3` to `1.4`). Support for both Gloo EE and Gloo OSS will be provided on stable branches of these releases. 

## Support will be release N through N-2

When stable branches are created for a particular release, i.e. a Gloo EE release, we'll support both EE and OSS from that branch. We will support the current release as well as the two previous releases. For example, if Gloo 1.5 is the latest EE release, we will be releasing 1.6.x from master (OSS) but supporting EE/OSS 1.5.x, 1.4.x, and 1.3.x. Support in this context means bug fixes and critical security fixes. Gloo EE customers will always have priority-fix support from Solo.io, but the OSS project will eventually get those fixes as well as whatever else the community contributes.

Gloo EE customers can purchase additional N-x support.

## New features developed on master

New features for Gloo EE/OSS will always be developed on `master`. If a Gloo EE customer wants a new feature, it's expected they will test a release candidate with that feature or wait for a stable release. For EE, these can be released as `beta` releases of Gloo EE. For Gloo OSS these would be released as patch releases off master for the latest version of Gloo OSS.

## Stable release process

To create a quality stable release on `master` the process looks like this:

* Suspend new feature development on `master`
* Begin release candidates and performing full suite of testing (`1.4.0-rc1`, `1.4.0-rc2` ... `1.4.0`)
* Testing includes all documented workflows, test matrix of all supported platforms, etc
* Preparing/vetting/staging the documentation for that release
* Resume feature development after the stable release



## Additional support information

### Kubernetes 
We officially support and test 1 year of previous versions of Kubernetes, however, we expect customers to run older versions and for Gloo EE would provide ability to do so. As we add new features to Gloo, we are mindful of what Kubernetes features we leverage so as not to raise the minimum required version. Currently our backward compatibility target is Kubernetes 1.11.

### Envoy
Officially, Envoy community releases are supported only N-1 (one prior release). We have the ability to back port to N-2 or more, etc in support situations without bumping the Envoy version (ie, we will fix based on the code you have deployed within the N-2 window). 

### Gloo
We will not do new feature development (or backport to) on stable branches. Additionally, we will backport critical patches, bug fixes, and documentation fixes on all actively supported branches.


Please reach out to us [on slack](https://slack.solo.io) or email [sales@solo.io](mailto:sales@solo.io) for any questions 
