# agentgateway syncer

This syncer configures xds updates for the [agentgateway](https://agentgateway.dev/) data plane.

To use the agentgateway control plane with kgateway, you need to enable the integration in the helm chart:
```yaml
agentgateway:
  enabled: true # set this to true
```

You can configure the agentgateway Gateway class to use a specific image by setting the image field on the
GatewayClass:
```yaml
kind: GatewayParameters
apiVersion: gateway.kgateway.dev/v1alpha1
metadata:
  name: kgateway
spec:
  kube:
    agentgateway:
      enabled: true
      logLevel: debug
      image:
        tag: bc92714
---
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: agentgateway
spec:
  controllerName: kgateway.dev/kgateway
  parametersRef:
    group: gateway.kgateway.dev
    kind: GatewayParameters
    name: kgateway
    namespace: default
---
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: agent-gateway
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
      allowedRoutes:
        namespaces:
          from: All
```

### APIs

The syncer uses the following APIs:

- [workload](https://github.com/agentgateway/agentgateway/tree/main/go/api/workload.pb.go)
- [resource](https://github.com/agentgateway/agentgateway/tree/main/go/api/resource.pb.go)

The workload API is originally derived from from Istio's [ztunnel](https://github.com/istio/ztunnel), where each address represents a unique address. The address API joins two sub-resources (Workload and Service) to support querying by IP address.

Resources contain agentgateway-specific config (Binds, Listeners, Routes, Backend, Policy, etc.).

#### Bind:

Bind resources define port bindings that associate gateway listeners with specific network ports. Each Bind contains:
- **Key**: Unique identifier in the format `port/namespace/name` (e.g., `8080/default/my-gateway`)
- **Port**: The network port number that the gateway listens on

Binds are created automatically when Gateway resources are processed, with one Bind per unique port used across all listeners.

#### Listener:

Listener resources represent individual gateway listeners with their configuration. Each Listener contains:
- **Key**: Unique identifier for the listener
- **Name**: The section name from the Gateway listener specification
- **BindKey**: References the associated Bind resource (format: `port/namespace/name`)
- **GatewayName**: The gateway this listener belongs to (format: `namespace/name`)
- **Hostname**: The hostname this listener accepts traffic for
- **Protocol**: The protocol type (HTTP, HTTPS, TCP, TLS)
- **TLS**: TLS configuration including certificates and termination mode

Listeners are created from Gateway API listener specifications and define how traffic is accepted and processed at the network level.

#### Routes:

Route resources define routing rules that determine how traffic is forwarded to backend services. Routes are created from various Gateway API route types:

- **HTTP Routes**: Convert from `HTTPRoute` resources with path, header, method, and query parameter matching
- **gRPC Routes**: Convert from `GRPCRoute` resources with service/method matching
- **TCP Routes**: Convert from `TCPRoute` resources for TCP traffic (catch-all matching)
- **TLS Routes**: Convert from `TLSRoute` resources for TLS passthrough (SNI matching at listener level)

Each Route contains:
- **Key**: Unique identifier (format: `namespace.name.rule.match`)
- **RouteName**: Source route name (format: `namespace/name`)
- **ListenerKey**: Associated listener (populated during gateway binding)
- **RuleName**: Optional rule name from the source route
- **Matches**: Traffic matching criteria (path, headers, method, query params)
- **Filters**: Request/response transformation filters
- **Backends**: Target backend services with load balancing and health checking
- **Hostnames**: Hostnames this route serves traffic for

Routes support various filters including header modification, redirects, URL rewrites, request mirroring, and policy attachments.

#### Backends:

Backend resources define target services and systems that traffic should be routed to. Unlike other resources, backends are global resources (not per-gateway) and are applied to all gateways in the agentgateway syncer.

Each backend has a unique name in the format `namespace/name`. Backends are processed through the plugin system that translates Kubernetes Backend CRDs to agentgateway API resources

Backends in kgateway are represented by the Backend CRD and support multiple backend types:

**Backend Types:**
- **AI**: Routes traffic to AI/LLM providers (OpenAI, Anthropic, etc.) with model-specific configurations
- **Static**: Routes to static host/IP with configurable ports and protocols. Note: In agentgateway only one host is supported (not list of hosts).
- **MCP**: Model Context Protocol backends for virtual MCP servers. Note if a static MCP backend target is used, kgateway will translate out two backends (one static, one mcp).

**Usage in Routes:**
Backends are referenced by HTTPRoute, GRPCRoute, TCPRoute, and TLSRoute resources using `backendRefs`:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
spec:
  rules:
    - backendRefs:
        - group: gateway.kgateway.dev
          kind: Backend
          name: my-backend
```

**Translation Process:**
1. Backend CRDs are watched by the agentgateway syncer
2. Each backend is processed through registered plugins based on its type
3. Plugins translate the backend configuration to agentgateway API format
4. The resulting backend resources and associated policies are distributed to **all** gateways via xDS

#### Policies:

Policies in kgateway can come from HTTPRoutes, Backends and TrafficPolicy crds. They configure the following agentgateway Policies:

Policies are configurable rules that control traffic behavior, security, and transformations for routes and backends.
- Request Header Modifier: Add, set, or remove HTTP request headers.
- Response Header Modifier: Add, set, or remove HTTP response headers.
- Request Redirect: Redirect incoming requests to a different scheme, authority, path, or status code.
- URL Rewrite: Rewrite the authority or path of requests before forwarding.
- Request Mirror: Mirror a percentage of requests to an additional backend for testing or analysis.
- CORS: Configure Cross-Origin Resource Sharing (CORS) settings for allowed origins, headers, methods, and credentials.
- A2A: Enable agent-to-agent (A2A) communication features.
- Backend Auth: Set up authentication for backend services (e.g., passthrough, key, GCP, AWS).
- Timeout: Set request and backend timeouts.
- Retry: Configure retry attempts, backoff, and which response codes should trigger retries.
- Transformations: Add, set or remove HTTP request and response headers and apply body transformations

### CEL Transformations

The agentgateway data plane supports [CEL](https://cel.dev/) (Common Expression Language) transformations through TrafficPolicy resources. CEL transformations allow you to modify requests and responses using powerful expression language.

Unlike the Envoy data plane transformations that support Inja, the agentgateway transformations use CEL expressions.

#### Supported Transformation Types

**Header Transformations:**
- `set`: Replace or create headers with new values
- `add`: Add headers (append if header already exists)
- `remove`: Remove headers by name

**Body Transformations:**
- Modify request/response body content
- Parse JSON using `json()` function
- Transform strings and objects

#### Example

Apply this config to setup basic response and request header transformations for agentgateway data plane.

```shell
kubectl apply -f- <<EOF
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: example-gateway
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
      allowedRoutes:
        namespaces:
          from: Same
---
apiVersion: v1
kind: Service
metadata:
  name: simple-svc
  labels:
    app: simple-svc
spec:
  ports:
    - name: http
      port: 8080
      targetPort: 3000
  selector:
    app.kubernetes.io/name: backend-0
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend-0
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: backend-0
      version: v1
  template:
    metadata:
      labels:
        app.kubernetes.io/name: backend-0
        version: v1
    spec:
      containers:
        - image: gcr.io/k8s-staging-gateway-api/echo-basic:v20231214-v1.0.0-140-gf544a46e
          imagePullPolicy: IfNotPresent
          name: backend-0
          ports:
            - containerPort: 3000
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: SERVICE_NAME
              value: simple-svc
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: example-route
spec:
  parentRefs:
    - name: example-gateway
  hostnames:
    - "example-gateway-attached-transform.com"
  rules:
    - backendRefs:
        - name: simple-svc
          port: 8080
---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: example-traffic-policy-for-gateway-attached-transform
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: example-gateway
  transformation:
    request:
      set:
        - name: request-gateway
          value: "'hello'"
    response:
      set:
        - name: response-gateway
          value: "'goodbye'"
EOF
```

#### Important Notes

- **parseAs field**: The `parseAs` field is not supported for agentgateway. Use `json()` function directly in CEL expressions instead
- **Data plane validation**: Invalid CEL expressions are handled by the agentgateway data plane

### Architecture

The agentgateway syncer only runs if `cfg.SetupOpts.GlobalSettings.EnableAgentgateway` is set. Otherwise,
only the Envoy proxy syncer will run by default.

```mermaid
flowchart TD
    subgraph "kgateway startup"
        A1["start.go"] --> A2["NewControllerBuilder()"]
        A2 --> A3{EnableAgentgateway?}
        A3 -->|true| A4["Create AgentGwSyncer"]
        A3 -->|false| A5["Skip Agentgateway"]
    end

    subgraph "agentgateway Syncer Initialization"
        A4 --> B1["agentgatewaysyncer.NewAgentGwSyncer()"]
        B1 --> B2["Set Configuration<br/>• controllerName<br/>• agentgatewayClassName<br/>• domainSuffix<br/>• clusterID"]
        B2 --> B3["syncer.Init()<br/>Build KRT Collections"]
    end

    subgraph "agentgateway translation"
        B3 --> C1["buildResourceCollections()"]

        C1 --> C2["Gateway Collection<br/>Filter by agentgateway class"]
        C1 --> C3["Route Collections<br/>HTTPRoute, GRPCRoute, etc.<br/>Builtin and attached policy via plugins"]
        C1 --> C4["Backend Collections<br/>Backend CRDs via plugins"]
        C1 --> C5["Policy Collections<br/>(InferencePools, A2A, etc.)"]
        C1 --> C6["Address Collections<br/>Services & Workloads"]

        C2 --> C7["Generate Bind Resources"]
        C2 --> C8["Generate Listener Resources"]
        C3 --> C9["Generate Route Resources"]
        C4 --> C10["Plugin Translation<br/>AI, Static, MCP backends"]
    end

    subgraph "XDS Collection Processing"
        C5 --> D1["buildXDSCollection()"]
        C6 --> D1
        C7 --> D1
        C8 --> D1
        C9 --> D1
        C10 --> D1

        D1 --> D2["Create agentGwXdsResources<br/>per Gateway"]
        D2 --> D3["ResourceConfig<br/>Bind + Listener + Route + Backend + Policy"]
        D2 --> D4["AddressConfig<br/>Services + Workloads"]
    end

    subgraph "XDS Output"
        D3 --> E1["Create agentGwSnapshot"]
        D4 --> E1

        E1 --> E2["xdsCache.SetSnapshot()<br/>with resource name"]
        E2 --> E3["XDS Server<br/>Serves via gRPC"]

        E3 --> E4["Agentgateway Proxy<br/>Receives & applies config"]
    end

    subgraph "Status"
        D2 --> F1["Status Reporting<br/>Gateway, Listener, Route"]
        F1 --> F2["Update K8s Resource Status"]
    end

    style A4 fill:#e1f5fe
    style B1 fill:#f3e5f5
    style C1 fill:#e8f5e8
    style D1 fill:#fff3e0
    style E1 fill:#fce4ec
    style E3 fill:#e0f2f1
    style E4 fill:#f1f8e9
```

### Translator tests 

The translator tests are unit tests that test the translation of the CRD input YAML resources to the agentgateway xDS API.

You can regenerate the golden output files by running the following command:

```shell
REFRESH_GOLDEN="true" go test -shuffle on -run "TestBasic" ./internal/kgateway/agentgatewaysyncer/...
```

### Conformance tests

Setup the cluster:

```shell
AGENTGATEWAY=true ./hack/kind/setup-kind.sh
```

Retag and load the image to match the default image tag in the values file for agentgateway, then run:

```
make run HELM_ADDITIONAL_VALUES=test/kubernetes/e2e/tests/manifests/agent-gateway-integration.yaml; CONFORMANCE_GATEWAY_CLASS=agentgateway make conformance
```

## Examples

Set up a kind cluster and install kgateway with the kubernetes Gateway APIs:
```shell
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.3.0/standard-install.yaml
helm upgrade -i --create-namespace --namespace kgateway-system --version v2.1.0-main kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds
helm upgrade -i --namespace kgateway-system --version v2.1.0-main kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway --set agentgateway.enabled=true --set inferenceExtension.enabled=true
```

#### HTTPRoute

Apply the httpbin test app:
```shell
kubectl apply -f  test/kubernetes/e2e/defaults/testdata/httpbin.yaml
```

Apply the following config to set up the HTTPRoute attached to the agentgateway Gateway:

```shell
kubectl apply -f- <<EOF
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: agent-gateway
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
      allowedRoutes:
        namespaces:
          from: All
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: httpbin
  labels:
    example: httpbin-route
spec:
  parentRefs:
    - name: agent-gateway
      namespace: default
  hostnames:
    - "www.example.com"
  rules:
    - backendRefs:
        - name: httpbin
          port: 8000
EOF
```

Port-forward and send a request through the gateway:
```shell
 curl localhost:8080 -v -H "host: www.example.com"
```

#### GRPC Route

Apply the following config to set up the GRPCRoute attached to the agentgateway Gateway:
```shell
kubectl apply -f- <<EOF
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: agent-gateway
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
      allowedRoutes:
        namespaces:
          from: All
---
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  name: grpc-route
spec:
  parentRefs:
    - name: agent-gateway
  hostnames:
    - "example.com"
  rules:
    - matches:
        - method:
            method: ServerReflectionInfo
            service: grpc.reflection.v1alpha.ServerReflection
        - method:
            method: Ping
      backendRefs:
        - name: grpc-echo-svc
          port: 3000
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grpc-echo
spec:
  selector:
    matchLabels:
      app: grpc-echo
  replicas: 1
  template:
    metadata:
      labels:
        app: grpc-echo
    spec:
      containers:
        - name: grpc-echo
          image: ghcr.io/projectcontour/yages:v0.1.0
          ports:
            - containerPort: 9000
              protocol: TCP
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: GRPC_ECHO_SERVER
              value: "true"
            - name: SERVICE_NAME
              value: grpc-echo
---
apiVersion: v1
kind: Service
metadata:
  name: grpc-echo-svc
spec:
  type: ClusterIP
  ports:
    - port: 3000
      protocol: TCP
      targetPort: 9000
      appProtocol: kubernetes.io/h2c
  selector:
    app: grpc-echo
---
apiVersion: v1
kind: Pod
metadata:
  name: grpcurl-client
spec:
  containers:
    - name: grpcurl
      image: docker.io/fullstorydev/grpcurl:v1.8.7-alpine
      command:
        - sleep
        - "infinity"
EOF
```

Port-forward, and send a request through the gateway:
```shell
grpcurl \
  -plaintext \
  -authority example.com \
  -d '{}' localhost:8080 yages.Echo/Ping
```

#### TCPRoute

Apply the following config to set up the TCPRoute attached to the agentgateway Gateway:
```shell
kubectl apply -f- <<EOF
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: tcp-gw-for-test
spec:
  gatewayClassName: agentgateway
  listeners:
    - name: tcp
      protocol: TCP
      port: 8080
      allowedRoutes:
        kinds:
          - kind: TCPRoute
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TCPRoute
metadata:
  name: tcp-app-1
spec:
  parentRefs:
    - name: tcp-gw-for-test
  rules:
    - name: test
      backendRefs:
        - name: tcp-backend
          port: 3001
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tcp-backend
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tcp-backend
      version: v1
  template:
    metadata:
      labels:
        app: tcp-backend
        version: v1
    spec:
      containers:
        - image: gcr.io/k8s-staging-gateway-api/echo-basic:v20231214-v1.0.0-140-gf544a46e
          imagePullPolicy: IfNotPresent
          name: tcp-backend
          ports:
            - containerPort: 3000
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: SERVICE_NAME
              value: tcp-backend
---
apiVersion: v1
kind: Service
metadata:
  name: tcp-backend
  labels:
    app: tcp-backend
spec:
  ports:
    - name: http
      port: 3001
      targetPort: 3000
  selector:
    app: tcp-backend
EOF
```

Port-forward, and send a request through the gateway:
```shell
curl localhost:8080/ -i
```

#### Static Backend routing

Apply the following config to set up the HTTPRoute pointing to the static backend:

```shell
kubectl apply -f- <<EOF
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: gw
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
      allowedRoutes:
        namespaces:
          from: All
---
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: json-route
spec:
  parentRefs:
    - name: gw
  hostnames:
    - "jsonplaceholder.typicode.com"
  rules:
    - backendRefs:
        - name: json-backend
          kind: Backend
          group: gateway.kgateway.dev
---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: json-backend
spec:
  type: Static
  static:
    hosts:
      - host: jsonplaceholder.typicode.com
        port: 80
EOF
```

Port-forward, and send a request through the gateway:
```shell
curl localhost:8080/ -v -H "host: jsonplaceholder.typicode.com"
```

#### AI Backend routing

First, create secret in the cluster with the API key:
```shell
kubectl create secret generic openai-secret \
--from-literal="Authorization=Bearer $OPENAI_API_KEY" \
--dry-run=client -oyaml | kubectl apply -f -
```

Apply the following config to set up the HTTPRoute pointing to the AI Backend:
```shell
kubectl apply -f- <<EOF
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: agent-gateway
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
      allowedRoutes:
        namespaces:
          from: All
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: openai
  labels:
    example: openai-route
spec:
  parentRefs:
    - name: agent-gateway
      namespace: default
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /openai
      backendRefs:
        - name: openai
          group: gateway.kgateway.dev
          kind: Backend
---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  labels:
    app: kgateway
  name: openai
spec:
  type: AI
  ai:
    llm:
      provider:
        openai:
          model: "gpt-4o-mini"
          authToken:
            kind: "SecretRef"
            secretRef:
              name: openai-secret
EOF
```

Port-forward, and send a request through the gateway:
```shell
curl localhost:8080/openai -H content-type:application/json -v -d'{
"model": "gpt-3.5-turbo",
"messages": [
  {
    "role": "user",
    "content": "Whats your favorite poem?"
  }
]}'
```

With agentgateway, you get a unified API to send requests to different providers in the same format.

Modify the HTTPRoute config to add another provider:

```shell
kubectl apply -f- <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: openai
  labels:
    example: openai-route
spec:
  parentRefs:
    - name: agent-gateway
      namespace: default
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /openai
      backendRefs:
        - name: openai
          group: gateway.kgateway.dev
          kind: Backend
    - matches:
        - path:
            type: PathPrefix
            value: /bedrock
      backendRefs:
        - group: gateway.kgateway.dev
          kind: Backend
          name: bedrock
---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: bedrock
spec:
  type: AI
  ai:
    llm:
      provider:
        bedrock:
          model: anthropic.claude-3-5-haiku-20241022-v1:0
          region: us-west-2
          auth:
            type: Secret
            secretRef:
              name: bedrock-secret
---
apiVersion: v1
kind: Secret
metadata:
  name: bedrock-secret
stringData:
  accessKey: ${AWS_ACCESS_KEY_ID}
  secretKey: ${AWS_SECRET_ACCESS_KEY}
  sessionToken: ${AWS_SESSION_TOKEN}
type: Opaque
EOF
```
The request you send can be formatted in the same Open AI format:

```shell
curl localhost:8080/ -H content-type:application/json -v -d'{
"model": "anthropic.claude-3-5-haiku-20241022-v1:0",
"messages": [
  {
    "role": "user",
    "content": "Whats your favorite poem?"
  }
]}'
```

You can send streaming requests using the Open AI format as well:
```shell
curl localhost:8080/ -H content-type:application/json -v -d'{
"model": "anthropic.claude-3-5-haiku-20241022-v1:0",
"stream": true,
"messages": [
  {
    "role": "user",
    "content": "Whats your favorite poem?"
  }
]}'
```

#### MCP Backend

Apply the following config to set up the HTTPRoute pointing to the MCP Backend:
```shell
kubectl apply -f- <<EOF
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: agent-gateway
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
      allowedRoutes:
        namespaces:
          from: All
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: mcp
  labels:
    example: mcp-route
