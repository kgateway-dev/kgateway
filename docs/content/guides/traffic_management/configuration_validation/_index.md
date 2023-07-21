---
menuTitle: Configuration Validation
title: Configuration Validation
weight: 60
description: (Kubernetes Only) Gloo Edge can be configured to validate configuration before it is applied to the cluster. With validation enabled, any attempt to apply invalid configuration to the cluster will be rejected.
---

Learn how to prevent invalid Gloo resource configuration from being propagated to the gateway proxies in your cluster to prevent bugs, service outages, or security vulnerabilities.

## Resource validation in Gloo Edge

Kubernetes provides dynamic admission control capabilities that intercept requests to the Kubernetes API server and validate or mutate objects before they are persisted in etcd. Gloo Edge leverages this capability and provides a [validation admission webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) that ensures that only valid Envoy configuration is written to etcd. The configuration 

The following image shows how Gloo Edge validates resource configuration with the validating admission webhook before it is applied in the cluster. 

<figure><img src="{{% versioned_link_path fromRoot="/img/admission-control.svg" %}}"/>
<figcaption style="text-align:center;font-style:italic">Resource validation in Gloo Edge</figcaption></figure>

1. The user performs an action on a Gloo custom resource that is configured in the validating addmission webhook, such as to create a Gloo virtual service. 
2. An API request is sent to the Kubernetes API server that contains the new Gloo resource configuration. After being successfully authenticated, authorized, and processed by any mutating admission webhooks that are configured in the cluster, the configuration is sent to the object schema validation component. This component performs an OpenAPI schema validation to verify that the provided YAML or JSON configuration is valid. If the configuration is not written in valid YAML or JSON format, it is rejected by the Kubernetes API server. For example, if you upgraded your Gloo version, but did not apply the matching Gloo custom resources, you might see errors similar to the following when fields in the custom resource configuration are unknown. 
   ```
   Error: UPGRADE FAILED: error validating "": error validating data: ValidationError(Settings.spec.gateway.validation): unknown field "validationServerGrpcMaxSizeBytes" in io.solo.gloo.v1.Settings.spec.gateway.validation
   ```

3. If the resource configuration passes the schema validation, the configuration is sent to the validation webhook server that is configured with the Gloo Edge validating admission webhook for semantic validation. 
4. The webhook kicks off the processes within the translation engine component to simulate the translation and xDS snapshot creation in Gloo Edge. 
5. The translation engine retrieves the current Gloo Edge snapshot and compares it with the changes in the new Gloo resource configuration. 
6. Gloo Edge tries to translate the resource into valid Envoy configuration and to store this configuration in a proxy resource. 
7. Gloo Edge tries to process the proxy resource along with service discovery data to create the final xDS snapshot. 
8. If the proxy resource was succesfully processed, Gloo Edge creates the xDS snapshot. 
9. The validation result is returned to the validation admission server. 
10. By default, the validating admission webhook is set up in permissive mode and only logs invalid resource configurations without rejecting them. You can [enable strict validation]({{% versioned_link_path fromRoot="/guides/traffic_management/configuration_validation/admission_control/#enable-strict-resource-validation" %}}) to reject invalid resource configuration before it is stored in etcd. Optionally, you can also configure the webhook to reject resource configuration that resulted in a `warning` status. For more information about the validating admission webhook, how to configure and test it, see the [admission control]({{% versioned_link_path fromRoot="/guides/traffic_management/configuration_validation/admission_control/" %}}) guide. 
11. If the resource configuration is found to be schematically and semantically correct, it is admitted by the validation webhook server and persisted in the etcd data store.
12. Gloo Edge monitors the status of custom resources and picks up the latest Gloo custom resource changes from the etcd data store. 
13. The resource configuration runs through the translation engine processes that are described in step 5-8. If the resource is successfully translated, the resource's status is changed to `Accepted` and stored in the proxy resource. If errors are detected during the translation process, Gloo Edge sets the resource's {{< protobuf name="core.solo.io.Status" display="Status">}} to `Rejected` and omits the resource from the proxy resource. For reviously admitted resources that become invalid due to an update, the last valid configuration of that resource in persisted in the proxy resource. If the resource configuration is considered valid, but references missing or misconfigured resources, the validation status is set to `Warning` and the resource is added to the proxy resource. Each proxy has its own configuration. If the configuration for one proxy is invalid, it does not affect any of the other proxies. The proxy that the gateway and its virtual services are applied to can be configured via the `proxyNames` option on the gateway resource. 
14. Gloo Edge processes the proxy along with service discovery data to create the final [Envoy xDS Snapshot](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol). The xDS snapshot is then sent to the Gloo xDS server component. 
15. Proxies in the cluster can pull the latest Envoy configuration from the Gloo xDS server. 

For more information about the validating admission webhook configuration, see the [admission control]({{% versioned_link_path fromRoot="/guides/traffic_management/configuration_validation/admission_control/" %}}) guide. 


## Sanitize resource configuration

Gloo Edge can be configured to pass partially-valid configuration to Envoy by admitting it through an internal process referred to as *sanitizing*. Rather than refusing to update Envoy with invalid configuration, Gloo Edge can replace invalid configuration with preconfigured defaults.

For more information about how to configure and use the Gloo Edge sanitization feature, see the [Route Replacement]({{% versioned_link_path fromRoot="/guides/traffic_management/configuration_validation/invalid_route_replacement/" %}}) guide. 

