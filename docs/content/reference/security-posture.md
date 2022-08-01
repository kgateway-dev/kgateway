---
title: "Security Posture"
description: Expected release cadence and support of Gloo Edge
weight: 33
---

Review the following information about security posture of Solo's Gloo Edge Envoy extensions. For more information, see the [Envoy threat model](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/security/threat_model).

## About the security posture {#about}

The security posture includes extensions for both the Open Source and Enterprise versions of Gloo Edge. Each extension has the filter name and the the classification of the filter's security posture. The following table describes the security posture values.

| Value | Description |
| ----- | ----------- |
| data_plane_agnostic | Do not expose this extension to data plane attacks, for both untrusted downstreams and upstream services. |
| requires_trusted_downstream_and_upstream | Use this extension only when both the downstream and upstream services are trusted.|
| robust_to_untrusted_downstream | You can use these hardened filters only with untrusted downstream services. Do not use with untrusted upstream services, as these filters assume that the upstream services are trusted.|
| robust_to_untrusted_downstream_and_upstream | You can use these hardened filters with both untrusted downstream and upstream services.|
| unknown | Use these filters with your own security procedures. These filters have an unknown security posture. |

## Security posture for extensions {#posture}

Review the following Open Source and Enterprise security postures for Gloo Edge Envoy extensions. You can also download this [YAML file](../security-posture.yaml).

```yaml
{{< readfile file="/reference/security-posture.yaml">}}
```