spec:
  parentRefs:
    - name: agent-gateway
      namespace: default
  rules:
    - backendRefs:
        - name: mcp-backend
          group: gateway.kgateway.dev
          kind: Backend
---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  labels:
    app: kgateway
  name: mcp-backend
spec:
  type: MCP
  mcp:
    name: mcp-virtual-server
    targets:
      - name: mcp-server
        selector:
          service:
            matchLabels:
              app: mcp-server
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mcp-server
  labels:
    app: mcp-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mcp-server
  template:
    metadata:
      labels:
        app: mcp-server
    spec:
      containers:
        - name: mcp-server
          image: node:20-alpine
          command: ["npx"]
          args: ["-y", "@modelcontextprotocol/server-everything", "streamableHttp"]
          ports:
            - containerPort: 3001
---
apiVersion: v1
kind: Service
metadata:
  name: mcp-server
  labels:
    app: mcp-server
spec:
  selector:
    app: mcp-server
  ports:
    - protocol: TCP
      port: 3001
      targetPort: 3001
      appProtocol: kgateway.dev/mcp
  type: ClusterIP
EOF
```

Note: Only streamable HTTP is currently supported for label selectors.

Port-forward, and send a request through the gateway to start a session:
```shell
curl localhost:8080/sse -v
```

You should see a response with the session id:
```shell
event: endpoint
data: ?sessionId=c1a54dcb-be11-4f91-91b5-a1abf67deca2

