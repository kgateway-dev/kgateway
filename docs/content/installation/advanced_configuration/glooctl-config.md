---
title: Glooctl Config File
weight: 60
description: Persistent configuration for `glooctl`
---

## Config File

When `glooctl` is invoked, it will attempt to read a configuration yaml file located at `$HOME/.gloo/glooctl-config.yaml`. The location of this file can be overridden by setting the `--config` value (aliased to `-f`) when invoking `glooctl`. If the file does not exist, `glooctl` will attempt to write it.

The available top-level values to set are:

* `disableUsageReporting` - `bool`; use this to disable the reporting of anonymous usage statistics. More information about this can be found on the corresponding docs page [here]({{< versioned_link_path fromRoot="/reference/usage_statistics/" >}}).

We send a signature along with these reports to help deduplicate them. This signature is just a random UUID and contains no personally-identifying information. Gloo Edge keeps it in-memory in the `gloo` pod, and `glooctl` will persist it on-disk at `~/.soloio/signature`. These signatures can be destroyed at any time with no negative consequences. These signatures will not be written or recorded if usage reporting is disabled as described above.
