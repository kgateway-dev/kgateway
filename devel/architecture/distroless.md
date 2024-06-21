# Distroless Support

## What is Distroless ?
Distroless images contain only the application and its runtime dependencies. They do not contain package managers, shells or any other programs that are generally found in a standard Linux distribution.
The use of distroless variants is a standard practice adopted by various open source projects and proprietary applications. Distroless images are very small and reduce the size of the images built. It also improves the signal to noise of scanners (e.g. CVE) by eliminating unnecessary packages and files.

## Why do we provide distroless images ?
Initially, Gloo Gateway was built using alpine as the base image, however due to `musl-libc` library issues in alpine, the base image moved over to ubuntu.
While this move fixed the issue by no longer relying on alpine's `musl-libc` library, the ubuntu images contained libraries and packages that were unnecessary and had troublesome licenses (e.g.: `berkeleydb/lib-db`) that made adoption of these images difficult among the user base.
Rather than managing troublesome alpine-based images for specific users and ubuntu-based images for general use, we decided to support distroless variants of our images and deprecate the alpine ones. This way, users who have restrictions based on licenses that are included in our images can use the distroless variant while others can use the standard image.
> Note: As of now we only support amd64 based images

## How is it configured in Gloo Gateway?
The image variant can be specified by using the `global.image.variant` Helm value. You can choose one of the following variants: 'standard',  and 'distroless. If the value is omitted, 'standard' is used.
The distroless images have the suffix `-distroless` in their respective image tag, such as `quay.io/solo-io/gloo-ee:v1.17.0-distroless`.

## How is it implemented in Gloo Gateway?
The distroless variants are based off the `gcr.io/distroless/base-debian11` [distroless image](https://github.com/GoogleContainerTools/distroless/blob/main/base/README.md#image-contents). This image contains `ca-certificates`, `/etc/passwd`, `/tmp`, `tzdata`, `glibc`, `libssl`, and `openssl` that are required for our application to run. We use the base distroless image and not the static one as some of our components compile with the` CGO_ENABLED=1` flag. Using this flag links the `go` binary with the C libraries  that are present on the container image and provided by `glibc`.
In addition to using distroless as the base image, we add a few packages that are required by our components (for probes, lifecycle hooks, etc.). These are defined in the [distroless/Dockerfile](https://github.com/solo-io/gloo/blob/main/projects/distroless/Dockerfile) that creates the `GLOO_DISTROLESS_BASE_IMAGE` image and [distroless/Dockerfile.utils](https://github.com/solo-io/gloo/blob/main/projects/distroless/Dockerfile.utils) that creates the `GLOO_DISTROLESS_BASE_WITH_UTILS_IMAGE` image.
Each component that supports a distroless variant has its own `Dockerfile.distroless` Dockerfile that defines the additional packages that are required. For example, the gloo [Dockerfile.distroless](https://github.com/solo-io/gloo/blob/main/projects/gloo/cmd/Dockerfile.distroless) copies over the Envoy binary and other libraries that are required by Envoy.
Finally, we use the appropriate customized distroless image (`GLOO_DISTROLESS_BASE_IMAGE` or `GLOO_DISTROLESS_BASE_WITH_UTILS_IMAGE`) as the base image in the Makefile when building our images.
> To ensure that both the distroless and standard variants hold up to the same standard, we run the PRs regression tests against the distroless variant and nightlies against the standard variant of our images.

## Which components have distroless variants built?
Gloo Gateway supports a distroless variant for the following images :
- gloo
- discovery
- gloo-envoy-wrapper
- sds
- certgen
- ingress
- access-logger
- kubectl
