---
title: Add support information
description: Review the details to include in your support. 
weight: 930
---

Collect valuable information for Solo to review and troubleshoot your support request. 

## Environment

1. Get your Gloo Gateway version. 
    ```shell
    glooctl version -o yaml
    ```
2. Get the version of Kubernetes that you run in your cluster. 
    ```shell
    kubectl version -o yaml
    ```
3. List the infrastructure provider that hosts your environment, such as AWS or an on-premise virtual machine (VM). 

## Setup

Share the installation method that you used to install Gloo Gateway, such as Helm, `glooctl`, or Argo CD, and the configuration that you used during the installation. 

{{< tabs >}}
{{% tab name="Helm" %}}
Use the following command to extract the Helm manifests that were used for the Gloo Gateway installation and the state of each manifest. Replace `releaseName` and `namespaceName` with the values that you used for your installation.
```shell
helm get all <releaseName> --namespace <namespaceName> > gloo_edge_helm.yaml
```
  
{{% /tab %}}

{{% tab name="glooctl"%}}
Share the complete command that you used to install Gloo Gateway. The following command shows an example command that you might have used. 
```shell
glooctl install gateway enterprise --license-key <license> --values foo,bar
```
{{% /tab %}}

{{% tab name="Argo CD"%}}
Get the Argo CD applications that define the installation of Gloo Gateway by using one of the following methods. 
* `kubectl`: 
  ```shell
  kubectl get applications.argoproj.io/<applicationName> -n <namespaceName> -o yaml
  ```
* Argo CD CLI: 
  ```shell
  argocd app get <applicationName> -o yaml
  ```
{{% /tab %}}
{{< /tabs >}}

## Issue

1. Provide a detailed description of the issue. Make sure to include the following information: 
   - If reproducable, steps to reproduce the issue 
   - If applicable, sample of a client request including the payload 
   - High-level diagram of the interactions from the perspective of the client request
   - The request protocol that is handled by the application(s) in question, such as HTTP / TCP / gRPC 
   - If the issue is related to authentication or authorization, details of the auth configuration
2. Describe the impact of the issue. For example, the issue might block an update or a demo, or cause the loss of data or an entire system.
3. Export the relevant configuration files that are related to the issue.
  {{< tabs >}}
  {{% tab name="Gloo Gateway resources"%}}
  - Typically, the Gloo Gateway `Settings` object is useful to understand the configuration of Gloo Gateway.
  - For traffic management issues, include the following list of Gloo Gateway resources:
      - `Gateway`
      - `VirtualService`
      - `RouteTable`
      - `Upstream`

  - Use the following script to dump all Gloo Gateway custom resources into a file. Attach the `gloo-gateway-configuration.yaml` file to your support request. 
    ```shell
    for n in $(kubectl get crds | grep solo.io | awk '{print $1}'); do kubectl get $n --all-namespaces -o yaml >> gloo-gateway-configuration.yaml; echo "---" >> gloo-gateway-configuration.yaml; done
    ```
  {{% /tab %}}
  {{% tab name="Istio resources"%}}
  If you use Gloo Gateway as a gateway to an Istio service mesh, provide details about how Istio is configured. You can use the following command to create an Istio bug report that you can attach to your support request. 

  ```shell
  istioctl bug-report --istio-namespace <istioControlPlaneNamespace>
  ```
  {{% /tab %}}
  {{< /tabs >}}

## Product-specific details

### Control plane

1. Capture the output of the `glooctl check` command.
    <br>Typically, the command output indicates any errors in the control plane components or associated resources, such as in the following example.

    ```
    Checking Deployments... 1 Errors!
    Checking Pods... 2 Errors!
    Checking Upstreams... OK
    Checking UpstreamGroups... OK
    Checking AuthConfigs... OK
    Checking RateLimitConfigs... OK
    Checking VirtualHostOptions... OK
    Checking RouteOptions... OK
    Checking Secrets... OK
    Checking VirtualServices... OK
    Checking Gateways... OK
    Checking Proxies... Skipping due to an error in checking deployments
    Skipping due to an error in checking deployments
    Error: 5 errors occurred:
    * Deployment gloo in namespace gloo-system is not available! Message: Deployment does not have minimum availability.
    * Pod gloo-8ddc4ff4c-g4mnf in namespace gloo-system is not ready! Message: containers with unready status: [gloo]
    * Not all containers in pod gloo-8ddc4ff4c-g4mnf in namespace gloo-system are ready! Message: containers with unready status: [gloo]
    * proxy check was skipped due to an error in checking deployments
    * xds metrics check was skipped due to an error in checking deployment
    ```
