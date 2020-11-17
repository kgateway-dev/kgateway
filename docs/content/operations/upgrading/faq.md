---
title: Frequently-Asked Questions
weight: 20
description: Frequently-Asked Questions about our upgrade steps
---

- **Is the upgrade procedure any different if I'm playing with Gloo Edge in a non-production/sandbox environment?**

If downtime is not a concern for your use case, the easiest upgrade procedure is simply to completely
uninstall Gloo Edge and start from scratch, thereby avoiding any annoyances around breaking changes.
Use `glooctl uninstall --all` followed by the normal installation procedure for the version of your choice.
 
- **What is the recommended way to upgrade if I'm running Gloo Edge in a production environment, where downtime is unacceptable?**

In Gloo Edge 1.2 and newer, we recommend `helm upgrade` with the proper readiness probes and healthchecks configured (see
the [1.3.0+ upgrade guide]({{< versioned_link_path fromRoot="/operations/upgrading/1.3.0" >}})). For versions prior
to Gloo Edge 1.2, enterprise customers have found success performing a blue/green deployment using two simultaneous deployments
of Gloo Edge. For a brief example, see the
[1.0.0 example upgrade]({{< versioned_link_path fromRoot="/operations/upgrading/1.0.0#example-upgrade-process" >}}).

If you have concerns not addressed in the docs here, reach out to us on our [public Slack](https://slack.solo.io/).

- **What will happen to my upstreams, virtual services, settings, and Gloo Edge state in general?**

A normal upgrade of Gloo Edge across minor versions should not cause any disruption to existing Gloo Edge state. In
the case of a breaking change, we will communicate through the changelog or other channels if some other
adjustment must be made to perform the upgrade.

As of open-source Gloo Edge version 0.21.1, there is a command available in `glooctl` that can help mitigate
some concern about Gloo Edge state: `glooctl debug yaml` can be used to dump the current Gloo Edge state to one
large YAML manifest. While this command is not yet really suitable as a robust backup tool, it is
a useful debug tool to have available.

- **How do I handle upgrading across a breaking change?**

See above; the short answer is that we will try to clearly communicate what, if anything, should be
done to accommodate preserving Gloo Edge state during an upgrade.

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

You may also try performing an installation of Gloo Edge that is scoped to a single namespace:

```yaml
global:
  glooRbac:
    namespaced: true
```

- **Why do I get an error about re-creating CRDs when upgrading using `helm install` or `helm upgrade`?**

See the explanation in the [upgrade steps]({{% versioned_link_path fromRoot="/operations/upgrading/upgrade_steps/#using-helm" %}}). Helm v2 does not manage CRDs well, so you may have to delete the CRDs and try again, or install using some other method.
