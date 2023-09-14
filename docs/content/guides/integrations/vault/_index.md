---
title: Vault Integration
description: Managing secrets using HashiCorp Vault
weight: 7
---

# Overview

Gloo Edge can integrate with HashiCorp Vault to provide an alternative way to manage secrets.

Managing secrets using HashiCorp Vault has many benefits such as:
- **Centralized Secret Management**: Vault provides a centralized solution for managing secrets which can be accessed across different environments. Only storing secrets in Kubernetes, on the other hand, gets limited so a single cluster, which may not be sufficient for complex or multi-cloud architectures.
- **Advanced Access Control**: Vault offers fine-grained access control and authentication mechanisms, allowing you to define policies which determine who can access and manage secrets.
- **Security Integrations**: Vault provides multiple methods of securing access to secrets through identity provider integration (IAM, LDAP, OAuth, etc.)
- **Secret Rotation**: Vault supports secret rotation and TTL (time-to-live) for secrets, which reduces the window or opportunity for attackers to use stolen credentials.
- **Secret Auditing**: Vault provides a detailed audit log of all secret access and management, which can be used for compliance and security purposes.
