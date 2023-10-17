---
title: FIPS Compliant Data Plane
weight: 80
description: Installing Gloo Edge Enterprise with FIPS-compliant crypto libraries 
---
## Installing FIPS compliant images 
Gloo Edge Enterprise binaries have images available that were built with FIPS-compliant crypto libraries.
These can be installed by setting `global.image.fips=true` via Helm.  
Add the following to your `value-overrides.yaml` file 
```yaml
global:
  image:
    fips: true
```
and use it to override the default values in the Gloo Edge chart with Helm 3
```bash
helm install gloo glooe/gloo-ee --namespace gloo-system \
  -f value-overrides.yaml --create-namespace --set-string license_key=YOUR_LICENSE_KEY
```

### ExtAuth Plugins
If you are building your own ExtAuth plugins, you will need to build those plugins with `goboring` as well. 
Follow the [Building External Auth Plugins](https://docs.solo.io/gloo-edge/latest/guides/dev/writing_auth_plugins/) guide 
and use the value of `FIPS_GO_BUILD_IMAGE` in your docker builds.

## What is FIPS compliance
FIPS-compliant cryptography modules have been certified by the National Institute of Standards and Technology and 
meet the security standards required for use in government settings. Using FIPS-compliant cryptography libraries is a requirement
for getting FIPS certification for your application.

### Caveats
The FIPS-compliant binaries are built with `goboring`, which uses `CGO` to call out to FIPS-compliant crypto libraries. 
This adds overhead to cryptography operations and can complicate cross-compilation. 
If your project does not require FIPS-compliant cryptography, installing FIPS-compliant Gloo Edge is not recommended.

### Validation
During the build and release process, the FIPS-compliant images are validated to ensure they are built with FIPS-compliant crypto libraries. Below are the steps that you can take to validate the images yourself:

1. Download the FIPS-compliant image.
   ```sh
   docker pull quay.io/solo-io/gloo-ee-fips:1.16.0-beta1
   ```
2. Create a container with the image that you downloaded. 
   ```sh
   docker create --name gloo-ee quay.io/solo-io/gloo-ee-fips:1.16.0-beta1
   ```
3. Copy the Gloo binary from the container to your local machine.
   ```sh
   docker cp gloo-ee:/usr/local/bin/gloo .
   ```
4. Download the lastest [goversion](https://github.com/rsc/goversion).
   ```sh
   go install github.com/rsc/goversion@latest
   ```
5. Use the `goversion` CLI to print the cryto libraries that are linked in the Gloo binary. 
   ```sh
   goversion -crypto gloo
   ```

   Example output for standard Gloo Edge images:
   ```
   gloo go1.20.9 (standard crypto)
   ```

   Example output for FIPS-compliant Gloo Edge images:
   ```
   gloo go1.20.9 X:boringcrypto (boring crypto)
   ```
