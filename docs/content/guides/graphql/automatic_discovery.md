---
title: Automatic schema generation
weight: 30
description: Explore automatic schema generation with GraphQL service discovery.
---

<!-- **QUESTION do we need to add the behind-the-scenes info for this feature?** eg Sai has a great overview here: https://docs.google.com/presentation/d/1ArxEdVVAOowz4wYcXIYlg4Wd-psadUTrOYH9DPPWwpk/edit#slide=id.g10a8760f3dd_24_181 -->

Gloo Edge can automatically discover API specifications and create GraphQL schemas. The generated `GraphQLApi` includes the configuration for REST <!--or gRPC -->resolvers and schema definitions for the types of data to return to graphQL queries.

Gloo Edge supports two modes of discovery: allowlist and blocklist. For more information about these discovery modes, see the [Function Discovery Service (FDS) guide]({{% versioned_link_path fromRoot="/installation/advanced_configuration/fds_mode/#function-discovery-service-fds" %}}).

## Allowlist mode

In allowlist mode, discovery is enabled manually for only specific services by labeling those services with the `function_discovery=enabled` label. This mode gives you full manual control over which services you want to expose as GraphQL services.

1. Label services for discovery.
   ```sh
   kubectl label service <service_name> discovery.solo.io/function_discovery=enabled
   ```

2. Allow automatic generation of GraphQL schemas by enabling FDS discovery in allowlist mode.
   ```sh
   kubectl patch settings -n gloo-system default --type=merge --patch '{"spec":{"discovery":{"fdsMode":"WHITELIST"}}}'
   ```

## Blocklist mode

In blocklist mode, discovery is enabled for all supported services, unless you explicitly disable discovery for a service by using the `function_discovery=disbled` label.

1. Label services that you do not want to be discovered.
   ```sh
   kubectl label service <service_name> discovery.solo.io/function_discovery=disabled
   ```

2. Allow automatic generation of GraphQL schemas by enabling FDS discovery in blocklist mode.
   ```sh
   kubectl patch settings -n gloo-system default --type=merge --patch '{"spec":{"discovery":{"fdsMode":"BLACKLIST"}}}'
   ```

## Verifying automatic schema generation

You can verify that OpenAPI specification discovery is enabled by viewing the GraphQL custom resource that was automatically generated for your service.
```sh
kubectl get graphqlapis -n gloo-system
```
```sh
kubectl get graphqlapis <api_name> -o yaml -n gloo-system
```

For more information about the contents of the generated GraphQL schema, see the [schema configuration documentation]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/graphql/resolver_config/" %}}).