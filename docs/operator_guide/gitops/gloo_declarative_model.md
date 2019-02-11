# Gloo as declarative infrastructure

At it's core, Gloo is a simple product that adheres to the declarative infrastructure model: 
- It watches the current state, known as a **snapshot**, consisting of `proxies`, `secrets`, `endpoints`, `upstreams`, and `artifacts`. 
- It runs an event loop that, when the snapshot changes, reconciles it with the current state and applies any necessary changes. 

## GitOps with Gloo

Following the GitOps methodology, custom Gloo configuration can be stored in a version control repo, 
and controlling how that configuration is reviewed, merged, and deployed can help mitigate operational risk. Coming soon, 
Gloo Enterprise will be shipping with a feature that simplifies the design of a GitOps process. With Gloo Enterprise, when 
users make changes in the Gloo UI, they will automatically persist in a changeset that is backed by a Git repository. Then, 
when the change is reviewed and merged in, the configuration will be deployed. 

## Solo Kit, the declarative product generator

Gloo was created using [Solo Kit](https://github.com/solo-io/solo-kit), an open source library that simplifies the creation of declarative products.
A product can simply define it's custom API objects in protobuffer format, and Solo Kit will automatically generate:
- Strongly typed clients for reading and writing those objects (i.e. `upstreams` or `virtualservices`). Solo Kit 
 clients are configured with a pluggable storage layer, and support Kubernetes CRDs, Consul, Vault, and many others out of the box. 
- An event loop that watches a configuration snapshot. Products simply define the object types that make up a snapshot, and the namespaces to watch for config changes.
- API Documentation in markdown format. 

The architecture of solo kit-generated projects has a few advantages: 
- Most of the code is automatically generated, speeding up development time.
- The user interfaces (CLI, enterprise UI) are very simple -- they simply edit yaml configuration. 
- Multiple solo kit products can run as a pipeline, each watching a writing a set of CRDs. For instance, Gloo deploys 
with another service called Discovery, that automatically detects upstreams and endpoints from Kubernetes, AWS, and elsewhere. When those get
written out to the storage layer, Gloo's main event loop now has an updated snapshot including the discovered objects.