---
menuTitle: Delegation
title: Delegating with Route Tables
weight: 10
description: Virtual Services can delegate ownership of configuration to Route Tables, top-level API objects which specify routes for a given domain and path prefix.

---


The Gloo Virtual Service makes it possible to define all routes for a domain on a single configuration resource.

However, condensing all routing config onto a single object can be cumbersome when dealing with a large number of routes.

Gloo provides a feature referred to as *delegation*. 
Delegation allows a complete routing configuration to be assembled from separate config objects. The root config object
*delegates* responsibility to other objects, forming a tree of config objects. 
The tree always has a *Virtual Service* as its root, which delgates to any number of *Route Tables*. 
Route Tables  can further delegate to other Route Tables.

## Motivation

Use cases for delegation include:

- Allowing multiple tenants to own add, remove, and update routes without requiring shared access to the root-level Virtual Service

- Sharing route configuration between Virtual Services

- Simplifying blue-green routing configurations by swapping the target Route Table for a delegated route.

- Simplifying very large routing configurations for a single Virtual Service

- Restricting ownership of routing configuration for a tenant to a subset of the whole Virtual Service.

Using delegation, organizations can *delegate* ownership of routing config to individuals or teams. Those individuals or teams can then further delegate routes to others.

## Config Model

A configuration using Delegation can be understood as a tree.

The root node in the tree is a Virtual Service while every child node is a RouteTable:

{{<mermaid align="left">}}
graph LR;
    vs[Virtual Service <br> <br> <code>*.petclinic.com</code>] -->|delegate <code>/api</code> prefix | rt1(Route Table <br> <br> <code>/api/pets</code> <br> <code>/api/vets</code>)
    
    vs -->|delegate <code>/site</code> prefix | rt2(Route Table <br> <br> <code>/site/login</code> <br> <code>/site/logout</code>)
    
   style vs fill:#0DFF00,stroke:#233,stroke-width:4px

{{< /mermaid >}}

Route Tables can be nested for any level of granularity:

{{<mermaid align="left">}}
graph LR;

    vs[Virtual Service <br> <br> <code>*.petclinic.com</code>] -->|delegate <code>/api</code> prefix | rt1(Route Table <br> <br> <code>/api/pets</code> <br> <code>/api/vets</code>)
    
    rt1  -->|delegate <code>/api/pets/.*</code> prefix | rt3(Route Table <br> <br> <code>/api/pets/.*/records</code> <br> <code>/api/pets/.*/billing</code>)
    
    vs -->|delegate <code>/site</code> prefix | rt2(Route Table <br> <br> <code>/site/login</code> <br> <code>/site/logout</code>)

    rt1  -->|delegate <code>/api/vets</code> prefix | rt4(Route Table <br> <br> <code>GET /api/vets</code> <br> <code>POST /api/vets</code>)
    
   style vs fill:#0DFF00,stroke:#233,stroke-width:4px

{{< /mermaid >}}

Non-delegating routes can be defined at every level of the config tree:

{{<mermaid align="left">}}
graph LR;
    vs[Virtual Service <br> <br> <code>*.petclinic.com</code>] -->|delegate <code>/api</code> prefix | rt1(Route Table <br> <br> <code>/api/pets</code> <br> <code>/api/vets</code>)
    
    vs -->|delegate <code>/site</code> prefix | rt2(Route Table <br> <br> <code>/site/login</code> <br> <code>/site/logout</code>)
    
    vs -->|route <code>/pharmacy</code> | us1(Upstream <br> <br> <code>pharmacy-svc.petstore.cluster.svc.local:80</code>)
    
    rt1 -->|route <code>/api/pets</code> | us2(Upstream <br> <br> <code>pet-svc.petstore.cluster.svc.local:80</code>)
    
     style vs fill:#0DFF00,stroke:#233,stroke-width:4px
     style us1 fill:#f9f,stroke:#333,stroke-width:4px
     style us2 fill:#f9f,stroke:#333,stroke-width:4px
    
    
{{< /mermaid >}}


