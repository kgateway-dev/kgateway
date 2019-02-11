# Declarative Infrastructure and GitOps

Kubernetes was built to support [declarative configuration management](https://kubernetes.io/docs/concepts/overview/object-management-kubectl/declarative-config/#how-apply-calculates-differences-and-merges-changes). 
With Kubernetes, you can describe the desired state of your application through a set of configuration files, 
and simply run `kubectl apply -f ...`. Kubernetes abstracts away the complexity of computing a diff and redeploying 
pods, services, or other objects that have changed, while making it easy to reason about the end state of the system after a configuration change. 

## GitOps

Configuration changes inherently create risk, in that the new configuration may cause a disruption in a running application. For enterprises, 
the risk of applications breaking can represent a significant financial, reputational, or even existential threat. Operators must be able to 
manage this configuration safely. 

A common approach for managing this risk is to store all of the configuration for an environment (i.e. production, staging, or dev) in a version control 
system like Git, a practice that is sometimes referred to as [GitOps](https://www.weave.works/blog/gitops-operations-by-pull-request). In this methodology, 
the Git repository contains the source of truth for what is deployed to a cluster. Organizations can create processes for 
submitting changes (pull requests), for managing and approving change requests (code reviews), and for automatically deploying 
new configuration when it merges in (CI/CD systems). 

At Solo, we use GitOps to manage the state of our development and production environments, by integrating with 
[GitHub](https://github.com) and [Google Cloud Build](https://cloud.google.com/cloud-build/).
For example, when we want to deploy a new version of Gloo Enterprise to our dev instance, hosted on a [GKE](https://cloud.google.com/kubernetes-engine/) cluster, we open a 
pull request in our repo containing the dev deployment state to update configuration. 
When this pull request is approved and merged in to the master branch, a build trigger runs 
`kubectl` to apply the new configuration. After this configuration is applied, a series of tests are run against the cluster, and 
the team is notified via [Slack](https://slack.com/) about the updates. 
