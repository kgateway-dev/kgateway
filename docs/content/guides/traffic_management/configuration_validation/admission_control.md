---
menuTitle: Admission control
title: Admission control
weight: 10
description: (Kubernetes Only) Gloo Gateway can be configured to validate configuration before it is applied to the cluster. With validation enabled, any attempt to apply invalid configuration to the cluster will be rejected.
---

Prevent invalid Gloo configuration from being applied to your Kubernetes cluster by using the Gloo Gateway validating admission webhook. 

## About the validating admission webhook

The [validating admission webhook configuration](https://github.com/solo-io/gloo/blob/main/install/helm/gloo/templates/5-gateway-validation-webhook-configuration.yaml) is enabled by default when you install Gloo Gateway with the Helm chart or the `glooctl install gateway` command. By default, the webhook only logs the validation result without rejecting invalid Gloo resource configuration. If the configuration you provide is written in valid YAML format, it is accepted by the Kubernetes API server and written to etcd. However, the configuration might contain invalid settings or inconsistencies that Gloo Gateway cannot interpret or process. This mode is also referred to as permissive validation. 

You can enable strict validation by setting the `alwaysAcceptResources` Helm option to false. Note that only resources that result in a `rejected` status are rejected on admission. Resources that result in a `warning` status are still admitted. To also reject resources with a `warning` status, set `alwaysAcceptResources=false` and `allowWarnings=false` in your Helm file. 

For more information about how resource configuration validation works in Gloo Gateway, see [Resource validation in Gloo Gateway]({{% versioned_link_path fromRoot="/guides/traffic_management/configuration_validation/#resource-validation-in-gloo-edge" %}}). 

## Enable strict resource validation

Configure the validating admission webhook to reject invalid Gloo custom resources before they are applied in the cluster. 

1. Enable strict resource validation by updating your Gloo Gateway installation and set the following Helm values.
   ```bash
   --set gateway.validation.alwaysAcceptResources=false
   --set gateway.validation.enabled=true
   ```
   {{% notice tip %}}
   To also reject Gloo custom resources that result in a `Warning` status, include `--set gateway.validation.allowWarnings=false`. 
   {{% /notice %}}

2. Verify that the validating admission webhook is enabled. 
   1. Create a virtual service that includes invalid Gloo configuration. 
      ```yaml
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

   2. Verify that the Gloo resource is rejected. You see an error message similar to the following.
      ```noop
      Error from server: error when creating "STDIN": admission webhook "gloo.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Validating *v1.VirtualService failed: 1 error occurred:
	    * Validating *v1.VirtualService failed: validating *v1.VirtualService name:"reject-me"  namespace:"gloo-system": Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream { gloo-system.does-not-exist } not found
      ```

      {{< notice tip >}}
      You can also use the validating admission webhook by running the <code>kubectl apply --dry-run=server</code> command to test your Gloo configuration before you apply it to your cluster. For more information, see <a href="#test-resource-configurations">Test resource configurations</a>. 
      {{< /notice >}}


## View the current validating admission webhook configuration

You can check whether strict or permissive validation is enabled in your Gloo Gateway installation by checking the {{< protobuf name="gloo.solo.io.Settings" display="Settings">}} resource. 

1. Get the details of the default settings resource. 
   ```sh
   kubectl get settings default -n gloo-system -o yaml
   ```

2. In your CLI output, find the `spec.gateway.validation.alwaysAccept` setting. If set to `true`, permissive mode is enabled in your Gloo Gateway setup and invalid Gloo resources are only logged, but not rejected. If set to `false`, strict validation mode is enabled and invalid resource configuration is rejected before being applied in the cluster. If `allowWarnings=false` is set alongside `alwaysAccept=false`, resources that result in a `Warning` status are also rejected. 

## Monitor the validation status of Gloo resources

When Gloo Gateway fails to process a resource, the error is reflected in the resource's {{< protobuf name="core.solo.io.Status" display="Status">}}. You can run `glooctl check` to easily view any configuration errors on resources that have been admitted to your cluster.

Additionally, you can configure Gloo Gateway to publish metrics that record the configuration status of the resources.

In the `observabilityOptions` of the Settings CRD, you can enable status metrics by specifying the resource type and any labels to apply
to the metric. The following example adds metrics for virtual services and upstreams, which both have labels that include the namespace and name of each individual resource:

```yaml
observabilityOptions:
  configStatusMetricLabels:
    Upstream.v1.gloo.solo.io:
      labelToPath:
        name: '{.metadata.name}'
        namespace: '{.metadata.namespace}'
    VirtualService.v1.gateway.solo.io:
      labelToPath:
        name: '{.metadata.name}'
        namespace: '{.metadata.namespace}'
```

After you complete the [Hello World guide]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) 
to generate some resources, you can see the metrics that you defined at `[http://localhost:9091/metrics](http://localhost:9091/metrics)`. If the port
forwarding is directed towards the Gloo pod, the `default-petstore-8080` upstream reports a healthy state:
```
validation_gateway_solo_io_upstream_config_status{name="default-petstore-8080",namespace="gloo-system"} 0
```

## Test resource configurations

You can use the Kubernetes [dry run capability](#dry-run) to verify your resource configuration or [send requests directly to the Gloo Gateway validation API](#validation-api). 

{{% notice note %}}
The information in this guide assumes that you enabled strict validation, including the rejection of resources that result in a `Warning` state. To enable these settings, update your Gloo Gateway installation and include `--set gateway.validation.alwaysAcceptResources=false`, `--set gateway.validation.enabled=true`, and `--set gateway.validation.allowWarnings=false`.
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
   Error from server: error when creating "STDIN": admission webhook "gloo.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Validating *v1.VirtualService failed: 1 error occurred:
	* Validating *v1.VirtualService failed: validating *v1.VirtualService name:"default"  namespace:"gloo-system": Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream { gloo-system.default-petstore-8080 } not found
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

### Send requests to the validation API directly {#validation-api}

Send a curl request to the validation API to test your resource configurations. For an overview of the fields that you must include as part of your request, see [Validation API reference](#validation-api-reference). 

{{< notice tip >}}
If an empty response <code>{}</code> is returned from the validation API, you might need to add or remove a bracket from your request. This response is returned also if the wrong bracket type is used, such as when you used <code>{}</code> instead of <code>[]</code>. 
{{< /notice >}}
{{% notice note %}}
The validation API currently assumes that all configuration that is sent to the API passes the Kubernetes object schema validation. For example, if your configuration contains valid Gloo configuration, but you use an API version or kind that does not exist in your cluster, the validation API logs a warning, but accepts the request. To ensure that your resource configuration passes the Kubernetes object schema validation, use the [dry run capability in Kubernetes](#dry-run) instead.
{{% /notice %}}

1. Port-forward the gloo service on port 8443. 
   ```sh
   kubectl -n gloo-system port-forward service/gloo 8443:443
   ```

2. Send a request with your resource configuration to the Gloo Gateway validation API. The following example shows successful and unsuccessful resource configuration validation for the upstream, gateway, and virtual service resources.
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
   
   1. Send a request to the validation API to create the upstream resource.
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

### Validation API reference {#validation-api-reference}

The Gloo Gateway validation API is implemented as a validating admission webhook in Kubernetes with the following sample JSON structure:

```json
{
  "request": {
    "uid": "12345",
    "kind": {
      "group": "gateway.solo.io",
      "version": "v1",
      "kind": "VirtualService"
    },
    "resource": {
      "group": "",
      "version": "",
      "resource": ""
    },
    "name": "vs-dry-run",
    "namespace": "gloo-system",
    "operation": "CREATE",
    "userInfo": {
      "username": "system:serviceaccount:kube-system:my-serviceaccount",
      "uid": "system:serviceaccount:kube-system:my-serviceaccount"
    },
    "object": {
      // The resource configuration that you want to validate in JSON format.
    }
  
```

|Parameter|Type|Required|Description|
|--|--|--|--|
|`request.uid`|String|No|A unique identifier for the validation request. You can use this field to find the validation output for a specific resource more easily.|
|`request.kind` |Object|Yes|Information about the type of Kubernetes object that is involved in the validation request. The following fields can be defined: <ul><li> `request.kind.group` (string): The API group of the resource that you want to validate, such as `gateway.solo.io`. </li><li>`request.kind.version` (string): The API version of the resource that you want to validate, such as `v1`. </li><li>`request.kind.kind` (string): The kind of resource that you want to validate, such as `VirtualService`. </li></ul> To find a list of supported group, version, and kind combinations, see the `rules` section in the Gloo Gateway [validating admission webhook configuration](https://github.com/solo-io/gloo/blob/main/install/helm/gloo/templates/5-gateway-validation-webhook-configuration.yaml).|
|`request.resource`|Object|Yes|Information about the resource that is admitted to the webhook. In most cases, the resource defined in `request.kind` and `request.resource` is the same. They might differ only when changes in API versions or variations in resource naming were introduced, or if the resource that you admit belongs to a subresource. If this is the case, you must include the `request.resource` field in your request to the validation API. If `request.kind` and `request.resource` are the same, the `request.resource` section can be omitted. </br></br>  The following fields can be defined: <ul><li> `request.resource.group` (string): The API group of the resource that you admit to the validation API. </li><li>`request.resource.version` (string): The API version of the resource that you want to admit. </li><li>`request.resource.kind` (string): The type of resource that you want to admit. </li></ul> |
|`request.name`|String|No|The name of the resource that you want to validate.|
|`request.namespace`|String|No|The namespace where you want to create, update, or delete the resource. |
|`request.operation`|String|Yes|The operation in Kubernetes that you want to use for your resource. The operation that you can set depends on the resource that you want to validate. You can find supported operations in the `rules` section in the Gloo Gateway [validating admission webhook configuration](https://github.com/solo-io/gloo/blob/main/install/helm/gloo/templates/5-gateway-validation-webhook-configuration.yaml).  |
|`request.userInfo`|Object|No|Information about the user that sends the validation request. The following fields can be provided: <ul><li>`request.userInfo.username` (string): The name of the user that sends the validation request, such as `my-serviceaccount`. </li><li>`request.userInfo.uid` (string): The unique identifier of the user. </li><li>`request.userInfo.groups` (array of strings): A list of groups that the user belongs to.</li></ul> 
|`request.object`|Object|Yes|The resource configuration that you want to validate, such as an upstream, gateway, or virtual service, in JSON format. Refer to the [API reference](https://docs.solo.io/gloo-edge/latest/reference/api/) for more information about the fields that you can set for each resource.|

   
## Disable resource validation in Gloo Gateway

Because the validation admission webhook is set up automatically in Gloo Gateway, a `ValidationWebhookConfiguration` resource is created in your cluster. You can disable the webhook, which prevents the `ValidationWebhookConfiguration` resource from being created. When validation is disabled, any Gloo resources that you create in your cluster are translated to Envoy proxy config, even if the config has errors or warnings. 

To disable validation, use the following `--set` options during installation, or configure your Helm values file accordingly.

```sh
--set gateway.enabled=false
--set gateway.validation.enabled=false
--set gateway.validation.webhook.enabled=false
```

## Questions or feedback

If you have questions or feedback regarding the Gloo Gateway resource validation or any other feature, reach out via the [Slack](https://slack.solo.io/) or open an issue in the [Gloo Gateway GitHub repository](https://github.com/solo-io/gloo).
