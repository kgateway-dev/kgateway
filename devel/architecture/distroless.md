# Distroless Support

## What is Distroless ?
Distroless images contain only the application and its runtime dependencies. They do not contain package managers, shells or any other programs that are generally found in a standard Linux distribution.
The use of distroless variants is a standard practice adopted by various open source projects and proprietary applications. Distroless images are very small and reduce the size of the images built. It also improves the signal to noise of scanners (e.g. CVE) by eliminating unnecessary packages and files.

## Why do we provide distroless images ?
Initially, Edge was built using alpine as the base image, however due to musl-libc issues in alpine, it was moved over to ubuntu.
While this fixed the issue by no longer relying on alpine's musl-libc, the ubuntu images contained libraries and packages that were unnecessary and had troublesome licenses (eg: berkeleydb/lib-db) that certain users could not adopt.
Rather than managing troublesome alpine based images for certain users and debian based images for general use, we decided to support distroless variants of our images and deprecate the alpine ones. This way users who have restrictions based on licenses included in our images can use the distroless variant while others can use the standard one
> Note: As of now we only support amd64 based images

## How is it configured in Gloo Edge ?
The image variant can be specified via the `global.image.variant` helm value. It can take the values 'standard', 'fips', 'distroless', 'fips-distroless'. It defaults to 'standard'. (The 'fips' and 'fips-distroless' variants are an Enterprise-only feature). This change also consequently deprecated the `global.image.fips` value.
The distroless images have the suffix `-distroless` in their respective image tag. Eg: quay.io/solo-io/gloo-ee:v1.17.0-distroless

## How is it implemented in Gloo Edge ?
The distroless variants are based off the `gcr.io/distroless/base-debian11` [distroless image](https://github.com/GoogleContainerTools/distroless/blob/main/base/README.md#image-contents). This contains ca-certificates, /etc/passwd, /tmp, tzdata, glibc, libssl and openssl that are required for our application to run. We use the base distroless image and not the static one as some of our components compile with the CGO_ENABLED=1 flag. Using this flag links the go binary with the C libraries present on the container image which are provided by glibc.
In addition to using distroless as the base image, we add a few packages that are required by our components (for probes, lifecycle hooks, etc.). These are defined in the [distroless/Dockerfile](https://github.com/solo-io/gloo/blob/main/projects/distroless/Dockerfile) that creates the GLOO_DISTROLESS_BASE_IMAGE and [distroless/Dockerfile.utils](https://github.com/solo-io/gloo/blob/main/projects/distroless/Dockerfile.utils) that creates the GLOO_DISTROLESS_BASE_WITH_UTILS_IMAGE.
Each component that supports a distroless variant has its own `Dockerfile.distroless` Dockerfile that defines the additional packages required. Eg: The gloo [Dockerfile.distroless](https://github.com/solo-io/gloo/blob/main/projects/gloo/cmd/Dockerfile.distroless) copies over the envoy binary and other libraries required by envoy.
Finally, we use the appropriate customized distroless image (GLOO_DISTROLESS_BASE_IMAGE or GLOO_DISTROLESS_BASE_WITH_UTILS_IMAGE) as the base image in the Makefile when building our images.
> To ensure that both the distroless and standard variants hold up to the same standard, we run the PRs regression tests against the distroless variant and nightlies against the standard variant of our images.

## Which components have distroless variants built?
Gloo Edge Enterprise supports a distroless variant for the following images :
- caching-ee
- discovery-ee
- discovery-ee-fips
- ext-auth-plugins
- extauth-ee
- extauth-ee-fips
- gloo-ee
- gloo-ee-envoy-wrapper
- gloo-ee-envoy-wrapper-fips
- gloo-ee-fips
- observability-ee
- rate-limit-ee
- rate-limit-ee-fips
- sds-ee
- sds-ee-fips
- gloo-fed
- gloo-fed-apiserver
- gloo-fed-rbac-validating-webhook
- gloo-federation-console
- gloo-fed-apiserver-envoy
