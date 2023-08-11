---
menuTitle: Admission Control
title: Admission Control
weight: 10
description: (Kubernetes Only) Gloo Edge can be configured to validate configuration before it is applied to the cluster. With validation enabled, any attempt to apply invalid configuration to the cluster will be rejected.
---

## Motivation

Gloo Edge can prevent invalid configuration from being written to Kubernetes with the use of a [Kubernetes Validating Admission Webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/).

This document explains how to enable and configure admission control in Gloo Edge.

## Using the Validating Admission Webhook

Admission Validation provides a safeguard to ensure Gloo Edge does not halt processing of configuration. If a resource 
would be written or modified in such a way to cause Gloo Edge to report an error, it is instead rejected by the Kubernetes 
API Server before it is written to persistent storage.

Gloo Edge runs a [Kubernetes Validating Admission Webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)
which is invoked whenever a `gateway.solo.io` custom resource is created or modified. This includes
{{< protobuf name="gateway.solo.io.Gateway" display="Gateways">}},
{{< protobuf name="gateway.solo.io.VirtualService" display="Virtual Services">}},
and {{< protobuf name="gateway.solo.io.RouteTable" display="Route Tables">}}.

The [validating webhook configuration](https://github.com/solo-io/gloo/blob/main/install/helm/gloo/templates/5-gateway-validation-webhook-configuration.yaml) is enabled by default by Gloo Edge's Helm chart and `glooctl install gateway`. This admission webhook can be disabled 
by removing the `ValidatingWebhookConfiguration`.

The webhook can be configured to perform strict or permissive validation, depending on the `gateway.validation.alwaysAccept` setting in the 
{{< protobuf name="gloo.solo.io.Settings" display="Settings">}} resource.

When `alwaysAccept` is `true` (currently the default is `true`), resources will only be rejected when Gloo Edge fails to 
deserialize them (due to invalid JSON/YAML).

To enable "strict" admission control (rejection of resources with invalid config), set `alwaysAccept` to false.

      {{< notice tip >}}
      You can also use the validating admission webhook by running the <code>kubectl apply --dry-run=server</code> command to test your Gloo configuration before you apply it to your cluster. For more information, see <a href="#test-resource-configurations">Test resource configurations</a>. 
      {{< /notice >}}

## Enabling Strict Validation Webhook 
 
 
By default, the Validation Webhook only logs the validation result, but always admits resources with valid YAML (even if the 
configuration options are inconsistent/invalid).

The webhook can be configured to reject invalid resources via the 
{{< protobuf name="gloo.solo.io.Settings" display="Settings">}} resource.

If using Helm to manage settings, set the following values:

```bash
--set gateway.validation.alwaysAcceptResources=false
--set gateway.validation.enabled=true
```

If writing Settings directly to Kubernetes, add the following to the `spec.gateway` block:

{{< highlight yaml "hl_lines=12-15" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  labels:
    app: gloo
  name: default
  namespace: gloo-system
spec:
  discoveryNamespace: gloo-system
  gloo:
    xdsBindAddr: 0.0.0.0:9977
  gateway:
    validation:
      alwaysAcceptResources: false
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  refreshRate: 60s
{{< /highlight >}}

Once these are applied to the cluster, we can test that validation is enabled:

```bash
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: reject-me
  namespace: gloo-system
spec:
  virtualHost:
    routes:
    - matchers:
      - headers:
        - name: foo
          value: bar
      routeAction:
        single:
          upstream:
            name: does-not-exist
            namespace: gloo-system
EOF

```

## Test resource configurations

You can use the Kubernetes [dry run capability](#dry-run) to verify your resource configuration<!-- or [send requests directly to the Gloo Edge validation API](#validation-api)-->. 

{{% notice note %}}
The information in this guide assumes that you enabled strict validation, including the rejection of resources that result in a `Warning` state. To enable these settings, run `kubectl edit settings default -n gloo-system` and set `alwaysAccept: false` and `allowWarnings: false` in the `spec.gateway.validation` section. 
{{% /notice %}}

### Use the dry run capability in Kubernetes {#dry-run}

To test whether a YAML file is accepted by the validation webhook, you can use the `kubectl apply --dry-run=server` command as shown in the following examples.  

{{< tabs >}}
{{% tab name="Upstream" %}}

1. Try to create an upstream without a valid host address and verify that your resource is denied by the validation API. 
   ```yaml
   kubectl apply --dry-run=server -f- <<EOF
   apiVersion: gloo.solo.io/v1
   kind: Upstream
   metadata:
     name: invalid-upstream
     namespace: gloo-system
   spec:
     static:
       hosts:
         - addr: ~
   EOF
   ``` 
   
   </br>

   Example output:
   ```
   Error from server: error when creating "STDIN": admission webhook "gloo.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Validating *v1.Upstream failed: 1 error occurred:
	   * Validating *v1.Upstream failed: validating *v1.Upstream name:"invalid-upstream"  namespace:"gloo-system": failed gloo validation resource reports: 2 errors occurred:
	* invalid resource gloo-system.invalid-upstream
	   * WARN: 
     [2 errors occurred:
	   * addr cannot be empty for host
	   * cluster was configured improperly by one or more plugins: name:"invalid-upstream_gloo-system"  type:STATIC  connect_timeout:{seconds:5}  metadata:{}: cluster type STATIC specified but LoadAssignment was empty
   ```

{{% /tab %}}
{{% tab name="Gateway" %}}

1. Try to create a gateway resource without a gateway type and verify that your resource is denied by the validation API. 
   ```yaml
   kubectl apply --dry-run=server -f- <<EOF 
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-without-type
     namespace: gloo-system
   spec:
     bindAddress: '::'
   EOF
   ```
   
   </br>

   Example output:
   ```
   Error from server: error when creating "STDIN": admission webhook "gloo.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Validating *v1.Gateway failed: 1 error occurred:
	   * Validating *v1.Gateway failed: validating *v1.Gateway name:"gateway-without-type"  namespace:"gloo-system": could not render proxy: 2 errors occurred:
	   * invalid resource gloo-system.gateway-without-type
	   * invalid gateway: gateway must contain gatewayType
   ```

2. Create another gateway resource that references an upstream that does not exist and verify that the resource is denied by the validation API. 
   ```yaml
   kubectl apply --dry-run=server -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: tcp
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8000
     tcpGateway:
       tcpHosts:
       - name: one
         destination:
           single:
             upstream:
               name: gloo-system-tcp-echo-1025
               namespace: gloo-system
     useProxyProto: false
   EOF
   ```

   </br>

   Example output: 
   ```
   Error from server: error when creating "STDIN": admission webhook "gloo.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Validating *v1.Gateway failed: 1 error occurred:
	   * Validating *v1.Gateway failed: validating *v1.Gateway name:"tcp"  namespace:"gloo-system": TcpHost Warning: InvalidDestinationWarning. Reason: listener listener-::-8000: TcpHost error: *v1.Upstream { gloo-system.gloo-system-tcp-echo-1025 } not found
   ```



{{% /tab %}}
{{% tab name="Virtual service" %}}  
1. Create a valid virtual gateway resource. Note that if you followed the [Hello world]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) guide, the virtual service already exists and is denied by the validation API because you cannot use the same domain in multiple virtual services. 
   ```yaml
   kubectl apply --dry-run=server -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: default
     namespace: gloo-system
     ownerReferences: []
   spec:
     virtualHost:
       domains:
       - '*'
       routes:
       - matchers:
         - exact: /all-pets
         options:
           prefixRewrite: /api/pets
         routeAction:
           single:
             upstream:
               name: default-petstore-8080
               namespace: gloo-system
   EOF
   ```

   </br>

   Example output if the virtual service does not exist:
   ```
   virtualservice.gateway.solo.io/default created (server dry run)
   ```

   </br>

   Example output if the virtual service already exists:
   ```
   Error from server: error when creating "STDIN": admission webhook "gloo.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Validating *v1.VirtualService failed: 1 error occurred:
	   * Validating *v1.VirtualService failed: validating *v1.VirtualService name:"default"  namespace:"gloo-system": could not render proxy: 4 errors occurred:
	   * invalid resource gloo-system.default
	   * domain conflict: the [*] domain is present in other virtual services that belong to the same Gateway as this one: [gloo-system.default-2]
	   * domain conflict: the [*] domain is present in other virtual services that belong to the same Gateway as this one: [gloo-system.default]
	   * domain conflict: the following domains are present in more than one of the virtual services associated with this gateway: [*]
   ```

3. Try to create a virtual service that points to an upstream that does not exist and verify that your resource is denied by the validation API. 
   ```yaml
   kubectl apply --dry-run=server -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: missing-upstream
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
        - unique1
       routes:
         - matchers:
           - methods:
              - GET
             prefix: /items/
           routeAction:
             single:
               upstream:
                 name: does-not-exist
                 namespace: anywhere
   EOF
   ```

   </br>

   Example output:
   ```
   Error from server: error when creating "STDIN": admission webhook "gloo.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Validating *v1.VirtualService failed: 1 error occurred:
	   * Validating *v1.VirtualService failed: validating *v1.VirtualService name:"missing-upstream"  namespace:"default": Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream { anywhere.does-not-exist } not found
   ```

4. Try to create another virtual service that does not specify any prefix matchers in the `delegateAction` section and verify that the resource is denied by the validation API.
   ```yaml
   kubectl apply --dry-run=server -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: method-matcher
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
        - unique2
       routes:
         - matchers:
           - exact: /delegated-nonprefix  # not allowed
           delegateAction:
             name: does-not-exist # also not allowed, but caught later
             namespace: anywhere
   EOF
   ```

   </br>

   Example output:
   ```
   Error from server: error when creating "STDIN": admission webhook "gloo.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Validating *v1.VirtualService failed: 1 error occurred:
	   * Validating *v1.VirtualService failed: validating *v1.VirtualService name:"method-matcher"  namespace:"default": could not render proxy: 2 errors occurred:
	   * invalid resource default.method-matcher
	   * invalid route: routes with delegate actions must use a prefix matcher
   ```
{{% /tab %}}
{{< /tabs >}}

<!--
### Send requests to the validation API directly {#validation-api}

Send a curl request to the validation API to test your resource configurations. 

{{< notice tip >}}
If an empty response <code>{}</code> is from the validation API, you might need to add or remove a bracket from your request. This response is returned also if the wrong bracket type is used, such as when you used <code>{}</code> instead of <code>[]</code>. 
{{< /notice >}}

1. Port-forward the gloo service on port 8443. 
   ```sh
   kubectl -n gloo-system port-forward service/gloo 8443:443
   ```

2. Send a request with your resource configuration to the Gloo Edge validation API. The following example shows successful and unsuccessful resource configuration validation for the upstream, gateway, and virtual service resources.
   {{< tabs >}}
   {{% tab name="Upstream" %}}

   Example YAML file:
   ```yaml   
   apiVersion: gloo.solo.io/v1
   kind: Upstream
   metadata:
     name: json-upstream
     namespace: gloo-system
   spec:
     static:
       hosts:
         - addr: jsonplaceholder.typicode.com
           port: 80
   ```

   </br>
   
   1. Send a request to the validation API.
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gloo.solo.io","version":"v1","kind":"Upstream"},"resource":{"group":"","version":"","resource":""},"name":"upstream","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object": { "apiVersion": "gloo.solo.io/v1", "kind": "Upstream", "metadata": { "name": "upstream", "namespace": "gloo-system" }, "spec": { "static": { "hosts": [ { "addr": "jsonplaceholder.typicode.com", "port": 80 } ]}}} }}' -H 'Content-Type: application/json' https://localhost:8443/validation 
      ```

      Example output for successful validation:
      ```
      {"response":{"uid":"1234","allowed":true}}
      ```

   2. Set the `hosts.addr` field to null and verify that your resource is denied by the validation API.
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gloo.solo.io","version":"v1","kind":"Upstream"},"resource":{"group":"","version":"","resource":""},"name":"upstream","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object": { "apiVersion": "gloo.solo.io/v1", "kind": "Upstream", "metadata": { "name": "upstream", "namespace": "gloo-system" }, "spec": { "static": { "hosts": [ { "addr": null, "port": 80 } ]}}} }}' -H 'Content-Type: application/json' https://localhost:8443/validation
      ```

      Example output: 
      ```
      {"response":{"uid":"1234","allowed":false,"status":{"metadata":{},"message":"resource incompatible with current Gloo snapshot: [Validating *v1.Upstream failed: 1 error occurred:\n\t* Validating *v1.Upstream failed: validating *v1.Upstream name:\"upstream\"  namespace:\"gloo-system\": failed gloo validation resource reports: 2 errors occurred:\n\t* invalid resource gloo-system.upstream\n\t* WARN: \n  [2 errors occurred:\n\t* addr cannot be empty for host\n\t* cluster was configured improperly by one or more plugins: name:\"upstream_gloo-system\"  type:STATIC  connect_timeout:{seconds:5}  metadata:{}: cluster type STATIC specified but LoadAssignment was empty\n\n]\n\n\n\n]","details":{"name":"upstream","group":"gloo.solo.io","kind":"Upstream","causes":[{"message":"Error Validating *v1.Upstream failed: 1 error occurred:\n\t* Validating *v1.Upstream failed: validating *v1.Upstream name:\"upstream\"  namespace:\"gloo-system\": failed gloo validation resource reports: 2 errors occurred:\n\t* invalid resource gloo-system.upstream\n\t* WARN: \n  [2 errors occurred:\n\t* addr cannot be empty for host\n\t* cluster was configured improperly by one or more plugins: name:\"upstream_gloo-system\"  type:STATIC  connect_timeout:{seconds:5}  metadata:{}: cluster type STATIC specified but LoadAssignment was empty\n\n]\n\n\n\n"}]}}}}%         
      ```

   {{% /tab %}}
   {{% tab name="Gateway" %}}

   Example YAML file
   ```yaml
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: tcp
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8000
     tcpGateway:
       tcpHosts:
       - name: one
         destination:
           single:
             upstream:
               name: gloo-system-tcp-echo-1025
               namespace: gloo-system
     useProxyProto: false
   ```

   </br>
   
   1. Send a request to the validation API to create the gateway resource. Note that because the resource references an upstream that does not exist, the validation API returns a validation error. 
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gateway.solo.io","version":"v1","kind":"Gateway"},"resource":{"group":"","version":"","resource":""},"name":"tcp","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object":{ "apiVersion": "gateway.solo.io/v1", "kind": "Gateway", "metadata": { "name": "tcp", "namespace": "gloo-system" }, "spec": { "bindAddress": "::", "bindPort": 8000, "tcpGateway": { "tcpHosts": [ { "name": "one", "destination": { "single": {  "upstream": { "name": "gloo-system-tcp-echo-1025", "namespace": "gloo-system" }} }}]}, "useProxyProto": false }} }}' -H 'Content-Type: application/json' https://localhost:8443/validation
      ```

      Example output:
      ```
      {"response":{"uid":"1234","allowed":false,"status":{"metadata":{},"message":"resource incompatible with current Gloo snapshot: [Validating *v1.Gateway failed: 1 error occurred:\n\t* Validating *v1.Gateway failed: validating *v1.Gateway name:\"tcp\"  namespace:\"gloo-system\": TcpHost Warning: InvalidDestinationWarning. Reason: listener listener-::-8000: TcpHost error: *v1.Upstream { gloo-system.gloo-system-tcp-echo-1025 } not found\n\n]","details":{"name":"tcp","group":"gateway.solo.io","kind":"Gateway","causes":[{"message":"Error Validating *v1.Gateway failed: 1 error occurred:\n\t* Validating *v1.Gateway failed: validating *v1.Gateway name:\"tcp\"  namespace:\"gloo-system\": TcpHost Warning: InvalidDestinationWarning. Reason: listener listener-::-8000: TcpHost error: *v1.Upstream { gloo-system.gloo-system-tcp-echo-1025 } not found\n\n"}]}}}}
      ```

   2. Change the boolean value in `useProxyProto` from `false` to `"false"` and verify that your resource configuration is denied by the validation API. 
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gateway.solo.io","version":"v1","kind":"Gateway"},"resource":{"group":"","version":"","resource":""},"name":"tcp","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object":{ "apiVersion": "gateway.solo.io/v1", "kind": "Gateway", "metadata": { "name": "tcp", "namespace": "gloo-system" }, "spec": { "bindAddress": "::", "bindPort": 8000, "tcpGateway": { "tcpHosts": [ { "name": "one", "destination": { "single": {  "upstream": { "name": "gloo-system-tcp-echo-1025", "namespace": "gloo-system" }} }}]}, "useProxyProto": "false" }} }}' -H 'Content-Type: application/json' https://localhost:8443/validation  
      ```

      Example output for failed validation:
      ```
      {"response":{"uid":"1234","allowed":false,"status":{"metadata":{},"message":"resource incompatible with current Gloo snapshot: [1 error occurred:\n\t* could not unmarshal raw object: parsing resource from crd spec tcp in namespace gloo-system into *v1.Gateway: json: cannot unmarshal string into Go value of type bool\n\n]","details":{"name":"tcp","group":"gateway.solo.io","kind":"Gateway","causes":[{"message":"Error 1 error occurred:\n\t* could not unmarshal raw object: parsing resource from crd spec tcp in namespace gloo-system into *v1.Gateway: json: cannot unmarshal string into Go value of type bool\n\n"}]}}}}
      ```

   3. Remove the `tcpGateway` section and verify that the validation API returns an error because no gateway type is provided. 
      ```sh
      curl -k -XPOST -d '{"request":"uid":"1234","kind":{"group":"gateway.solo.io","version":"v1","kind":"Gateway"},"resource":{"group":"","version":"","resource":""},"name":"tcp","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object":{ "apiVersion": "gateway.solo.io/v1", "kind": "Gateway", "metadata": { "name": "tcp", "namespace": "gloo-system" }, "spec": { "bindAddress": "::", "bindPort": 8000, "useProxyProto": "false" }} }}' -H 'Content-Type: application/json' https://localhost:8443/validation
      ```

      Example output for invalid validation: 
      ```
      {}
      ```
   
   {{% /tab %}}
   {{% tab name="Virtual service" %}}

   Example YAML file:
   ```yaml
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: update-response-code
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
         - foo.com
       routes:
         - matchers:
             - prefix: /
           routeAction:
             single:
               upstream:
                 name: postman-echo
                 namespace: gloo-system
           options:
             autoHostRewrite: true
      options:
         transformations:
           responseTransformation:
             transformationTemplate:
               headers:
                 ':status':
                   text: >-
                     {% if default(data.error.message, "") != "" %}400{% else %}{{
                     header(":status") }}{% endif %}
   ```

   </br>
   
   1. Send a request to the validation API and verify that your resource is denied because you reference an upstream that does not exist. 
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gateway.solo.io","version":"v1","kind":"VirtualService"},"resource":{"group":"","version":"","resource":""},"name":"update-response-code","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object":{ "apiVersion": "gateway.solo.io/v1", "kind": "VirtualService", "metadata": { "name": "update-response-code", "namespace": "gloo-system" }, "spec": { "virtualHost": { "domains": [ "foo.com" ], "routes": [ { "matchers": [ { "prefix": "/" } ], "routeAction": { "single": { "upstream": { "name": "postman-echo", "namespace": "gloo-system" } } }, "options": { "autoHostRewrite": true } } ], "options": { "transformations": { "responseTransformation": { "transformationTemplate": { "headers": { ":status": { "text": "{% if default(data.error.message, \"\") != \"\" %}400{% else %}{{ header(\":status\") }}{% endif %}" } } } } } } } } }}}' -H 'Content-Type: application/json' https://localhost:8443/validation
      ```

      Example output:
      ```
      {"response":{"uid":"1234","allowed":false,"status":{"metadata":{},"message":"resource incompatible with current Gloo snapshot: [Validating *v1.VirtualService failed: 1 error occurred:\n\t* Validating *v1.VirtualService failed: validating *v1.VirtualService name:\"update-response-code\"  namespace:\"gloo-system\": Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream { gloo-system.postman-echo } not found\n\n]","details":{"name":"update-response-code","group":"gateway.solo.io","kind":"VirtualService","causes":[{"message":"Error Validating *v1.VirtualService failed: 1 error occurred:\n\t* Validating *v1.VirtualService failed: validating *v1.VirtualService name:\"update-response-code\"  namespace:\"gloo-system\": Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream { gloo-system.postman-echo } not found\n\n"}]}}}}%    
      ```

   2. Change the `options` field to `optional` to force a validation error. 
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gateway.solo.io","version":"v1","kind":"VirtualService"},"resource":{"group":"","version":"","resource":""},"name":"update-response-code","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object":{ "apiVersion": "gateway.solo.io/v1", "kind": "VirtualService", "metadata": { "name": "update-response-code", "namespace": "gloo-system" }, "spec": { "virtualHost": { "domains": [ "foo.com" ], "routes": [ { "matchers": [ { "prefix": "/" } ], "routeAction": { "single": { "upstream": { "name": "postman-echo", "namespace": "gloo-system" } } }, "options": { "autoHostRewrite": true } } ], "optional": { "transformations": { "responseTransformation": { "transformationTemplate": { "headers": { ":status": { "text": "{% if default(data.error.message, \"\") != \"\" %}400{% else %}{{ header(\":status\") }}{% endif %}" } } } } } } } } }}}' -H 'Content-Type: application/json' https://localhost:8443/validation
      ```

      Example output:
      ```
      {"response":{"uid":"1234","allowed":false,"status":{"metadata":{},"message":"resource incompatible with current Gloo snapshot: [1 error occurred:\n\t* could not unmarshal raw object: parsing resource from crd spec update-response-code in namespace gloo-system into *v1.VirtualService: unknown field \"optional\" in gateway.solo.io.Route\n\n]","details":{"name":"update-response-code","group":"gateway.solo.io","kind":"VirtualService","causes":[{"message":"Error 1 error occurred:\n\t* could not unmarshal raw object: parsing resource from crd spec update-response-code in namespace gloo-system into *v1.VirtualService: unknown field \"optional\" in gateway.solo.io.Route\n\n"}]}}}}
      ```

   {{% /tab %}}
   {{< /tabs >}}

-->
   
## Disable resource validation in Gloo Edge

```noop
Error from server: error when creating "STDIN": admission webhook "gateway.gloo-system.svc" denied the request: resource incompatible with current Gloo Edge snapshot: [Route Error: InvalidMatcherError. Reason: no path specifier provided]
```

Great! Validation is working, providing us a quick feedback mechanism and preventing Gloo Edge from receiving invalid config.

Another way to use the validation webhook is via `kubectl apply --server-dry-run`, which allows users to test
configuration before attempting to apply it to their cluster.

We appreciate questions and feedback on Gloo Edge validation or any other feature on [the solo.io slack channel](https://slack.solo.io/) as well as our [GitHub issues page](https://github.com/solo-io/gloo).
