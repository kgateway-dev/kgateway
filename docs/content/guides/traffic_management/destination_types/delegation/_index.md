---
menuTitle: Delegation
title: Delegating with Route Tables
weight: 70
description: Delegate ownership of configuration to Route Tables for a given domain and path prefix
---


The Gloo Edge Virtual Service makes it possible to define all routes for a domain on a single configuration resource.

However, condensing all routing config onto a single object can be cumbersome when dealing with a large number of routes.

Gloo Edge provides a feature referred to as *delegation*. Delegation allows a complete routing configuration to be assembled from separate config objects. The root config object *delegates* responsibility to other objects, forming a tree of config objects. The tree always has a *Virtual Service* as its root, which delegates to any number of *Route Tables*. Route Tables can further delegate to other Route Tables.

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


Routes defined at any level *must* inherit the prefix delegated to them, else Gloo Edge will not consider the config tree valid:

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

Gloo Edge will flatten the non-delegated routes defined in config tree down to a single {{< protobuf name="gloo.solo.io.Proxy" display="Proxy">}} object, such that:


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
The `delegateAction` object (which can be defined on routes, both on `VirtualServices` and `RouteTables`) can assume 
one of two forms:

1. `ref`: delegates to a specific route table;
1. `selector`: delegates to all the route tables that match the selection criteria.

In the next two sections we will see examples of both these delegation actions.

### Delegation via direct reference
A complete configuration that uses a `delegateAction` which references specific route tables might look as follows:

A root-level **VirtualService** which delegates routing decisions to the `a-routes` and `b-routes` **RouteTables**. 
Please note that routes with `delegateActions` can only use a `prefix` matcher.

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'example'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - 'example.com'
    routes:
    - matchers:
       - prefix: '/a' # delegate ownership of routes for `example.com/a`
      delegateAction:
        ref:
          name: 'a-routes'
          namespace: 'a'
    - matchers:
       - prefix: '/b' # delegate ownership of routes for `example.com/b`
      delegateAction:
        ref:
          name: 'b-routes'
          namespace: 'b'
```

A **RouteTable** which defines two routes.

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

A **RouteTable** which both *defines a route* and *delegates to* another **RouteTable**.

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
        ref:
          name: 'c-routes'
          namespace: 'c'

```


A RouteTable which is a child of another route table.

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


The above configuration can be visualized as:

{{<mermaid align="left">}}
graph LR;

    vs[Virtual Service <br> <br> <code>example.com</code>] 
    
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

    px{Proxy <br> <br> <code>example.com</code>}
    
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

### Delegation via route table selector
By using a {{< protobuf name="gateway.solo.io.RouteTableSelector" display="RouteTableSelector" >}}, a route can delegate to multiple route tables. 
You can specify three types of selection criteria (`labels` and `expressions` cannot be used together):

