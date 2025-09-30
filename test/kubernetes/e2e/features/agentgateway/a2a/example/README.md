# A2A Test Server 

This is a simple example of a server that can be used to test A2A gateways. It's based on the Google guide: https://github.com/a2aproject/A2A/tree/main/docs/tutorials/python

## Setup

1. Install `uv` per this guide https://docs.astral.sh/uv/getting-started/installation/

2. UV setup

```shell
uv init --package test/kubernetes/e2e/features/agentgateway/a2a/example
cd  test/kubernetes/e2e/features/agentgateway/a2a/example
```

3. Create virtual environment

```shell
uv venv .venv
```

Note: For this and any future terminal windows you open, you'll need to source this venv

```shell
source .venv/bin/activate
```

4. Install needed Python dependencies along with the A2A SDK and its dependencies:

```shell
uv pip install -r ./requirements.txt
```

5. Verify

```shell
uv run python -c "import a2a; print('A2A SDK imported successfully')"
```

6. Get `helloworld` sample server.

you can use instructions from here - https://github.com/a2aproject/a2a-samples/tree/main/samples/python/agents/helloworld and build / run the container with something like that: 

```bash
docker build . -f Containerfile -t test-a2a-server
docker run -p 9999:9999 helloworld-a2a-server
```


or you can use pre-build container `mcallisterpetr/test-a2a-server:0.1.0

7. Run the container:

```bash
docker run -p 9999:9999 test-a2a-server
```

```output
warning: `VIRTUAL_ENV=.` does not match the project environment path `.venv` and will be ignored; use `--active` to target the active environment instead
   Building helloworld @ file:///opt/app-root
      Built helloworld @ file:///opt/app-root