```

Then you can send a request using the sessionId to initialize the connection:
```shell
curl "http://localhost:8080/mcp" -v \
  -H "Accept: text/event-stream,application/json" \
  --json '{"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{"roots":{}},"clientInfo":{"name":"claude-code","version":"1.0.60"}},"jsonrpc":"2.0","id":0}'
```

Or inspect the mcp tool with [MCP Inspector](https://github.com/modelcontextprotocol/inspector):
```shell
npx @modelcontextprotocol/inspector
```

You can also use static targets. This will create two backends 1) static backend for the target, 2) mcp backend.

Apply the following config to set up the HTTPRoute pointing to the MCP Backend with a static target:
```shell
kubectl apply -f- <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: mcp
spec:
  parentRefs:
  - name: agent-gateway
  rules:
    - backendRefs:
      - name: mcp-backend
        group: gateway.kgateway.dev
        kind: Backend
---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: mcp-backend
spec:
  type: MCP
  mcp:
    name: mcp-server
    targets:
    - name: mcp-target
      static:
        host: mcp-website-fetcher.default.svc.cluster.local
        port: 80
        protocol: SSE
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mcp-website-fetcher
spec:
  selector:
    matchLabels:
      app: mcp-website-fetcher
  template:
    metadata:
      labels:
        app: mcp-website-fetcher
    spec:
      containers:
      - name: mcp-website-fetcher
        image: ghcr.io/peterj/mcp-website-fetcher:main
        imagePullPolicy: Always
