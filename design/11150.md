# EP-11150: agentgateway Support

* Issue: [#11150](https://github.com/kgateway-dev/kgateway/issues/11150)

## Background

Integrate kgateway as the control plane for the [agentgateway](https://github.com/agentgateway/agentgateway) proxy, with agentgateway serving as the data plane 
that enables AI connectivity for agents and tools in across environments.

agentgateway is compatible with any agentic framework supporting the Model Context Protocol (MCP) or agent-to-agent (A2A)
protocol, including LangGraph, AutoGen, kagent, Claude Desktop, and OpenAI SDK. 

This allows a user of kgateway to:
- Proxy requests to MCP servers: Route traffic from MCP clients to upstream MCP tools with built-in support for security, observability, traffic policies, and governance. 
- Proxy requests to A2A servers: Enable direct routing and traffic control between agents for agent-to-agent communication.
- Federate tools across multiple MCP servers: Use MCP multiplexing to expose tools from multiple upstream MCP servers through a single agentgateway.

agentgateway is not built on top of Envoy, it is a Rust-based dataplane that can be configured via an xDS interface. For example,
agentgateway can define a SSE MCP listener:

```json
{
  "type": "static",
  "listeners": [
    {
      "name": "sse",
      "protocol": "MCP",
      "sse": {
        "address": "[::]",
        "port": 3000
      }
    }
  ]
} 
```

The agentgateway also has configuration to define a `target`, which represents a destination for which you want Agent 
Gateway to proxy traffic for. For example, you can define a target for a MCP server `server-everything`:

```json
{
     "type": "static",
     "listeners": [
       {
         "name": "sse",
         "protocol": "MCP",
         "sse": {
           "address": "[::]",
           "port": 3000
         }
       }
     ],
     "targets": {
       "mcp": [
         {
           "name": "everything",
           "stdio": {
             "cmd": "npx",
             "args": [
               "@modelcontextprotocol/server-everything"
             ]
           }
         }
       ]
     }
   }
```

kgateway will need to add support for syncing the configuration for agentgateway via the xDS interface, along with any additional
APIs that kgateway needs to support agentgateway.

## Motivation

As agent-based architectures grow more complex, there's a growing need to centralize the management of networking, 
security, and observability across diverse deployments. agentgateway provides a unified data plane purpose-built for 
AI connectivity, with first-class support for protocols like MCP and A2A. 

By integrating kgateway with the agentgateway, users can build and operate sophisticated agent ecosystems while 
cleanly separating infrastructure concerns from agent and tool logic. This enables secure, observable, and scalable 
connectivity for agents and tools running in Kubernetes.

Instead of natively supporting MCP and A2A in Envoy, integrating kgateway with agentgateway allows us to offload 
protocol-specific logic to a purpose-built data plane while still leveraging Envoy’s mature traffic routing, 
security, and observability features. 

This separation of concerns allows us to use Envoy as a general-purpose, protocol-agnostic proxy, while agentgateway 
will handle agent-specific connectivity patterns. It also allows faster iteration on agent-native features without 
modifying core proxy infrastructure.

The kgateway control plane will configure both Envoy and agentgateway, enabling unified management while preserving the 
flexibility to evolve each data plane independently. This avoids entangling agent-specific logic in Envoy configuration 
while benefiting from agentgateway's quick iteration cycles on new agent-specific features.

### Goals
- Enable users to deploy kgateway to configure agentgateway as a data plane.
- Support configuring proxying requests to MCP-compatible tool servers via kgateway configured agentgateway.
- Support configuring proxying agent-to-agent (A2A) communication via kgateway configured agentgateway.
- Support configuring MCP multiplexing to federate tools across multiple upstream MCP servers behind a single agentgateway endpoint.
- Be flexible to support other protocols additions in agentgateway (OpenAPI MCP server, etc.)

### Non-Goals
- Implementing MCP or A2A protocol logic directly within Envoy.
- Make sure existing kgateway security, observability, and traffic management features are supported with agentgateway listeners. The security policies will be handled as part of the [JWT EP](https://github.com/kgateway-dev/kgateway/pull/11194).
- Providing a user interface for managing agentgateway config (UI integration out of initial scope of this EP).
- Handling model inference, orchestration, or agent logic outside proxying and routing concerns. This EP does not touch existing 
LLM provider routing functionality or the Gateway Inference API support in kgateway. It is only focused on the integration of kgateway with agentgateway.
- Native integration of kgateway with the agentgateway's [observability features](https://agentgateway.dev/docs/observability/) 
such as metrics and tracing are in scope for this initial EP. These features should work alongside the agentgateway proxy out of the box, but kgateway will not expose any functionality to configure these features separately.
- Supporting configuring an OpenAPI-based MCP server via kgateway for agent and tool discovery is out of the initial scope. 
Allowing the user to configure an OpenAPI server as an MCP server is out of the initial scope, but would be a useful future extension. 
This would allow kgateway to serve an OpenAPI-compatible interface for tool discovery and interaction. The approach should be flexible to support this in the future if a user specifies a `openapi-mcp` type protocol.

## Implementation Details

An earlier MCP-specific implementation was introduced in kgateway [PR #11034](https://github.com/kgateway-dev/kgateway/pull/11034).
This EP will extend that support to other agentgateway features (e.g. A2A, MCP multiplexing).

### Configuration
- Add helm option to enable/disable kgateway and agentgateway integration
- Add support for MCP and A2A protocol configuration when used in a Kubernetes Gateway API 
[Gateway](https://kubernetes.io/docs/concepts/services-networking/gateway/#api-kind-gateway) resource with kgateway.
- Add support for kgateway to configure authorization for agentgateway.

#### Gateway protocol support

First a user will need to create a `GatewayParameters` and `GatewayClass` for kgateway to use to configure agentgateway. 
The `GatewayParameters` resource that allows a user to set up self-managed Gateways, that
instructs kgateway to not automatically spin up a gateway proxy, but instead wait for the custom gateway proxy deployment.
The `GatewayClass` will need to bind it to the custom GatewayParameters.

```yaml
kind: GatewayParameters
apiVersion: gateway.kgateway.dev/v1alpha1
metadata:
  name: kgateway
spec:
  selfManaged: {}
---
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: mcp
spec:
  controllerName: kgateway.dev/kgateway
  parametersRef:
    group: gateway.kgateway.dev
    kind: GatewayParameters
    name: kgateway
    namespace: default
```

The `Gateway` resource needs to be configured to use the custom `GatewayClass`. The custom `GatewayClass` instructs 
kgateway to not automatically spin up a gateway proxy deployment and to wait for the custom deployment. The `Gateway` resource
will also need to define the `protocol` based on the protocol that the user wants to proxy to the agentgateway.

For example, you can open up an MCP listener on port 8080:

```yaml
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: mcp-gateway
spec:
  gatewayClassName: mcp
  listeners:
  - protocol: kgateway.dev/mcp
    port: 8080
    name: http
    allowedRoutes:
      namespaces:
        from: All
```

Then the user can define a Kubernetes-native Service:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: mcp
  namespace: gwtest
  labels:
    app: mcp
spec:
  clusterIP: "10.0.0.11"
  ports:
    - name: http
      port: 8080
      targetPort: 8080
      appProtocol: kgateway.dev/mcp
  selector:
    app: mcp
```

### Controllers
No new controllers are needed to configure the initial agentgateway Listeners and Targets, since no new APIs are introduced.
However, a new syncer is required to convert Gateway resources into agentgateway Listener and Target configurations. 
This syncer should watch Gateway resources that use supported agentgateway protocols (e.g., `kgateway.dev/mcp`, `kgateway.dev/a2a`).

To support JWT authentication, a new custom resource must be introduced for JWT configuration. A new controller will be
needed to reconcile this resource, manage its status, and handle updates. This controller should only run when the 
feature is enabled and the resource is present. RBAC rules must be updated to allow access to the new JWT AuthPolicy resource.
This is out of scope of the initial EP and will be handled as part of the separate JWT EP.

### Deployer
The kgateway Helm chart will need to be extended to allow users to enable agentgateway integration via a new Helm value (e.g., `.Values.agentgateway.enabled`).

Initially the agentgateway integration will be self-managed. The GatewayParameters resource can be configured to 
instruct kgateway to not automatically spin up a gateway proxy, but instead wait for your custom gateway proxy deployment.

The second phase of this EP will allow kgateway to manage the agentgateway deployment. This will require addition Helm values
to configure the agentgateway deployment (e.g., `.Values.agentgateway.deployment.version`, `.Values.agentgateway.deployment.image`). The 
deployment management mode (`selfManaged`) should work for either Envoy or agentgateway deployments.

### Translator and Proxy Syncer
A new syncer should be added to kgateway to translate Gateway configuration into agentgateway resources. This syncer will 
be responsible for converting relevant Gateway definitions into agentgateway-compatible Listener and Target configurations.

The syncer will need to handle the new protocols (e.g., `kgateway.dev/mcp`, `kgateway.dev/a2a`). The `allowedRoutes` field
on the `Gateway` resource can be used to determine which namespaces will be selected for the agentgateway. This allows target
discovery to be restricted to specific namespaces if desired.

### Plugin
No plugin changes will be required as the agentgateway syncer will be responsible for syncing the configuration independently of the plugin system.

For the JWT Authentication and rbac policies, a new set of plugins will need to be added to the agentgateway (none of the existing plugins can be used
since they are responsible for configuring Envoy, not the agentgateway). This work is out of scope for this EP and will be handled as part of the JWT EP.

### Reporting
Statuses for new syncer reported in the resource's respective status fields. For example, a `Gateway` status can be:
- Pending: The gateway is not yet ready to be used (configuration hasn't been accepted yet, etc.)
- Warning: The gateway is ready to be used, but there are some issues that need to be addressed from the agent syncer 
- Error: The gateway is not ready to be used, and there are issues that need to be addressed from the agent syncer before the Gateway can be used
- Ready: The gateway is ready to be used and has been successfully synced

This status syncing will need to be handled separately from the proxy_syncer status updates since the agentgateway can be self-managed
or managed by a separate controller. 

### Test Plan

- Create plugin-level unit tests for kgateway to test agentgateway configuration.
- Create setup unit tests for kgateway to test agentgateway configuration.
- Create an e2e testing framework for kgateway to test agentgateway integration. This will require test MCP servers and A2A servers.

## Alternatives
- Not supporting agentgateway integration in kgateway. This would limit kgateway's ability to support the rising demand for Agentic infrastructure and routing traffic to A2A agents and MCP Servers.
- Natively implement A2A and MCP support in kgateway. This would require Envoy changes and add additional maintenance burden to kgateway relying on a custom Envoy fork.

## Open Questions
- The OpenAPI server configuration in the [agentgateway docs](https://agentgateway.dev/docs/targets/openapi/) requires additional configuration from the user. For example, a ConfigMap can be defined to get JSON, but kgateway will need a way to select the ConfigMap. OpenAPI spec will be out of scope for this initial EP. 
- The initial scope of this EP adds support for agentgateway protocols defined on the Gateway level. We may want to support Route configuration in the future. What are the use cases for an `agentgatewayRoute` resource?