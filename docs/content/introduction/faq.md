---
title: FAQs
weight: 80
description: Frequently Asked Questions
---

## Gloo Edge API Gateway

This section covers some of the high-level questions we commonly see in community meetings, on our [public slack](https://slack.solo.io), or on [GitHub issues](https://github.com/solo-io/gloo/issues). Gloo Edge is an API Gateway built on top of [Envoy Proxy](https://envoyproxy.io) that comes with a simple yet powerful control plane for managing Envoy as an edge ingress, API Gateway, or service proxy. Gloo Edge's control plane is built on a plugin model that enables extension and customization depending on your environment and comes with an out of the box Discovery plugin that can discover services running VMs, registered in Consul, running in Kubernetes, or deployed on a public cloud including Functions running in a Cloud Functions environment.  The Envoy community moves fast and no two operational environments are identical, so we built Gloo Edge with this flexibility in mind.

#### What are Gloo Edge's primary use cases?

Gloo Edge was built to support the difficult challenges of monolith to microservice migration, which includes being able to "gloo" multiple types of compute resources (those running on VMs/monoliths with those running on containers and Kubernetes with those running on cloud/on-prem FaaS) as well as security and observability domains. Operational environments are always heterogeneous and Gloo Edge bridges that world to provide "hybrid integration".

Other use cases Gloo Edge can solve:

* Kubernetes cluster Ingress (supporting both [Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/) as well as a [more powerful API]({{< versioned_link_path fromRoot="/reference/api" >}}))
* API Gateway functionality running *outside* Kubernetes
* GraphQL endpoint for the services that Gloo Edge can discover

#### What's the difference between Gloo Edge and Envoy

Envoy Proxy is a data-plane component with powerful routing, observability, and resilience capabilities. Envoy can be difficult to operationalize and complex to configure. Gloo Edge adds the following:

* A [flexible control plane]({{< versioned_link_path fromRoot="/guides/dev" >}}) with extensibility in mind
* More ergonomic, [domain-specific APIs]({{< versioned_link_path fromRoot="/introduction/architecture/concepts" >}}) to drive Envoy configuration
* [Function-level routing]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/" >}}); Envoy understands routing to clusters (`host:port`) while Gloo Edge understands routing to a Swagger/OAS endpoint, gRPC function, Cloud Function like Lambda, etc.
* [Transformation of request/response](https://github.com/solo-io/envoy-gloo/tree/main/source/extensions/filters/http/transformation) via a super-fast C++ templating filter [built on Inja](https://github.com/pantor/inja)
* Envoy filters to call [AWS Lambda directly](https://github.com/solo-io/envoy-gloo/tree/main/source/extensions/filters/http/aws_lambda), handling the complex security handshaking
* [Discovery of services running in a hybrid platform]({{< versioned_link_path fromRoot="/introduction/architecture#discovery-architecture" >}}) (like VMs, containers, infrastructure as code, function as a service, etc)
* Out of the box caching filters - enterprise feature
* [Rate-limiting service]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/simple/">}}) with pluggable storage, multiple options for API (simplified, [or more flexible]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/">}}), depending on what you need) - enterprise feature
* [OIDC integration]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/oauth/" >}}), pluggable [external-auth service]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/" >}}) - enterprise feature


#### What's the difference between Gloo Edge and Istio

Gloo Edge is NOT a service mesh but can be deployed complementary to a service mesh like Istio. Istio solves the challenges of service-to-service communication by controlling requests as they flow through the system. Gloo Edge can be deployed at the edge of the service-mesh boundary, between service meshes, or within the mesh to add the following capabilities:

* Oauth flows for end-user authentication
* GraphQL endpoints for aggregation of multiple services/APIs
* Transformation of request/response to decouple backend APIs from front end
* Function routing to Google Cloud Function, AWS Lambda, Azure Functions, etc
* Request/response caching
* Unified discovery services of infrastructure like Kubernetes, Consul, Vault, AWS EC2
* Unified discovery services of functions like REST/OAS spec, gRPC reflection, SOAP/WSDL, GraphQL, WebSockets, Cloud Functions, AWS Lambda

