

<h1 align="center">
    <img src="docs/Gloo-01.png" alt="Gloo" width="200" height="242">
  <br>
  The Function Gateway
</h1>

Gloo is a function gateway built on top of the [Envoy Proxy](https://www.Envoyproxy.io). Gloo provides a unified entry point for access to all services and serverless functions, translating from any interface spoken by a client to any interface spoken by a backend. Gloo aggregates REST APIs and events calls from clients, "glueing" together services in-cluster, out of cluster, across clusters, along with any provider of serverless functions.

What makes Gloo special is its use of function-level routing, which is made possible by the fact that Gloo intimately knows the APIs of the upstreams it routes to. This means that the client and server do not have to speak the same protocol, the same version, or the same language. Users can configure Gloo (or enable automatic discovery services) to make Gloo aware of functional back-ends (such as AWS Lambda, Google Functions, or RESTful services) and enable function-level routing. Gloo features an entirely pluggable architecture, providing the ability to extend its configuration language with plugins which add new types of upstreams and route features.

It is entirely possible to run Gloo as a traditional API gateway, without leveraging function-level capabilities. Gloo can be configured as a fully-featured API gateway, simply by using upstreams that don't support functions.

### About gloo:
* [Introduction](docs/introduction/introduction.md): Introduction to Gloo with a basic overview of Gloo itself and its use cases 
* [Concepts](docs/introduction/concepts.md): Explanation of the key concepts used in Gloo.
* [Architecture](docs/introduction/architecture.md): Overview of Gloo's architecture. Covers architecture at a high level, and 
the component architecture
### Installation:
* [Installing on Kubernetes](docs/installation/kubernetes.md): Installation guide for Kubernetes (recommended)
* [Installing on OpenShift](docs/installation/openshift.md): Installation guide for OpenShift
### Getting Started:
* [Getting Started on Kubernetes](docs/getting_started/kubernetes/1.md): Getting started with Kubernetes (recommended for first time users)
* [Function Routing on Kubernetes](docs/getting_started/kubernetes/2.md): Introduction to Function Routing with Gloo
* [Getting Started on OpenShift](docs/getting_started/openshift/1.md): Getting started with OpenShift
* [Function Routing on OpenShift](docs/getting_started/openshift/2.md): Introduction to Function Routing with Gloo (OpenShift version)
* [AWS Lambda](docs/getting_started/aws/lambda.md): Basic AWS Lambda with Gloo
### Tutorials
* [Refactoring Monoliths with Gloo](docs/tutorials/refactor_monolith.md): Using Gloo to refactor monolithic apps
<!--* [Extending microservices with AWS Lambda](docs/tutorials/extend_microservice.md): Using Gloo to refactor monolithic apps-->
* [Converting webhooks to NATS Messages with Gloo](docs/tutorials/source_events_from_github.md): Using Gloo to convert webhooks to NATS messages for event-driven architectures.

### Plugins:
* [AWS Lambda Plugin](docs/plugins/aws.md): Description of the AWS Lambda Plugin and config rules for AWS Lambda Upstreams and Functions 
* [Kubernetes Plugin](docs/plugins/kubernetes.md): Description of the Kubernetes Plugin and config rules for Kubernetes Upstreams  
* [Service Plugin](docs/plugins/service.md): Description of the Service Plugin and config rules for Service Upstreams
* [Request Transformation Plugin](docs/plugins/request_transformation.md): Description of the Request Transformation Plugin and config rules for Request Transformation Routes and Functions 

### v1 API reference:
* [Upstreams](docs/v1/upstream.md): API Specification for the Gloo Upstream Config Object
* [Virtual](docs/v1/virtualservice.md): API Specification for the Gloo Virtual Service Config Object
* [Metadata](docs/v1/metadata.md): API Specification for Gloo Config Object Metadata
* [Status](docs/v1/status.md): API Specification for Gloo Config Object Status


Blogs & Demos
-----
* [Announcement Blog](https://medium.com/solo-io/announcing-gloo-the-function-gateway-3f0860ef6600)
* [Building hybrid app demo](https://www.youtube.com/watch?time_continue=1&v=ISR3G0CAZM0)


Community
-----
Join us on our slack channel: [https://slack.solo.io/](https://slack.solo.io/)

---

### Thanks

**Gloo** would not be possible without the valuable open-source work of projects in the community. We would like to extend a special thank-you to [Envoy](https://www.envoyproxy.io).



<!--# Features
- GCF plugin
- Openapi upstream extension
- Route extensions plugin
- Transformation plugin
- Ingress Controller
- kubernetes service discovery
- gloo config
  - kubernetes
  - vault secret watcher
  - file
- gloo event plugin / gateway
- gloo-sdk-go
- gloo-sdk-node
- SNI config
- Detailed virtualservice rules
- Detailed upstream rules
- glooctl
- thetool
- function discovery
- building without the tool
- deployment without the tool

- getting started in cluster
- getting started out of cluster no kube
- geting started with istio
- getting started using discovery services
- getting started hybrid app example
- getting started multiplexing example
- getting started event gateway
- architecture
- writing plugins (all different kinds of plugins)
  - plugin stages
# document that we call GetFilters after the other plugins (maybe document the order of everything)
-->