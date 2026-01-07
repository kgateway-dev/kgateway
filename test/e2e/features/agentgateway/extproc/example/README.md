# ExtProc Test Server

This is an example external processing service used to test ExtProc functionality in KGateway with agentgateway as the data-plane.

The service is based on the basic-sink example from https://github.com/solo-io/ext-proc-examples

## What It Does

This external processing service implements Envoy's External Processing filter interface. It takes instructions in a request or response header named `instructions` and can:
- Add headers to requests/responses
- Remove headers from requests/responses
- Set body content
- Set trailers

The `instructions` header must be a JSON string in this format:
```json
{
  "addHeaders": {
    "header1": "value1",
    "header2": "value2"
  },
  "removeHeaders": ["header3", "header4"],
  "setBody": "this is the new body",
  "setTrailers": {
    "trailer1": "value1",
    "trailer2": "value2"
  }
}
```

All fields are optional.

## Building and Publishing

### 1. Navigate to this directory

```bash
cd $(git rev-parse --show-toplevel)/test/e2e/features/agentgateway/extproc/example
```

### 2. Build the Docker image

```bash
export REPO="ghcr.io/kgateway-dev"
export IMAGE="test-extproc-server"
export IMAGE_VERSION="0.0.<version>"
docker build -t $REPO/$IMAGE:$IMAGE_VERSION .
```

### 3. Update the version reference to the container in the test manifests

It is located at `test/e2e/features/agentgateway/extproc/testdata/extproc-service.yaml`


