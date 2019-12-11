---
title: Virtual Services
weight: 20
description: Virtual Services define a "virtual API" that gets exposed to clients along with a set of routing rules to backend upstream services / functions.
---

*Virtual Services* define a "virtual API" that gets exposed to clients along with a set of routing rules to backend upstream services / functions. You can also specify TLS/SNI configuration at this level to present certificates to callers and even require certificates from clients (mutual TLS). See the [Virtual Service section](../../introduction/concepts#virtual-services) in the concepts documentation for more background.

The sections in *Virtual Services* are listed below for reference. If you are new to *Virtual Services* we recommend starting with the [Hello World]({{% versioned_link_path fromRoot="/gloo_routing/hello_world/" %}}) example and then trying out different matching rules starting with [Path Matching]({{% versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/matching_rules/path_matching/" %}}).

---

{{% children description="true" depth="2" %}}