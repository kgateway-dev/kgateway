---
title: Authentication and Authorization
weight: 20
description: An overview of authentication and authorization options with Gloo Edge.
---

## Why Authenticate in API Gateway Environments

API Gateways act as a control point for the outside world to access the various application services (monoliths, microservices, serverless functions) running in your environment. In microservices or hybrid application architecture, any number of these workloads need to accept incoming requests from external end users (clients). Incoming requests are treated as anonymous or authenticated and depending on the service. You may want to establish and validate who the client is, the service they are requesting, and define any access or traffic control policies.

Gloo Edge provides several mechanisms for authenticating requests. Gloo Edge Enterprise includes an external auth (Ext Auth) service that has built in support for authenticating with Identity Providers over LDAP or OIDC. It also supports other forms of authentication, including basic auth and API keys. Ext Auth has a plugin framework so that custom business logic for bespoke auth protocols can be loaded and configured easily with Gloo Edge. Ext Auth also supports a dynamic, flexible language called Rego for applying fine-grained authorization policies using Open Policy Agent. Ext Auth configuration can be chained to perform a multi-step authentication and authorization process.

You can also use JSON Web Tokens (JWT) to authenticate requests. In this case, Gloo Edge merely needs to trust the source of the token and not necessarily perform an authentication handoff. JWT verification is fast, requires minimal resources, and can be performed directly in Envoy, rather than as a remote call to the external auth service.

Finally, you can write your own custom authentication service and integrate it with Gloo Edge. 

The Ext Auth section below includes guides for all the different authentication sources supported out of the box, and a guide to creating your own plugins for a specialized authentication source. Also included in this section is a guide for developing a Custom Auth service and guides for working with JSON Web Tokens.


{{% children description="true" depth="2" %}}
