# Threat Model

This document provides a threat model for kgateway. This analysis identifies potential security threats, attack vectors, and mitigation strategies to help secure kgateway deployments.

## Audience

This threat model is intended for **deployers and security engineers** responsible for running kgateway.

## Related Documentation

* [Envoy Threat Model](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/security/threat_model) - Essential reading for understanding the dataplane security model

## Potential Attack Surfaces

* Tenant-provided Gateway API resources: Gateways, HTTPRoutes, TCPRoutes, etc.
* Network traffic: HTTP/TCP/UDP/gRPC requests from tenants
* TLS certificates: Untrusted external certs for TLS termination
* Ingress rules: Misconfigured or maliciously crafted rules

## System Trusts

* Kubernetes control plane and RBAC enforcement
* Other cluster controllers (e.g., cert-manager)
* Envoy internals deployed by kgateway
* Operator-applied configuration

## Key Assets at Risk

* Traffic routing control: Compromise could allow intercepting/misrouting requests
* TLS keys/certificates: Exposure enables Man-in-the-Middle (MITM) attacks
* Gateway/route configuration: Could influence routing, auth, or rate limiting
* Dataplane state: Metrics or cached routes affecting traffic handling

## Threats & Potential Impacts

* Unauthorized route modification → bypass policies, intercept traffic
* Denial-of-Service (DoS) → overload kgateway controller or dataplane
* TLS key compromise → Man-in-the-Middle (MITM) attacks
* Misuse of request auth/RBAC → unauthorized access
* Data exfiltration via misconfigured routes → leak tenant/cluster data

## Mitigations

* RBAC & namespace isolation: Enforce fine-grained permissions
* Gateway API validations: Schema checks, allowed fields, limits
* Rate limiting / circuit breakers: Prevent DoS
* TLS management: Use PKI best practices, cert-manager, rotate keys
* Logging & observability: Detect anomalous behavior
* Deployment best practices: Dedicated gateways per tenant, GitOps, CI/CD security checks, latest images, Pod Security Admission
* Supply chain security: SLSA, Sigstore, SBOM, VEX verification