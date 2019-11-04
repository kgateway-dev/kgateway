---
title: Gloo RBAC Configuration
weight: 70
description: Options for Gloo's RBAC configuration
---

## Conditional Installation

You may want to prevent Gloo's RBAC resources from being created at install time. An example of this use
case is if you are installing to a cluster where you have insufficient permissions to create privileged
resources like RBAC roles and role-bindings.

Gloo's RBAC resources will not be installed if the Helm flag `global.glooRbac.create` is set to `false`.

Our Grafana and Prometheus subcharts will still attempt to create their own RBAC resources. If you would
like to also prevent those resources from being creeated, you must also set the flags `grafana.rbac.create`
and `prometheus.rbac.create` to `false`.

## Uniquely Identifying Cluster-Scoped Resources

By default, Gloo's RBAC resources are created as cluster-scoped. This may present a problem if, for example,
you upgrade Gloo by performing a 
[blue-green deployment](https://blog.christianposta.com/deploy/blue-green-deployments-a-b-testing-and-canary-releases/)
and deploying Gloo to two namespaces simultaneously within the same cluster. The identically-named 
cluster-scoped roles and role-bindings will conflict with each other and will cause an error.

To resolve this, you may set the Helm flag `global.glooRbac.roleSuffix` to some string of your choosing.
That will cause the string `"-$roleSuffix"` to be appended to the roles' and role-bindings' names, where
`$roleSuffix` is the string you passed to the Helm flag. For example, setting `global.glooRbac.roleSuffix`
to the string `"blue-deployment"` will cause the ClusterRole normally named `gloo-resource-reader` to
instead be named `gloo-resource-reader-blue-deployment`.

{{% notice note %}}
Our Grafana and Prometheus subcharts currently do not allow customization of their own RBAC resources' names, so
you may still have a conflict on these. The current best-practice is to either disable the install of
Grafana/Prometheus, or manually fix after installation.
{{% /notice %}}
