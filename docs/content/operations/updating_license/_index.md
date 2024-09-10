---
title: Updating Enterprise Licenses
description: How do I replace an expired license for Gloo Gateway Enterprise?
weight: 50
---

{{< readfile file="static/content/license-key" markdown="true">}} This guide provides instructions for how to update the enterprise license.

The license key is stored as a Kubernetes secret in the cluster. When the key expires, the pods that mount the secret might crash. To update the license key, patch the secret and restart the Gloo Gateway deployments. During the upgrade, the data plane continues to run, but you might not be able to modify the configurations for Gloo custom resources through the management plane.

{{% notice tip %}}
When you first install Gloo Gateway in your cluster, confirm the license key expiration date with your Account Representative, such as in **30 days**. Then, set a reminder for before the license key expires, and complete these steps, such as on Day 30, so that your Gloo Gateway pods do not crash.
{{% /notice %}}

## Confirm that your license expired

Whether you're a prospective user using a trial license or a full Gloo Gateway subscriber, this license can expire. When it does, you may see warnings on your Gloo Gateway deployments that are new to you.

For example, when your license expires, you may see the following logs on the `observability` deployment:

```bash
% kubectl logs deploy/observability -n gloo-system
{"level":"warn","ts":"2023-03-23T18:36:16.282Z","caller":"setup/setup.go:85","msg":"LICENSE WARNING: License expired, please contact support to renew."}
```

You can confirm that the license key is expired by copying and pasting your current license key into the [jwt.io debugger](http://jwt.io). Note that the date indicated by the `exp` header is in the past.

![jwt.io Confirms Expired License]({{% versioned_link_path fromRoot="/img/jwt-io-license-expired.png" %}})

## Replace the Expired License

If you're a new user whose trial license has expired, contact your Solo.io Account Representative for a new license key, or fill out [this form](https://lp.solo.io/request-trial). Make sure to note the expiration date so that you can later replace this new license key before it expires.

The Gloo Gateway Enterprise license is installed by default into a Kubernetes `Secret` named `license` in the `gloo-system` namespace. If that is the case for your installation, then you can use a simple bash script to replace the expired key by patching the `license` secret:

1. Save the new license key in an environment variable.
   ```bash
   export GLOO_LICENSE_KEY=<your-new-enterprise-key-string>
   ```

2. Update the `license` secret with the new key.
   ```sh
   kubectl create secret generic --save-config -n gloo-system license --from-literal=license-key=$GLOO_LICENSE_KEY --dry-run=client -o yaml | kubectl apply -f - 
   ```

3. Restart the Gloo deployment to pick up the license secret changes.
   ```bash
   kubectl -n gloo-system rollout restart deploy/gloo
   ```

## Verify the New License

To quickly test whether the new license has resolved your problem, try restarting all the deployments that were stuck in `CrashLoopBackoff` state, like this:

```bash
% kubectl rollout restart deployment observability -n gloo-system
deployment.apps/observability restarted
% kubectl rollout restart deployment gloo-fed-console -n gloo-system
deployment.apps/gloo-fed-console restarted
% kubectl rollout restart deployment gloo-fed -n gloo-system
deployment.apps/
gloo-fed restarted
```

Taking a fresh look at the k9s console shows us that all three of the failing pods have recently restarted -- see the `AGE` attribute -- and are now in a healthy state.

![k9s Display with Refreshed License]({{% versioned_link_path fromRoot="/img/k9s-license-refreshed.png" %}})

Congratulations! You have successfully replaced your Gloo Gateway Enterprise license key.

## Updating legacy-formatted licenses

The internal format of licenses has been recently updated. If your license was created prior to this change, you might see warnings about the deprecated license format similar to the following:

```bash
% kubectl logs deploy/observability -n gloo-system
{"level":"warn","ts":"2023-03-23T18:56:48.554Z","logger":"observability","caller":"client/client.go:195","msg":"Your gloo license graphql addon is outdated. Please contact support to update your license.","version":"1.14.0-beta10"}
```

Gloo Gateway still functions correctly, as both the new and deprecated license formats are accepted. If you want to remove the warning in the logs, you can contact support to get a new license key.