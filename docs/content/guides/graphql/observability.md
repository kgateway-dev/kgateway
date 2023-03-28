---
title: Observability
weight: 60
description: Gather metrics for your GraphQL services in Prometheus and Grafana.
---

Gather metrics for your GraphQL APIs in Prometheus and Grafana.

## Access Envoy logs in Prometheus and Grafana

Review the Gloo Edge [Observability documentation]({{< versioned_link_path fromRoot="/guides/observability/" >}}) to access the default Prometheus and Grafana deployments that are installed with Gloo Edge, or configure your own Prometheus or Grafana instances.

For example, you can use the following commands to access the Envoy pod logs in the default Prometheus and Grafana dashboards:
* Prometheus:
  ```sh
  kubectl -n gloo-system port-forward deployment/gateway-proxy 19000

  curl http://localhost:19000/stats/prometheus
  ```
* Grafana:
  ```sh
  kubectl -n gloo-system port-forward deployment/glooe-grafana 3000

  open http://localhost:3000/
  ```

## Envoy metrics for GraphQL

The following Envoy metrics are collected for GraphQL APIs in your Gloo Edge environment. The metrics include the name of your GraphQL API resource with hyphens replaced by underscores, such as `<api_name>` in the following example.

```
envoy_gloo_system_<api_name>_graphql_Query_productsForHome_rest_resolver_failed_resolutions
envoy_gloo_system_<api_name>_graphql_Query_productsForHome_rest_resolver_total_resolutions
envoy_gloo_system_<api_name>_graphql_author_rest_resolver_failed_resolutions 
envoy_gloo_system_<api_name>_graphql_author_rest_resolver_total_resolutions
envoy_gloo_system_<api_name>_graphql_pages_rest_resolver_failed_resolutions
envoy_gloo_system_<api_name>_graphql_pages_rest_resolver_total_resolutions
envoy_gloo_system_<api_name>_graphql_review_rest_resolver_failed_resolutions envoy_gloo_system_<api_name>_graphql_review_rest_resolver_total_resolutions
envoy_gloo_system_<api_name>_graphql_rq_error
envoy_gloo_system_<api_name>_graphql_rq_invalid_query_error
envoy_gloo_system_<api_name>_graphql_rq_parse_json_error
envoy_gloo_system_<api_name>_graphql_rq_parse_query_error
envoy_gloo_system_<api_name>_graphql_rq_total
envoy_gloo_system_<api_name>_graphql_year_rest_resolver_failed_resolutions
envoy_gloo_system_<api_name>_graphql_year_rest_resolver_total_resolutions
```

## Using logs for debugging

If you encounter errors or unexpected behavior in your Gloo GraphQL setup, you can use logs to help debug your resources.

1. Edit your `GraphQLApi` resource.
   ```sh
   kubectl edit graphqlapi -n gloo-system <name>
   ```

2. Add the `logSensitiveInfo: true` setting to the `spec.options` section.
   {{< highlight yaml "hl_lines=9-10" >}}
   apiVersion: graphql.gloo.solo.io/v1beta1
   kind: GraphQLApi
   metadata:
     name: my-api
     namespace: gloo-system
   spec:
     executableSchema:
       ...
     options:
       logSensitiveInfo: true
   {{< /highlight >}}

3. Use the following command. Logs are now collected for your GraphQL resources, and are served by the gateway proxy pod.
   ```sh
   glooctl proxy logs debug