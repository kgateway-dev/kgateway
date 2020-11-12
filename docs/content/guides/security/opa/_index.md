---
title: Open Policy Agent (OPA)
weight: 80
description: Define fine-grained policies to control Gloo Edge configuration itself.
---

In Kubernetes, Gloo Edge stores its configuration as Custom Resource Definitions (CRDs). You can use normal Kubernetes Role Based Access Control (RBAC) to create a policy that grants users the ability to create a Gloo Edge VirtualService. RBAC only allows administrators to grant permissions to entire objects. With the Open Policy Agent, one can specify very fine-grained control over Gloo Edge objects. For example, with RBAC you can say, "user john@example.com is allowed to create virtual service" With OPA, in addition to specifying access,  you can say "virtual services must point to the domain example.com". 

You can of-course combine both, as you see fit.

In this document we will show a simple OPA policy that dictates that all virtual services must not have a prefix re-write.

---

### Prerequisites

Before you get started, you will need to have Gloo Edge running in a Kubernetes cluster. For more information, you can follow this [installation guide]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}).

### Setup

First, setup OPA as a validating web hook. In this mode, OPA validates the Kubernetes objects before they are visible to the controllers that act on them (Gloo Edge in our case).

You can use the [setup.sh](setup.sh) script for that purpose.

This script follows the docs outlined in [official OPA docs](https://www.openpolicyagent.org/docs/latest/kubernetes-admission-control/) with some small adaptations for the Gloo Edge API.

{{% expand "Click to see the full setup.sh file that should be used for this project." %}}
```
{{% readfile file="guides/security/opa/setup.sh" %}}
```
{{% /expand %}}

---

## Policy

OPA Policies are written in `Rego`. A language specifically designed for policy decisions.

Let's apply [this](vs-no-prefix-rewrite.rego) policy, forbidding virtual service with prefix re-write:

```
{{% readfile file="guides/security/opa/vs-no-prefix-rewrite.rego" %}}
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
	input.request.object.spec.virtualHost.routes[_].options.prefixRewrite
```
(3) This object has a prefixRewrite stanza.

If all these conditions are true, the object will be denied with this message:
```
	msg := "prefix re-write not allowed"
```

---

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
    \"UPDATE\"}\n\ndeny[msg] {\n\tinput.request.kind.kind == \"VirtualService\"\n\toperations[input.request.operation]\n\tinput.request.object.spec.virtualHost.routes[_].options.prefixRewrite\n\tmsg
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

---

## Verify

Time to test!

We have prepared two virtual services for testing (click on each to see the content):

{{% expand "Valid VirtualService (vs-ok.yaml)" %}}

```
{{< readfile file="guides/security/opa/vs-ok.yaml" >}}
```

{{% /expand %}}

{{% expand "Denied VirtualService (vs-err.yaml)" %}}

```
{{< readfile file="guides/security/opa/vs-err.yaml" >}}
```

{{% /expand %}}

Try it:
```shell
kubectl apply -f vs-ok.yaml
virtualservice.gateway.solo.io/default created
```

```shell
kubectl apply -f vs-err.yaml
Error from server (prefix re-write not allowed): error when applying patch:
{"metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"gateway.solo.io/v1\",\"kind\":\"VirtualService\",\"metadata\":{\"annotations\":{},\"name\":\"default\",\"namespace\":\"gloo-system\"},\"spec\":{\"virtualHost\":{\"domains\":[\"*\"],\"routes\":[{\"matchers\":[{\"exact\":\"/sample-route-1\"}],\"options\":{\"prefixRewrite\":\"/api/pets\"},\"routeAction\":{\"single\":{\"upstream\":{\"name\":\"default-petstore-8080\",\"namespace\":\"gloo-system\"}}}}]}}}\n"}},"spec":{"virtualHost":{"routes":[{"matchers":[{"exact":"/sample-route-1"}],"options":{"prefixRewrite":"/api/pets"},"routeAction":{"single":{"upstream":{"name":"default-petstore-8080","namespace":"gloo-system"}}}}]}}}
to:
Resource: "gateway.solo.io/v1, Resource=virtualservices", GroupVersionKind: "gateway.solo.io/v1, Kind=VirtualService"
Name: "default", Namespace: "gloo-system"
Object: &{map["apiVersion":"gateway.solo.io/v1" "kind":"VirtualService" "metadata":map["annotations":map["kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"gateway.solo.io/v1\",\"kind\":\"VirtualService\",\"metadata\":{\"annotations\":{},\"name\":\"default\",\"namespace\":\"gloo-system\"},\"spec\":{\"virtualHost\":{\"domains\":[\"*\"],\"routes\":[{\"matchers\":[{\"exact\":\"/sample-route-1\"}],\"routeAction\":{\"single\":{\"upstream\":{\"name\":\"default-petstore-8080\",\"namespace\":\"gloo-system\"}}}}]}}}\n"] "creationTimestamp":"2020-01-29T14:41:28Z" "generation":'\x06' "name":"default" "namespace":"gloo-system" "resourceVersion":"7076134" "selfLink":"/apis/gateway.solo.io/v1/namespaces/gloo-system/virtualservices/default" "uid":"6ed4d802-42a5-11ea-84a5-56542bf21e7d"] "spec":map["virtualHost":map["domains":["*"] "routes":[map["matchers":[map["exact":"/sample-route-1"]] "routeAction":map["single":map["upstream":map["name":"default-petstore-8080" "namespace":"gloo-system"]]]]]]] "status":map["reported_by":"gateway" "state":'\x01' "subresource_statuses":map["*v1.Proxy.gloo-system.gateway-proxy":map["reported_by":"gloo" "state":'\x01']]]]}
for: "vs-err.yaml": admission webhook "validating-webhook.openpolicyagent.org" denied the request: prefix re-write not allowed
```

---

## Cleanup
you can use the [teardown.sh](teardown.sh) to clean-up the resources created in this document.

For your convenience, here's the content of teardown.sh:
```
{{% readfile file="guides/security/opa/teardown.sh" %}}
```

---

## Next Steps

Now that you've see how to configure a basic policy with OPA, you can go further down the rabbit hole of policies, or check out some of the other security features in Gloo Edge.

* [**Web Application Firewall**]({{% versioned_link_path fromRoot="/guides/security/waf/" %}})
* [**Data Loss Prevention**]({{% versioned_link_path fromRoot="/guides/security/data_loss_prevention/" %}})
* [**Cross-Origin Resource Sharing**]({{% versioned_link_path fromRoot="/guides/security/cors/" %}})

Or you might want to learn more about the various features available to Routes on a Virtual Service in our [Traffic Management guides]({{% versioned_link_path fromRoot="/guides/traffic_management/" %}}).
