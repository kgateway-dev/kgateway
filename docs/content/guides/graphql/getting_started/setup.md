---
title: Install GraphQL
weight: 10
description: Install GraphQL in Gloo Edge and enable API spec discovery for upstreams.
---

Set up API gateway and GraphQL server functionality for your apps in the same process by using Gloo Edge.

{{% notice note %}}
This feature is available only in Gloo Edge Enterprise. Remote execution is supported only in versions 1.14.0 and later.
{{% /notice %}}

## Step 1: Install GraphQL in Gloo Edge

Enable GraphQL functionality in your Gloo Edge installation.

1. [Contact your account representative](https://www.solo.io/company/talk-to-an-expert/) to request a Gloo Edge Enterprise license that specifically enables the GraphQL capability.

2. Install or update Gloo Edge with your GraphQL-enabled license key.  For the latest available version, see the [Gloo Edge Enterprise changelog]({{% versioned_link_path fromRoot="/reference/changelog/enterprise/" %}}).
   * Install:
     ```sh
     glooctl install gateway enterprise --version {{< readfile file="static/content/version_gee_latest.md" markdown="true">}} --license-key=<GRAPHQL_ENABLED_LICENSE_KEY>
     ```
   * Update: See the steps for [updating your license]({{% versioned_link_path fromRoot="/operations/updating_license/" %}}).

## Step 2: Enable API spec discovery for upstreams

To allow Gloo Edge to automatically discover API specifications, turn on FDS discovery. When discovery is enabled, Gloo Edge automatically creates `graphqlapi` resources based on the discovered specifications. Discovery is supported for gRPC, OpenAPI, and GraphQL server upstreams.

```sh
kubectl patch settings -n gloo-system default --type=merge --patch '{"spec":{"discovery":{"fdsMode":"BLACKLIST"}}}'
```

Note that this setting enables discovery for all upstreams. To enable discovery for only specified upstreams, see the [Function Discovery Service (FDS) guide]({{% versioned_link_path fromRoot="/installation/advanced_configuration/fds_mode/#function-discovery-service-fds" %}}).

**Up next**: [Explore basic GraphQL service discovery with the Pet Store sample application.]({{% versioned_link_path fromRoot="/guides/graphql/getting_started/simple_discovery" %}})