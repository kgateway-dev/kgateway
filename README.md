

<h1 align="center">
    <img src="docs/Gloo-01.png" alt="Gloo" width="200" height="242">
  <br>
  The Hybrid Application Gateway
</h1>

### What is Gloo?

Gloo is a high-performance, plugin-extendable, platform-agnostic Hybrid Application Gateway built on top of Envoy. Gloo is designed for microservice, monolithic, and serverless applications. By employing function-level routing, Gloo can completely decouple client APIs from upstream APIs at the routing level. Gloo serves as an abstraction layer between clients and upstream services, allowing front-end teams to work independently of teams developing the microservices their apps connect to.

<BR>
<p align="center">
<img src="docs/introduction/high_level_architecture.png" alt="Gloo" width="676" height="400">
</p>    
<BR>
    
## Features

**Microservice Platform Integrations with Service Discovery**:
* Kubernetes
* OpenShift
* HashiCorp Stack (Vault, Consul, Nomad)
* Cloud Foundry

**Serverless Platform Integrations with Function Discovery**:
* AWS Lambda
* Microsoft Azure Functions
* Google Cloud Platform Functions
* Fission
* OpenFaaS
* ProjectFn

**Routing Features**:
* **Dynamic Load Balancing**: Load balance traffic across multiple upstream services.
* **Health Checks**: Active and passive monitoring of your upstream services.
* **OpenTracing**: Monitor requests using the well-supported OpenTracing standard
* **Monitoring**: Export HTTP metrics to Prometheus or Statsd
* **SSL**: Highly customizable options for adding SSL encryption to upstream services with full support for SNI.
* **Transformations**: Add, remove, or manipulate HTTP requests and responses.
* **Automated API Translation**: Automatically transform client requests to upstream API calls using Gloo’s Function Discovery
* **CLI**: Control your Gloo cluster from the command line.
* **Declarative API**: Gloo features a declarative YAML-based API; store your configuration as code and commit it with your projects.
* **Failure Recovery**: Gloo is completely stateless and will immediately return to the desired configuration at boot time.
* **Scalability**: Gloo acts as a control plane for Envoy, allowing Envoy instances and Gloo instances to be scaled independently. Both Gloo and Envoy are stateless.
* **Performance**: Gloo leverages Envoy for its high performance and low footprint.
* **Plugins**: Extendable architecture for adding functionality and integrations to Gloo.
* **Tooling**: Build and Deployment tool for customized builds and deployment options
* **Events**: Invoke APIs using CloudEvents.
* **Pub/Sub**: Publish HTTP requests to NATS
* **JSON-to-gRPC transcoding**: Connect JSON clients to gRPC services

## Documentation

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
