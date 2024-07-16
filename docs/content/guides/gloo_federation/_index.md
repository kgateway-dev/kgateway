---
title: Gloo Gateway Federation
description: Gloo Gateway Federation documentation
weight: 55
---

Gloo Gateway Federation adds the following capabilities to Gloo Gateway:
- **Federated configuration**: You can manage the configuration for all of your Gloo Gateway instances from one place, no matter what platform the instances run on.
- **Cross-cluster failover**: Gloo Gateway automatically re-routes requests to the gateway of the closest cluster if the service is not available locally. 
- **Multicluster RBAC**: You can control user access to view and configure Gloo Gateway resources across clusters based on the user role.

The following sections describe how to use Gloo Gateway Federation.

{{% children description="true" %}}