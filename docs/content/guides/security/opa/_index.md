---
title: Open Policy Agent (OPA)
weight: 80
description: Define fine-grained policies to control Gloo Edge configuration itself.
---

In Kubernetes, Gloo Edge stores its configuration as Custom Resource Definitions (CRDs). Because these CRDs are Kubernetes objects, you can use normal Kubernetes Role Based Access Control (RBAC) to create a policy that grants users the ability to create Gloo Edge resources, such as a VirtualService object. 

However, RBAC only allows administrators to grant permissions to entire objects. With the Open Policy Agent (OPA), you can specify fine-grained control over Gloo Edge objects. Compare the differences in the following examples.

* **RBAC**: User `john@example.com` is allowed to create virtual service. 
* **OPA**: User `john@example.com` is allowed to create virtual services that only point to the domain `example.com`.

You can use both Kubernetes RBAC and OPA together.

In this guide, you create a simple OPA policy that dictates that all virtual services must not have a prefix re-write.

---

## Before you begin

1. [Install Gloo Edge in a Kubernetes cluster]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}).
2. [Follow the OPA documentation](https://www.openpolicyagent.org/docs/latest/kubernetes-tutorial/) to set up OPA as an admission controller in your cluster. Then, OPA validates Kubernetes objects before they become visible to other controllers that act on them, including Gloo Edge.

Depending on your security preferences, you might want to create a separate cluster role and role binding to restrict Gloo Edge resources to view-only permissions, such as in the following example.

{{% expand "Click for an example YAML file for a cluster role and cluster role binding you might add to your OPA admission controller setup." %}}

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gloo-view
rules:
- apiGroups:
  - gateway.solo.io
  resources:
  - virtualservices
  verbs:
  - get
  - list
  - watch
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: opa-gloo-viewer
roleRef:
  kind: ClusterRole
  name: gloo-view
  apiGroup: rbac.authorization.k8s.io
subjects:
- kind: Group
  name: system:serviceaccounts:opa
  apiGroup: rbac.authorization.k8s.io
```

{{% /expand %}}

---

## Policy

OPA Policies are written in `Rego`, a language specifically designed for policy decisions.

Write a [Rego policy](vs-no-prefix-rewrite.rego), which denies virtual services that include a prefix re-write.

```
{{% readfile file="guides/security/opa/vs-no-prefix-rewrite.rego" %}}
```

_Table: Understanding this Rego policy_

| Part of the policy | Description |
| ------------------ | ----------- |
| `operations = {"CREATE", "UPDATE"}` | Applies the policy only to objects that are created or updated. |
| `deny[msg] {}` | Starts a policy to deny to creating or updating objects, if all of the conditions in the braces are met. |
| `input.request.kind.kind == "VirtualService"` | Specifies that the object must be a VirtualService. |
| `operations[input.request.operation]` | Specifies that the object has a create or update operation. |
| `input.request.object.spec.virtualHost.routes[_].options.prefixRewrite` | Specifies that the object has a prefixRewrite stanza. |
| `msg := "prefix re-write not allowed"` | Returns the `"prefix re-write not allowed"` message when all the conditions are true and the object is denied. |

---

## Apply Policy

Apply the policy by writing it to a configmap in the `opa` namespace.

```shell
kubectl --namespace=opa create configmap vs-no-prefix-rewrite --from-file=vs-no-prefix-rewrite.rego
```

After a moment, check that the policy status changes to **ok**.

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

1. Click and save the following two virtual service configuration files to test valid and denied scenarios.

{{% expand "Valid VirtualService (vs-ok.yaml)." %}}

```
{{< readfile file="guides/security/opa/vs-ok.yaml" >}}
```

{{% /expand %}}

{{% expand "Denied VirtualService (vs-err.yaml), includes a prefix rewrite." %}}

```
{{< readfile file="guides/security/opa/vs-err.yaml" >}}
```

{{% /expand %}}

2. Apply the valid virtual service in your cluster, and verify that the virtual service is created.
```shell
kubectl apply -f vs-ok.yaml
virtualservice.gateway.solo.io/default created
```

3. Apply the denied virtual service in your cluster. Verify that the virtual service is denied because it includes a prefix rewrite that your OPA policy does not allow.
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
If you no longer want the OPA admission controller, you can uninstall it from your cluster. For example, you might use a [teardown script](teardown.sh), such as the following example.

```
{{% readfile file="guides/security/opa/teardown.sh" %}}
```

---

## Next Steps

Now that you know how to configure a basic policy with OPA, you can continue learning about policies, or check out some of the other security features in Gloo Edge.

* [**Web Application Firewall**]({{% versioned_link_path fromRoot="/guides/security/waf/" %}})
* [**Data Loss Prevention**]({{% versioned_link_path fromRoot="/guides/security/data_loss_prevention/" %}})
* [**Cross-Origin Resource Sharing**]({{% versioned_link_path fromRoot="/guides/security/cors/" %}})

You might also want to learn about the various features available to Routes on a Virtual Service in the [Traffic Management guides]({{% versioned_link_path fromRoot="/guides/traffic_management/" %}}).
