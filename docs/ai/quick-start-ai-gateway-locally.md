
# Quick Start: Using Ollama with AI Gateway

## Setup Gateway

Follow the [quickstart](https://kgateway.dev/docs/quickstart/) to quickly launch the gateway.

## Setup AI Gateway

If you are using a cloud provider's K8s resources, just follow the [official guide](https://kgateway.dev/docs/ai/setup/#gateway).  
If you are using a local environment, please note: In the [Create An AI Gateway](https://kgateway.dev/docs/ai/setup/#gateway) step, use `NodePort` instead of `LoadBalancer`.

```yaml
kubectl apply -f- <<EOF
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: ai-gateway
  namespace: kgateway-system
  labels:
    app: ai-kgateway
spec:
  kube:
    aiExtension:
      enabled: true
      ports:
      - name: ai-monitoring
        containerPort: 9092
    service:
      type: NodePort # Not LoadBalancer here!
EOF
```

## Setup LLM Locally

1. Run ipconfig (or ifconfig on Linux/macOS) in your terminal to find your IP address.

   Example output:

   ```text
   en0: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500
   	...
   	inet 192.168.181.210 netmask 0xffffff00 broadcast 192.168.181.255
   	...
   ```

2. Start the Ollama service  
   Run the following command, replacing `192.168.181.210` with your actual IP address from the previous step:
   `OLLAMA_HOST=192.168.181.210 ollama serve`
   
   Example output:

   ```text
   time=2025-05-21T12:33:42.433+08:00 level=INFO source=routes.go:1205 msg="server config"
   env="map[HTTPS_PROXY: HTTP_PROXY: NO_PROXY: OLLAMA_CONTEXT_LENGTH:4096
   OLLAMA_DEBUG:INFO OLLAMA_FLASH_ATTENTION:false OLLAMA_GPU_OVERHEAD:0
   OLLAMA_HOST:http://192.168.181.210:11434 OLLAMA_KEEP_ALIVE:5m0s OLLAMA_KV_CACHE_TYPE:
   OLLAMA_LLM_LIBRARY: OLLAMA_LOAD_TIMEOUT:5m0s OLLAMA_MAX_LOADED_MODELS:0
   OLLAMA_MAX_QUEUE:512 OLLAMA_MODELS:/Users/zhengkezhou/.ollama/models
   OLLAMA_MULTIUSER_CACHE:false OLLAMA_NEW_ENGINE:false OLLAMA_NOHISTORY:false
   OLLAMA_NOPRUNE:false OLLAMA_NUM_PARALLEL:0 OLLAMA_ORIGINS:[http://localhost https://localhost
   http://localhost:* https://localhost:* http://127.0.0.1 https://127.0.0.1 http://127.0.0.1:* https://127.0.0.1:*
   http://0.0.0.0 https://0.0.0.0 http://0.0.0.0:* https://0.0.0.0:* app://* file://* tauri://* vscode-webview://*
   vscode-file://*] OLLAMA_SCHED_SPREAD:false http_proxy: https_proxy: no_proxy:]"
   time=2025-05-21T12:33:42.436+08:00 level=INFO source=images.go:463 msg="total blobs: 12"
   time=2025-05-21T12:33:42.437+08:00 level=INFO source=images.go:470 msg="total unused blobs removed: 0"
   time=2025-05-21T12:33:42.437+08:00 level=INFO source=routes.go:1258 msg="Listening on 192.168.181.210:11434 (version 0.7.0)"
   time=2025-05-21T12:33:42.478+08:00 level=INFO source=types.go:130 msg="inference compute" id=0 library=metal variant="" compute="" driver=0.0 name="" total="16.0 GiB" available="16.0 GiB"
   ```

## Using Inline Token

1. Create the Backend Resource. Although authentication is not required, the `authToken` field is mandatory for creating an AI `Backend`. You can provide any placeholder value for the inline token.

    ```yaml
    kubectl apply -f- <<EOF
    apiVersion: gateway.kgateway.dev/v1alpha1
    kind: Backend
    metadata:
      labels:
        app: ai-kgateway
      name: llama
      namespace: kgateway-system
    spec:
      type: AI
      ai:
        llm:
          hostOverride:
            host: 192.168.181.210 # replace with your IP address
            port: 11434
          provider:
            openai:
              model: "llama3.2" # replace with your model
              authToken:
                kind: Inline
                inline: "$TOKEN"
    EOF
    ```

2. Create an HTTPRoute resource that routes incoming traffic to the Backend. The following example sets up a route on the ollama path to the Backend you previously created. The `URLRewrite` filter rewrites the path from ollama to the API path you want to use in the LLM provider, `/v1/models`.

    ```yaml
    kubectl apply -f- <<EOF
    apiVersion: gateway.networking.k8s.io/v1
    kind: HTTPRoute
    metadata:
      name: llama
      namespace: kgateway-system
      labels:
        app: ai-kgateway
    spec:
      parentRefs:
        - name: ai-gateway
          namespace: kgateway-system
      rules:
      - matches:
        - path:
            type: PathPrefix
            value: /ollama
        filters:
        - type: URLRewrite
          urlRewrite:
            path:
              type: ReplaceFullPath
              replaceFullPath: /models
        backendRefs:
        - name: llama
          namespace: kgateway-system
          group: gateway.kgateway.dev
          kind: Backend
    EOF
    ```

3. Send a request to the Ollama server we set up before. Verify that the request succeeds and that you get back a response from the chat completion API.

    ```bash
    curl -v "localhost:8080/ollama" \
        -H "Content-Type: application/json" \
        -d '{
            "model": "llama3.2",
            "messages": [
                {
                    "role": "system",
                    "content": "You are a helpful assistant."
                },
                {
                    "role": "user",
                    "content": "Hello!"
                }
            ]
        }' | jq
    ```
   
    Example output:

    ```json
    {
      "id": "chatcmpl-534",
      "object": "chat.completion",
      "created": 1747805667,
      "model": "llama3.2",
      "system_fingerprint": "fp_ollama",
      "choices": [
        {
          "index": 0,
          "message": {
            "role": "assistant",
            "content": "It's nice to meet you. Is there something I can help you with, or would you like to chat?"
          },
          "finish_reason": "stop"
        }
      ],
      "usage": {
        "prompt_tokens": 33,
        "completion_tokens": 24,
        "total_tokens": 57
      }
    }
    ```
