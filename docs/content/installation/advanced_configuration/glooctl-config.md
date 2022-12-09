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

  The maximum length of time to wait, in seconds, before giving up on an entire `glooctl check` call. A value of zero means no timeout.
* `checkTimeoutSeconds: int`. (default 0)

  The maximum length of time to wait, in seconds, before giving up on trying to connect to the cluster. A value of zero means no timeout.
* `checkConnectionTimeoutSeconds: int`.(default 0).

  Override the default value for all the values below.
* `defaultTimeoutSeconds: int`.  (default 0).

  The maximum length of time to wait, in seconds, before giving up on a request for a given resource type. A value of zero means no timeout.
* `deploymentClientSeconds: int`.  (default 0).
* `podClientTimeoutSeconds: int`.  (default 0).
* `settingsClientTimeoutSeconds: int`.  (default 0).
* `upstreamsClientTimeoutSeconds: int`.  (default 0).
* `upstreamGroupsClientTimeoutSeconds: int`.  (default 0).
* `authConfigsClientTimeoutSeconds: int`.  (default 0).
* `rateLimitConfigsClientTimeoutSeconds: int`.  (default 0).
* `virtualHostOptionsClientSeconds: int`.  (default 0).
* `routeOptionsClientSeconds: int`.  (default 0).
* `secretClientTimeoutSeconds: int`.  (default 30).
* `virtualServicesClientTimeoutSeconds: int`.  (default 0).
* `gatewaysClientTimeoutSeconds: int`.  (default 0).
* `proxyClientTimeoutSeconds: int`.  (default 0).
* `xdsMetricsTimeoutSeconds: int`.  (default 0).
