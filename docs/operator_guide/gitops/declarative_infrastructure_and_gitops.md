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
submitting changes (pull requests), for managing and approving change requests (code reviews), and for kicking off deployment 
pipelines when changes are merged in (integrating with a CI/CD system). 

For example, in a large enterprise, a team of operators may be responsible for managing the current state of production, 
for which the source of truth is a Git repository containing a set of yaml configurations. In order to push changes to 
production, the following steps must occur: 

1. Developers build, test, and release new versions of a service and publish containers to the enterprise's docker registry.
2. Developers or operators request an upgrade to the new version by opening a pull request and updating the image version in the yaml configuration. 
3. A pre-approval CI process runs to validate the change, attempting to deploy the updated configuration to a sandbox environment. 
4. An operator reviews the change. 
5. When the change has passed checks and been approved, it merges in during an approved maintenance window. 
6. When the change is merged in, the next phase of the deployment pipeline picks up the change and automatically deploys it to production. 
7. A set of acceptance tests are run (by developers, operators, and/or automated systems) against prod to test the newly deployed configuration. 
8. If a problem is detected, a pull request can be quickly opened, tested, and reviewed to revert the configuration change. 

## Using GitOps internally at Solo

At Solo, we use GitOps to manage the state of our development and production environments, by integrating with 
[GitHub](https://github.com) and [Google Cloud Build](https://cloud.google.com/cloud-build/).
For example, after a new version of Gloo Enterprise is released, we want to deploy it our dev instance, which is hosted on a [GKE](https://cloud.google.com/kubernetes-engine/) cluster. 
The developer responsible for the upgrade opens a pull request to the repo containing the dev deployment state, updating the configuration
to the new release. 
When this pull request is approved and merged in to the master branch, a build trigger runs 
`kubectl` to apply the new configuration to the cluster. After this configuration is applied, a series of tests are 
run against the cluster, and the team is notified via [Slack](https://slack.com/) about the updates. This is part of our larger
release and deployment pipeline we use to test new versions of our product.   