---
apiVersion: v1
kind: Service
metadata:
  name: mcp-website-fetcher
  labels:
    app: mcp-website-fetcher
spec:
  selector:
    app: mcp-website-fetcher
  ports:
  - port: 80
    targetPort: 8000
    appProtocol: kgateway.dev/mcp
EOF
```

#### A2A Backend

Build the sample kgateway a2a application and load it into the kind cluster with `VERSION=$VERSION make kind-build-and-load-test-a2a-agent`.

Apply the sample app:
```shell
kubectl apply -f- <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: a2a-agent
  labels:
    app: a2a-agent
spec:
  selector:
    matchLabels:
      app: a2a-agent
  template:
    metadata:
      labels:
        app: a2a-agent
    spec:
      containers:
        - name: a2a-agent
          image: ghcr.io/kgateway-dev/test-a2a-agent:1.0.0-ci1
          ports:
            - containerPort: 9090
---
apiVersion: v1
kind: Service
metadata:
  name: a2a-agent
spec:
  selector:
    app: a2a-agent
  type: ClusterIP
  ports:
    - protocol: TCP
      port: 9090
      targetPort: 9090
      appProtocol: kgateway.dev/a2a
EOF
```

Note, you must use `kgateway.dev/a2a` as the app protocol for the kgateway control plane to configure agentgateway to use a2a.

Apply the routing config:
```shell
kubectl apply -f- <<EOF
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: agent-gateway
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
      allowedRoutes:
        namespaces:
          from: All
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: a2a
  labels:
    example: a2a-route
