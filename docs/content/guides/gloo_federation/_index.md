---
title: Gloo Edge Federation
description: Gloo Edge Federation documentation
weight: 55
---

Gloo Edge Federation brings additional value to Gloo Edge with the following capabilities:
- **Federated configuration**: allows users to manage the configuration for all of their Gloo Edge instances from one place, no matter what platform they run on 
- **Cross-cluster failover**: if a given service is not available locally, then Gloo Federation will re-route the request to the Gateway of the closest cluster  
- **Multicluster RBAC**: decide who can configure what part of Gloo Edge on which cluster(s)

The following sections describe how to use Gloo Edge Federation.

{{% children description="true" %}}