2. Collect the logs for various control plane components, such as `gloo`, `gloo-fed`, `redis`, or `observability` by using the `debug` log level (if possible). The components vary depending on your Gloo Gateway setup and can be found in the `gloo-system` namespace. At a minimum, include the logs for the `gloo` pod in your support request. 
    <br>To enable the `debug` log level, see [Debugging control plane]({{< versioned_link_path fromRoot="/operations/debugging_gloo/#debugging-the-control-plane" >}}).
    <br><br>Follow the steps below to get the logs for the `gloo` controller pod.
    1. Set the log level to `debug`.
        ```shell
        kubectl port-forward deploy/gloo -n <controlplaneNamespace> 9091:9091 > /dev/null 2>&1 &
        PID=$!
        curl -X PUT -H "Content-Type: application/json" -d '{"level": "debug"}' http://localhost:9091/logging
        ```
    2. Capture the logs when reproducing the issue.
        ```shell
        kubectl logs -f deploy/gloo -n <controlplaneNamespace> > gloo.log
        ```
    3. After you capture the logs, reset the log level to `info`.
        ```shell
        curl -X PUT -H "Content-Type: application/json" -d '{"level": "info"}' http://localhost:9091/logging
        kill -9 $PID
        ```
    Repeat these steps for all the control plane components.

### Data plane

1. Capture the xDS configuration that is currently served.
   ```shell
   glooctl proxy served-config -n <controlplaneNamespace> > served-config.yaml
   ```
2. Get the configuration that is served in the `gateway-proxy` Envoy pod(s). 
   <br>For more information, see [Dumping Envoy configuration]({{< versioned_link_path fromRoot="/operations/debugging_gloo/#dumping-envoy-configuration" >}}).
   ```shell
   kubectl port-forward deploy/gateway-proxy -n <proxyNamespace> 19000:19000 > /dev/null 2>&1 &
   PID=$!
   curl -s localhost:19000/config_dump\?include_eds > gateway-config.json
   kill -9 $PID
   ```
3. Get the access log(s) for failed request from the `gateway-proxy` pod(s). If access logging is not enabled, refer to [this guide]({{< versioned_link_path fromRoot="/guides/security/access_logging" >}}) to enable it.
4. If possible, collect the logs from the `gateway-proxy` Envoy pod(s) in `debug` log level for the failed request.
   {{% notice tip %}}
   The `gateway-proxy` component comes with several loggers. Setting the log level to `debug` for all loggers can get very noisy. Instead, you can change the log level for a specific logger only. For more information, see [Viewing Envoy logs]({{< versioned_link_path fromRoot="/operations/debugging_gloo/#viewing-envoy-logs" >}}).
   {{% /notice %}}
   1. Choose the logger that you want to get logs for. For a list of available loggers, see [Viewing Envoy logs]({{< versioned_link_path fromRoot="/operations/debugging_gloo/#viewing-envoy-logs" >}}).
   2. Port-forward the `gateway-proxy` pod  on port 19000.
        ```shell
        kubectl -n gloo-system port-forward deploy/gateway-proxy 19000 &
        ```
    3. Change the log level to `debug` for the selected logger. The following example changes the log level for the `grpc` logger. 
        ```shell
        curl -X POST "127.0.0.1:19000/logging?grpc=debug"
        ```
        
    4. Capture the logs when reproducing the issue. 
        ```shell
        kubectl logs -f deploy/gateway-proxy -n gloo-system > gateway-proxy.log
        ```
    3. After you capture the logs, reset the log level to `info`.
        ```shell
        curl -X POST "127.0.0.1:19000/logging?grpc=info"
        ```
5. Gather the stats from the proxy pod(s).
   ```shell
   glooctl proxy stats > proxy-stats.log
   ```