---
title: Multi-gateway deployment
weight: 80
description: Deploying more gateways and gateway-proxies
---
Create multiple Envoy gateway proxies with Gloo Edge to segregate and customize traffic controls in an environment with multiple types of traffic, such as public internet and a private intranet.
## Multiple gateway architecture and terminology

Gloo Edge offers a flexible architecture by providing custom resource definitions (CRDs) that you can use to configure _proxies_ and _gateways_. These two terms describe the physical and logical architecture of a gateway system.
- **Proxies** - it's the _physical_ gateways, or reverse proxies, that fire up Envoy instances. They are running in pods (e.g. `gateway-proxy`). They are defined in your Helm values.
- **Gateways** - this one is the _logical_ Gateway. It's actually an Envoy _listener_, which represents a server socket and a protocol. By default, you will get two of them for handling plain text HTTP and HTTPS connections. You can create more `Gateway` _Custom Resources_, which will generate additional Envoy listeners.

See the diagram below about the [Custom Resource Usage]({{< versioned_link_path fromRoot="/introduction/architecture/custom_resources/" >}}). The blue squares are Kubernetes _Custom Resources_ and the **Gateway** and **Gloo** circles are Kubernetes deployments / CR controllers:

![Gateway and Proxy Configuration]({{< versioned_link_path fromRoot="/img/gateway-cr.png" >}})

If we take a closer look at the cardinality of the relationships:

![Gateways and Gateway-proxies]({{< versioned_link_path fromRoot="/img/gateways-relationship.png" >}})

* From the middle to the right-hand side of this graphic: \
 The `Gateway` CRD defines the server host and port Envoy will be listening to, as in an "Envoy listener". \
 The `Proxy` CRs are created automatically by the Gloo controller. You should not modify them. \
 That means you can have several Gateways - or Envoy listeners - bound to one single "_Proxy_"/Envoy instance. \
 This is a mean for you to differentiate the incoming traffic and to apply different kind of server configurations (like TLS, mTLS, TCP, etc.). \
 Of course, you can also have multiple (Envoy) proxies running on your cluster.

* On the left-hand side: \
 A `Gateway` CR can select one or more `VirtualService(s)`, using a discrete list or Kubernetes labels. \
 Of course, if you define a `Gateway` with `ssl: true`, then you must provide `VirtualServices` with a `sslConfig` block.


Below is a example of a `Gateway` that selects a particular (Envoy) Proxy and a some `VirtualServices` by a selector:

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: public-gw-ssl
  namespace: default
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8443
  httpGateway:
    virtualServiceSelector:
      gateway-type: public # label set on the VirtualService
  useProxyProto: false
  ssl: true
  proxyNames:
  - public-gw # name of the Envoy proxy
```


## Full example

In the Helm values below, we will define two Envoy proxies, named `publicGw` and `corpGw`. One could be internet facing, whereas the latter could be for intranet usage.

By default, for each Proxy, Helm will create two `Gateways`: one for HTTP and another one for HTTPS. \
In our example, we will disable the HTTP `Gateway` for the internet-facing Proxy, and also disable the HTTPS Gateway for the traffic coming from the Intranet.

Overview:

![Full example overview]({{< versioned_link_path fromRoot="/img/gw-proxies-full-example.png" >}})

If you want additional `Gateways` for a given Proxy, consider crafting `Gateway` _Custom Resources_ youself, similarly to what you can do with `VirtualServices`.

As you will see in the example below, you can declare as many Envoy proxies as you want under the Helm's `gloo.gatewayProxies` property.

```yaml
gloo:
  gatewayProxies:
    publicGw: # Proxy name for public access (Internet facing)
      disabled: false # overwrite the "default" value in the merge step
      kind:
        deployment:
          replicas: 2
      service:
        kubeResourceOverride: # workaround for https://github.com/solo-io/gloo/issues/5297
          spec:
            ports:
              - port: 443
                protocol: TCP
                name: https
                targetPort: 8443
            type: LoadBalancer
      gatewaySettings:
        customHttpsGateway: # using the default HTTPS Gateway
          virtualServiceSelector:
            gateway-type: public # label set on the VirtualService
        disableHttpGateway: true # disable the default HTTP Gateway
    corpGw: # Proxy name for private access (intranet facing)
      disabled: false # overwrite the "default" value in the merge step
      service:
        httpPort: 80
        httpsFirst: false
        httpsPort: 443
        httpNodePort: 32080 # random port to be fixed in your private network
        type: NodePort
      gatewaySettings:
        customHttpGateway: # using the default HTTP Gateway
          virtualServiceSelector:
            gateway-type: private # label set on the VirtualService
        disableHttpsGateway: true # disable the default HTTPS Gateway
    gatewayProxy:
      disabled: true # disable the default gateway-proxy deployment and its 2 default Gateway CRs
