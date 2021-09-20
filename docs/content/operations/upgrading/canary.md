---
title: Canary Upgrade (1.9.0+)
weight: 5
description: Upgrading Gloo Edge with a canary workflow
---

In this guide we will describe the necessary steps to upgrade your Gloo Edge or Gloo Edge Enterprise deployments using
a canary model. This guide assumes the older Gloo Edge version is at least 1.9.0.

{{% notice note %}}

In versions prior to 1.9.0, status reporting on Gloo CRs was not per namespace, thus we had little insight into the
state of a resource from the perspective of each canary installation. Further, the helm value overrides for the xds
service address and xds service port had not yet been added, e.g.:

```yaml
gatewayProxies:
  gatewayProxy: # do the following for each gateway proxy
    xdsServiceAddress: xds-relay.default.svc.cluster.local
    xdsServicePort: 9991
```

In versions prior to 1.8.0, there was a [bug](https://github.com/solo-io/gloo/issues/5030) that prevented canary deploys
from working as the old control plane would crash loop when it saw newly added fields.

{{% /notice %}}

##### Prereqs

Gloo Edge 1.9.0 or later installed.

##### Simple Canary

- Install
  - Install Gloo Edge with the new version to another namespace, e.g. `glooctl install gateway --version 1.9.0 -n gloo-system-canary`.
- Test
  - Test your routes, monitor your metrics, run `glooctl check` on the new installation until you are happy with the installation.
- Validate
  - `gloooctl uninstall -n gloo-system`, or `helm delete` the original installation.

##### Advanced Canary: Separate Control and Data Planes

The recommended installation above installs the full Gloo Edge chart twice for simplicity. This means that there will
be duplicate data planes, which may be expensive if you don't want to roll out duplicate envoy fleets.

One way to avoid this is to install the second canary control plane without any proxies, i.e. override the 
`gatewayProxies` (`gloo.gatewayProxies` in enterprise) helm value with an empty list. Then you can incrementally use
the following helm values per proxy to incrementally upgrade your envoy fleets to the new control plane once satisfied:

```yaml
gatewayProxies:
  gatewayProxy: # do the following for each gateway proxy
    xdsServiceAddress: xds-relay.default.svc.cluster.local # replace with the appropriate value for the canary gloo svc
    xdsServicePort: 9991 # replace with the appropriate value for the canary gloo svc
```

{{% notice note %}}
This will update all envoys in a deployment. If you'd rather test a single envoy first, update the corresponding envoy
bootstrap configmap (`{{ $gatewayProxies.proxyName | kebabcase }}-envoy-config`) to point to the new xds server and
bounce a single envoy pod to test.

{{% /notice %}}

Once updated, we want the new helm release to take ownership of these envoys by updating the following labels as
appropriate (helm 3 only):

- `meta.helm.sh/release-name: <RELEASE_NAME>`
- `meta.helm.sh/release-namespace: <RELEASE_NAMESPACE>`

Finally, make the same update to the new helm release's helm values so your envoy fleet isn't scaled to zero upon the
next upgrade.

##### Appendix: Alternate "Canary" using XDS-Relay

Consider combining these upgrade approaches with [xds-relay]({{< versioned_link_path fromRoot="/operations/production_deployment/#xds-relay" >}}) if you desire extra resiliency.