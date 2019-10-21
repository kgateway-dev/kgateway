---
title: Internal Usage Statistics
weight: 100
description: Gloo's usage stats collection
---

## Internal Usage Statistics

We periodically collect usage data from instances of Gloo and `glooctl`. The details of this
collection can be found [here](https://github.com/solo-io/reporting-client). Briefly, the data
that is collected includes:

* Operating system
* Architecture
* Usage statistics (number of running Envoy instances, total number of requests handled, etc.)
* CLI args, in the case of `glooctl`

Usage statistics collection can be disabled in Gloo by setting the 
`USAGE_REPORTING_DISABLE` environment variable on the `gloo` pod. This can be done at install 
time by setting the helm value `gloo.deployment.disableUsageStatistics` to `true`.

For `glooctl`, providing the `--disable-usage-statistics` flag serves the same purpose, and disables
the collection of these statistics. 