spec:
  parentRefs:
    - name: agent-gateway
      namespace: default
  rules:
    - backendRefs:
        - name: a2a-agent
          port: 9090
EOF
```

Port-forward, and send a request through the gateway:
```shell
 curl -X POST http://localhost:8080/ \
-H "Content-Type: application/json" \
  -v \
  -d '{
"jsonrpc": "2.0",
"id": "1",
"method": "tasks/send",
"params": {
  "id": "1",
  "message": {
    "role": "user",
    "parts": [
      {
        "type": "text",
        "text": "hello gateway!"
      }
    ]
  }
}
}'
```

### Tracing and Observability

The agentgateway data plane supports comprehensive observability through OpenTelemetry (OTEL) tracing. You can configure tracing using custom ConfigMaps to integrate with various observability platforms and add custom trace fields for enhanced monitoring of your AI/LLM traffic.

For detailed information about tracing configuration and observability features, see the [agentgateway observability documentation](https://agentgateway.dev/docs/reference/observability/traces/).

#### Configuring Tracing with ConfigMaps

> **Note**: This approach statically configures tracing at startup. Changes to the ConfigMap are only picked up during agentgateway pod initialization, so you must restart the pod to apply configuration updates.

To enable tracing, you need to:

1. **Create a custom ConfigMap** with your tracing configuration
2. **Reference the ConfigMap** in your GatewayParameters
3. **Deploy your Gateway** with the agentgateway class

**Step 1: Create a ConfigMap**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: agent-gateway-config
  namespace: default
data:
  config.yaml: |-
    config:
      tracing:
        otlpEndpoint: http://jaeger-collector.observability.svc.cluster.local:4317
        otlpProtocol: grpc
        randomSampling: true
        fields:
          add:
            gen_ai.operation.name: '"chat"'
            gen_ai.system: "llm.provider"
            gen_ai.request.model: "llm.request_model"
            gen_ai.response.model: "llm.response_model"
            gen_ai.usage.completion_tokens: "llm.output_tokens"
            gen_ai.usage.prompt_tokens: "llm.input_tokens"
```