Uninstalled 1 package in 15ms
Installed 1 package in 1ms
Bytecode compiled 1810 files in 217ms
INFO:     Started server process [37]
INFO:     Waiting for application startup.
INFO:     Application startup complete.
INFO:     Uvicorn running on http://0.0.0.0:9999 (Press CTRL+C to quit)
```

8. Run client

```bash
uv run python test_client.py
```

```output
warning: No `requires-python` value found in the workspace. Defaulting to `>=3.13`.
INFO:__main__:Attempting to fetch public agent card from: http://localhost:9999/.well-known/agent-card.json
INFO:httpx:HTTP Request: GET http://localhost:9999/.well-known/agent-card.json "HTTP/1.1 200 OK"
INFO:a2a.client.card_resolver:Successfully fetched agent card data from http://localhost:9999/.well-known/agent-card.json: {'capabilities': {'streaming': True}, 'defaultInputModes': ['text'], 'defaultOutputModes': ['text'], 'description': 'Just a hello world agent', 'name': 'Hello World Agent', 'preferredTransport': 'JSONRPC', 'protocolVersion': '0.3.0', 'skills': [{'description': 'just returns hello world', 'examples': ['hi', 'hello world'], 'id': 'hello_world', 'name': 'Returns hello world', 'tags': ['hello world']}], 'supportsAuthenticatedExtendedCard': True, 'url': 'http://localhost:9999/', 'version': '1.0.0'}
INFO:__main__:Successfully fetched public agent card:
INFO:__main__:{
  "capabilities": {
    "streaming": true
  },
  "defaultInputModes": [
    "text"
  ],
  "defaultOutputModes": [
    "text"
  ],
  "description": "Just a hello world agent",
  "name": "Hello World Agent",
  "preferredTransport": "JSONRPC",
  "protocolVersion": "0.3.0",
  "skills": [
    {
      "description": "just returns hello world",
      "examples": [
        "hi",
        "hello world"
      ],
      "id": "hello_world",
      "name": "Returns hello world",
      "tags": [
        "hello world"
      ]
    }
  ],
  "supportsAuthenticatedExtendedCard": true,
  "url": "http://localhost:9999/",
  "version": "1.0.0"
}
INFO:__main__:
Using PUBLIC agent card for client initialization (default).
INFO:__main__:
Public card supports authenticated extended card. Attempting to fetch from: http://localhost:9999/agent/authenticatedExtendedCard
INFO:httpx:HTTP Request: GET http://localhost:9999/agent/authenticatedExtendedCard "HTTP/1.1 200 OK"
INFO:a2a.client.card_resolver:Successfully fetched agent card data from http://localhost:9999/agent/authenticatedExtendedCard: {'capabilities': {'streaming': True}, 'defaultInputModes': ['text'], 'defaultOutputModes': ['text'], 'description': 'The full-featured hello world agent for authenticated users.', 'name': 'Hello World Agent - Extended Edition', 'preferredTransport': 'JSONRPC', 'protocolVersion': '0.3.0', 'skills': [{'description': 'just returns hello world', 'examples': ['hi', 'hello world'], 'id': 'hello_world', 'name': 'Returns hello world', 'tags': ['hello world']}, {'description': 'A more enthusiastic greeting, only for authenticated users.', 'examples': ['super hi', 'give me a super hello'], 'id': 'super_hello_world', 'name': 'Returns a SUPER Hello World', 'tags': ['hello world', 'super', 'extended']}], 'supportsAuthenticatedExtendedCard': True, 'url': 'http://localhost:9999/', 'version': '1.0.1'}
INFO:__main__:Successfully fetched authenticated extended agent card:
INFO:__main__:{
  "capabilities": {
    "streaming": true
  },
  "defaultInputModes": [
    "text"
  ],
  "defaultOutputModes": [
    "text"
  ],
  "description": "The full-featured hello world agent for authenticated users.",
  "name": "Hello World Agent - Extended Edition",
  "preferredTransport": "JSONRPC",
  "protocolVersion": "0.3.0",
  "skills": [
    {
      "description": "just returns hello world",
      "examples": [
        "hi",
        "hello world"
      ],
      "id": "hello_world",
      "name": "Returns hello world",
      "tags": [
        "hello world"
      ]
    },
    {
      "description": "A more enthusiastic greeting, only for authenticated users.",
      "examples": [
        "super hi",
        "give me a super hello"
      ],
      "id": "super_hello_world",
      "name": "Returns a SUPER Hello World",
      "tags": [
        "hello world",
        "super",
        "extended"
      ]
    }
  ],
  "supportsAuthenticatedExtendedCard": true,
  "url": "http://localhost:9999/",
  "version": "1.0.1"
}
INFO:__main__:
Using AUTHENTICATED EXTENDED agent card for client initialization.
/home/ubuntu/kgateway/test/kubernetes/e2e/features/agentgateway/a2a/example/test_client.py:105: DeprecationWarning: A2AClient is deprecated and will be removed in a future version. Use ClientFactory to create a client with a JSON-RPC transport.
  client = A2AClient(
INFO:__main__:A2AClient initialized.
INFO:httpx:HTTP Request: POST http://localhost:9999/ "HTTP/1.1 200 OK"
{'id': '43ad0d9d-f1f6-495f-8169-2562200f54f6', 'jsonrpc': '2.0', 'result': {'kind': 'message', 'messageId': '2767d454-c706-4727-b503-d94ced518c75', 'parts': [{'kind': 'text', 'text': 'Hello World'}], 'role': 'agent'}}
INFO:httpx:HTTP Request: POST http://localhost:9999/ "HTTP/1.1 200 OK"
{'id': 'cd38cf36-9d8f-47e7-be87-8f31345badf0', 'jsonrpc': '2.0', 'result': {'kind': 'message', 'messageId': '714c902a-1516-4bba-b1fb-0c9847e8c9d0', 'parts': [{'kind': 'text', 'text': 'Hello World'}], 'role': 'agent'}}
```

9. Test with curl:

```bash
curl -X POST http://localhost:9999/ \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": "test-123",
    "method": "message/send",
    "params": {
      "message": {
        "messageId": "msg-123",
        "role": "user",
        "parts": [
          {
            "kind": "text",
            "text": "hello"
          }
        ]
      }
    }
  }'
  ```

  ```output
  {"id":"test-123","jsonrpc":"2.0","result":{"kind":"message","messageId":"2376c97a-c818-44ef-9122-cc721124cbc2","parts":[{"kind":"text","text":"Hello World"}],"role":"agent"}}
  ```