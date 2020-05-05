---
title: Authentication and Authorization
weight: 20
description: An overview of authentication and authorization options with Gloo.
---

## Why Authenticate in API Gateway Environments

API Gateways act as a control point for the outside world to access the various application services (monoliths, microservices, serverless functions) running in your environment. In microservices or hybrid application architecture, any number of these workloads need to accept incoming requests from external end users (clients). Incoming requests are treated as anonymous or authenticated and depending on the service. You may want to establish and validate who the client is, the service they are requesting, and define any access or traffic control policies.

Gloo provides several mechanisms for authenticating requests. The external authentication (Ext Auth) service is a Gloo Enterprise feature that runs within the cluster and connects to authentication environments using a plugin system. Ext Auth can hook into LDAP, OIDC, or API keys and pass the authentication process over to those Identity Providers.

You can also use JSON Web Tokens (JWT) to authenticate requests. In this case, Gloo merely needs to trust the source of the token and not necessarily perform an authentication handoff.

Finally, you can write your own custom authentication service and integrate it with Gloo. 

The Ext Auth section below includes guides for all the different authentication sources supported out of the box, and a guide to creating your own plugins for a specialized authentication source. Also included in this section is a guide for developing a Custom Auth service and guides for working with JSON Web Tokens.


{{% children description="true" depth="2" %}}