**Step 2: Configure GatewayParameters**

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: kgateway
spec:
  kube:
    agentgateway:
      enabled: true
      logLevel: debug
      customConfigMapName: agent-gateway-config
```

**Step 3: Create Gateway with agentgateway class**

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: agentgateway
spec:
  controllerName: kgateway.dev/kgateway
  parametersRef:
    group: gateway.kgateway.dev
    kind: GatewayParameters
    name: kgateway
    namespace: default
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: agent-gateway
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
```

#### Tracing Configuration Options

**Basic OTEL Configuration:**
```yaml
config:
  tracing:
    otlpEndpoint: http://localhost:4317      # OTEL collector endpoint
    otlpProtocol: grpc                       # grpc or http
    randomSampling: true                     # Enable/disable sampling
    headers:                                 # Optional headers for authentication
      Authorization: "Bearer <token>"
```

**Custom Trace Fields:**
Use CEL expressions to add custom fields to your traces:

```yaml
config:
  tracing:
    fields:
      add:
        # Standard OpenTelemetry AI semantic conventions
        gen_ai.operation.name: '"chat"'
        gen_ai.system: "llm.provider"
        gen_ai.request.model: "llm.request_model"
        gen_ai.response.model: "llm.response_model"
        gen_ai.usage.completion_tokens: "llm.output_tokens"
        gen_ai.usage.prompt_tokens: "llm.input_tokens"
        
        # Custom business logic fields
        user.id: "request.headers['x-user-id']"
        request.path: "request.path"
        backend.type: "llm.provider"
```

#### Integration Examples

**Jaeger Integration:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: jaeger-tracing-config
data:
  config.yaml: |-
    config:
      tracing:
        otlpEndpoint: http://jaeger-collector.jaeger.svc.cluster.local:4317
        otlpProtocol: grpc
        randomSampling: true
        fields:
          add:
            gen_ai.operation.name: '"chat"'
            gen_ai.system: "llm.provider"
            gen_ai.request.model: "llm.request_model"
            gen_ai.response.model: "llm.response_model"
            gen_ai.usage.completion_tokens: "llm.output_tokens"
            gen_ai.usage.prompt_tokens: "llm.input_tokens"
