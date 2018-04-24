

<h1 align="center">
    <img src="Gloo-01.png" alt="Gloo" width="200" height="242">
  <br>
  The Function Gateway
</h1>


### About gloo:
* [Introduction](introduction/introduction.md): Introduction to Gloo with a basic overview of Gloo itself and its use cases 
* [Concepts](introduction/concepts.md): Explanation of the key concepts used in Gloo.
* [Architecture](introduction/architecture.md): Overview of Gloo's architecture. Covers architecture at a high level, and 
the component architecture
### Installation:
* [Installing on Kubernetes](installation/kubernetes.md): Installation guide for Kubernetes (recommended) 
* [Installing on OpenShift](installation/openshift.md): Installation guide for OpenShift
### Getting Started:
* [Getting Started on Kubernetes](getting_started/kubernetes/1.md): Getting started with Kubernetes (recommended for first time users)
* [Function Routing on Kubernetes](getting_started/kubernetes/2.md): Introduction to Function Routing with Gloo
* [Getting Started on OpenShift](getting_started/openshift/1.md): Getting started with OpenShift
* [Function Routing on OpenShift](getting_started/openshift/2.md): Introduction to Function Routing with Gloo (OpenShift version)
* [AWS Lambda](getting_started/aws/lambda.md): Basic AWS Lambda with Gloo
### Tutorials
* [Refactoring Monoliths with Gloo](tutorials/refactor_monolith.md): Using Gloo to refactor monolithic apps
<!--* [Extending microservices with AWS Lambda](tutorials/extend_microservice.md): Using Gloo to refactor monolithic apps-->
* [Converting webhooks to NATS Messages with Gloo](tutorials/source_events_from_github.md): Using Gloo to convert webhooks to NATS messages for event-driven architectures.
### Plugins:
* [AWS Lambda Plugin](plugins/aws.md): Description of the AWS Lambda Plugin and config rules for AWS Lambda Upstreams and Functions 
* [Kubernetes Plugin](plugins/kubernetes.md): Description of the Kubernetes Plugin and config rules for Kubernetes Upstreams  
* [Service Plugin](plugins/service.md): Description of the Service Plugin and config rules for Service Upstreams
* [Request Transformation Plugin](plugins/request_transformation.md): Description of the Request Transformation Plugin and config rules for Request Transformation Routes and Functions 

### v1 API reference:
* [Upstreams](v1/upstream.md): API Specification for the Gloo Upstream Config Object
* [Virtual](v1/virtualservice.md): API Specification for the Gloo Virtual Service Config Object
* [Metadata](v1/metadata.md): API Specification for Gloo Config Object Metadata
* [Status](v1/status.md): API Specification for Gloo Config Object Status




<!--# Features
- GCF plugin
- Openapi upstream extension
- Route extensions plugin
- Transformation plugin
- Ingress Controller
- Istio controller  + gloo with istio
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