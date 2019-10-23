---
title: Glooctl Config File
weight: 60
description: Persistent configuration for `glooctl`
---

## Config File

When `glooctl` is invoked, it will attempt to read a configuration yaml file
located at `$HOME/.gloo/glooctl-config.yaml`. The location of this file can be overridden
by setting the `--config` value (aliased to `-f`) when invoking `glooctl`. If the file does
not exist, `glooctl` will attempt to write it.

The available top-level values to set are:

* `disableUsageReporting` - `bool`; use this to disable the reporting of anonymous usage
statistics. More information about this can be found on the corresponding docs page 
[here](../../observability/usage_statistics).
