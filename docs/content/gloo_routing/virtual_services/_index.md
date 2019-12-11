---
title: Virtual Services
weight: 20
description: Virtual Services define a "virtual API" that gets exposed to clients along with a set of routing rules to backend upstream services / functions.
---

Virtual Services define a "virtual API" that gets exposed to clients along with a set of routing rules to backend upstream services / functions. You can also specify TLS/SNI configuration at this level to present certificates to callers and even require certificates from clients (mutual TLS). See the [Virtual Service section](../../introduction/concepts#virtual-services) in the concepts documentation for more.

{{% children description="true" depth="2" %}}