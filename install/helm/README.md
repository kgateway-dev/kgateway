# Overview

This directory contains the resources to deploy the project via [Helm](https://helm.sh/docs/helm/helm_install/).

## Directory Structure

- `kgateway`: contains the WIP kgateway chart

### /kgateway

The kgateway chart contains the new Kubernetes Gateway API implementation. The RBAC configurations in `templates/rbac.yaml` are generated from the API definitions in `api/v1alpha1` using kubebuilder's controller-gen tool.
