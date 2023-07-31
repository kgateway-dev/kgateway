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

If the port forwarding is switched to the gateway pod, you can see the metrics defined for virtual services by
revisiting the metrics endpoint: `[http://localhost:9091/metrics](http://localhost:9091/metrics)`.
```
validation_gateway_solo_io_virtual_service_config_status{name="default",namespace="gloo-system"} 0
```

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
