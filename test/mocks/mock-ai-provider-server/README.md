## Creating a mock for a new test

To create a new mock for a new test, follow these steps:

1. Create a new file in the `mock-ai-provider-server/data` directory with the name of the test you want to mock (routing, streaming, etc.).
2. Capture the request and response data for the real provider and save it in the appropriate format (JSON, txt, etc.).

You can do this a couple of ways:
- Use [vcrpy](https://vcrpy.readthedocs.io/en/latest/index.html) to record the data from the real provider. You can write
the e2e test and then annotate the test with the cassette name to use for the mock:

```python
import vcr

my_vcr = vcr.VCR(
    cassette_library_dir='cassettes/',
    record_mode=vcr.record_mode.RecordMode.ONCE,
    match_on=['method', 'uri', 'body'],
    filter_headers=["Authorization"],
)

def test_azure_openai_completion_stream(self):
    with my_vcr.use_cassette("test_azure_streaming_completion.yaml"):
        # your test code here
```

- Use a tool like [curl](https://curl.se/) to make the request and capture the response data manually.

3. Create a new route if necessary in the `main.go` file to handle the request using the [gin](https://github.com/gin-gonic/gin)
framework. You can use the existing routes as a reference.

```go
r.POST("/my/mock/path", func(c *gin.Context) {
	// handle request data assertions and mock response data
}
```

4. Calculate the hash of the request data and save it in the `main.go` mapping of hashes to test data paths. You can calculate
the request hash by sending a request to the mock server and extracting the hash from the logs:

```bash
data: map[messages:[map[content:You are a poetic assistant, skilled in explaining complex programming concepts with creative flair. role:system] map[content:Compose a poem that explains the concept of recursion in programming. role:user]] model:gpt-4o-mini provider:azure stream:true], hash: daa5badeb5cfabcb85b36bb0d6d8daa2a63536329f3c48e654137a6b3dc8c3d6
```

## Mocking gzip responses

Some providers (such as OpenAI) may respond with gzip-compressed data, which needs to be properly handled when mocking responses. To ensure compatibility, the mock server should detect when gzip encoding is requested and return appropriately compressed responses.

When returning a response in gzip format, you need to:

1. Compress the JSON response. You can use the example in `convert_to_gzip.sh` as a template
2. Set the Content-Encoding header to "gzip" so clients can decode it properly.

## Mocking streaming responses 

Streaming responses are stored in Server-Sent Events (SSE) format and sent back in chunks. 

To mock SSE streaming responses in your server, you need to:
1. Set the correct response headers for streaming (`text/event-stream`)
2. Use a generator to send data in chunks.

## Mocking Requests

You can run the gin mock server locally with:
```shell
go run main.go
```

Here are some example requests for the different providers using insecure mode (`-k`):

```shell
# gemini
curl https://localhost:5001/v1beta/models/gemini-1.5-flash:generateContent -vk \
  -H "Content-Type: application/json" \
  -d '{"contents":[{"parts":[{"text":"Compose a poem that explains the concept of recursion in programming."}],"role":"user"}],"generationConfig":{}}'

# streaming 
curl https://localhost:5001/v1beta/models/gemini-1.5-flash:streamGenerateContent -vk \
  -H "Content-Type: application/json" \
  -d '{"contents":[{"parts":[{"text":"Compose a poem that explains the concept of recursion in programming."}],"role":"user"}],"generationConfig":{}}'

```

```shell
# vertex ai
# /v1/projects/kgateway-project/locations/us/publishers/google/models/gemini-1.5-flash-001:generateContent
# /v1/projects/kgateway-project/locations/us-central1/publishers/google/models/gemini-1.5-flash-001:generateContent
curl https://localhost:5001/v1/projects/kgateway-project/locations/us-central1/publishers/google/models/gemini-1.5-flash-001:generateContent -vk \
  -H "Content-Type: application/json" \
  -d '{"contents":[{"parts":[{"text":"Compose a poem that explains the concept of recursion in programming."}],"role":"user"}]}'

# streaming
curl https://localhost:5001/v1/projects/kgateway-project/locations/us-central1/publishers/google/models/gemini-1.5-flash-001:streamGenerateContent -vk \
  -H "Content-Type: application/json" \
  -d '{"contents":[{"parts":[{"text":"Compose a poem that explains the concept of recursion in programming."}],"role":"user"}]}'
```

```shell
# azure 
curl https://localhost:5001/openai/deployments/gpt-4o-mini/chat/completions?api-version=2024-02-15-preview \
  -vk -H "Content-Type: application/json" \
  -d '{
        "messages": [
          {
            "role": "system", 
            "content": "You are a poetic assistant, skilled in explaining complex programming concepts with creative flair."
          },
          {
            "role": "user", 
            "content": "Compose a poem that explains the concept of recursion in programming."
          }
        ], 
        "model": "gpt-4o-mini"
      }'

# streaming 
curl https://localhost:5001/openai/deployments/gpt-4o-mini/chat/completions?api-version=2024-02-15-preview \
  -vk -H "Content-Type: application/json" \
  -d '{
    "messages": [
      {
        "role": "system",
        "content": "You are a poetic assistant, skilled in explaining complex programming concepts with creative flair."
      },
      {
        "role": "user",
        "content": "Compose a poem that explains the concept of recursion in programming."
      }
    ],
    "model": "gpt-4o-mini",
    "stream": true
  }'

```

```shell
# openai
curl https://localhost:5001/v1/chat/completions -vk -H "Content-Type: application/json" -d '{
  "messages": [
    {
      "role": "system",
      "content": "You are a poetic assistant, skilled in explaining complex programming concepts with creative flair."
    },
    {
      "role": "user",
      "content": "Compose a poem that explains the concept of recursion in programming."
    }
  ],
  "model": "gpt-4o-mini"
}'

# streaming
curl https://localhost:5001/v1/chat/completions -vk -H "Content-Type: application/json" -d '{
  "messages": [
    {
      "role": "system",
      "content": "You are a poetic assistant, skilled in explaining complex programming concepts with creative flair."
    },
    {
      "role": "user",
      "content": "Compose a poem that explains the concept of recursion in programming."
    }
  ],
  "model": "gpt-4o-mini",
  "stream": true
}'

```