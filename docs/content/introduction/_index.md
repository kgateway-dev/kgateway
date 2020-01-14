---
title: Introduction
weight: 10
---

## What is Gloo?

Gloo is a feature-rich, Kubernetes-native ingress controller, and next-generation API gateway. Gloo is exceptional in its function-level routing; its support for legacy apps, microservices and serverless; its discovery capabilities; its numerous features; and its tight integration with leading open-source projects. Gloo is uniquely designed to support hybrid applications, in which multiple technologies, architectures, protocols, and clouds can coexist.

![Gloo Architecture]({{% versioned_link_path fromRoot="/img/gloo-architecture-envoys.png" %}})

---

## Why Gloo?

Gloo makes it easy to solve your challenges of managing ingress traffic into your application architectures (not just Kubernetes) regardless of where they run. Backend services can be discovered when running or registered in Kubernetes, AWS Lambda, VMs, Terraform, EC2, Consul, et. al. Gloo is so powerful it was also selected to be the [first alternative ingress endpoint for the KNative project](https://knative.dev/docs/install/knative-with-gloo/). Please see the [Gloo announcement](https://medium.com/solo-io/announcing-gloo-the-function-gateway-3f0860ef6600) for more on its origin. 

* **Solve difficult cloud-native and hybrid challenges**: Microservices make understanding an application's API difficult. Gloo implements the [API Gateway pattern](https://microservices.io/patterns/apigateway.html) to add shape and structure to your architecture.

* **Build on Envoy proxy the right way**: Gloo is the decoupled control plane for Envoy Proxy enabling developers and operators to dynamically update Envoy using the xDS gRPC APIs in a declarative format. Please see our blogs on [building a control plane for Envoy](https://medium.com/solo-io/guidance-for-building-a-control-plane-to-manage-envoy-proxy-at-the-edge-as-a-gateway-or-in-a-mesh-badb6c36a2af) and [control plane deployment strategies.](https://medium.com/solo-io/guidance-for-building-a-control-plane-for-envoy-part-5-deployment-tradeoffs-a6ef55c06327)

* **Stepping stone to Service Mesh**: Gloo adds service-mesh capabilities to your cluster ingress without being a service mesh itself. Gloo allows you to iteratively take small steps towards advanced features and ties in with systems like Flagger for [canary automation](https://docs.flagger.app/usage/gloo-progressive-delivery), and plugs in natively to [service-mesh implementations](../../gloo_integrations/service_mesh/) like Istio, Linkerd or Consul.

* **Integration of legacy applications**: Gloo can route requests directly to _functions_, an API call on a microservice or a legacy service, or publishing to a message queue. This unique ability makes Gloo the only API gateway supporting hybrid apps without tying the user to a specific paradigm.

* **Incorporate vetted open-source projects for broad functionality**: Gloo support high-quality features by integrating with top open-source projects, including gRPC, GraphQL, OpenTracing, NATS and more. Gloo's architecture allows rapid integration of future popular open-source projects as they emerge.

* **Fully automated discovery lets users move fast**: Upon launch, Gloo creates a catalog of all available destinations, and continuously maintains it up to date. Gloo discovers across IaaS, PaaS and FaaS providers, as well as Swagger, gRPC, and GraphQL.

* **Integration with existing tools**: with Gloo, users are free to choose their favorite tools for scheduling (such as K8s, Nomad, OpenShift, etc), persistence (K8s, Consul, etcd, etc) and security (K8s, Vault).

---

## Next Steps

* [Getting Started]({{% versioned_link_path fromRoot="/getting_started/" %}}): Get started with your own Gloo deployment

{{% children description="true" %}}