```

**Langfuse Integration:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: langfuse-tracing-config
data:
  config.yaml: |-
    config:
      tracing:
        otlpEndpoint: https://us.cloud.langfuse.com/api/public/otel
        otlpProtocol: http
        headers:
          Authorization: "Basic <base64-encoded-credentials>"
        randomSampling: true
        fields:
          add:
            gen_ai.operation.name: '"chat"'
            gen_ai.system: "llm.provider"
            gen_ai.prompt: "llm.prompt"
            gen_ai.completion: 'llm.completion.map(c, {"role":"assistant", "content": c})'
            gen_ai.usage.completion_tokens: "llm.output_tokens"
            gen_ai.usage.prompt_tokens: "llm.input_tokens"
            gen_ai.request.model: "llm.request_model"
            gen_ai.response.model: "llm.response_model"
            gen_ai.request: "flatten(llm.params)"
```

**Phoenix (Arize) Integration:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: phoenix-tracing-config
data:
  config.yaml: |-
    config:
      tracing:
        otlpEndpoint: http://localhost:4317
        randomSampling: true
        fields:
          add:
            span.name: '"openai.chat"'
            openinference.span.kind: '"LLM"'
            llm.system: "llm.provider"
            llm.input_messages: 'flatten_recursive(llm.prompt.map(c, {"message": c}))'
            llm.output_messages: 'flatten_recursive(llm.completion.map(c, {"role":"assistant", "content": c}))'
            llm.token_count.completion: "llm.output_tokens"
            llm.token_count.prompt: "llm.input_tokens"
            llm.token_count.total: "llm.total_tokens"
```

**OpenLLMetry Integration:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: openllmetry-tracing-config
data:
  config.yaml: |-
    config:
      tracing:
        otlpEndpoint: http://localhost:4317
        randomSampling: true
        fields:
          add:
            span.name: '"openai.chat"'
            gen_ai.operation.name: '"chat"'
            gen_ai.system: "llm.provider"
            gen_ai.prompt: "flatten_recursive(llm.prompt)"
            gen_ai.completion: 'flatten_recursive(llm.completion.map(c, {"role":"assistant", "content": c}))'
            gen_ai.usage.completion_tokens: "llm.output_tokens"
            gen_ai.usage.prompt_tokens: "llm.input_tokens"
            gen_ai.request.model: "llm.request_model"
            gen_ai.response.model: "llm.response_model"
            gen_ai.request: "flatten(llm.params)"
            llm.is_streaming: "llm.streaming"
```

#### Important Notes

- **ConfigMap Updates**: The ConfigMap is only read during agentgateway pod startup. To apply ConfigMap changes, restart the agentgateway pod
- **Namespace**: The ConfigMap must be in the same namespace as the GatewayParameters resource
- **Validation**: Invalid CEL expressions in trace fields will be logged but won't prevent the gateway from starting
- **Performance**: Be mindful of the number and complexity of custom trace fields, as they impact performance
- **Sampling**: Use `randomSampling` to control trace volume in production environments

#### Complete Example with AI Backend

```yaml
# ConfigMap with tracing configuration
apiVersion: v1
kind: ConfigMap
metadata:
  name: ai-gateway-tracing
  namespace: default
data:
  config.yaml: |-
    config:
      tracing:
        otlpEndpoint: http://jaeger-collector.observability.svc.cluster.local:4317
        otlpProtocol: grpc
        randomSampling: true
        fields:
          add:
            gen_ai.operation.name: '"chat"'
            gen_ai.system: "llm.provider"
            gen_ai.request.model: "llm.request_model"
            gen_ai.response.model: "llm.response_model"
            gen_ai.usage.completion_tokens: "llm.output_tokens"
            gen_ai.usage.prompt_tokens: "llm.input_tokens"
            user.id: "request.headers['x-user-id'] || 'anonymous'"
            request.path: "request.path"
---
# GatewayParameters referencing the ConfigMap
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: kgateway
spec:
  kube:
    agentgateway:
      enabled: true
      logLevel: debug
      customConfigMapName: ai-gateway-tracing
---
# GatewayClass and Gateway configuration
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: agentgateway
spec:
  controllerName: kgateway.dev/kgateway
  parametersRef:
    group: gateway.kgateway.dev
    kind: GatewayParameters
    name: kgateway
    namespace: default
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: ai-gateway
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
---
# AI Backend and Route
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: openai-backend
spec:
  type: AI
  ai:
    llm:
      provider:
        openai:
          model: "gpt-4o-mini"
          authToken:
            kind: "SecretRef"
            secretRef:
              name: openai-secret
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: ai-route
spec:
  parentRefs:
    - name: ai-gateway
  rules:
    - backendRefs:
        - name: openai-backend
          group: gateway.kgateway.dev
          kind: Backend
```