See our blog on [API Gateways and Service Mesh](https://medium.com/solo-io/api-gateways-are-going-through-an-identity-crisis-d1d833a313d7) as well as [Integrating Gloo Edge with Istio 1.1](https://medium.com/solo-io/integrating-istio-1-1-mtls-and-gloo-proxy-f84be943e65e)

### Functionality

We strive to write good documentation and lots of tutorials in our user guides. If you have a suggestion for how to improve, please tell us! In this section, we'll look at some frequent questions asked when getting started:

#### How to change the ports on which Gloo Edge gateway proxy listens?

When considering changing the ports, it's important to understand that the Gloo Edge `gateway-proxy` (Envoy) listens on a port, and when running in Kubernetes, the Kubernetes service maps to a routable service:port as well.

Gloo Edge's `gateway-proxy` listens on port `8080` and `8443` by default. The listeners for a Gloo Edge `gateway-proxy` are defined with Gateway resources and can be found with:

```shell
kubectl --namespace gloo-system get gateway
```

```noop
NAME                AGE
gateway-proxy       61s
gateway-proxy-ssl   61s
```

Each Gateway object specifies a `bindPort` that ultimately gets converted to an Envoy listener:

```shell
kubectl --namespace gloo-system get gateway gateway-proxy-ssl --output yaml
```

{{< highlight yaml "hl_lines=13-14" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: gateway-proxy-ssl
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8443
  httpGateway: {}
  proxyNames:
  - gateway-proxy
  ssl: true
  useProxyProto: false
{{< /highlight >}}

You can change the bindPort in the Gateway resource.

You also need to be aware of the Kubernetes service. By default the Kubernetes service for `gateway-proxy` maps port `80` to port `8080` on the `gateway-proxy` and port `443` to the `8443` port on the `gateway-proxy`. Let's take a look:

```bash
kubectl --namespace gloo-system get svc gateway-proxy --output yaml
```

{{< highlight yaml "hl_lines=12-22" >}}
apiVersion: v1
kind: Service
metadata:
  labels:
    app: gloo
    gloo: gateway-proxy
  name: gateway-proxy
  namespace: gloo-system
spec:
  clusterIP: 10.111.36.9
  externalTrafficPolicy: Cluster
  ports:
  - name: http
    nodePort: 30160
    port: 80
    protocol: TCP
    targetPort: 8080
  - name: https
    nodePort: 31767
    port: 443
    protocol: TCP
    targetPort: 8443
  selector:
    gloo: gateway-proxy
  sessionAffinity: None
  type: LoadBalancer
status:
  loadBalancer: {}
{{< /highlight >}}

If you expose Gloo Edge's `gateway-proxy` outside your Kubernetes cluster with a Cloud loadbalancer or NodePort, you should keep in mind that you will route to port `80` and `443` as defined in the Kubernetes service.

#### How do VirtualServices get associated with Gateways/listeners

By default, when you create a VirtualService *without* TLS/SSL configuration, it will be bound to the HTTP port.

If you create a VirtualService and assign it TLS/SSL configuration, it will be bound to the HTTPS port.

#### How can I associate a specific VirtualService to a specific listener/Gateway?

In the event you have multiple Gateways/listeners or you want more fine-grained control over how a VirtualService gets associated with a Gateway, you can explicitly add the VirtualService name to the Gateway resource like this:

{{< highlight yaml "hl_lines=9-12" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    virtualServices:
    - name: pet-virtual-service
      namespace: gloo-system
status:
  reportedBy: gateway
  state: 1
  subresourceStatuses:
    '*v1.Proxy gloo-system gateway-proxy':
      reportedBy: gloo
      state: 1
{{< /highlight >}}

Or you can match on a map of labels that you have applied to the VirtualService:

{{< highlight yaml "hl_lines=9-12" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    virtualServiceSelector:
      label-name: label-value
      anotherlabel-name: anotherlabel-value
{{< /highlight >}}

#### How do I configure TLS for Gloo Edge?

Gloo Edge can be configured with TLS and SNI for multiple virtual hosts. Please [see the documentation for how to do that]({{< versioned_link_path fromRoot="/guides/security/tls/server_tls/">}})

#### I want to call my HTTP/HTTPS services; what URL do I use?

The `gateway-proxy`, as discussed in this FAQ, listens on various ports and may be attached to a cloud load balancer. The easiest way to figure out what URL to use when calling your APIs through the `gateway-proxy` is to use the `glooctl` command line:

```shell
glooctl proxy url
```

```noop
http://192.168.64.50:30160
```

To get the `hostname:port` for the HTTPS port:

```shell
glooctl proxy url --port https
```

```noop
https://192.168.64.50:31767
```

### Debugging

There will be times when a configuration goes awry or you encounter unexpected behavior. Here are some helpful hints to diagnose these problems.

#### How can I see exactly what configuration the Gloo Edge gateway-proxy should see and is seeing?

To show what configuration the `gateway-proxy` *should* see, check the Gloo proxy. Gloo uses the proxy configuration (which also reads in configuration from other Gloo resources such as gateways and virtual services) to translate to an Envoy proxy configuration.

```shell
glooctl get proxy <proxy> -o yaml
```

{{< highlight yaml "hl_lines=4-6 8-10" >}}
...
  spec:
    listeners:
    - bindAddress: '::'
      bindPort: 8080
      httpListener: {}
      name: listener-::-8080
    - bindAddress: '::'
      bindPort: 8443
      httpListener:
        virtualHosts:
        - domains:
          - animalstore.example.com
          name: gloo-system.animal
          routes:
          - matchers:
             - exact: /animals
            routeAction:
              single:
                upstream:
                  name: default-petstore-8080
                  namespace: gloo-system
            options:
              prefixRewrite: /api/pets
        - domains:
          - '*'
          name: gloo-system.default
          routes:
          - matchers:
             - exact: /sample-route-1
            routeAction:
              single:
                upstream:
                  name: default-petstore-8080
                  namespace: gloo-system
            options:
              prefixRewrite: /api/pets
      name: listener-::-8443
      sslConfiguations:
      - secretRef:
          name: animal-certs
          namespace: gloo-system
        sniDomains:
        - animalstore.example.com
      - secretRef:
          name: gateway-tls
          namespace: gloo-system
  status:
    reported_by: gloo
    state: 1
kind: List
{{< /highlight >}}

In this example, you can see the Gateway and VirtualService objects are merged into the proxy that then drives the Envoy xDS/configuration model. To see *exactly* what the Envoy configuration is:

```bash
glooctl proxy dump
```

{{< highlight json >}}
{
 "configs": [
  {
   "@type": "type.googleapis.com/envoy.admin.v2alpha.BootstrapConfigDump",
   "bootstrap": {
    "node": {
     "id": "gateway-proxy-9b55c99c7-x7r7c.gloo-system",
     "cluster": "gateway",
     "metadata": {
      "role": "gloo-system~gateway-proxy"
     },
     "build_version": "b696ded71901e11cf4b83fb547fe4f7e5a2fdba0/1.10.0-dev/Distribution/RELEASE/BoringSSL"
    },
    "dynamic_resources": {
     "lds_config": {
      "ads": {}
     },
     "cds_config": {
      "ads": {}
     },
     "ads_config": {
      "api_type": "GRPC",
      "grpc_services": [
       {
        "envoy_grpc": {
         "cluster_name": "xds_cluster"
        }
       }
      ]
     }
    },
    "admin": {
     "access_log_path": "/dev/null",
     "address": {
      "socket_address": {
       "address": "127.0.0.1",
       "port_value": 19000
      }
     }
    }
   },
   "last_updated": "2019-03-20T14:33:57.685Z"
  },


  <"clipped">


  {
   "@type": "type.googleapis.com/envoy.admin.v2alpha.RoutesConfigDump",
   "dynamic_route_configs": [
    {
     "version_info": "14438543344735199235",
     "route_config": {
      "name": "listener-::-8443-routes",
      "virtual_hosts": [
       {
        "name": "gloo-system.animal",
        "domains": [
         "animalstore.example.com"
        ],
        "routes": [
         {
          "match": {
           "path": "/animals"
          },
          "route": {
           "cluster": "gloo-system.default-petstore-8080",
           "prefix_rewrite": "/api/pets"
          }
         }
        ],
        "require_tls": "ALL"
       },
       {
        "name": "gloo-system.default",
        "domains": [
         "*"
        ],
        "routes": [
         {
          "match": {
           "path": "/sample-route-1"
          },
          "route": {
           "cluster": "gloo-system.default-petstore-8080",
           "prefix_rewrite": "/api/pets"
          }
         }
        ],
        "require_tls": "ALL"
       }
      ]
     },
     "last_updated": "2019-03-20T19:04:06.778Z"
    }
   ]
  }
 ]
}
{{< /highlight >}}

You can then compare the output to what the Envoy config should look like.

If you want to quickly get the logs for the proxy:

```bash
glooctl proxy logs -f
```

#### Why are the ports on my Gloo Edge gateway proxy not opened?

For Envoy to open the ports and actually listen, you need to have a Route defined in one of the VirtualServices that will be associated with that particular Gateway/Listener. For example, if have only **one** VirtualService and that has **zero** routes, the corresponding listeners on the `gateway-proxy` will not be active:

```bash
glooctl get virtualservice default
```

```noop
+-----------------|--------------|---------|------|----------|-----------------|--------+
| VIRTUAL SERVICE | DISPLAY NAME | DOMAINS | SSL  |  STATUS  | LISTENERPLUGINS | ROUTES |
+-----------------|--------------|---------|------|----------|-----------------|--------+
| default         | default      | *       | none | Accepted |                 |        |
+-----------------|--------------|---------|------|----------|-----------------|--------+
```

This is by design with the intention of not over-exposing your cluster by accident (for security). If you feel this behavior is not justified, please let us know.

#### Why am I getting error: multiple "filter chains with the same matching rules are defined"

When you create multiple VirtualServices that have TLS/SSL configuration, Gloo Edge will use SNI to try and route to the correct VirtualService. For this to work, you need to specify the `domain` explicitly in your VirtualService as well as the SNI domains. [See the TLS documentation for more]({{< versioned_link_path fromRoot="/guides/security/tls/server_tls/">}}). If you don't do this, then you'll have multiple VirtualServices with different certificate information and Envoy will not know which one to use since the hosts are the same.

#### When I have both HTTP and HTTPS routes, why are they merged and only available on HTTPS?

This is similar to the previous FAQ: if you use wildcard domains on all your VirtualServices, they will be merged. If you happen to have wildcard domain on both an HTTP-intended VirtualService (ie, one without TLS/SSL config) and wildcard on the HTTPS-intended VirtualService (ie, one WITH TLS/SSL config), then you need to be explicit about which Gateway should serve which VirtualService. Using the examples from another FAQ in this document, we can explicitly list the VirtualServices for a Gateway:

{{< highlight yaml "hl_lines=9-11" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  virtualServices:
  - name: pet-virtual-service
    namespace: gloo-system
status:
  reportedBy: gateway
  state: 1
  subresourceStatuses:
    '*v1.Proxy gloo-system gateway-proxy':
      reportedBy: gloo
      state: 1
{{< /highlight >}}