Routes defined at any level *must* inherit the prefix delegated to them, else Gloo will not consider the config tree valid:

{{<mermaid align="left">}}
graph LR;
    subgraph invalid
    vs[Virtual Service <br> <br> <code>*.petclinic.com</code>] -->|delegate <code>/api</code> prefix | rt1(Route Table <br> <br> <code>/api/v1</code> <br> <code>/v2</code>)
    
     style vs fill:#f54,stroke:#233,stroke-width:4px
    end 

    subgraph valid
    vsValid[Virtual Service <br> <br> <code>*.petclinic.com</code>] -->|delegate <code>/api</code> prefix | rt1Valid(Route Table <br> <br> <code>/api/v1</code> <br> <code>/api/v2</code>)
    
     style vsValid fill:#0DFF00,stroke:#233,stroke-width:4px
    end 
    
{{< /mermaid >}}

Gloo will flatten the non-delegated routes defined in config tree down to a single [`Proxy`]({{< ref "/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk.md" >}}) object, such that:


{{<mermaid align="left">}}
graph LR;

    vs[Virtual Service <br> <br> <code>*.petclinic.com</code>] -->|delegate <code>/api</code> prefix | rt1(Route Table <br> <br> <code>/api/pets</code> <br> <code>/api/vets</code>)
    
    vs -->|route <code>/pharmacy</code> | us1(Upstream <br> <br> <code>pharmacy-svc.petstore.cluster.svc.local:80</code>)
    
    rt1 -->|route <code>/api/pets</code> | us2(Upstream <br> <br> <code>pet-svc.petstore.cluster.svc.local:80</code>)
    
    rt1 -->|route <code>/api/vets</code> | us3(Upstream <br> <br> <code>vet-svc.petstore.cluster.svc.local:80</code>)
    
     style vs fill:#0DFF00,stroke:#233,stroke-width:4px
     style us1 fill:#f9f,stroke:#333,stroke-width:4px
     style us2 fill:#f9f,stroke:#333,stroke-width:4px
     style us3 fill:#f9f,stroke:#333,stroke-width:4px
    
    
{{< /mermaid >}}

Would become:


{{<mermaid align="left">}}
graph LR;

    px{Proxy <br> <br> <code>*.petclinic.com</code>} --> |route <code>/api/pets</code> | us2(Upstream <br> <br> <code>pet-svc.petstore.cluster.svc.local:80</code>)
    
    px -->|route <code>/api/vets</code> | us3(Upstream <br> <br> <code>vet-svc.petstore.cluster.svc.local:80</code>)
    
    px  --> |route <code>/pharmacy</code> | us1(Upstream <br> <br> <code>pharmacy-svc.petstore.cluster.svc.local:80</code>)
    
     style px fill:#0DFFDD,stroke:#333,stroke-width:4px
     style us1 fill:#f9f,stroke:#333,stroke-width:4px
     style us2 fill:#f9f,stroke:#333,stroke-width:4px
     style us3 fill:#f9f,stroke:#333,stroke-width:4px
    
    
{{< /mermaid >}}

## Example Configuration


A complete configuration might look as follows:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'any'
  namespace: 'any'
spec:
  virtualHost:
    domains:
    - 'any.com'
    routes:
    - matchers:
       - prefix: '/a' # delegate ownership of routes for `any.com/a`
      delegateAction:
        name: 'a-routes'
        namespace: 'a'
    - matchers:
       - prefix: '/b' # delegate ownership of routes for `any.com/b`
      delegateAction:
        name: 'b-routes'
        namespace: 'b'
```

* A root-level **VirtualService** which delegates routing to to the `a-routes` and `b-routes` **RouteTables**.
* Routes with `delegateActions` can only use a `prefix` matcher.

```yaml
apiVersion: gateway.solo.io/v1
kind: RouteTable
metadata:
  name: 'a-routes'
  namespace: 'a'
spec:
  routes:
    - matchers:
        # the path matchers in this RouteTable must begin with the prefix `/a/`
       - prefix: '/a/1'
      routeAction:
        single:
          upstream:
            name: 'foo-upstream'

    - matchers:
       - prefix: '/a/2'
      routeAction:
        single:
          upstream:
            name: 'bar-upstream'
