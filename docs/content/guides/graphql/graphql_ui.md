---
title: GraphQL UI
weight: 20
description: Visualize your GraphQL API and services in the GraphQL UI.
---

Use the Gloo Edge UI to review the health and configuration of your GraphQL APIs, test out your GraphQL API functionality, and create new GraphQL APIs.

## List GraphQL APIs in the Gloo Edge UI

The Gloo Edge UI is served from the `gloo-fed-console` service on port 8090. For more information about how to use the UI, see the [UI documentation]({{< versioned_link_path fromRoot="/guides/gloo_federation/read_only_console/" >}}).

1. Open the Gloo Edge UI.
   * `glooctl`: For more information, see the [CLI documentation]({{< versioned_link_path fromRoot="/reference/cli/glooctl_dashboard/" >}}).
     ```shell
     glooctl dashboard
     ```
   * `kubectl`: Port-forward the `gloo-mesh-ui` service on 8090.
     ```shell
     kubectl port-forward svc/gloo-fed-console -n gloo-system 8090:8090
     ```
2. Open your browser and connect to [http://localhost:8090](http://localhost:8090).
3. Optional: If authentication is enabled, sign in.
4. In the navigation bar, click **APIs**. The GraphQL, REST, and gRPC APIs in your Gloo Edge environment are listed.
5. Under _API Type_, select the GraphQL filter.
6. Review the overview details for each API, such as the namespace it is deployed in, the number of resolvers defined in the API, and the current status of the API.
7. In the **Actions** column, you can optionally download the configuration files for the API, or delete the API configuration from your environment.

<figure><img src="{{% versioned_link_path fromRoot="/img/screenshots/graphql_ov.png" %}}">
<figcaption style="text-align:center;font-style:italic">Figure: GraphQL APIs overview</figcaption></figure>

## Review GraphQL API details

Review the details of a GraphQL API, including its configuration, the resolvers for each query, and more.

1. From the list of GraphQL APIs, click the name of a GraphQL API schema.
2. In the **API Details** tab, you can view the raw configuration files, or explore the details of the _Configuration_, _Schema_, and _Upstreams_ for the API.
   * _Configuration_: Click the **View Raw Config** button to view the raw configuration in the UI, and the **&lt;file-name&gt;.yaml** button to download the configuration YAML file.
   * _Schema_: To review the defined fields and values, click to expand each section. For example, you can expand **Query** section to review the field names that are defined in the GraphQL query, the type of data returned by each field, and the resolver that processes the request and returns the data. Additionally, you can filter the displayed fields by using the search bar.
   * _Upstreams_: To review the upstream services that the GraphQL server exposes, click the name of one of the listed services. The **Upstreams** page for the service opens. For more information about the upstream services page, see [Exploring Virtual Services and Upstreams]({{< versioned_link_path fromRoot="/guides/gloo_federation/read_only_console/#exploring-virtual-services-and-upstreams" >}}).

<figure><img src="{{% versioned_link_path fromRoot="/img/screenshots/graphql_details.png" %}}">
<figcaption style="text-align:center;font-style:italic">Figure: GraphQL API details page</figcaption></figure>

## Test GraphQL API functionality

Explore the functionality of an API by sending sample queries.

1. From the **APIs** overview page, click the name of a GraphQL API schema.
2. Click the **Explore** tab.
3. In the GraphiQL panel, you can specify example requests to send to the GraphQL API. The GraphiQL interface includes autocomplete based on the fields defined in your API configuration. For example, you might select one of your defined queries, and the fields within the query that you want data for.
4. Click **Run**, which sends the request, and returns the response in the middle panel.
5. You can also explore the documentation for the API by clicking the **< Docs** button to expand the Documentation Explorer panel.

## Create a GraphQL API

Define a new GraphQL API by using the UI.

1. From the **APIs** overview page, click **Create API**.
   1. Enter a name for the API.
   2. Select an excecutable API, such as for REST or gRPC services, or a stitched API for combined GraphQL APIs.
   3. Click **Upload Schema** to add a `.gql` configuration file.
2. Click **Create API**. The details page for the API opens.
3. If no resolvers are defined, you might see a warning. To define a resolver:
   1. In the **API Details** tab, expand a configuration _Schema_ section. For example, you might start with the section for the top-level query.
   2. In the **Resolver** column, click **Resolver**.
   3. For the Resolver Type, choose a REST or gRPC resolver. 
   4. For the Upstream, choose a service for the upstream reference. The drop-down list is populated by the upstream services that are currently defined in your Gloo Edge environment.
   5. For the Resolver Config, fill out values for the provided fields. Note that you might not require all provided fields. Additionally, you can choose an existing resolver configuration that you already created from the drop-down list to modify for this field. For more information about how to configure each type of resolver, see [Manual schema configuration]({{< versioned_link_path fromRoot="/guides/graphql/resolver_config/" >}}).
   6. Click **Submit**.
   7. You can optionally click the **View Raw Config** button to verify that the resolver was added to your configuration.
   8. Repeat these steps to define a resolver for each configuration field.
4. To apply changes to your API configuration, toggle **Schema Introspection**, and click **Update**.

<figure><img src="{{% versioned_link_path fromRoot="/img/screenshots/graphql_resolver.png" %}}">
<figcaption style="text-align:center;font-style:italic">Figure: GraphQL API resolver configuration</figcaption></figure>