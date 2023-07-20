---
menuTitle: Admission control
title: Admission control
weight: 10
description: (Kubernetes Only) Gloo Edge can be configured to validate configuration before it is applied to the cluster. With validation enabled, any attempt to apply invalid configuration to the cluster will be rejected.
---

Prevent invalid Gloo configuration from being applied to your Kubernetes cluster by using the Gloo Edge validating admission webhook. 

## About the validating admission webhook

Kubernetes provides dynamic admission control capabilities that intercept requests to the Kubernetes API server and validate or mutate objects before they are persisted in etcd. Gloo Edge leverages this capability and provides a [validation admission webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) that ensures that only valid Envoy configuration is written to etcd. If you create or modify a `gateway.solo.io` custom resource, the resource configuration is validated by the validation admission webhook and, if considered invalid, rejected by the Kubernetes API server.  

The validating admission webhook is invoked for the following Gloo resources: 
- {{< protobuf name="gateway.solo.io.Gateway" display="Gateways">}},
- {{< protobuf name="gateway.solo.io.VirtualService" display="Virtual Services">}},
- {{< protobuf name="gateway.solo.io.RouteTable" display="Route Tables">}}

The following image shows how the validation admission webhook validates Gloo Edge configuration before it is applied in the cluster. 

<figure><img src="{{% versioned_link_path fromRoot="/img/admission-control.svg" %}}"/>
<figcaption style="text-align:center;font-style:italic">Resource validation in Gloo Edge</figcaption></figure>

1. The user creates Gloo gateway, virtual services, or route table resources in the cluster.
2. The Gloo resource configuration is sent to the Kubernetes API server. The API server performs an OpenAPI schema validation to verify that the provided YAML or JSON configuration is valid. 
3. If schematically validated, the resource configuration is sent to the validation webhook server that is configured with the Gloo Edge validating admission webhook for semantic validation. The webhook verifies that Gloo Edge can successfully read and process the resource configuration.
4. If the resource configuration is found to be schematically and semantically correct, it is persisted in the etcd data store.
5. The resource configuration is sent to the Gloo Edge xDS server where the configuration is further processed and translated into valid Envoy configuration. Translation errors and warning are logged and can be accessed as metrics. 
6. The Envoy configuration is then applied to the gateway proxy. 

The [validating admission webhook configuration](https://github.com/solo-io/gloo/blob/main/install/helm/gloo/templates/5-gateway-validation-webhook-configuration.yaml) is enabled by default when you install Gloo Edge with the Helm chart or the `glooctl install gateway` command. By default, the webhook only logs the validation result without rejecting invalid Gloo resource configuration. If the configuration you provide is written in valid YAML format, it is accepted by the Kubernetes API server and written to etcd. However, the configuration might contain invalid settings or inconsistencies that Gloo Edge cannot interpret or process. This mode is also referred to as permissive validation. 

You can enable strict validation by setting the `alwaysAcceptResources` Helm option to false. Note that only resources that result in a `rejected` status are rejected on admission. Resources that result in a `warning` status are still admitted. To also reject resources with a `warning` status, set `alwaysAcceptResources=false` and `allowWarnings=false` in your Helm file. 

## Enable strict resource validation 

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

      {{% notice tip %}}
      You can also use the validating admission webhook by running the `kubectl apply --server-dry-run` command to test your Gloo configuration before you apply it to your cluster.
      {{% /notice %}}

## View the current validating admission webhook settings

You can check whether strict or permissive validation is enabled in your Gloo Edge installation by checking the {{< protobuf name="gloo.solo.io.Settings" display="Settings">}} resource. 

1. Get the details of the default settings resource. 
   ```sh
   kubectl get settings default -n gloo-system -o yaml
   ```

2. In your CLI output, find the `spec.gateway.validation.alwaysAccept` setting. If set to `true`, permissive mode is enabled in your Gloo Edge setup and invalid Gloo resources are only logged, but not rejected. If set to `false`, strict validation mode is enabled and invalid resource configuration is rejected before being applied in the cluster. 

## Disable resource validation in Gloo Edge

Because the validation admission webhook is set up automatically in Gloo Edge, a `ValidationWebhookConfiguration` resource is created in your cluster. To disable the webhook and prevent the `ValidationWebhookConfiguration` from being created, set the following values in your Helm values file: 

```sh
--set gateway.enabled=false
--set gateway.validation.enabled=false
--set gateway.validation.webhook.enabled=false
```

## Questions or feedback 

If you have questions or feedback regarding the Gloo Edge resource validation or any other feature, reach out via the [Slack](https://slack.solo.io/) or open an issue in the [Gloo Edge GitHub repository](https://github.com/solo-io/gloo). 



















