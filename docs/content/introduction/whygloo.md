---
title: "Why Gloo?"
weight: 10
---

Gloo is an open-source, cloud-native API Gateway built on top of Envoy Proxy and runs natively in Kubernetes or in other platforms. Gloo makes it easy to solve your challenges of managing ingress traffic into your application architectures (not just Kubernetes) regardless of where they run. Backend services can be discovered when running or registered in Kubernetes, AWS Lambda, VMs, Terraform, EC2, Consul, et. al. Gloo is so powerful it was also selected to be the [first alternative ingress endpoint for the KNative project](https://knative.dev/docs/install/knative-with-gloo/). Please see the [Gloo announcement](https://medium.com/solo-io/announcing-gloo-the-function-gateway-3f0860ef6600) for more on its origin. 

## Solve difficult cloud-native and hybrid architecture challenges

Microservices make it difficult to understand the application's API. When you break a monolithic system into cooperating services, there is no longer any single, natural, entry point. Gloo implements the [API Gateway pattern](https://microservices.io/patterns/apigateway.html) which helps add shape and structure to your microservices architecture, regardless what technologies or protocols you wish to use internally to your system. Specifically, Gloo solves for things such as:

* Dynamically discovery services 
* Eventually consistent routing configurations
* Hybrid compute platforms
* Traffic management / shaping
* Metric / telemetry collection
* Security
* Rate limiting
* Caching
* API orchestration
* Calling Cloud Functions


## Gloo builds on Envoy proxy the right way

Gloo provides an extensible control plane for Envoy Proxy that allows developers and operators to manage configurations as CRDs (CustomResourceDefinitions) when running Kubernetes or as declarative YAML documents outside of Kubernetes. Gloo uses, and has always used, the Envoy xDS gRPC APIs to dynamically update configuration with deep visibility into proxy health, configuration status, and builds confidence in your deployment of Envoy. The Gloo team is deep experts in Envoy and has built filters to handle caching, calling Amazon Lambda directly, as well as for other protocols like NATS streaming, etc. Lastly, Gloo's control plane is decoupled from the running gateways which gives it the ability to be secured and scaled independently. Please see our blogs on [building a control plane for Envoy](https://medium.com/solo-io/guidance-for-building-a-control-plane-to-manage-envoy-proxy-at-the-edge-as-a-gateway-or-in-a-mesh-badb6c36a2af), specifically the one on tradeoffs around [control plane deployment strategies](https://medium.com/solo-io/guidance-for-building-a-control-plane-for-envoy-part-5-deployment-tradeoffs-a6ef55c06327)

## Stepping stone to Service Mesh

Gloo adds service-mesh capabilities (but is NOT a service mesh in and of itself) to the ingress of your cluster like TLS routing, traffic control, rate limiting, distributed tracing, service-to-service verification, observability and more. Service mesh provides a lot of L7 control but can also be complex to adopt and introduce in your organization. Gloo allows you to iteratively take small steps toward adopting these features, ties in with systems like Flagger for [canary automation](https://docs.flagger.app/usage/gloo-progressive-delivery), and plugs in natively to [service-mesh implementations](../../gloo_integrations/service_mesh/) like Istio, Linkerd or Consul. 