```

* A **RouteTable** which defines two routes.

```yaml
apiVersion: gateway.solo.io/v1
kind: RouteTable
metadata:
  name: 'b-routes'
  namespace: 'b'
spec:
  routes:
    - matchers:
        # the path matchers in this RouteTable must begin with the prefix `/b/`
       - regex: '/b/3'
      routeAction:
        single:
          upstream:
            name: 'baz-upstream'
    - matchers:
       - prefix: '/b/c/'
      # routes in the RouteTable can perform any action, including a delegateAction
      delegateAction:
        name: 'c-routes'
        namespace: 'c'

```

* A **RouteTable** which both *defines a route* and *delegates to* another **RouteTable**.


```yaml
apiVersion: gateway.solo.io/v1
kind: RouteTable
metadata:
  name: 'c-routes'
  namespace: 'c'
spec:
  routes:
    - matchers:
       - exact: '/b/c/4'
      routeAction:
        single:
          upstream:
            name: 'qux-upstream'
```

* A RouteTable which is a child of another route table.

The above configuration can be visualized as:

{{<mermaid align="left">}}
graph LR;

    vs[Virtual Service <br> <br> <code>any.com</code>] 
    
    vs -->|delegate <code>/a</code> prefix | rt1(Route Table <br> <br> <code>/a/1</code> <br> <code>/a/2</code>)

    vs -->|delegate <code>/b</code> prefix | rt2(Route Table <br> <br> <code>/b/1</code> <br> <code>/b/2</code>)
    
    rt1 -->|route <code>/a/1</code> | us1(Upstream <br> <br> <code>foo-upstream</code>)
    
    rt1 -->|route <code>/a/2</code> | us2(Upstream <br> <br> <code>bar-upstream</code>)
    
    rt2 -->|route <code>/b/3</code> | us3(Upstream <br> <br> <code>baz-upstream</code>)
    
    rt2 -->|route <code>/b/c</code> | rt3(Route Table <br> <br> <code>/b/c/4</code>)
    
    rt3 -->|route <code>/b/c/4</code> | us4(Upstream <br> <br> <code>qux-upstream</code>)
    
     style vs fill:#0DFF00,stroke:#233,stroke-width:4px
     style us1 fill:#f9f,stroke:#333,stroke-width:4px
     style us2 fill:#f9f,stroke:#333,stroke-width:4px
     style us3 fill:#f9f,stroke:#333,stroke-width:4px
     style us4 fill:#f9f,stroke:#333,stroke-width:4px
    
{{< /mermaid >}}

And would result in the following Proxy:


{{<mermaid align="left">}}
graph LR;

    px{Proxy <br> <br> <code>any.com</code>}
    
    style px fill:#0DFFDD,stroke:#333,stroke-width:4px
    
    px -->|route <code>/a/1</code> | us1(Upstream <br> <br> <code>foo-upstream</code>)
    
    px -->|route <code>/a/2</code> | us2(Upstream <br> <br> <code>bar-upstream</code>)
    
    px -->|route <code>/b/3</code> | us3(Upstream <br> <br> <code>baz-upstream</code>)
    
    px -->|route <code>/b/c/4</code> | us4(Upstream <br> <br> <code>qux-upstream</code>)
    
     style us1 fill:#f9f,stroke:#333,stroke-width:4px
     style us2 fill:#f9f,stroke:#333,stroke-width:4px
     style us3 fill:#f9f,stroke:#333,stroke-width:4px
     style us4 fill:#f9f,stroke:#333,stroke-width:4px
    
{{< /mermaid >}}

## Learn more

Explore Gloo's Routing API in the API documentation:

- {{< protobuf name="gateway.solo.io.VirtualService" display="Virtual Services">}}

- {{< protobuf name="gateway.solo.io.RouteTable" display="Route Tables">}}

Please submit questions and feedback to [the solo.io slack channel](https://slack.solo.io/), or [open an issue on GitHub](https://github.com/solo-io/gloo).


