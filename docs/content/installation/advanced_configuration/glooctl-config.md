---
title: Glooctl Config File
weight: 60
description: Persistent configuration for `glooctl`
---

## Config File

When you use `glooctl`, it tries to read a configuration file located at `$HOME/.gloo/glooctl-config.yaml`. You can override the location of this file by setting the `--config` value (or alias `-f`) when you run a `glooctl` command. If the file does not exist, `glooctl` tries to write it.

You can set the following top-level values.

* `disableUsageReporting: bool`. When set to true, this setting disables the reporting of anonymous usage statistics.

  {{% notice note %}}
  A signature is sent to help deduplicate the usage reports. This signature is a random UUID and contains no personally identifying information. Gloo Edge keeps the signature in-memory in the `gloo` pod, and `glooctl` keeps it on-disk at `~/.soloio/signature`. These signatures can be destroyed at any time with no negative consequences. These signatures will not be written or recorded if usage reporting is disabled as described above.
  {{% /notice %}}

* `secretClientTimeoutSeconds: int`. The maximum length of time to wait, in seconds, before giving up on a secret request (default 30). A value of zero means no timeout.
