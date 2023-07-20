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

## Enable strict resource validation 

The [validating admission webhook configuration](https://github.com/solo-io/gloo/blob/main/install/helm/gloo/templates/5-gateway-validation-webhook-configuration.yaml) is enabled by default when you install Gloo Edge with the Helm chart or the `glooctl install gateway` command. By default, the webhook only logs the validation result without rejecting invalid Gloo resource configuration. If the configuration you provide is written in valid YAML format, it is accepted by the Kubernetes API server and written to etcd. However, the configuration might contain invalid settings or inconsistency that Gloo Edge cannot interpret or process. This mode is also referred to as permissive validation. 

You can enable strict validation by setting the `alwaysAcceptResources` Helm option to false. Note that only resources that result in a `rejected` status are rejected on admission. Resources that result in a `warning` status are still admitted. To also reject resources with a `warning` status, set `alwaysAcceptResources=false` and `allowWarnings=false` in your Helm file. 



When `alwaysAccept` is `true` (currently the default is `true`), resources will only be rejected when Gloo Edge fails to 
deserialize them (due to invalid JSON/YAML).


1. Enable strict resource validation by using one of the following options: 
   * **Update the Helm settings**: Update your Gloo Edge installation and set the following Helm values.
     ```bash
     --set gateway.validation.alwaysAcceptResources=false
     --set gateway.validation.enabled=true
     ```
   * **Update the settings resources**: Add the following `spec.gateway` block to the settings resource. 
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

2. Create a virtual service that includes invalid Gloo configuration. 
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

3. Verify that the Gloo resource is rejected. You see an error message similar to the following.
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


## Questions or feedback 

If you have questions or feedback regarding the Gloo Edge resource validation or any other feature, reach out via the [Slack](https://slack.solo.io/) or open an issue in the [Gloo Edge GitHub repository](https://github.com/solo-io/gloo). 

-----
This admission webhook can be disabled 
by removing the `ValidatingWebhookConfiguration`.





## Using the Validating Admission Webhook

Admission Validation provides a safeguard to ensure Gloo Edge does not halt processing of configuration. If a resource 
would be written or modified in such a way to cause Gloo Edge to report an error, it is instead rejected by the Kubernetes 
API Server before it is written to persistent storage.












