---
menuTitle: Admission control
title: Admission control
weight: 10
description: (Kubernetes Only) Gloo Edge can be configured to validate configuration before it is applied to the cluster. With validation enabled, any attempt to apply invalid configuration to the cluster will be rejected.
---

Prevent invalid Gloo configuration from being applied to your Kubernetes cluster by using the Gloo Edge validating admission webhook. 

## About the validating admission webhook

The [validating admission webhook configuration](https://github.com/solo-io/gloo/blob/main/install/helm/gloo/templates/5-gateway-validation-webhook-configuration.yaml) is enabled by default when you install Gloo Edge with the Helm chart or the `glooctl install gateway` command. By default, the webhook only logs the validation result without rejecting invalid Gloo resource configuration. If the configuration you provide is written in valid YAML format, it is accepted by the Kubernetes API server and written to etcd. However, the configuration might contain invalid settings or inconsistencies that Gloo Edge cannot interpret or process. This mode is also referred to as permissive validation. 

You can enable strict validation by setting the `alwaysAcceptResources` Helm option to false. Note that only resources that result in a `rejected` status are rejected on admission. Resources that result in a `warning` status are still admitted. To also reject resources with a `warning` status, set `alwaysAcceptResources=false` and `allowWarnings=false` in your Helm file. 

For more information about how resource configuration validation works in Gloo Edge, see [Resource validation in Gloo Edge]({{% versioned_link_path fromRoot="/guides/traffic_management/configuration_validation/#resource-validation-in-gloo-edge" %}}). 

## Enable strict resource validation

Configure the validating admission webhook to reject invalid Gloo custom resources before they are applied in the cluster. 

1. Enable strict resource validation by using one of the following options: 
   * **Update the Helm settings**: Update your Gloo Edge installation and set the following Helm values.
     ```bash
     --set gateway.validation.alwaysAcceptResources=false
     --set gateway.validation.enabled=true
     ```
   * **Update the settings resources**: Add the following `spec.gateway.validation` block to the settings resource. Note that settings that you manually add to this resource might be overwritten during a Helm upgrade. 
     {{< highlight yaml "hl_lines=12-14" >}}
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
  
   {{% notice tip %}}
   To also reject Gloo custom resources that result in a `Warning` status, set `allowWarnings=false`. 
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
      Error from server: error when creating "STDIN": admission webhook "gateway.gloo-system.svc" denied the request: resource incompatible with current Gloo Edge snapshot: [Route 
      Error: InvalidMatcherError. Reason: no path specifier provided]
      ```

      {{< notice tip >}}
      You can also use the validating admission webhook by running the <code>kubectl apply --server-dry-run</code> command to test your Gloo configuration before you apply it to your cluster.
      {{< /notice >}}


## View the current validating admission webhook configuration

You can check whether strict or permissive validation is enabled in your Gloo Edge installation by checking the {{< protobuf name="gloo.solo.io.Settings" display="Settings">}} resource. 

1. Get the details of the default settings resource. 
   ```sh
   kubectl get settings default -n gloo-system -o yaml
   ```

2. In your CLI output, find the `spec.gateway.validation.alwaysAccept` setting. If set to `true`, permissive mode is enabled in your Gloo Edge setup and invalid Gloo resources are only logged, but not rejected. If set to `false`, strict validation mode is enabled and invalid resource configuration is rejected before being applied in the cluster. If `allowWarnings=false` is set alongside `alwaysAccept=false`, resources that result in a `Warning` status are also rejected. 

## Monitor the validation status of Gloo resources

When Gloo Edge fails to process a resource, the error is reflected in the resource's {{< protobuf name="core.solo.io.Status" display="Status">}}. You can run `glooctl check` to easily view any configuration errors on resources that have been admitted to your cluster.

Additionally, you can configure Gloo Edge to publish metrics that record the configuration status of the resources.

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

## Test resource validation 

You can use the Gloo Edge validation API to test if a resource configuration is valid before applying it. 

### Use the `--server-dry-run` capability

To test whether a YAML file is accepted by the validation webhook, you can use the `kubectl apply --server-dry-run` command.

{{< tabs >}}
{{% tab name="Upstream" %}}

1. Create a valid upstream resource.
   ```yaml
   kubectl apply --server-dry-run -f- <<EOF
   apiVersion: gloo.solo.io/v1
   kind: Upstream
   metadata:
     labels:
       app: petstore
       discovered_by: kubernetesplugin
     name: default-petstore-8080
     namespace: gloo-system
   spec:
     discoveryMetadata: {}
     kube:
       selector:
         app: petstore
       serviceName: petstore
       serviceNamespace: default
       servicePort: 8080
   EOF
   ```

   Example output:
   ```
   ```

2. Try to create an upstream without a valid host address and verify that your resource is denied by the validation API. 
   ```yaml
   kubectl apply --server-dry-run -f- <<EOF
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

   Example output:
   ```
   ```

{{% /tab %}}
{{% tab name="Gateway" %}}

1. Create a valid gateway resource.
   ```yaml
   kubectl apply --server-dry-run -n gloo-system -f - <<EOF
   apiVersion: gloo.solo.io/v1
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

   Example output:
   ```
   ```

2. Try to create a gateway resource without a gateway type and verify that your resource is denied by the validation API. 
   ```yaml
   kubectl apply -f- <<EOF 
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-without-type
     namespace: gloo-system
   spec:
     bindAddress: '::'
   EOF
   ```

   Example output:
   ```
   Error from server: error when creating "STDIN": admission webhook "gloo.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Validating *v1.Gateway failed: 1 error occurred:
	   * Validating *v1.Gateway failed: validating *v1.Gateway name:"gateway-without-type"  namespace:"gloo-system": could not render proxy: 2 errors occurred:
	   * invalid resource gloo-system.gateway-without-type
	   * invalid gateway: gateway must contain gatewayType
   ```

{{% /tab %}}
{{% tab name="Virtual service" %}}  
1. Create a valid virtual gateway resource. Note that if you followed the [Hello world]() guide, the virtual service already exists and is denied by the validation API because you cannot 
   ```yaml
   kubectl apply -f- <<EOF
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

   Example output if the virtual service does not exist:
   ```
   ```

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
   kubectl apply -f- <<EOF
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

   Example output:
   ```
   Error from server: error when creating "STDIN": admission webhook "gloo.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Validating *v1.VirtualService failed: 1 error occurred:
	   * Validating *v1.VirtualService failed: validating *v1.VirtualService name:"missing-upstream"  namespace:"default": Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream { anywhere.does-not-exist } not found
   ```

4. Try to create another virtual service that does not specify any
   ```yaml
   kubectl apply -f- <<EOF
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

   Example output:
   ```
   Error from server: error when creating "STDIN": admission webhook "gloo.gloo-system.svc" denied the request: resource incompatible with current Gloo snapshot: [Validating *v1.VirtualService failed: 1 error occurred:
	   * Validating *v1.VirtualService failed: validating *v1.VirtualService name:"method-matcher"  namespace:"default": could not render proxy: 2 errors occurred:
	   * invalid resource default.method-matcher
	   * invalid route: routes with delegate actions must use a prefix matcher
   ```
{{% /tab %}}
{{< /tabs >}}

### Send requests to the validation API directly

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
   
   1. Send a request to the validation API.
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gloo.solo.io","version":"v1","kind":"Upstream"},"resource":{"group":"","version":"","resource":""},"name":"upstream","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object": { "apiVersion": "gloo.solo.io/v1", "kind": "Upstream", "metadata": { "name": "upstream", "namespace": "gloo-system" }, "spec": { "static": { "hosts": [ { "addr": "jsonplaceholder.typicode.com", "port": 80 } ]}}} }}' -H 'Content-Type: application/json' https://localhost:8443/validation 
      ```

      Example output for successful validation:
      ```
      {"response":{"uid":"1234","allowed":true}}
      ```

   1. Change the `port` value from `80` to `"80"`.
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gloo.solo.io","version":"v1","kind":"Upstream"},"resource":{"group":"","version":"","resource":""},"name":"upstream","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object": { "apiVersion": "gloo.solo.io/v1", "kind": "Upstream", "metadata": { "name": "upstream", "namespace": "gloo-system" }, "spec": { "static": { "hosts": [ { "addr": jsonplaceholder.typicode.com, "port": 80 } ]}}} }}' -H 'Content-Type: application/json' https://localhost:8443/validation
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
   
   1. Send a request to the validation API.
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gateway.solo.io","version":"v1","kind":"Gateway"},"resource":{"group":"","version":"","resource":""},"name":"tcp","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object":{ "apiVersion": "gateway.solo.io/v1", "kind": "Gateway", "metadata": { "name": "tcp", "namespace": "gloo-system" }, "spec": { "bindAddress": "::", "bindPort": 8000, "tcpGateway": { "tcpHosts": [ { "name": "one", "destination": { "single": {  "upstream": { "name": "gloo-system-tcp-echo-1025", "namespace": "gloo-system" }} }}]}, "useProxyProto": false }} }}' -H 'Content-Type: application/json' https://localhost:8443/validation  
      ```

      Example output for successful validation:
      ```
      {"response":{"uid":"1234","allowed":true}}
      ```

   2. Change the boolean value in `useProxyProto` from `false` to `"false"`. 
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gateway.solo.io","version":"v1","kind":"Gateway"},"resource":{"group":"","version":"","resource":""},"name":"tcp","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object":{ "apiVersion": "gateway.solo.io/v1", "kind": "Gateway", "metadata": { "name": "tcp", "namespace": "gloo-system" }, "spec": { "bindAddress": "::", "bindPort": 8000, "tcpGateway": { "tcpHosts": [ { "name": "one", "destination": { "single": {  "upstream": { "name": "gloo-system-tcp-echo-1025", "namespace": "gloo-system" }} }}]}, "useProxyProto": "false" }} }}' -H 'Content-Type: application/json' https://localhost:8443/validation  
      ```

      Example output for failed validation:
      ```
      {"response":{"uid":"1234","allowed":false,"status":{"metadata":{},"message":"resource incompatible with current Gloo snapshot: [1 error occurred:\n\t* could not unmarshal raw object: parsing resource from crd spec tcp in namespace gloo-system into *v1.Gateway: json: cannot unmarshal string into Go value of type bool\n\n]","details":{"name":"tcp","group":"gateway.solo.io","kind":"Gateway","causes":[{"message":"Error 1 error occurred:\n\t* could not unmarshal raw object: parsing resource from crd spec tcp in namespace gloo-system into *v1.Gateway: json: cannot unmarshal string into Go value of type bool\n\n"}]}}}}
      ```

   3. Remove an opening bracket from the request to send a request that is symantically incorrect. 
      ```sh
      curl -k -XPOST -d '{"request":"uid":"1234","kind":{"group":"gateway.solo.io","version":"v1","kind":"Gateway"},"resource":{"group":"","version":"","resource":""},"name":"tcp","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object":{ "apiVersion": "gateway.solo.io/v1", "kind": "Gateway", "metadata": { "name": "tcp", "namespace": "gloo-system" }, "spec": { "bindAddress": "::", "bindPort": 8000, "tcpGateway": { "tcpHosts": [ { "name": "one", "destination": { "single": {  "upstream": { "name": "gloo-system-tcp-echo-1025", "namespace": "gloo-system" }} }}]}, "useProxyProto": "false" }} }}' -H 'Content-Type: application/json' https://localhost:8443/validation
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
   
   1. Send a request to the validation API.
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gateway.solo.io","version":"v1","kind":"VirtualService"},"resource":{"group":"","version":"","resource":""},"name":"update-response-code","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object":{ "apiVersion": "gateway.solo.io/v1", "kind": "VirtualService", "metadata": { "name": "update-response-code", "namespace": "gloo-system" }, "spec": { "virtualHost": { "domains": [ "foo.com" ], "routes": [ { "matchers": [ { "prefix": "/" } ], "routeAction": { "single": { "upstream": { "name": "postman-echo", "namespace": "gloo-system" } } }, "options": { "autoHostRewrite": true } } ], "options": { "transformations": { "responseTransformation": { "transformationTemplate": { "headers": { ":status": { "text": "{% if default(data.error.message, \"\") != \"\" %}400{% else %}{{ header(\":status\") }}{% endif %}" } } } } } } } } }}}' -H 'Content-Type: application/json' https://localhost:8443/validation
      ```

      Example output for successful validation:
      ```
      {"response":{"uid":"1234","allowed":true}}
      ```

   2. Change the `options` field to `optional` to force a validation error. 
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gateway.solo.io","version":"v1","kind":"VirtualService"},"resource":{"group":"","version":"","resource":""},"name":"update-response-code","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object":{ "apiVersion": "gateway.solo.io/v1", "kind": "VirtualService", "metadata": { "name": "update-response-code", "namespace": "gloo-system" }, "spec": { "virtualHost": { "domains": [ "foo.com" ], "routes": [ { "matchers": [ { "prefix": "/" } ], "routeAction": { "single": { "upstream": { "name": "postman-echo", "namespace": "gloo-system" } } }, "options": { "autoHostRewrite": true } } ], "optional": { "transformations": { "responseTransformation": { "transformationTemplate": { "headers": { ":status": { "text": "{% if default(data.error.message, \"\") != \"\" %}400{% else %}{{ header(\":status\") }}{% endif %}" } } } } } } } } }}}' -H 'Content-Type: application/json' https://localhost:8443/validation
      ```

      Example output for failed validation:
      ```
      {"response":{"uid":"1234","allowed":false,"status":{"metadata":{},"message":"resource incompatible with current Gloo snapshot: [1 error occurred:\n\t* could not unmarshal raw object: parsing resource from crd spec update-response-code in namespace gloo-system into *v1.VirtualService: unknown field \"optional\" in gateway.solo.io.Route\n\n]","details":{"name":"update-response-code","group":"gateway.solo.io","kind":"VirtualService","causes":[{"message":"Error 1 error occurred:\n\t* could not unmarshal raw object: parsing resource from crd spec update-response-code in namespace gloo-system into *v1.VirtualService: unknown field \"optional\" in gateway.solo.io.Route\n\n"}]}}}}
      ```

   3. Remove a closing bracket from the request to send a request that is symantically incorrect. 
      ```sh
      curl -k -XPOST -d '{"request":{"uid":"1234","kind":{"group":"gateway.solo.io","version":"v1","kind":"VirtualService"},"resource":{"group":"","version":"","resource":""},"name":"update-response-code","namespace":"gloo-system","operation":"CREATE","userInfo":{},"object":{ "apiVersion": "gateway.solo.io/v1", "kind": "VirtualService", "metadata": { "name": "update-response-code", "namespace": "gloo-system" }, "spec": { "virtualHost": { "domains": [ "foo.com" ], "routes": [ { "matchers": [ { "prefix": "/" } ], "routeAction": { "single": { "upstream": { "name": "postman-echo", "namespace": "gloo-system" } } }, "options": { "autoHostRewrite": true } } ], "options": { "transformations": { "responseTransformation": { "transformationTemplate": { "headers": { ":status": { "text": "{% if default(data.error.message, \"\") != \"\" %}400{% else %}{{ header(\":status\") }}{% endif %}" } } } } } } } } }}' -H 'Content-Type: application/json' https://localhost:8443/validation
      ```

      Example output for invalid validation: 
      ```
      {}
      ```

   {{% /tab %}}
   {{< /tabs >}}

   
## Disable resource validation in Gloo Edge

Because the validation admission webhook is set up automatically in Gloo Edge, a `ValidationWebhookConfiguration` resource is created in your cluster. You can disable the webhook, which prevents the `ValidationWebhookConfiguration` resource from being created. When validation is disabled, any Gloo resources that you create in your cluster are translated to Envoy proxy config, even if the config has errors or warnings. 

To disable validation, use the following `--set` options during installation, or configure your Helm values file accordingly.

```sh
--set gateway.enabled=false
--set gateway.validation.enabled=false
--set gateway.validation.webhook.enabled=false
```

## Questions or feedback

If you have questions or feedback regarding the Gloo Edge resource validation or any other feature, reach out via the [Slack](https://slack.solo.io/) or open an issue in the [Gloo Edge GitHub repository](https://github.com/solo-io/gloo).