1. `labels`: if present, Gloo Edge will select route tables whose labels match the specified ones;
1. `expressions`: if present, Gloo Edge will select according to the expression (adhering to the same semantics as [kubernetes label selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#equality-based-requirement)).
An example config for this selection model follows [here](#route-table-selector-expression);
1. `namespaces`: if present, Gloo Edge will select route tables in these namespaces. If omitted, Gloo Edge will only select route 
tables in the same namespace as the resource (Virtual Service or Route Table) that owns this selector. The reserved 
value `*` can be used to select Route Tables in all namespaces watched by Gloo Edge.


{{% notice warning %}}
If a `RouteTableSelector` matches multiple route tables and the route tables do not specify different `weights`, 
Gloo Edge will sort the routes which belong to those tables to avoid short-circuiting (e.g. having a route with a `/foo` 
prefix matcher coming before a route with a `/foo/bar` one). The sorting occurs by descending specificity: 
routes with longer paths will come first, and in case of equal paths, precedence will be given to the route that defines 
the more restrictive matchers. The algorithm used for sorting the routes can be found 
[here](https://github.com/solo-io/gloo/blob/v1.3.2/projects/gloo/pkg/utils/sort_routes.go#L23).
In this scenario, Gloo Edge will also alert the user by adding a warning to the status of the parent resource (the one that 
specifies the `RouteTableSelector`).

Please see the [Route Table weight](#route-table-weight) section below for more information about how to control 
the ordering of your delegated routes.
{{% /notice %}}

A complete configuration might look as follows:

A root-level **VirtualService** which delegates routing decisions to any **RouteTables** in the `gloo-system` namespace that 
contain the `domain: example.com` label.

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'example'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - 'example.com'
    routes:
    - matchers:
       - prefix: '/' # delegate ownership of all routes for `example.com`
      delegateAction:
        selector:
          labels:
            domain: example.com
          namespaces:
          - gloo-system
```

Two **RouteTables** which match the selection criteria:

```yaml
apiVersion: gateway.solo.io/v1
kind: RouteTable
metadata:
  name: 'a-routes'
  namespace: 'gloo-system'
  labels:
    domain: example.com
spec:
  weight: 20
  routes:
    - matchers:
        # the path matchers in this RouteTable can begin with any prefix
       - prefix: '/a'
      routeAction:
        single:
          upstream:
            name: 'foo-upstream'
```

```yaml
apiVersion: gateway.solo.io/v1
kind: RouteTable
metadata:
  name: 'a-b-routes'
  namespace: 'gloo-system'
  labels:
    domain: example.com
spec:
  weight: 10
  routes:
    - matchers:
        # the path matchers in this RouteTable can begin with any prefix
       - regex: '/a/b'
      routeAction:
        single:
          upstream:
            name: 'bar-upstream'

```

The above configuration can be visualized as:

{{<mermaid align="left">}}
graph LR;

    vs[Virtual Service <br> <br> <code>example.com</code>] 
    
    vs -->|delegate <code>/</code> prefix | rt1(Route Table <br> <br> <code>/a</code>)

    vs -->|delegate <code>/</code> prefix | rt2(Route Table <br> <br> <code>/a/b/</code>)
    
    rt1 -->|route <code>/a</code> | us1(Upstream <br> <br> <code>foo-upstream</code>)
    
    rt2 -->|route <code>/a/b</code> | us3(Upstream <br> <br> <code>bar-upstream</code>)
    
     style vs fill:#0DFF00,stroke:#233,stroke-width:4px
     style us1 fill:#f9f,stroke:#333,stroke-width:4px
     style us3 fill:#f9f,stroke:#333,stroke-width:4px
    
{{< /mermaid >}}

And would result in the following Proxy:

{{<mermaid align="left">}}
graph LR;

    px{Proxy <br> <br> <code>example.com</code>}
    
    style px fill:#0DFFDD,stroke:#333,stroke-width:4px
    
    px -->|route <code>/a/b</code> | us1(Upstream <br> <br> <code>bar-upstream</code>)
    
    px -->|route <code>/a</code> | us2(Upstream <br> <br> <code>foo-upstream</code>)
    
     style us1 fill:#f9f,stroke:#333,stroke-width:4px
     style us2 fill:#f9f,stroke:#333,stroke-width:4px
    
{{< /mermaid >}}

#### Route Table Selector Expression

To allow for flexible route table label matching in more complex architecture scenarios, the `expressions` attribute of the `delegateAction.selector` field can be used as such:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'example'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - 'example.com'
    routes:
    - matchers:
       - prefix: '/'
      delegateAction:
        selector:
          # 'expressions' and 'labels' cannot coexist within a single selector
          expressions:
            - key: domain
              operator: In
              values:
                - example.com
            # Do not include route tables exposing the 'shouldnotexist' label
            - key: shouldnotexist
              operator: !
          namespaces:
          - gloo-system
```

{{% notice warning %}}
Note that candidate route tables must match **all** selector expressions (logical AND) to be selected.
{{% /notice %}}

#### Route Table weight
As you might have noticed, we specified a `weight` attribute on the above route tables. This attribute can be used 
to determine the order in which the routes will appear on the final `Proxy` resource when multiple route tables match 
a `RouteTableSelector`. The field is optional; if no value is specified, the `weight` defaults to 0 (zero). 
Gloo Edge will process the route tables matched by a selector in ascending order by weight and collect the routes of each 
route table in the order they are defined. 

In the above example, we want the `/a/b` route to come before the `/a` route, to avoid the latter one short-circuiting 
the former; hence, we set the weight of the `a-b-routes` table to `10` and the weight of the `a-routes` table to `20`.
As you can see in the diagram above, the resulting `Proxy` object defines the routes in the desired order.

#### Matcher restrictions
The Gloo Edge route delegation model imposes some restrictions on the virtual service and parent route table's
matchers (i.e., any resource delegating routing config to another route table). Most notably, parent matchers must have
only a single prefix matcher. Further, any parent matcher must have its own prefix start with the same prefix as _its_
parent (if any).

Further, in versions prior to **Gloo Edge 1.5.0-beta21**, parent matchers cannot use header, query parameter, or method matchers.
In more recent Gloo Edge versions, parent matchers can now use those matchers so long as their child route tables have
headers, query parameters, and methods that are a superset of those defined on the parent. In Gloo Edge versions
**Gloo Edge 1.5.0-beta25** and higher, the `inheritableMatchers` boolean field was added to virtual services and route
tables, which allows users to enable matcher inheritance for non-path matchers from parents (rather than requiring the
whole subset enumerated on any chlidren).

In all versions of Gloo Edge, the leaf route table can use any kind of path matcher, so long as it begins with the same prefix
as its parent.

## Learn more

Explore Gloo Edge's Routing API in the API documentation:

- {{< protobuf name="gateway.solo.io.VirtualService" display="Virtual Services">}}

- {{< protobuf name="gateway.solo.io.RouteTable" display="Route Tables">}}

Please submit questions and feedback to [the solo.io slack channel](https://slack.solo.io/), or [open an issue on GitHub](https://github.com/solo-io/gloo).


