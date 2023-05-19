---
title: Federated Configuration
description: Setting up federated configuration
weight: 30
---

Gloo Edge Federation enables you to create consistent configurations across multiple Gloo Edge instances. You might configure resources such as Upstreams, UpstreamGroups, and VirtualServices. In this guide, you learn how to add a Federated Upstream and VirtualService to a registered cluster that is managed by Gloo Edge Federation.

![Figure of federated architecture]({{% versioned_link_path fromRoot="/img/gloo-fed-arch-fed-res.png" %}})

## Prerequisites

To successfully follow this guide, you need to have Gloo Edge Federation deployed on an admin cluster and a registered cluster to use for your configuration. For instructions, follow the Gloo Edge Federation [installation guide]({{% versioned_link_path fromRoot="/guides/gloo_federation/installation/" %}}) and [cluster registration guide]({{% versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" %}}).

## Create the Federated Resources

In this guide, you create a Federated Upstream and Federated VirtualService. You can do this by using `kubectl` to create the necessary Custom Resources. After the CRs are created, the Gloo Edge Federation controller creates the necessary resources on the designated clusters in the configured namespace.

In this example, you use the admin cluster where Gloo Edge Federation is running. You can select a different cluster by changing the placement values. The registered cluster is named `local` and Gloo Edge Enterprise is using the default `gloo-system` namespace.

To list available clusters, run the following command.
```shell
kubectl --context gloo-fed -n gloo-system get kubernetesclusters
```

### Create the Federated Upstream

1. Create the Federated Upstream by running the following command in the context of the admin cluster where Gloo Edge Federation is running.
   * `placement`: Specify that the Upstream should be created in the `local` cluster in the `gloo-system` namespace. 
   * `template`: Define the properties of the Upstream being created, such as the static host address and port.

   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: fed.gloo.solo.io/v1
   kind: FederatedUpstream
   metadata:
     name: my-federated-upstream
     namespace: gloo-system
   spec:
     placement:
       clusters:
         - local
       namespaces:
         - gloo-system
     template:
       spec:
         static:
           hosts:
             - addr: solo.io
               port: 80
       metadata:
         name: fed-upstream
   EOF
   ```
2. Verify that the Upstream is successfully created.

   ```
   kubectl get federatedupstreams -n gloo-system -oyaml
   ```

   In the `status` output, check that the state is `PLACED`.

   ```yaml
     status:
       placementStatus:
         clusters:
           local:
             namespaces:
               gloo-system:
                 state: PLACED
         observedGeneration: "1"
         state: PLACED
         writtenBy: gloo-fed-5dd98c7bfd-96sn2
   ```

3. Verify that the Upstream is created in the registered cluster, `local`.

   ```
   kubectl get upstream -n gloo-system fed-upstream
   ```
   
   ```
   NAME              AGE
   fed-upstream      97m
   ```

Now, you can create a VirtualService for the Upstream.

### Create a Federated Virtual Service

1. Create a VirtualService that exposes the Upstream from the previous step. Run the following command in the context of the admin cluster where Gloo Edge Federation runs.

   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: fed.gateway.solo.io/v1
   kind: FederatedVirtualService
   metadata:
     name: my-federated-vs
     namespace: gloo-system
   spec:
     placement:
       clusters:
         - local
       namespaces:
         - gloo-system
     template:
       spec:
         virtualHost:
           domains:
             - "*"
           routes:
             - matchers:
                 - exact: /solo
               options:
                 prefixRewrite: /
               routeAction:
                 single:
                   upstream:
                     name: fed-upstream
                     namespace: gloo-system
       metadata:
         name: fed-virtualservice
   EOF
   ```
2. Verify that the VirtualService is successfully created.

   ```
   kubectl get federatedvirtualservice -n gloo-system -oyaml
   ```

   In the `status` output, check that the state is `PLACED`.

   ```yaml
   status:
     placementStatus:
       clusters:
         local:
           namespaces:
             gloo-system:
               state: PLACED
       observedGeneration: "1"
       state: PLACED
       writtenBy: gloo-fed-5dd98c7bfd-96sn2
   ```
Once we run the command, we can validate that it was successful by running the following:

```
kubectl get federatedvirtualservice -n gloo-system -oyaml
```

In the resulting output you should see the state as `PLACED` in the status section:

```yaml
  status:
    placementStatus:
      clusters:
        local:
          namespaces:
            gloo-system:
              state: PLACED
      observedGeneration: "1"
      state: PLACED
      writtenBy: gloo-fed-5dd98c7bfd-96sn2
```
3. Verify that the VirtualService is created in the registered cluster, `local`.

   ```
   kubectl get virtualservice -n gloo-system fed-virtualservice
   ```
   
   ```
   NAME              AGE
   fed-virtualservice   4m39s
   ```

Now, any updates that you make to the Federated Upstream or Federated VirtualService are automatically applied to all of the registered clusters that have the Custom Resource.

## Check all the federated resources
From the admin cluster that runs `gloo-fed`, you can run `glooctl check` to verify all the resources are OK, across the clusters.

```shell
glooctl check
```

```shell
Checking deployments... OK
Checking pods... OK
Checking upstreams... OK
Checking upstream groups... OK
Checking auth configs... OK
Checking rate limit configs... OK
Checking VirtualHostOptions... OK
Checking RouteOptions... OK
Checking secrets... OK
Checking virtual services... OK
Checking gateways... OK
Checking proxies... OK
Checking rate limit server... OK
No problems detected.

Detected Gloo Federation!

Checking Gloo Instance remote-1-gloo-system...
Checking deployments... OK
Checking pods... OK
Checking settings... OK
Checking upstreams... OK
Checking upstream groups... OK
Checking auth configs... OK
Checking virtual services... OK
Checking route tables... OK
Checking gateways... OK
Checking proxies... OK


Checking Gloo Instance remote-2-gloo-system...
Checking deployments... OK
Checking pods... OK
Checking settings... OK
Checking upstreams... OK
Checking upstream groups... OK
Checking auth configs... OK
Checking virtual services... OK
Checking route tables... OK
Checking gateways... OK
Checking proxies... OK
```

Note that it is best to have `gloo-edge` installed on this admin cluster. To do so, if needed, you can use the following values and command.
````shell
cat <<EOF > values-local.yaml
gloo:
  license_secret_name: gloo-license # default license name was already used by the gloo-fed Helm release
gloo-fed: # disable because gloo-fed is already deployed as a seperate gloo-fed Helm release
  enabled: false
  glooFedApiserver:
    enable: false
EOF

helm upgrade -i gloo glooe/gloo-ee --namespace gloo-system --version ${GLOO_VERSION} \
  --create-namespace --set-string license_key="$LICENSE_KEY" -f values-local.yaml
````


## Next Steps

Setting up Federated Configuration also enables Service Failover. You can check out the [guide for Service Failover]({{% versioned_link_path fromRoot="/guides/gloo_federation/service_failover/" %}}) next, or learn more about the [concepts]({{% versioned_link_path fromRoot="/introduction/gloo_federation/" %}}) behind Gloo Edge Federation.