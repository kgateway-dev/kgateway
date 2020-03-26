---
title: Frequently-Asked Questions
weight: 4
description: Frequently-Asked Questions about our upgrade steps
---

- **Is the upgrade procedure any different if I'm playing with Gloo in a non-production/sandbox environment?**

If downtime is not a concern for your use case, the easiest upgrade procedure is simply to completely
uninstall Gloo and start from scratch, thereby avoiding any annoyances around breaking changes.
Use `glooctl uninstall --all` followed by the normal installation procedure for the version of your choice.
 
- **What is the recommended way to upgrade if I'm running Gloo in a production environment, where downtime is unacceptable?**

We are still working on an exact recommendation in this case, but enterprise customers have found success
in performing a blue/green deployment using two simultaneous deployments of Gloo. There are still questions
outstanding about, for example, how to maintain Gloo state (virtual services, settings, etc.) across the
different deployments, if the blue/green deployment is happening across datacenters.

We will be providing more docs on this soon, but in the meantime, reach out to us on our 
[public Slack](https://slack.solo.io/).

- **What will happen to my upstreams, virtual services, settings, and Gloo state in general?**

A normal upgrade of Gloo across minor versions should not cause any disruption to existing Gloo state. In
the case of a breaking change, we will communicate through the changelog or other channels if some other
adjustment must be made to perform the upgrade.

As of open-source Gloo version 0.21.1, there is a command available in `glooctl` that can help mitigate
some concern about Gloo state: `glooctl debug yaml` can be used to dump the current Gloo state to one
large YAML manifest. While this command is not yet really suitable as a robust backup tool, it is
a very useful debug tool to have available.

- **How do I handle upgrading across a breaking change?**

See above; the short answer is that we will try to clearly communicate what, if anything, should be
done to accommodate preserving Gloo state during an upgrade.

- **Is the upgrade procedure any different if I am not an administrator of the cluster being installed to?**

If you are not an administrator of your cluster, you may have trouble creating both custom resource definitions
and other cluster-scoped resources (like RBAC ClusterRoles/ClusterRoleBindings). If you run into trouble with
this during an installation, you can disable the creation of these resources by overriding the values:

```yaml
global:
  glooRbac:
    create: false
crds:
  create: false
```

You may also try performing an installation of Gloo that is scoped to a single namespace:

```yaml
global:
  glooRbac:
    namespaced: true
```

- **Why do I get an error about re-creating CRDs when upgrading using `helm install` or `helm upgrade`?**

See the explanation in the [upgrade steps]({{% versioned_link_path fromRoot="/upgrading/upgrade_steps/#using-helm" %}}). Helm v2 does not manage CRDs well, so you may have to delete the CRDs and try again, or install using some other method.
