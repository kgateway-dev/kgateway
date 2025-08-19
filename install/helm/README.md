# Overview

This directory contains the Helm charts to deploy the kgateway project via [Helm](https://helm.sh/docs/helm/helm_install/).

## Directory Structure

- `kgateway-crds/`: Contains the Custom Resource Definitions (CRDs) chart
  - This chart must be installed before the main kgateway chart
  - Generated from API definitions in `api/v1alpha1`

- `kgateway/`: Contains the control plane chart
  - Deploys the control plane components that extend the Kubernetes Gateway API
  - Includes RBAC configurations in `templates/rbac.yaml` for control plane access
  - Generated from API definitions in `api/v1alpha1`

- `kgateway-dashboards/`: Contains Grafana dashboards which can be used for monitoring.
  - Dashboards install as ConfigMaps, which can be detected automatically, if using kube-prometheus-stack.
  - They are optional and are not required to use kgateway.
  - The dashboards are intended only as a reference implementation for working with control-plane and data-plane metrics.
  - Users are encouraged to extend or expand upon these dashboards based on their particular monitoring needs.

## Installation Order

1. Install the CRDs first:
   ```bash
   helm install kgateway-crds ./kgateway-crds
   ```

2. Install the control plane:
   ```bash
   helm install kgateway ./kgateway
   ```

3. (Optional) Install the monitoring dashboards:
   ```bash
   helm install kgateway-dashboards ./kgateway-dashboards
   ```

For detailed configuration options, please refer to the `values.yaml` file in each chart directory.
