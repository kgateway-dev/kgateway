---
title: Open Policy Agent (OPA)
weight: 20
description: Fine grained control over Gloo Configuration using Open Policy Agent
---

## Motivation

In Kubernetes, Gloo stores its configuration as Custom Resource Definitions (CRDs). You can use
normal Kubernetes Role Based Access Control (RBAC) to create a policy that grants users the ability
to create a Gloo VirtualService. RBAC only allows to grant permissions entire objects.
With the Open Policy Agent, one can specify very fine grain control over Gloo objects.
For example, with RBAC you can say, "user john@example.com is allowed to create virtual service"
With OPA, in addition to specifying access,  you can say "virtual services must point to the domain example.com". 

You can of-course combine both, as you see fit.

In this document we will show a simple OPA policy that dictates that all virtual services must not 
have a prefix re-write.

### Prereqs
- Install Gloo gateway.

### Setup

First, setup OPA as a validating web hook. In this mode, OPA validates the Kubernetes objects before
they are visible to the controllers that act on them (Gloo in our case).

You can use the [setup.sh](setup.sh) script for that purpose.
Note this script follows the docs outlined in [official OPA docs](https://www.openpolicyagent.org/docs/latest/kubernetes-admission-control/)
with some small adaptations for the Gloo API.

For your convenience, here's the content of setup.sh (click to reveal):
<details><summary>[setup.sh](setup.sh)</summary>
```
{{% readfile file="security/opa/setup.sh" %}}
```
</details>

## Policy

OPA Policies are written in `Rego`. A language specifically designed for policy decisions.

Let's apply [this](vs-no-prefix-rewrite.rego) policy, forbidding virtual service with prefix re-write:

```
{{% readfile file="security/opa/vs-no-prefix-rewrite.rego" %}}
```

Let's break this down:
```
operations = {"CREATE", "UPDATE"}
```
This policy only applies to objects that are created or updated.

```
deny[msg] {
```
Start a policy to deny to object creation \ update, if all conditions in the braces hold.

The conditions are:
```
	input.request.kind.kind == "VirtualService"
```
(1) This object is a VirtualService

```
	operations[input.request.operation]
```
(2) This object is created or updated.

```
	input.request.object.spec.virtualHost.routes[_].routePlugins.prefixRewrite
```
(3) This object has a prefixRewrite stanza.

If all these conditions are true, the object will be denied with this message:
```
	msg := "prefix re-write not allowed"
```

## Apply Policy

You can use this command to apply the policy, by writing it to a configmap in the `opa` namespace
```shell
kubectl --namespace=opa create configmap vs-no-prefix-rewrite --from-file=vs-no-prefix-rewrite.rego
```

Give it a second, and you will see the policy status changes to ok:
```shell
kubectl get configmaps -n opa vs-no-prefix-rewrite -o yaml
```

{{< highlight yaml "hl_lines=9" >}}
apiVersion: v1
data:
  vs-no-prefix-rewrite.rego: "package kubernetes.admission\n\noperations = {\"CREATE\",
    \"UPDATE\"}\n\ndeny[msg] {\n\tinput.request.kind.kind == \"VirtualService\"\n\toperations[input.request.operation]\n\tinput.request.object.spec.virtualHost.routes[_].routePlugins.prefixRewrite\n\tmsg
    := \"prefix re-write not allowed\"\n}\n"
kind: ConfigMap
metadata:
  annotations:
    openpolicyagent.org/policy-status: '{"status":"ok"}'
  creationTimestamp: "2019-08-20T11:10:55Z"
  name: vs-no-prefix-rewrite
  namespace: opa
  resourceVersion: "39558874"
  selfLink: /api/v1/namespaces/opa/configmaps/vs-no-prefix-rewrite
  uid: 2de8732f-c33b-11e9-8be1-42010a8000dc
{{< /highlight >}}

## Verify

Time to test!
we have prepared two virtual services for testing:

<details><summary>[vs-ok.yaml](vs-ok.yaml)</summary>
```
{{% readfile file="security/opa/vs-ok.yaml" %}}
```
</details>
<details><summary>[vs-err.yaml](vs-err.yaml)</summary>
```
{{% readfile file="security/opa/vs-err.yaml" %}}
```
</details>

Try it:
```shell
kubectl apply -f vs-ok.yaml
virtualservice.gateway.solo.io/default created
```

```shell
kubectl apply -f vs-err.yaml
Error from server (prefix re-write not allowed): error when applying patch:
{"metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"gateway.solo.io/v1\",\"kind\":\"VirtualService\",\"metadata\":{\"annotations\":{},\"name\":\"default\",\"namespace\":\"gloo-system\"},\"spec\":{\"virtualHost\":{\"domains\":[\"*\"],\"name\":\"gloo-system.default\",\"routes\":[{\"matcher\":{\"exact\":\"/sample-route-1\"},\"routeAction\":{\"single\":{\"upstream\":{\"name\":\"default-petstore-8080\",\"namespace\":\"gloo-system\"}}},\"routePlugins\":{\"prefixRewrite\":{\"prefixRewrite\":\"/api/pets\"}}}]}}}\n"}},"spec":{"virtualHost":{"routes":[{"matcher":{"exact":"/sample-route-1"},"routeAction":{"single":{"upstream":{"name":"default-petstore-8080","namespace":"gloo-system"}}},"routePlugins":{"prefixRewrite":{"prefixRewrite":"/api/pets"}}}]}}}
to:
Resource: "gateway.solo.io/v1, Resource=virtualservices", GroupVersionKind: "gateway.solo.io/v1, Kind=VirtualService"
Name: "default", Namespace: "gloo-system"
Object: &{map["apiVersion":"gateway.solo.io/v1" "kind":"VirtualService" "metadata":map["annotations":map["kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"gateway.solo.io/v1\",\"kind\":\"VirtualService\",\"metadata\":{\"annotations\":{},\"name\":\"default\",\"namespace\":\"gloo-system\"},\"spec\":{\"virtualHost\":{\"domains\":[\"*\"],\"name\":\"gloo-system.default\",\"routes\":[{\"matcher\":{\"exact\":\"/sample-route-1\"},\"routeAction\":{\"single\":{\"upstream\":{\"name\":\"default-petstore-8080\",\"namespace\":\"gloo-system\"}}}}]}}}\n"] "creationTimestamp":"2019-08-20T11:09:00Z" "generation":'\x01' "name":"default" "namespace":"gloo-system" "resourceVersion":"39558469" "selfLink":"/apis/gateway.solo.io/v1/namespaces/gloo-system/virtualservices/default" "uid":"e99ba1a0-c33a-11e9-8be1-42010a8000dc"] "spec":map["virtualHost":map["domains":["*"] "name":"gloo-system.default" "routes":[map["matcher":map["exact":"/sample-route-1"] "routeAction":map["single":map["upstream":map["name":"default-petstore-8080" "namespace":"gloo-system"]]]]]]] "status":map["reported_by":"gateway" "state":'\x01' "subresource_statuses":map["*v1.Proxy gloo-system gateway-proxy-v2":map["reported_by":"gloo" "state":'\x01']]]]}
for: "vs-err.yaml": admission webhook "validating-webhook.openpolicyagent.org" denied the request: prefix re-write not allowed
```

## Cleanup
you can use the [teardown.sh](teardown.sh) to clean-up the resources created in this document.

For your convenience, here's the content of teardown.sh (click to reveal):
<details><summary>[teardown.sh](teardown.sh)</summary>
```
{{% readfile file="security/opa/teardown.sh" %}}
```
</details>
