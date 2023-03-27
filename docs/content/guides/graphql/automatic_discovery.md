---
title: Automatic schema generation
weight: 30
description: Explore automatic schema generation with GraphQL service discovery.
---

Gloo Edge can automatically discover API specifications and create GraphQL schemas for gRPC and REST OpenAPI upsteams, as well as GraphQL servers.
* gRPC and REST OpenAPI upsteams: The generated `GraphQLApi` includes the configuration for resolvers and schema definitions for the types of data to return to GraphQL queries. In this case, Gloo Edge uses _local execution_, which means the Envoy server executes GraphQL queries locally before it proxies them to the upstreams that provide the data requested in the queries.
* GraphQL server upstreams (1.14.0 and later only): When you have a GraphQL server upstream that includes its own resolvers, the generated `GraphQLApi` includes only the schema definitions for the types of data to return to GraphQL queries. In this case, Gloo Edge uses _remote execution_, which means that Envoy proxies the requests to the GraphQL server without executing the request first, and the GraphQL upstream both executes the query and provides the requested data. Additionally, Envoy validates the schema of the GraphQL server.

Gloo Edge supports two modes of discovery: allowlist and blocklist. For more information about these discovery modes, see the [Function Discovery Service (FDS) guide]({{% versioned_link_path fromRoot="/installation/advanced_configuration/fds_mode/#function-discovery-service-fds" %}}).

{{% notice note %}}
GraphQL discovery supports only OpenAPI v3.
{{% /notice %}}
{{% notice note %}}
GraphQL discovery can be disabled entirely in the `Settings` resource.
```sh
kubectl patch settings -n gloo-system default --type=merge --patch '{"spec":{"discovery":{"fdsOptions":{"graphqlEnabled":"false"}}}}'
```
{{% /notice %}}

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

For more information about the contents of the generated GraphQL schema, see the [schema configuration documentation]({{% versioned_link_path fromRoot="/guides/graphql/resolver_config/" %}}).
