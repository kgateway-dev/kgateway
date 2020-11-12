---
title: Service Mesh
weight: 10
description: Complement any service mesh including Istio, Linkerd, Consul Connect, and AWS App Mesh.
---

Service mesh technologies solve problems with service-to-service communications across cloud networks. Problems such as service identity, consistent L7 network telemetry gathering, service resilience, traffic routing between services, as well as policy enforcement (like quotas, rate limiting, etc) can be solved with a service mesh. For a service mesh to operate correctly, it needs a way to get traffic into the mesh. The problems with getting traffic from the edge into the cluster are a bit different from service-to-service problems. Things like edge caching, first-hop security and traffic management, Oauth and end-user authentication/authorization, per-user rate limiting, web-application firewalling, etc are all things an Ingress gateway can and should help with. Gloo Edge solves these problems and complements any service mesh including Istio, Linkerd, Consul Connect, and AWS App Mesh.

---

## Gloo Edge integration with service-mesh technology

{{% children description="true" %}}


* [Gloo Edge as ingress for Linkerd](https://linkerd.io/2/tasks/using-ingress/#gloo)

* Consul Connect (Soon to come!)
