# Introduction

- [What is Gloo?](#what-is-gloo)
- [Using Gloo](#using-gloo)
- [Basic Workflow](#basic-workflow)





<a name="What is Gloo?"></a>

### What is Gloo?


Gloo is a function gateway built on top of the [Envoy Proxy](https://www.Envoyproxy.io). Gloo provides a unified entry point
for access to all services and serverless functions, translating from any interface spoken by a client to any interface
spoken by a backend. Gloo aggregates REST APIs and events calls from clients, "glueing" together services in-cluster, 
out of cluster, across clusters, along with any provider of serverless functions.


What truly makes Gloo special is its use of function-level routing, which is made possible by the fact that Gloo
intimately knows the APIs of the upstreams it routes to. This means that the client and server do not have to speak the
same protocol, the same version, or the same language. Users can configure Gloo (or enable automatic discovery services) 
to make Gloo aware of functional back-ends (such as AWS Lambda, Google Functions, or RESTful services) 
and enable function-level routing. Gloo features an entirely pluggable architecture, providing the ability to extend 
its configuration language with plugins which add new types of upstreams and route features. 


API gateways that exist today route primarily on the basis of "API-to-service". They accept an API call (`GET /users/1234`) 
and route that call to a service (`my-kubernetes-service`). It then becomes the responsiblity of the service to do all 
of the work to handle the specific API request. That means that the client and server must speak the same protocol, the 
same version, the same language. It also means that the gateway must be very dumb. All it knows how to do is take an API 
call and forward it to the right destination. 


Gloo stands apart by intimately knowing the APIs of the upstreams it routes to. Users can configure Gloo 
(or enable automatic discovery services<!--(TODO)-->) to make Gloo aware of functional back-ends (such as AWS Lambda, Google 
Functions, and OpenAPI services) and enable function-level routing. Users can add routes to Gloo which route
to specific functions. 


Gloo features an [entirely pluggable architecture](architecture.md), providing the ability to extend its configuration language with 
plugins<!--(TODO)--> which add new types of upstreams and route features. For the ease of developing & deploying Envoy filters and/or 
Gloo plugins, we have created & open-sourced [TheTool](https://github.com/solo-io/thetool). See thetool's documentation<!--(TODO)--> 
for help getting started writing and building extensions to Gloo. 


It is entirely possible to run Gloo as a traditional API gateway, without leveraging function-level capabilities. Gloo
can be configured as a fully-featured<!--(TODO)--> API gateway, simply by using upstreams that don't support functions.


However, we at [solo.io](solo.io) believe that function level routing will open the door to many new use cases and improve
existing ones. <!--(TODO)-->





<a name="Using Gloo"></a>

### Using Gloo



The "API" of Gloo is accessed through the storage layer (selected by the user). Config objects 
(see the [Gloo v1 API specification](../v1/virtualservice.md)) for Gloo are written by the user in one of the following ways:

- manually writing them to storage (e.g. a file or kubernetes custom resource)
- glooctl <!--(TODO)-->
- discovery services <!--(TODO)-->
- using the [Gloo-storage Go client](https://github.com/solo-io/gloo/pkg/storage) 

Gloo then translates user configuration into the v2 Envoy config language and provides live updates to Envoy via the 
Envoy ADS API. Some of Gloo's features are supported natively by Envoy; others are implemented by custom Envoy filters<!--(TODO)-->.

Gloo is able to provide configuration to Envoy for all filters. Gloo can be extended to configure new types of Envoy filters
through language plugins<!--(TODO)-->. 





<a name="Basic Workflow"></a>

### Basic Workflow

The basic Gloo workflow looks like the following (these can be done in any order):

1. Deploy Gloo (e.g. as a kubernetes pod, docker container, etc. It's just a single go binary that will run anywhere).
2. *Optionally* deploy Gloo discovery services<!--(TODO)--> for automated creation of gloo config.
2. Deploy at least 1 Envoy proxy configured to use Gloo as its ADS service<!--(TODO)-->.
    * *Note: we recommend using TheTool<!--(TODO)--> to automate the above steps for you.*
3. Write some Gloo configuration objects<!--(TODO)-->. At least one [route](../v1/virtualservice.md#Route) and one
[upstream](../v1/upstream.md) are required for Gloo to begin routing.
