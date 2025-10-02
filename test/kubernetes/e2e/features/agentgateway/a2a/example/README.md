# A2A Test Server 

This is a simple example of a server that can be used to test A2A gateways. It's based on the Google guide:  https://github.com/a2aproject/a2a-samples/tree/main/samples/python/agents/helloworld

## Setup

1. Change the directory of this README.md and clone the sample.

```bash
cd test/kubernetes/e2e/features/agentgateway/a2a/example
git clone https://github.com/a2aproject/a2a-samples.git
```

2. Copy Dockerfile to the sample it allows much smaller container built that the default.

```bash
cp Dockerfile a2a-samples/samples/python/agents/helloworld/ 
cd a2a-samples/samples/python/agents/helloworld
```

3. Build the Docker container: 

```shell
docker build . -f Dockerfile -t test-a2a-server-local
```

4. Start the container

```shell
docker run -d --name test-local -p 9999:9999 test-a2a-server-local
```


## Test:

1. Simple card test:

```bash
curl -s http://localhost:9999/.well-known/agent-card.json | head -3
```

output

```json
{"capabilities":{"streaming":true},"defaultInputModes":["text"],"defaultOutputModes":["text"],"description":"Just a hello world agent","name":"Hello World Agent","preferredTransport":"JSONRPC","protocolVersion":"0.3.0","skills":[{"description":"just returns hello world","examples":["hi","hello world"],"id":"hello_world","name":"Returns hello world","tags":["hello world"]}],"supportsAuthenticatedExtendedCard":true,"url":"http://localhost:9999/","version":"1.0.0"}
```

2. Full request call:

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

Note: if you want to update test container:

1. Tag the container:

```bash
docker tag test-a2a-server-local ghcr.io/kgateway-dev/test-a2a-server:0.0.<version>
```

2. Push to Github repo:

```bash
docker push ghcr.io/kgateway-dev/test-a2a-server:0.0.<version>
```

3. Search locally in this repo and update the version reference to the container.