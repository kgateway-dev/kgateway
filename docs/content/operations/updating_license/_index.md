---
title: Updating Enterprise Licenses
description: How do I replace an expired license for Gloo Edge Enterprise?
weight: 50
---

Gloo Edge Enterprise requires a time-limited license key in order to fully operate. You initially provide this license at installation time, as described [here]({{< versioned_link_path fromRoot="/installation/enterprise/" >}}).

The license key is stored as a Kubernetes secret in the cluster. When the key expires, the pods that mount the secret might crash. To update the license key, patch the secret and restart the Gloo Edge deployments.

{{% notice tip %}}
When you first install Gloo Edge in your cluster, confirm the license key expiration date with your Account Representative, such as in **30 days**. Then, set a reminder for before the license key expires, and complete these steps, such as on Day 30, so that your Gloo Edge pods do not crash.
{{% /notice %}}

## Diagnose the Problem

Whether you're a prospective user using a trial license or a full Gloo Edge subscriber, this license can expire. When it does, you may see certain Gloo Edge pods start to display errors that are new to you.

From the [k9s](https://k9scli.io/) display below, you can see that certain `gloo-system` pods fall into a `CrashLoopBackoff` state.

![k9s Display with Expired License]({{% versioned_link_path fromRoot="/img/k9s-license-expired.png" %}})

Following up with `glooctl check` confirms that there's something wrong, but it doesn't precisely point its finger at the root cause of the problem.

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

But if you take a look at the logs for the failing `observability` deployment, they give us a more precise diagnosis:

```bash
% kubectl logs deploy/observability -n gloo-system
{"level":"fatal","ts":1628879186.1552186,"logger":"observability","caller":"cmd/main.go:24","msg":"License is invalid or expired, crashing - license expired","version":"1.8.0","stacktrace":"main.main\n\t/workspace/solo-projects/projects/observability/cmd/main.go:24\nruntime.main\n\t/usr/local/go/src/runtime/proc.go:225"}
```

One easy way to confirm this diagnosis is to paste your current license key into the [jwt.io debugger](http://jwt.io). Note that the date indicated by the `exp` header is in the past.

![jwt.io Confirms Expired License]({{% versioned_link_path fromRoot="/img/jwt-io-license-expired.png" %}})

## Replace the Expired License

If you're a new user whose trial license has expired, contact your Solo.io Account Executive for a fresh one, or fill out [this form](https://lp.solo.io/request-trial).

The Gloo Edge Enterprise license is installed by default into a Kubernetes `Secret` named `license` in the `gloo-system` namespace. If that is the case for your installation, then you can use a simple bash script to replace the expired key by patching the `license` secret:

```bash
GLOO_KEY=your-new-enterprise-key-string
echo $GLOO_KEY | base64 | read output;kubectl patch secret license -n gloo-system -p="{\"data\":{\"license-key\": \"$output\"}}" -v=1
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