```


This will generate the following two `Gateway` CRs and also two Envoy deployments called `public-gw` and `private-gw`:

```bash {hl_lines=["4-5","17-18"]}
$ kubectl -n gloo-system get gw,deploy

NAME                                    AGE
gateway.gateway.solo.io/corp-gw         3m7s
gateway.gateway.solo.io/public-gw-ssl   3m7s

NAME                                                  READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/discovery                             1/1     1            1           3m8s
deployment.apps/gateway                               1/1     1            1           3m8s
deployment.apps/gloo                                  1/1     1            1           3m8s
deployment.apps/gloo-fed                              1/1     1            1           3m8s
deployment.apps/gloo-fed-console                      1/1     1            1           3m7s
deployment.apps/glooe-grafana                         1/1     1            1           3m7s
deployment.apps/glooe-prometheus-kube-state-metrics   1/1     1            1           3m8s
deployment.apps/glooe-prometheus-server               1/1     1            1           3m8s
deployment.apps/observability                         1/1     1            1           3m8s
deployment.apps/corp-gw                               1/1     1            1           3m8s
deployment.apps/public-gw                             2/2     2            2           3m8s
```


The associated `VirtualServices` could be something like this:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: httpbin
  namespace: gloo-system
  labels:
    gateway-type: public # label used by the "public" Gateway
spec:
  sslConfig: # the internet-facing proxy uses TLS
    secretRef:
      name: upstream-tls
      namespace: gloo-system
  virtualHost:
    domains:
    - '*.mycompany.com' # listen on these public domain names
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: default-httpbin-8000
            namespace: gloo-system
---
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: httpbin-private
  namespace: gloo-system
  labels:
    gateway-type: private # label used by the "corp" Gateway
spec:
  virtualHost:
    domains:
    - '*.mycompany.corp' # listen on these private domain names
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: default-httpbin-8000
            namespace: gloo-system
```

You can check everything is correct with `glooctl` commands:

```bash
$ glooctl get vs
+-----------------+--------------+------------------+------------+----------+-----------------+----------------------------------+
| VIRTUAL SERVICE | DISPLAY NAME |     DOMAINS      |    SSL     |  STATUS  | LISTENERPLUGINS |              ROUTES              |
+-----------------+--------------+------------------+------------+----------+-----------------+----------------------------------+
| httpbin         |              | *.mycompany.com  | secret_ref | Accepted |                 | / ->                             |
|                 |              |                  |            |          |                 | gloo-system.default-httpbin-8000 |
|                 |              |                  |            |          |                 | (upstream)                       |
| httpbin-private |              | *.mycompany.corp | none       | Accepted |                 | / ->                             |
|                 |              |                  |            |          |                 | gloo-system.default-httpbin-8000 |
|                 |              |                  |            |          |                 | (upstream)                       |
+-----------------+--------------+------------------+------------+----------+-----------------+----------------------------------+

$ glooctl get proxy
+-----------+-----------+---------------+----------+
|   PROXY   | LISTENERS | VIRTUAL HOSTS |  STATUS  |
+-----------+-----------+---------------+----------+
| corp-gw   | :::8080   | 1             | Accepted |
| public-gw | :::8443   | 1             | Accepted |
+-----------+-----------+---------------+----------+
```
