# Threat Model

This document provides a threat model for kgateway. This analysis identifies potential security threats, attack vectors, and mitigation strategies to help secure kgateway deployments.

## Audience

This threat model is intended for **deployers and security engineers** responsible for running kgateway.

## Related Documentation

* [Envoy Threat Model](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/security/threat_model) - Essential reading for understanding the dataplane security model

## Potential Attack Surfaces

* Tenant-provided Gateway API resources: Gateways, HTTPRoutes, TCPRoutes, etc.
  * Note: a `Gateway`'s `infrastructure.parametersRef` and any `GatewayClass`/`GatewayParameters` it references are **not** tenant-level input — see [GatewayParameters Is Operator-Level Configuration](#gatewayparameters-is-operator-level-configuration-not-tenant-input) below.
  * Note: the *content* of these resources is untrusted input to the dataplane, but the *ability to submit them at all* is a trusted capability — see [Configuration Authorship Is a Trusted Capability](#configuration-authorship-is-a-trusted-capability) below.
* Network traffic: HTTP/TCP/UDP/gRPC requests from tenants
* TLS certificates: Untrusted external certs for TLS termination
* Ingress rules: Misconfigured or maliciously crafted rules

## System Trusts

* Kubernetes control plane and RBAC enforcement
* Other cluster controllers (e.g., cert-manager)
* Envoy internals deployed by kgateway
* Operator-applied configuration, including `GatewayParameters` (GWP) and any other resource that controls how the Envoy proxy Deployment/Pod is rendered
* Authors of any configuration the control plane watches (routes, policies, backends, and other reconciled resource types) to act in good faith with respect to control-plane resource consumption — see [Configuration Authorship Is a Trusted Capability](#configuration-authorship-is-a-trusted-capability)

## GatewayParameters Is Operator-Level Configuration, Not Tenant Input

`GatewayParameters` (and the `deploymentOverlay` it exposes) let the author control the full Pod spec of the Envoy proxy Deployment that kgateway creates for a `Gateway` — including container image, `command`/`args`, security context, host namespaces, and volumes. This is intentional: GWP is kgateway's supported mechanism for Operators to customize proxy deployments (for example, to satisfy OpenShift's default security context requirements).

Because the kgateway controller applies the resolved overlay using its own elevated `deployments:*` RBAC, **the ability to create or update a `GatewayParameters` resource, or to create/update a `GatewayClass`/`Gateway` that references one via `parametersRef`, is equivalent to holding direct `create`/`update` permission on Deployments in the proxy's namespace.** This is the same trust relationship inherent to any controller that renders a CRD into a lower-level workload resource it has RBAC for — it is not a privilege-escalation bug in kgateway.

**Operators must treat RBAC on `gatewayparameters` (create/update/patch) and on `gatewayclasses`/`gateways` with a `parametersRef` the same way they treat direct Deployment-create RBAC.** Do not grant these permissions to Tenants who should not be able to run arbitrary privileged workloads on the cluster. A Tenant granted this access should be considered an Operator for the purposes of this threat model, regardless of how limited their other permissions are.

The `$patch: delete` example shown in the `GatewayParameters` documentation for clearing `securityContext` is an OpenShift-specific accommodation (OpenShift's default Security Context Constraints assign UIDs and drop capabilities that conflict with Envoy's defaults) — it is not general guidance to remove Pod/container security constraints, and should not be used as a template outside that context.

## Configuration Authorship Is a Trusted Capability

kgateway's control plane watches the Kubernetes resources it translates — Gateways, routes, policies, backends, and the other resource types it reconciles — and holds their state in memory in order to render Envoy configuration. Two consequences follow, and both are inherent to how a Kubernetes control plane operates rather than specific to kgateway:

* **Consumption scales with what is written.** Anyone with write access to a watched resource type can create resources in bulk and cause the control plane to consume memory and CPU in proportion to that volume. The controller must observe and process whatever the API server accepts; no amount of per-feature validation changes this.
* **Cost is determined by content, not only by count.** The work required to translate a configuration is a function of what that configuration expresses. Some configurations are substantially more expensive to translate than others of the same size, and the relationship between the size of a configuration and the cost of translating it is not guaranteed to be linear.

**The ability to create configuration that the kgateway control plane watches is therefore a trusted capability in this threat model.** kgateway does not treat control-plane resource consumption caused by an authorized author's own configuration as a trust-boundary violation — whether that consumption arises from the number of resources created, from the cost of translating them, or from both. kgateway does not attempt to enforce fairness in resource consumption between authors sharing a control-plane instance, and no individual limit or validation should be understood as providing that guarantee.

This is a Kubernetes multi-tenancy concern rather than a gateway-specific one. The same write access lets an author apply pressure to the API server and etcd directly, and it is addressed with the same tools: RBAC, quotas, and isolation.

Correctness constraints are a separate matter and *are* enforced: configuration that cannot be resolved into valid Envoy config is rejected and surfaced in resource status. What is out of scope here is the cost of processing configuration that is otherwise well-formed.

kgateway may add defensive bounds on translation work over time as robustness hardening. Operators should treat any such bound as defense in depth against accidental misconfiguration, **not** as a security control that makes configuration authorship safe to delegate to untrusted parties.

**Operators serving mutually untrusting tenants must isolate them rather than rely on the control plane to arbitrate.** Recommended measures:

* Restrict RBAC on kgateway-watched resource types to tenants trusted not to degrade shared infrastructure, and treat that RBAC as a control-plane-availability-sensitive grant.
* Constrain which namespaces may attach routes to a listener via `spec.listeners[].allowedRoutes.namespaces`.
* Run dedicated control-plane instances per tenant group, each scoped to a disjoint set of namespaces, so that one tenant's configuration cannot affect another tenant's config propagation.
* Bound the volume of configuration a tenant can create, for example with a `ResourceQuota` on the relevant resource counts per namespace.
* Monitor control-plane translation latency, memory and CPU saturation, and bursts of configuration writes; treat sustained deviation from baseline as an operational signal.

## Key Assets at Risk

* Traffic routing control: Compromise could allow intercepting/misrouting requests
* TLS keys/certificates: Exposure enables Man-in-the-Middle (MITM) attacks
* Gateway/route configuration: Could influence routing, auth, or rate limiting
* Dataplane state: Metrics or cached routes affecting traffic handling

## Threats & Potential Impacts

* Unauthorized route modification → bypass policies, intercept traffic
* Denial-of-Service (DoS) → overload kgateway controller or dataplane via untrusted network traffic or untrusted external input (control-plane load caused by an authorized author's own configuration is out of scope — see [Configuration Authorship Is a Trusted Capability](#configuration-authorship-is-a-trusted-capability))
* TLS key compromise → Man-in-the-Middle (MITM) attacks
* Misuse of request auth/RBAC → unauthorized access
* Data exfiltration via misconfigured routes → leak tenant/cluster data

## Mitigations

* RBAC & namespace isolation: Enforce fine-grained permissions
* GatewayParameters RBAC: Restrict `gatewayparameters` create/update and the ability to reference one via `parametersRef` to Operators; treat this RBAC as equivalent in sensitivity to Deployment-create RBAC
* Configuration authorship RBAC: Restrict who may create resources the control plane watches; isolate mutually untrusting tenants onto separate control-plane instances rather than relying on the control plane to arbitrate between them
* Gateway API validations: Schema checks, allowed fields, limits
* Rate limiting / circuit breakers: Prevent DoS
* TLS management: Use PKI best practices, cert-manager, rotate keys
* Logging & observability: Detect anomalous behavior
* Deployment best practices: Dedicated gateways per tenant, GitOps, CI/CD security checks, latest images, Pod Security Admission
* Supply chain security: SLSA, Sigstore, SBOM, VEX verification