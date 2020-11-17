---
title: Internal Usage Statistics
weight: 40
description: Gloo Edge's usage stats collection
---

## Internal Usage Statistics

We periodically collect usage data from instances of Gloo Edge and `glooctl`. The details of this
collection can be found [here](https://github.com/solo-io/reporting-client). Briefly, the data
that is collected includes:

* Operating system
* Architecture
* Usage statistics (number of running Envoy instances, total number of requests handled, etc.)
* CLI args, in the case of `glooctl`

`glooctl` records a unique ID in the .soloio config directory 
(see [this page]({{% versioned_link_path fromRoot="/installation/advanced_configuration/glooctl-config/#config-file" %}}) in our docs for more info
on where that directory can be found) in a file named `signature`. This contains no 
personally-identifying information; it is just a random UUID used to associate multiple 
usage records with the same source. As with the rest of this data, it will not be created or
recorded if this statistics collection is disabled.

Usage statistics collection can be disabled in Gloo Edge by setting the 
`DISABLE_USAGE_REPORTING` environment variable on the `gloo` pod. This can be done at install 
time by setting the helm value `gloo.deployment.disableUsageStatistics` to `true`.
For `glooctl`, setting the `disableUsageReporting` key to `true` in its config file serves
the same purpose. See the docs page referenced above for more information on the config
file for `glooctl`.
