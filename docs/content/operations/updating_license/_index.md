---
title: Updating Enterprise Licenses
description: How do I replace an expired license for Gloo Edge Enterprise?
weight: 50
---

Gloo Edge Enterprise requires a time-limited license key in order to fully operate. You initially provide this license at installation time, as described [here]({{< versioned_link_path fromRoot="/installation/enterprise/" >}}).

The license key is stored as a Kubernetes secret in the cluster. When the key expires, the pods that mount the secret might crash. To update the license key, patch the secret and restart the Gloo Edge deployments. During the upgrade, the data plane continues to run, but you might not be able to modify the configurations for Gloo custom resources through the management plane.

{{% notice tip %}}
When you first install Gloo Edge in your cluster, confirm the license key expiration date with your Account Representative, such as in **30 days**. Then, set a reminder for before the license key expires, and complete these steps, such as on Day 30, so that your Gloo Edge pods do not crash.
{{% /notice %}}

## Confirm that your license expired

Whether you're a prospective user using a trial license or a full Gloo Edge subscriber, this license can expire. When it does, you may see certain Gloo Edge pods start to display errors that are new to you.

For example, from the following [k9s](https://k9scli.io/) display, you can see that certain `gloo-system` pods fall into a `CrashLoopBackoff` state.

![k9s Display with Expired License]({{% versioned_link_path fromRoot="/img/k9s-license-expired.png" %}})

Next, you can use `glooctl check` to confirm that the deployments have errors.

```bash
% glooctl check
Checking deployments... 3 Errors!
Checking pods... 6 Errors!
Checking upstreams... OK
Checking upstream groups... OK
Checking auth configs... OK
Checking rate limit configs... OK
Checking VirtualHostOptions... OK
Checking RouteOptions... OK
Checking secrets... OK
Checking virtual services... OK
Checking gateways... OK
Checking proxies... Skipping due to an error in checking deployments
Skipping due to an error in checking deployments
Error: 11 errors occurred:
	* Deployment gloo-fed in namespace gloo-system is not available! Message: Deployment does not have minimum availability.
	* Deployment gloo-fed-console in namespace gloo-system is not available! Message: Deployment does not have minimum availability.
	* Deployment observability in namespace gloo-system is not available! Message: Deployment does not have minimum availability.
	* Pod gloo-fed-6f58b97cb6-5qktr in namespace gloo-system is not ready! Message: containers with unready status: [gloo-fed]
	* Not all containers in pod gloo-fed-6f58b97cb6-5qktr in namespace gloo-system are ready! Message: containers with unready status: [gloo-fed]
	* Pod gloo-fed-console-845767f58-tvl4k in namespace gloo-system is not ready! Message: containers with unready status: [apiserver]
	* Not all containers in pod gloo-fed-console-845767f58-tvl4k in namespace gloo-system are ready! Message: containers with unready status: [apiserver]
	* Pod observability-958575cf6-fkhsw in namespace gloo-system is not ready! Message: containers with unready status: [observability]
	* Not all containers in pod observability-958575cf6-fkhsw in namespace gloo-system are ready! Message: containers with unready status: [observability]
	* proxy check was skipped due to an error in checking deployments
* xds metrics check was skipped due to an error in checking deployments
```

Finally, to get a more precise diagnosis, look at the logs for the failing `observability` deployment.

```bash
% kubectl logs deploy/observability -n gloo-system
{"level":"fatal","ts":1628879186.1552186,"logger":"observability","caller":"cmd/main.go:24","msg":"License is invalid or expired, crashing - license expired","version":"1.8.0","stacktrace":"main.main\n\t/workspace/solo-projects/projects/observability/cmd/main.go:24\nruntime.main\n\t/usr/local/go/src/runtime/proc.go:225"}
```

You can confirm that the license key is expired by copying and pasting your current license key into the [jwt.io debugger](http://jwt.io). Note that the date indicated by the `exp` header is in the past.

![jwt.io Confirms Expired License]({{% versioned_link_path fromRoot="/img/jwt-io-license-expired.png" %}})

## Replace the Expired License

If you're a new user whose trial license has expired, contact your Solo.io Account Representative for a new license key, or fill out [this form](https://lp.solo.io/request-trial). Make sure to note the expiration date so that you can later replace this new license key before it expires.

The Gloo Edge Enterprise license is installed by default into a Kubernetes `Secret` named `license` in the `gloo-system` namespace. If that is the case for your installation, then you can use a simple bash script to replace the expired key by patching the `license` secret:

1. Save the new license key in an environment variable.
   ```bash
   export GLOO_KEY=<your-new-enterprise-key-string>
   ```

2. Encode the license key in base64.
   ```sh
   export GLOO_KEY_BASE64=$(echo $GLOO_KEY | base64 -w 0)
   echo $GLOO_KEY_BASE64
   ```

3. Patch the `license` secret with the base64-encoded key.
   ```sh
   kubectl patch secret license -n gloo-system -p="{\"data\":{\"license-key\": \"$GLOO_KEY_BASE64\"}}" -v=1
   ```

If successful, this script should respond with: `secret/license patched`.

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

Congratulations! You have successfully replaced your Gloo Edge Enterprise license key.
