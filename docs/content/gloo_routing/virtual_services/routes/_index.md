---
title: Routes
weight: 10
description: Routes are the primary building block of the virtual service. A route contains matchers and an upstream destination.
---


Routes are the primary building block of the *Virtual Service*. A route contains matchers and an upstream which could be a single destination, a list of weighted destinations, or an upstream group. 

There are many types of **matchers**, including **Path Matching**, **Header Matching**, **Query Parameter Matching**, and **HTTP Method Matching**. Matchers can be combined in a single rule to further refine which requests will be matched against that rule.

Gloo is capable of sending matching requests to many different types of *Upstreams*, including **Single Upstream**, **Multiple Upstream**, **Upstream Groups**, Kubernetes services, and Consul services. The ability to route a request to multiple *Upstreams* or *Upstream Groups* allows Gloo to load balance requests and perform Canary Releases.

Gloo can also alter requests before sending them to a destination, including **Transformation**, **Fault Injection**, response header editing, and **Prefix Rewrite**. The ability to edit requests on the fly gives Gloo the power specify the proper parameters for a function or transform and error check incoming requests before passing them along.

The sections in Routes are listed below for reference. If you'd like to get started with routing, then try out different matching rules starting with [Path Matching]({{% versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/matching_rules/path_matching/" %}}).

---

{{% children description="true" depth="2" %}}
