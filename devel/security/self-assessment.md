# kgateway Self-assessment

_This document provides a self-assessment of the kgateway project following the guidelines outlined by the [CNCF TAG Security and Compliance group](https://tag-security.cncf.io/community/assessments/guide/self-assessment/#self-assessment). The purpose is to evaluate kgateway’s current security posture and alignment with best practices, ensuring that it is suitable for adoption at a CNCF incubation level._

## Table of Contents

* [Metadata](#metadata)
  * [Version history](#version-history)
  * [Security links](#security-links)
* [Overview](#overview)
  * [Background](#background)
  * [Actors](#actors)
  * [Actions](#actions)
  * [Goals](#goals)
  * [Non-goals](#non-goals)
* [Self-assessment use](#self-assessment-use)
* [Security functions and features](#security-functions-and-features)
* [Project compliance](#project-compliance)
  * [Future state](#future-state)
* [Secure development practices](#secure-development-practices)
  * [Deployment pipeline](#deployment-pipeline)
  * [Communication channels](#communication-channels)
* [Security issue resolution](#security-issue-resolution)
* [Appendix](#appendix)

## Metadata

### Version history

|   |  |
| - | - |
| September 5, 2025 | Initial Draft _(Sam Heilbron, Lin Sun, Jenny Shu)_  |
|  |  |

### Security links

|   |  |
| - | - |
| Software | https://github.com/kgateway-dev/kgateway  |
| Security Provider |  No. kgateway is designed to facilitate security and compliance validation, but it should not be considered a security provider.  |
| Languages | Golang, Yaml, Python |
| SBOM | See [Project Compliance > Future State](#future-state) |
| Security Insights | See [Project Compliance > Future State](#future-state) |
| Security File | See [Project Compliance > Future State](#future-state) |
| Cosign pub-key | See [Project Compliance > Future State](#future-state) |
| | |

## Overview

kgateway is an Envoy-powered, Kubernetes-native API Gateway that serves as a comprehensive ingress controller, advanced API gateway, AI gateway, and service mesh waypoint proxy. Built on the Kubernetes Gateway API, kgateway provides production-ready traffic management, security, and observability features for modern cloud-native applications.

### Background

kgateway simplifies ingress, transformation, and AI-driven routing on Kubernetes, providing robust authentication, authorization, rate-limiting, observability, and transformation capabilities.

This project has been production-ready since 2019 (previously known as Gloo).

### Actors

#### kgateway-proxy

* Role: wrapper around the Envoy proxy, based on [envoy-gloo](https://github.com/solo-io/envoy-gloo). Handles traffic from downstream clients or applications.
* Isolation: runs as its own Pod

#### kgateway

* Role: controller for API definitions of routing or policy behaviors.
* Isolation: runs as its own pod

#### sds

* Role: Implements the SDS ([Secret Discovery Service](https://www.envoyproxy.io/docs/envoy/latest/configuration/security/secret)) protocol, to allow certificates to be distributed to kgateway-proxy dynamically, without mounting them into the proxy container
* Isolation: runs as a sidecar to the kgateway-proxy

#### kgateway-ai-extension

* Role: data plane extension to integrate with LLMs.
* Isolation: runs as a sidecar to kgateway-proxy

#### kgwctl

* Role: CLI (command-line interface) for interacting with an installation of kgateway
* Isolation: runs as its own binary

### Actions

#### kgateway-proxy

* Handles traffic from downstream applications or clients, executes the defined encoding filters and  routing logic, forwards the request to the upstream service, collects the response, applies the decoding filters, and returns the response to the client.

#### kgateway

* Processes user-defined routing APIs (Kubernetes Gateway API) and policy APIs (kgateway CRDs) and produces xDS snapshot to serve to kgateway-proxy

#### sds

* Serves certificates via SDS to the kgateway-proxy

#### kgateway-ai-extension

* Routes traffic to LLMs

#### kgwctl

* A user executes to a command against the running installation, to inspect the runtime state

### Goals

* Provide secure, policy‑driven ingress and routing.
* Enforce authentication, authorization, rate limiting, and transformations.
* Enable observability while preserving confidentiality.

### Non-goals

* Does not guarantee serverless or upstream service security. It does provide mechanisms to defend against them (response manipulation), but security of those are outside this project's scope.
* Not responsible for protecting platform-level vulnerabilities (e.g., Kubernetes node compromise).

## Self-Assessment Use

This self-assessment is created by the kgateway team to perform an internal analysis of the project's security. It is not intended to provide a security audit of kgateway, or function as an independent assessment or attestation of kgateway's security health.

This document serves to provide kgateway users with an initial understanding of kgateway's security, where to find existing security documentation, kgateway plans for security, and general overview of kgateway security practices, both for development of kgateway as well as security of kgateway.

This document provides kgateway maintainers and stakeholders with additional context to help inform the roadmap creation process, so that security and feature improvements can be prioritized accordingly.

## Security Functions and Features

### Authentication and Authorization

* **JWT Authentication:** Comprehensive JWT support with configurable providers, token validation, and claim-based authorization through the unified JWT authentication system
* **External Authentication (ExtAuth):** Integration with external authentication services for centralized identity management
* **Role-Based Access Control (RBAC):** Authorization policies supporting fine-grained access controls at the gateway, route, and service levels
* **API Key Authentication:** Support for API key-based authentication with secure secret management

### Data Encryption

* **TLS Termination:** Full TLS termination capabilities with support for multiple certificate sources (Kubernetes secrets, file-based certificates)
* **Mutual TLS (mTLS):** Support for mutual TLS authentication between clients and services
* **Backend TLS:** Secure upstream connections with configurable TLS policies including certificate validation and SNI support
* **Certificate Management:** Automatic certificate rotation and validation with SDS (Secret Discovery Service) integration
* **Encryption in Transit:** All communications encrypted using industry-standard TLS protocols

### Traffic Security

* **Rate Limiting:** Both local and global distributed rate limiting to prevent abuse and ensure service availability
* **CORS Protection:** Comprehensive Cross-Origin Resource Sharing (CORS) policy enforcement
* **CSRF Protection:** Cross-Site Request Forgery protection mechanisms
* **Request/Response Transformation:** Secure header manipulation and request/response transformation capabilities
* **IP Allowlisting/Denylisting:** Network-level access controls based on client IP addresses

## Project Compliance

kgateway is not currently targeting formal standards (PCI-DSS, GDPR). However, there are standards within the cloud-native ecosystem that kgateway does target. They are listed below.

### Industry Standards

* **Gateway API Compliance:** Full compliance with Kubernetes Gateway API specifications and [security](https://gateway-api.sigs.k8s.io/guides/security/)
* **Envoy Proxy Standards:** Built on battle-tested Envoy proxy with its [security model](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/security/security)
* **Cloud Native Security:** Adherence to [CNCF security best practices and guidelines](https://github.com/cncf/sig-security/blob/main/security-whitepaper/CNCF_security_whitepaper-v2.pdf)
* **OpenTelemetry Integration:** Observability and security monitoring through OpenTelemetry standards

### Kubernetes Integration

* **RBAC Integration:** Native Kubernetes RBAC support for access control
* **Pod Security Standards:** Compliance with Kubernetes Pod Security Standards
* **Network Policy Support:** Integration with Kubernetes network policies for micro-segmentation

### Future State

In the future, kgateway intends to build and maintain compliance with several industry standards and frameworks:

**Supply Chain Levels for Software Artifacts (SLSA)**:

* All release artifacts include signed provenance attestations with cryptographic verification
* Build process isolation and non-falsifiable provenance are implemented
* Both container images and release binaries have complete SLSA provenance chains

**Container Security Standards**:

* All container images are signed with Cosign using keyless signing
* Software Bill of Materials (SBOM) generation for all releases
* Multi-architecture container builds with attestation

## Secure Development Practices

### Deployment Pipeline

#### Contribution Security

* **Contributor Guidelines:** Clear security guidelines in the contribution documentation
* **Code Signing:** Support for signed commits and releases

#### Code Security

* **Static Code Analysis:** Integrated golangci-lint with security-focused linters
* **Code Reviews:** Mandatory peer code reviews with security considerations as part of the review process
* **Secure Development Lifecycle:** Security considerations integrated throughout the development process

#### Testing and Validation

* **End-to-End Testing:** Extensive end-to-end testing including security-focused test cases
* **Load Testing:** Performance and security testing under various load conditions

#### CI/CD

* **GitHub Actions:** Use GitHub Actions for continuous integration and deployment

### Communication Channels

|   |  |
| - | - |
| Documentation | https://kgateway.dev/docs |
| Contributing | https://github.com/kgateway-dev/kgateway/blob/main/CONTRIBUTING.md |
| Mailing list | cncf-kgateway-maintainers@lists.cncf.io |
| Slack | https://cloud-native.slack.com/archives/C080D3PJMS4 |
| LinkedIn | https://www.linkedin.com/company/kgateway/ |
| Twitter / X | https://x.com/kgatewaydev |
| Youtube | https://www.youtube.com/@kgateway-dev |
| Community meetings | https://calendar.google.com/calendar/u/1?cid=ZDI0MzgzOWExMGYwMzAxZjVkYjQ0YTU0NmQ1MDJmODA5YTBjZDcwZGI4ZTBhZGNhMzIwYWRlZjJkOTQ4MzU5Y0Bncm91cC5jYWxlbmRhci5nb29nbGUuY29t |
| | |

## Security Issue Resolution

See [kgateway community CVE document](https://github.com/kgateway-dev/community/blob/main/CVE.md). This covers the disclosure practices as well as the incident response process.

## Appendix

### Known issues over time

* Known issues are currently tracked in the project roadmap.
* kgateway does not have any reported security vulnerabilities to date. All known issues and bugs are tracked in the project's GitHub Issues and are addressed promptly by the maintainers.
* The project has a strong track record of catching issues during code review and automated testing, with no critical vulnerabilities discovered post-release.

### OpenSSF best practices

[![OpenSSF Best Practices](https://bestpractices.coreinfrastructure.org/projects/10534/badge)](https://bestpractices.coreinfrastructure.org/projects/10534)

kgateway already has a passing openSSF badge for supporting best practices.

### Case studies

#### Case Study 1: Rate Limiting to Protect Credential Stuffing Attacks

A company experienced credential stuffing attacks targeting their login endpoints. Rather than modify core applications, they deployed kgateway in front of the API and configured IP-based and user-based rate limiting policies.

ref: https://kgateway.dev/docs/main/security/ratelimit/global/

#### Case Study 2: Audit Logging for Compliance in a Regulated Environment

A healthcare provider needed to implement audit logging for all external API requests as part of HIPAA compliance. They used kgateway’s pluggable logging capabilities to log request metadata (e.g., IP, user agent, endpoint accessed, response status) in a structured format. This enabled easy querying of suspicious patterns and helped pass third-party audits.

ref: https://kgateway.dev/docs/main/security/access-logging/

### Related projects/vendors

* [Envoy Gateway](https://gateway.envoyproxy.io/)
* [Kong](https://developer.konghq.com/gateway/)