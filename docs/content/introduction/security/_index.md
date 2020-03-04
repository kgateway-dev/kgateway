---
title: Security
weight: 30
---

One of the core responsibilities of an API Gateway is to secure your cluster. This can take the form of applying network encryption, invoking external authentication, or filtering requests with a Web Application Firewall (WAF). The following sections expand on the different aspects of security in Gloo and provide links to guides for implementing the features.

Some of the security features are only available on the Enterprise version of Gloo. They have been marked as such where applicable.

---

## External Authentication

API Gateways act as a control point for the outside world to access the various application running in your environment. These application need to accept incoming requests from external end users. The incoming requests can be treated as anonymous or authenticated and depending on the requirements of your application. External authentication provides you with the ability to establish and validate who the client is, the service they are requesting, and define access or traffic control policies.

External authentication is a Gloo Enterprise feature. It is possible to implement a [custom authentication server] when using the open-source version of Gloo.

External authentication in Gloo supports several forms of authentication:

* Basic authentication - simple username and password
* OAuth - authentication using OpenID Connect (OIDC)
* JSON Web Tokens - cryptographically signed tokens
* API Keys - long-lived, secure UUIDs
* OPA Authorization - fine-grained policies with the Open Policy Agent
* LDAP - Lightweight Directory Access Protocol for common LDAP or Active Directory

---

## Network Encryption

An API gateway sits between the downstream client and the upstream service it wants to connect with. The network traffic between the API gateway and the downstream client, and between the API gateway and the upstream service should be encrypted using Transport Layer Security (TLS). Gloo Gateway acts as the control plane, configuring Envoy through the xDS protocol. Ideally, that network traffic should also be encrypted through mutual TLS (mTLS).

Mutual TLS requires that both the client and server present valid and trusted certificates when creating the TLS tunnel. Server-side TLS only requires that the server present a valid and trusted certificate to the client.

Gloo is capable of configuring [server TLS] with downstream clients, [client TLS] with upstream services, and [mTLS between Gloo and Envoy].

---

## Rate limiting

API Gateways act as a control point for the outside world to access the various application running in your environment.  Incoming requests can possibly overwhelm the capacity of your upstream services, resulting in poor performance and reduced functionality. Using an API gateway we can define client request limits to upstream services and protect them from becoming overwhelmed.



---

## Open Policy Agent
