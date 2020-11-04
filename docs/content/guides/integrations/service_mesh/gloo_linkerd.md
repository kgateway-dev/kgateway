---
title: Gloo Edge and Linkerd
weight: 3
---

If you're planning on injecting Linkerd into the Gloo Edge proxy pods, there is some configuration required. Linkerd discovers services based on the `:authority` or `Host` header. This allows Linkerd to understand what service a request is destined for without being dependent on DNS or IPs.

Gloo Edge does not rewrite the incoming header (`example.com`) to the internal service name (`example.default.svc.cluster.local`) by default. In this example, when Linkerd receives the outgoing request it thinks the request is destined for `example.com` and not `example.default.svc.cluster.local`. This creates an infinite loop that can be pretty frustrating!

Gloo Edge can be configured to automatically or manually modify the `Host` header to satisfy Linkerd's requirement. 

---

## Tutorial

This uses `books` as an example, take a look at [Demo: Books](https://linkerd.io/2/tasks/books/) for instructions on how to run it.

If you installed Gloo Edge using the Gateway method (`gloo install gateway`), then you'll need a VirtualService to be able to route traffic to your **Books** application.

To use Gloo Edge with Linkerd, you can choose one of two options:

### Automatic

As of Gloo Edge v0.13.20, Gloo Edge has native integration with Linkerd, so that the
required Linkerd headers are added automatically.

Assuming you installed gloo to the default location, you can enable the native
integration like so:

```bash
kubectl patch settings -n gloo-system default -p '{"spec":{"linkerd":true}}' --type=merge
```

Gloo Edge will now automatically add the `l5d-dst-override` header to every
kubernetes upstream.

Now simply add a route to the books app upstream:

```bash
glooctl add route --path-prefix=/ --dest-name booksapp-webapp-7000
```

### Manual

As explained in the beginning of this document, you'll need to instruct Gloo Edge to add a header which will allow Linkerd to identify where to send traffic to.

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  creationTimestamp: "2019-04-18T13:39:49Z"
  generation: 7
  name: books
  namespace: gloo-system
  resourceVersion: "8418"
  selfLink: /apis/gateway.solo.io/v1/namespaces/gloo-system/virtualservices/books
  uid: 6fb092ae-61df-11e9-a158-080027b5157f
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
       - prefix: /
      routeAction:
        single:
          upstream:
            name: booksapp-webapp-7000
            namespace: gloo-system
      options:
        headerManipulation:
          requestHeadersToAdd:
          - header:
              key: l5d-dst-override
              value: webapp.booksapp.svc.cluster.local:7000
```

The important stanza here is:

```yaml
      options:
        headerManipulation:
          requestHeadersToAdd:
          - header:
              key: l5d-dst-override
              value: webapp.booksapp.svc.cluster.local:7000
```

Using the content transformation engine built-in in Gloo Edge, you can instruct it to add the needed `l5d-dst-override` header which in the example above is pointing to the service's FDQN and port: `webapp.booksapp.svc.cluster.local:7000`
