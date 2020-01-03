---
title: Introduction
weight: 10
---

## Using Gloo

* **Built on Envoy Proxy**: Envoy has emerged as a versatile cloud-native service proxy that can be used to provide a uniform data plane for all application-level traffic. Envoy is feature rich (traffic routing, telemetry collection, circuit breaking, request shadowing, load balancing, service discovery, and many others), and uses a runtime API for dynamic configuration without expensive hot reloading or instability. Gloo builds on this and extends this with function-level routing, discovery, and API Gateway features (see below).

* **Next-generation API gateway** : Gloo provides a long list of API gateway features, including rate limiting, circuit breaking, retries, caching, external authentication and authorization, transformation, service-mesh integration, and security. Getting traffic to your microservices and existing monoliths can be an expensive and complicated matter; Gloo solves these problems by keeping a slim profile, secure and separate control plane, as well as layers into infrastructure you may already have (i.e., Kubernetes, Consul, EC2, etc).

* **Kubernetes ingress controller**: Gloo can function as a feature-rich ingress controller when deployed on Kubernetes and especially simplifies routing capabilities when deployed into public clouds like AWS EKS.

* **Hybrid apps**: Gloo creates applications that route to backends implemented as microservices, serverless functions, and legacy apps. This feature can help users to gradually migrate from their legacy code to microservices and serverless; can let users add new functionalities using cloud-native technologies while maintaining their legacy codebase; can be used in cases where different teams in an organization choose different architectures; and more. See [here](https://www.solo.io/hybrid-app) for more on the Hybrid App paradigm.

* **Service Mesh Ingress**: Service mesh technologies solve problems with service-to-service communications across cloud networks. Problems such as service identity, consistent L7 network telemetry gathering, service resilience, traffic routing between services, as well as policy enforcement (like quotas, rate limiting, etc) can be solved with a service mesh. For a service mesh to operate correctly, it needs a way to get traffic into the mesh. The problems with getting traffic from the edge into the cluster are a bit different from service-to-service problems. Things like edge caching, first-hop security and traffic management, Oauth and end-user authentication/authorization, per-user rate limiting, web-application firewalling, etc are all things an Ingress gateway can and should help with. Gloo solves these problems and complements any service mesh including Istio, Linkerd, Consul Connect, and AWS App Mesh.

## What makes Gloo unique

* **Function-level routing allows integration of legacy applications, microservices and serverless**: Gloo can route
requests directly to _functions_, which can be: a serverless function call (e.g. Lambda, Google Cloud Function, OpenFaaS function, etc.);
an API call on a microservice or a legacy service (e.g. a REST API call, OpenAPI operation, XML/SOAP request etc.);
or publishing to a message queue (e.g. NATS, AMQP, etc.). This unique ability is what makes Gloo the only API gateway
that supports hybrid apps, as well as the only one that does not tie the user to a specific paradigm.

* **Gloo incorporates vetted open-source projects to provide broad functionality**: Gloo supports high-quality features by integrating with top open-source projects, including gRPC, GraphQL, OpenTracing, NATS and more. Gloo's architecture allows rapid integration of future popular open-source projects as they emerge.

* **Full automated discovery lets users move fast**: Upon launch, Gloo creates a catalog of all available destinations, and continuously maintains it up to date. This takes the responsibility for 'bookkeeping' away from the developers, and guarantees that new feature become available as soon as they are ready. Gloo discovers across IaaS, PaaS and FaaS providers, as well as Swagger, gRPC, and GraphQL.

* **Gloo integrates intimately with the user's environment**: with Gloo, users are free to choose their favorite tools for scheduling (such as K8s, Nomad, OpenShift, etc), persistence (K8s, Consul, etcd, etc) and security (K8s, Vault).

## Routing Features

* **Dynamic Load Balancing**: Load balance traffic across multiple upstream services.

* **Health Checks**: Active and passive monitoring of your upstream services.

* **OpenTracing**: Monitor requests using the well-supported OpenTracing standard.

* **Monitoring**: Export HTTP metrics to Prometheus or Statsd.

* **SSL**: Highly customizable options for adding SSL encryption to upstream services with full support for SNI.

* **Transformations**: Add, remove, or manipulate HTTP requests and responses.

* **Automated API Translation**: Automatically transform client requests to upstream API calls using Gloo’s Function Discovery.

* **Command Line Interface**: Control your Gloo cluster from the command line with `glooctl`.

* **Declarative API**: Gloo features a declarative YAML-based API; store your configuration as code and commit it with your projects.

* **Failure Recovery**: Gloo is completely stateless and will immediately return to the desired configuration at boot time.

* **Scalability**: Gloo acts as a control plane for Envoy, allowing Envoy instances and Gloo instances to be scaled independently. Both Gloo and Envoy are stateless.

* **Performance**: Gloo leverages Envoy for its high performance and low footprint.

* **Plugins**: Extendable architecture for adding functionality and integrations to Gloo.

* **Tooling**: Build and Deployment tool for customized builds and deployment options.

* **Events**: Invoke APIs using CloudEvents.

* **Pub/Sub**: Publish HTTP requests to NATS.

* **JSON-to-gRPC transcoding**: Connect JSON clients to gRPC services.

## Supported Platforms

* Kubernetes
* HashiCorp Stack (Vault, Consul, Nomad)
* AWS Lambda
* Knative
* Microsoft Azure Functions
* Google Cloud Platform Functions
