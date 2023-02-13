---
title: Security
weight: 30
---

One of the core responsibilities of an API Gateway is to secure your cluster. This can take the form of applying network encryption, invoking external authentication, or filtering requests with a Web Application Firewall (WAF). The following sections expand on the different aspects of security in Gloo Edge and provide links to guides for implementing the features.

Some of the security features are only available on the Enterprise version of Gloo Edge. They have been marked as such where applicable.

---

## External authentication

API Gateways act as a control point for the outside world to access the various applications running in your environment. These applications need to accept incoming requests from external end users. The incoming requests can be treated as anonymous or authenticated depending on the requirements of your application. External authentication provides you with the ability to establish and validate who the client is, the service they are requesting, and define access or traffic control policies.

{{% notice note %}}
External authentication is a Gloo Edge Enterprise feature. It is possible to implement a [custom authentication server]({{% versioned_link_path fromRoot="/guides/security/auth/custom_auth/" %}}) when using the open-source version of Gloo Edge.
{{% /notice %}}

### Humans versus machines

One of the key factors to consider when working with authentication is the consumer of the service. Humans and machines tend to authentication is different ways. A typical human authentication interaction would involved a redirect to an authentication provider, which would allow the human being to input credentials and receive an authentication token. The sections dealing with [external authentication]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/" %}}) and building a custom authentication server can be useful for this context.

Machines are more likely to have a pre-provisioned authentication token using JSON Web Tokens. They will not be using a manual authentication process or be redirected to an Identity Provider. The section dealing with [JSON Web Tokens]({{% versioned_link_path fromRoot="/guides/security/auth/jwt/" %}}) can be useful when planning machine access. 

In addition to considering how machines will authenticate, it is probably a good idea to consider rate-limiting. Machines are more likely to access an endpoint programmatically at a high volume. Implementing rate limits will prevent a particular machine source from overwhelming your service.

### External authentication types

External authentication in Gloo Edge supports several forms of authentication:

* **[Basic authentication]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/basic_auth/" %}})** - simple username and password
* **[OAuth]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/oauth/" %}})** - authentication using [OpenID Connect](https://openid.net/connect/) (OIDC)
* **[JSON Web Tokens]({{% versioned_link_path fromRoot="/guides/security/auth/jwt/" %}})** - cryptographically signed tokens
* **[API Keys]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/apikey_auth/" %}})** - long-lived, secure UUIDs
* **[OPA Authorization]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/opa/" %}})** - fine-grained policies with the Open Policy Agent
* **[LDAP]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/ldap/" %}})** - Lightweight Directory Access Protocol for common LDAP or Active Directory

---

## Network encryption

An API gateway sits between the downstream client and the upstream service it wants to connect with. The network traffic between the API gateway and the downstream client, and between the API gateway and the upstream service should be encrypted using Transport Layer Security (TLS). Gloo Edge can configure [server TLS]({{% versioned_link_path fromRoot="/guides/security/tls/server_tls//" %}}) to present a valid certificate to downstream clients and [client TLS]({{% versioned_link_path fromRoot="/guides/security/tls/client_tls//" %}}) to present a valid certificate to upstream services.

We must also consider the control plane used by Gloo Edge to configure Envoy through the xDS protocol. The xDS communication may contain sensitive data, and should be encrypted through mutual TLS (mTLS) to validate the identity of both parties and encrypt the traffic between them. Mutual TLS requires that both the client and server present valid and trusted certificates when creating the TLS tunnel. Gloo Edge is capable of configuring [mTLS between Gloo Edge and Envoy]({{% versioned_link_path fromRoot="/guides/security/tls/mtls/" %}}).

---

## Rate limiting

API Gateways act as a control point for the outside world to access the various applications running in your environment.  Incoming requests could possibly overwhelm the capacity of your upstream services, resulting in poor performance and reduced functionality. While setting these limits at the application level is possible, it greatly increases the complexity and administrative burden. Using an API gateway, we can define client request limits to upstream services, ensure that limits are enforced consistently, and protect the services from becoming overwhelmed all from a single control point.

Gloo Edge makes use of the [rate-limit API in Envoy]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/" %}}), exposing it through the `settings` spec for Gloo Edge. Enhanced options are available in Gloo Edge Enterprise for features like [rule priority]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/#rule-priority-and-weights" %}}), a [simplified API]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/simple/" %}}), [custom data store backing]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/enterprise/" %}}), and [rate-limit metrics]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/metrics/" %}}).

---

## Open Policy Agent

When configuring permissions for your developers and administrators, you may wish to take a granular and proscriptive approach. While Kubernetes RBAC does allow the creation of policies to govern who may perform actions on a given resource, it does not allow for granular controls within that resource. The [Open Policy Agent](https://www.openpolicyagent.org/docs/latest/) (OPA) is an open-source, general-purpose policy engine that works across microservices, Kubernetes, API gateways, and more. Gloo Edge is able to leverage OPA to create extensive and granular controls over the Custom Resources utilized by Gloo Edge to manage objects like Virtual Services and Upstreams.

As a simple example, you may want to allow a developer access to create Virtual Services, but only for the domain *example.com*. Kubernetes RBAC would allows you to grant a developer access to create the Virtual Service, but it does not have a way to constrain the creation to a specific domain. OPA can evaluate the Virtual Service custom resource when it is submitted and reject any Virtual Services that are not defined for *example.com*.

The [following guide]({{% versioned_link_path fromRoot="/guides/security/opa/" %}}) shows how to configure a simple OPA policy dictating that all Virtual Services must not have a prefix re-write.

---

## Personal information

Personally identifying information (PII) includes names, certain IP addresses, email addresses, or other data that might identify you or your customers. To prevent PII, consider implementing security practices such as the following:

* Do not include personal information in the name of resources such as Kubernetes namespaces or Gloo Edge virtual services.
* Store personal information properly in your environment, such as in Kubernetes secrets or with a secret manager such as HashiCorp Vault.
* [Install Gloo Edge]({{% versioned_link_path fromRoot="/installation/" %}}) with the `gatewayProxies.gatewayProxy.disableCoreDumps` Helm setting. This setting disables writing core dumps that might have personal information in case of an Envoy crash.
* [Set up Data Loss Prevention (DLP)]({{% versioned_link_path fromRoot="/guides/security/data_loss_prevention/" %}}) to prevent personal information in your network traffic from being logged or leaked.

---

## Next Steps

Now that you have an understanding of how Gloo Edge handles security we have a few suggested paths:

* **[Traffic management]({{% versioned_link_path fromRoot="/introduction/traffic_management/" %}})** - learn more about Gloo Edge and traffic management
* **[Setup]({{% versioned_link_path fromRoot="/installation/" %}})** - Deploy your own instance of Gloo Edge
* **[Security guides]({{% versioned_link_path fromRoot="/guides/security/" %}})** - Try out the security guides to learn more