#### Example with MCP Tool Calling and Trace Verification

Here's a complete example that demonstrates tracing MCP tool calls, which generates rich trace spans for `list_tools` and `call_tool` operations:

```yaml
# Tracing Configuration ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: agentgateway-tracing-config
  namespace: default
data:
  config.yaml: |-
    config:
      tracing:
        otlpEndpoint: http://localhost:4317
        randomSampling: true
        fields:
          add:
            # MCP-specific trace fields
            mcp.operation.name: "request.path"
            mcp.tool.name: "request.headers['x-tool-name'] || 'unknown'"
            mcp.session.id: "request.headers['x-session-id'] || 'anonymous'"
            backend.type: '"mcp"'
            request.method: "request.method"
            response.status: "response.status_code"
---
# Gateway Parameters with tracing configuration
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: agentgateway-params
  namespace: default
spec:
  kube:
    agentgateway:
      enabled: true
      logLevel: debug
      customConfigMapName: agentgateway-tracing-config
---
# Gateway Class
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: agentgateway
spec:
  controllerName: kgateway.dev/kgateway
  parametersRef:
    group: gateway.kgateway.dev
    kind: GatewayParameters
    name: agentgateway-params
    namespace: default
---
# Gateway
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: agentgateway
  namespace: default
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 3000
      name: http
      allowedRoutes:
        namespaces:
          from: All
---
# MCP Backend
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: mcp-everything-backend
  namespace: default
spec:
  type: MCP
  mcp:
    targets:
      - name: everything
        selector:
          service:
            matchLabels:
              app: mcp-everything
---
# HTTPRoute with CORS policy and MCP backend
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: mcp-route
  namespace: default
spec:
  parentRefs:
    - name: agentgateway
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: "/"
      filters:
        # CORS policy
        - type: CORS
          cors:
            allowOrigins:
            - "http://localhost:3000"
            - "http://localhost:8080" 
            - "http://localhost:15000"
            - "http://127.0.0.1:3000"
            - "http://127.0.0.1:8080"
            - "http://127.0.0.1:15000"
            allowMethods:
            - "GET"
            - "POST"
            - "PUT"
            - "DELETE"
            - "OPTIONS"
            allowHeaders:
            - "Content-Type"
            - "Authorization"
            - "Accept"
            - "mcp-protocol-version"
            - "Cache-Control"
            maxAge: 86400
      backendRefs:
        - name: mcp-everything-backend
          group: gateway.kgateway.dev
          kind: Backend
---
# MCP Everything Server Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mcp-everything
  namespace: default
  labels:
    app: mcp-everything
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mcp-everything
  template:
    metadata:
      labels:
        app: mcp-everything
    spec:
      containers:
        - name: mcp-everything
          image: node:20-alpine
          command: ["npx"]
          args: ["@modelcontextprotocol/server-everything", "streamableHttp"]
          ports:
            - containerPort: 3001
          env:
            - name: PORT
              value: "3001"
---
# Service for MCP Everything Server
apiVersion: v1
kind: Service
metadata:
  name: mcp-everything-service
  namespace: default
  labels:
    app: mcp-everything
spec:
  selector:
    app: mcp-everything
  ports:
    - protocol: TCP
      port: 3001
      targetPort: 3001
      appProtocol: kgateway.dev/mcp
  type: ClusterIP
```

**Testing MCP Tool Calls with Traces:**

1. **Set up port forwarding:**
```bash
# Port forward to the gateway pod
kubectl port-forward deployment/agentgateway 15000:15000 3000:3000
```

**Verify traces:**

1. **Open the agentgateway UI** to view your listener and target configuration.

2. **Connect to the MCP server** with the agentgateway UI playground.
   - From the navigation menu, click **Playground**.

3. **In the Testing card**, review your Connection details and click **Connect**. The agentgateway UI connects to the target that you configured and retrieves the tools that are exposed on the target.

4. **Verify that you see a list of Available Tools**.

5. **Verify access to a tool**:
   - From the Available Tools list, select the **echo** tool.
   - In the message field, enter any string, such as `hello world`, and click **Run Tool**.
   - Verify that you see your message echoed in the Response card.

6. **Open the Jaeger UI**.

7. **View traces**:
   - From the Service drop down, select **agentgateway**.
   - Click **Find Traces**.
   - Verify that you can see trace spans for listing the MCP tools (`list_tools`) and calling a tool (`call_tool`).

This configuration provides comprehensive tracing for your MCP tool interactions, making it easy to debug issues and monitor performance of your agent-to-agent communications.