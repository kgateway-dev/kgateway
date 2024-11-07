---
title: Argo CD
weight: 30
description: Use Argo CD to automate the deployment and management of Gloo Gateway.
---

[Argo Continuous Delivery (Argo CD)](https://argo-cd.readthedocs.io/en/stable/) is a declarative, Kubernetes-native continuous deployment tool that can read and pull code from Git repositories and deploy it to your cluster. Because of that, you can integrate Argo CD into your GitOps pipeline to automate the deployment and synchronization of your apps. 

## Before you begin 

1. Install the following command line tools: 
   * [`kubectl`](https://kubernetes.io/docs/tasks/tools/#kubectl), the Kubernetes command line tool. Download the `kubectl` version that is within one minor version of the Kubernetes clusters you plan to use.
   * [argocd](https://argo-cd.readthedocs.io/en/stable/cli_installation/), the Argo CD command line tool. 
   
2. Create or use an existing Kubernetes cluster. 

## Set up Argo CD

1. Install Argo CD in your cluster. 
   ```sh
   kubectl create namespace argocd
   until kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/v2.12.3/manifests/install.yaml > /dev/null 2>&1; do sleep 2; done
   # wait for deployment to complete
   kubectl -n argocd rollout status deploy/argocd-applicationset-controller
   kubectl -n argocd rollout status deploy/argocd-dex-server
   kubectl -n argocd rollout status deploy/argocd-notifications-controller
   kubectl -n argocd rollout status deploy/argocd-redis
   kubectl -n argocd rollout status deploy/argocd-repo-server
   kubectl -n argocd rollout status deploy/argocd-server
   ```

2. Update the default Argo CD password for the admin user to solo.io.
   ```sh
   # bcrypt(password)=$2a$10$79yaoOg9dL5MO8pn8hGqtO4xQDejSEVNWAGQR268JHLdrCw6UCYmy
   # password: solo.io
   kubectl -n argocd patch secret argocd-secret \
     -p '{"stringData": {
       "admin.password": "$2a$10$79yaoOg9dL5MO8pn8hGqtO4xQDejSEVNWAGQR268JHLdrCw6UCYmy",
       "admin.passwordMtime": "'$(date +%FT%T%Z)'"
     }}'
   ```
   
3. Port-forward the Argo CD server on port 9999.
   ```sh
   kubectl port-forward svc/argocd-server -n argocd 9999:443
   ```

4. Open the [Argo CD UI](https://localhost:9999/).

5. Log in with the `admin` username and `solo.io` password.

## Install Gloo Gateway
   
1. Create an Argo CD application to install the Gloo Gateway open source Helm chart. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: argoproj.io/v1alpha1
   kind: Application
   metadata:
     name: gloo-gateway-oss-helm
     namespace: argocd
   spec:
     destination:
       namespace: gloo-system
       server: https://kubernetes.default.svc
     project: default
     source:
       chart: gloo
       helm:
         skipCrds: false
         values: |
           kubeGateway:
             enabled: false
       repoURL: https://storage.googleapis.com/solo-public-helm
       targetRevision: {{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}
     syncPolicy:
       automated:
         # Prune resources during auto-syncing (default is false)
         prune: true 
         # Sync the app in part when resources are changed only in the target Kubernetes cluster
         # but not in the git source (default is false).
         selfHeal: true 
       syncOptions:
       - CreateNamespace=true 
   EOF
   ```
   
2. Verify that the `gloo` control plane is up and running.
   ```sh
   kubectl get pods -n gloo-system 
   ```
   
   Example output: 
   ```
   NAME                                  READY   STATUS      RESTARTS   AGE
   discovery-76768ff46-db4cb             1/1     Running     0          76s
   gateway-proxy-7d6b9db55b-6flwm        1/1     Running     0          76s
   gloo-7b5c894cd7-lp7rr                 1/1     Running     0          76s
   gloo-resource-migration-xxqph         0/1     Completed   0          103s
   gloo-resource-rollout-check-bj9ft     0/1     Completed   0          62s
   gloo-resource-rollout-cleanup-j6575   0/1     Completed   0          93s
   gloo-resource-rollout-vzt7s           0/1     Completed   0          76s
   ```

3. Verify that the `gloo-gateway` GatewayClass is created. You can optionally take a look at how the gateway class is configured by adding the `-o yaml` option to your command.
   ```sh
   kubectl get gatewayclass gloo-gateway
   ```

4. Open the Argo CD UI and verify that you see the Argo CD application with a `Healthy` and `Synced` status.

## Optional: Cleanup

If you no longer need this quick-start Gloo Gateway environment, you can uninstall your setup by following these steps: 

{{< tabs >}}
{{% tab name="Argo CD UI" %}}
1. Port-forward the Argo CD server on port 9999.
   ```sh
   kubectl port-forward svc/argocd-server -n argocd 9999:443
   ```

2. Open the [Argo CD UI](https://localhost:9999/applications).

3. Log in with the `admin` username and `solo.io` password.
4. Find the application that you want to delete and click **x**. 
5. Select **Foreground** and click **Ok**. 
6. Verify that the pods were removed from the `gloo-system` namespace. 
   ```sh
   kubectl get pods -n gloo-system
   ```
   
   Example output: 
   ```  
   No resources found in gloo-system namespace.
   ```

{{% /tab %}}
{{% tab name="Argo CD CLI" %}}
1. Port-forward the Argo CD server on port 9999.
   ```sh
   kubectl port-forward svc/argocd-server -n argocd 9999:443
   ```
   
2. Log in to the Argo CD UI. 
   ```sh
   argocd login localhost:9999 --username admin --password solo.io --insecure
   ```
   
3. Delete the application.
   ```sh
   argocd app delete gloo-gateway-oss-helm --cascade --server localhost:9999 --insecure
   ```
   
   Example output: 
   ```
   Are you sure you want to delete 'gloo-gateway-oss-helm' and all its resources? [y/n] y
   application 'gloo-gateway-oss-helm' deleted   
   ```

4. Verify that the pods were removed from the `gloo-system` namespace. 
   ```sh
   kubectl get pods -n gloo-system
   ```
   
   Example output: 
   ```  
   No resources found in gloo-system namespace.
   ```
{{% /tab %}}
{{< /tabs >}}