---
title: Glooctl Config File
weight: 60
description: Persistent configuration for `glooctl`
---

## Config File

When you use `glooctl`, it tries to read a configuration file located at `$HOME/.gloo/glooctl-config.yaml`. You can override the location of this file by setting the `--config` value (or alias `-f`) when you run a `glooctl` command. If the file does not exist, `glooctl` tries to write it.

You can set the following top-level values.

* `checkTimeoutSeconds: int`. The maximum length of time to wait, in seconds, before giving up on an entire `glooctl check` call. A value of zero means no timeout. (default 0s)
