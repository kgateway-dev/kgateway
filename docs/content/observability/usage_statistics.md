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

`glooctl` records a unique ID in the gloo config directory 
(see [this page](../../advanced_configuration/glooctl-config#config-file) in our docs for more info
on where that directory can be found) in a file named `usage-signature`. This contains no 
personally-identifying information; it is just a random UUID used to associate multiple 
usage records with the same source.

Usage statistics collection can be disabled in Gloo by setting the 
`DISABLE_USAGE_REPORTING` environment variable on the `gloo` pod. This can be done at install 
time by setting the helm value `gloo.deployment.disableUsageStatistics` to `true`.

For `glooctl`, providing the `--disable-usage-statistics` flag serves the same purpose, and disables
the collection of these statistics. 
